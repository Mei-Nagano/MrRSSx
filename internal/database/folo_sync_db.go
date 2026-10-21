package database

import (
	"database/sql"
	"time"
)

// FoloMappedArticle links a local article to the Folo entry it mirrors and
// records the last state both sides agreed on, so the sync can tell which side
// changed.
type FoloMappedArticle struct {
	ArticleID    int64
	EntryID      string
	ArticleURL   string
	LocalRead    bool
	LocalFav     bool
	SnapshotRead bool
	SnapshotFav  bool
}

// InitFoloSyncTable creates the Folo entry mapping table if it doesn't exist.
func InitFoloSyncTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS folo_entry_map (
		article_id INTEGER PRIMARY KEY,
		entry_id TEXT NOT NULL UNIQUE,
		article_url TEXT NOT NULL,
		last_read INTEGER NOT NULL DEFAULT 0,
		last_favorite INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_folo_entry_map_entry ON folo_entry_map(entry_id);
	CREATE INDEX IF NOT EXISTS idx_folo_entry_map_url ON folo_entry_map(article_url);
	`
	_, err := db.Exec(query)
	return err
}

// UpsertFoloEntry records that a local article mirrors a Folo entry. One remote
// entry can only belong to one local article, so an older link to the same
// entry is replaced rather than duplicated.
func (db *DB) UpsertFoloEntry(articleID int64, entryID, articleURL string, read, favorite bool) error {
	db.WaitForReady()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM folo_entry_map WHERE entry_id = ? AND article_id != ?", entryID, articleID); err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO folo_entry_map (article_id, entry_id, article_url, last_read, last_favorite, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(article_id) DO UPDATE SET
			entry_id = excluded.entry_id,
			article_url = excluded.article_url,
			last_read = excluded.last_read,
			last_favorite = excluded.last_favorite,
			updated_at = excluded.updated_at`,
		articleID, entryID, articleURL, boolToInt(read), boolToInt(favorite), time.Now().Unix())
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListFoloMappedArticles returns mappings whose local article still exists, most
// recently touched first.
func (db *DB) ListFoloMappedArticles(limit int) ([]FoloMappedArticle, error) {
	db.WaitForReady()
	if limit <= 0 {
		limit = 500
	}
	rows, err := db.Query(`
		SELECT m.article_id, m.entry_id, m.article_url, a.is_read, a.is_favorite, m.last_read, m.last_favorite
		FROM folo_entry_map m
		JOIN articles a ON a.id = m.article_id
		ORDER BY m.updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mapped := make([]FoloMappedArticle, 0, limit)
	for rows.Next() {
		var item FoloMappedArticle
		if err := rows.Scan(
			&item.ArticleID, &item.EntryID, &item.ArticleURL,
			&item.LocalRead, &item.LocalFav, &item.SnapshotRead, &item.SnapshotFav,
		); err != nil {
			return nil, err
		}
		mapped = append(mapped, item)
	}
	return mapped, rows.Err()
}

// GetFoloMapping returns the remote entry linked to a local article.
func (db *DB) GetFoloMapping(articleID int64) (entryID string, lastRead, lastFavorite bool, ok bool, err error) {
	db.WaitForReady()
	row := db.QueryRow("SELECT entry_id, last_read, last_favorite FROM folo_entry_map WHERE article_id = ?", articleID)
	err = row.Scan(&entryID, &lastRead, &lastFavorite)
	if err == sql.ErrNoRows {
		return "", false, false, false, nil
	}
	if err != nil {
		return "", false, false, false, err
	}
	return entryID, lastRead, lastFavorite, true, nil
}

// UpdateFoloSnapshot records the state both sides now agree on.
func (db *DB) UpdateFoloSnapshot(articleID int64, read, favorite bool) error {
	db.WaitForReady()
	_, err := db.Exec(
		"UPDATE folo_entry_map SET last_read = ?, last_favorite = ?, updated_at = ? WHERE article_id = ?",
		boolToInt(read), boolToInt(favorite), time.Now().Unix(), articleID)
	return err
}

// CountFoloMappings reports how many local articles are linked to Folo entries.
func (db *DB) CountFoloMappings() (int, error) {
	db.WaitForReady()
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM folo_entry_map m
		JOIN articles a ON a.id = m.article_id`).Scan(&count)
	return count, err
}

// ClearFoloMappings drops every link. Logging into a different Folo account
// invalidates the snapshots taken against the previous one.
func (db *DB) ClearFoloMappings() (int64, error) {
	db.WaitForReady()
	result, err := db.Exec("DELETE FROM folo_entry_map")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteOrphanedFoloEntries drops mappings whose local article is gone, which
// keeps the sync from pushing state for articles MrRSS no longer has.
func (db *DB) DeleteOrphanedFoloEntries() (int64, error) {
	db.WaitForReady()
	result, err := db.Exec(`
		DELETE FROM folo_entry_map
		WHERE article_id NOT IN (SELECT id FROM articles)`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
