package database

import (
	"database/sql"
	"time"
)

// FoloMappedArticle links a local article to the Folo entry it mirrors and
// records the last state both sides agreed on, so the sync can tell which side
// changed.
type FoloMappedArticle struct {
	ArticleID  int64
	EntryID    string
	ArticleURL string
	// PublishedAt is the local article's publish time, used as a fallback window
	// for mappings created before the remote time was recorded.
	PublishedAt string
	// RemotePublishedAt is the Folo entry's own publish time. Re-reading an entry
	// must use this: matching is by URL, and the two sides do not always agree on
	// the publication instant.
	RemotePublishedAt string
	LocalRead         bool
	LocalFav          bool
	SnapshotRead      bool
	SnapshotFav       bool
}

// InitFoloSyncTable creates the Folo entry mapping table if it doesn't exist.
func InitFoloSyncTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS folo_entry_map (
		article_id INTEGER PRIMARY KEY,
		entry_id TEXT NOT NULL UNIQUE,
		article_url TEXT NOT NULL,
		published_at TEXT NOT NULL DEFAULT '',
		last_read INTEGER NOT NULL DEFAULT 0,
		last_favorite INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_folo_entry_map_entry ON folo_entry_map(entry_id);
	CREATE INDEX IF NOT EXISTS idx_folo_entry_map_url ON folo_entry_map(article_url);
	`
	_, err := db.Exec(query)
	if err != nil {
		return err
	}
	// Databases created before published_at existed get it added in place; the
	// duplicate-column error is the expected outcome for current ones.
	_, _ = db.Exec(`ALTER TABLE folo_entry_map ADD COLUMN published_at TEXT NOT NULL DEFAULT ''`)
	return nil
}

// UpsertFoloEntry records that a local article mirrors a Folo entry. One remote
// entry can only belong to one local article, so an older link to the same
// entry is replaced rather than duplicated.
func (db *DB) UpsertFoloEntry(articleID int64, entryID, articleURL, publishedAt string, read, favorite bool) error {
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
		INSERT INTO folo_entry_map (article_id, entry_id, article_url, published_at, last_read, last_favorite, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(article_id) DO UPDATE SET
			entry_id = excluded.entry_id,
			article_url = excluded.article_url,
			published_at = CASE
				WHEN excluded.published_at != '' THEN excluded.published_at
				ELSE folo_entry_map.published_at
			END,
			last_read = excluded.last_read,
			last_favorite = excluded.last_favorite,
			updated_at = excluded.updated_at`,
		articleID, entryID, articleURL, publishedAt, boolToInt(read), boolToInt(favorite), time.Now().UnixMilli())
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SetFoloEntryPublishedAt records the entry's own publish time. It is the only
// window the entry API can be queried by, and it is not necessarily the article's
// publish time: matching is by URL, so the two sides can disagree.
func (db *DB) SetFoloEntryPublishedAt(articleID int64, publishedAt string) error {
	if publishedAt == "" {
		return nil
	}
	db.WaitForReady()
	_, err := db.Exec(
		"UPDATE folo_entry_map SET published_at = ? WHERE article_id = ? AND published_at != ?",
		publishedAt, articleID, publishedAt)
	return err
}

// ListFoloMappedArticles returns mappings whose local article still exists,
// least recently touched first.
//
// The batch is a window, not a snapshot of "the interesting rows": ordering by
// updated_at DESC meant the same 500 rows were re-read on every pass and a
// mapping older than the window could never be reached again. Oldest-first with
// a stable tiebreak rotates the whole table through the budget, because every
// processed row is stamped by UpdateFoloSnapshot and moves to the back.
func (db *DB) ListFoloMappedArticles(limit int) ([]FoloMappedArticle, error) {
	db.WaitForReady()
	if limit <= 0 {
		limit = 500
	}
	rows, err := db.Query(`
		SELECT m.article_id, m.entry_id, m.article_url,
		       COALESCE(m.published_at, ''), COALESCE(a.published_at, ''), a.is_read, a.is_favorite, m.last_read, m.last_favorite
		FROM folo_entry_map m
		JOIN articles a ON a.id = m.article_id
		ORDER BY m.updated_at ASC, m.article_id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mapped := make([]FoloMappedArticle, 0, limit)
	for rows.Next() {
		var item FoloMappedArticle
		if err := rows.Scan(
			&item.ArticleID, &item.EntryID, &item.ArticleURL, &item.RemotePublishedAt, &item.PublishedAt,
			&item.LocalRead, &item.LocalFav, &item.SnapshotRead, &item.SnapshotFav,
		); err != nil {
			return nil, err
		}
		mapped = append(mapped, item)
	}
	return mapped, rows.Err()
}

// ListFoloMappingsForRecheck returns mappings with an article_id greater than
// afterID, oldest link first.
//
// Re-reading an old entry needs its own cursor rather than the batch window of
// ListFoloMappedArticles: that window is rotated by UpdateFoloSnapshot, and a
// recheck that stamped rows would push pending local changes further back in the
// push queue.
func (db *DB) ListFoloMappingsForRecheck(afterID int64, limit int) ([]FoloMappedArticle, error) {
	db.WaitForReady()
	if limit <= 0 {
		limit = 8
	}
	rows, err := db.Query(`
		SELECT m.article_id, m.entry_id, m.article_url,
		       COALESCE(m.published_at, ''), COALESCE(a.published_at, ''), a.is_read, a.is_favorite, m.last_read, m.last_favorite
		FROM folo_entry_map m
		JOIN articles a ON a.id = m.article_id
		WHERE m.article_id > ?
		ORDER BY m.article_id ASC
		LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mapped := make([]FoloMappedArticle, 0, limit)
	for rows.Next() {
		var item FoloMappedArticle
		if err := rows.Scan(
			&item.ArticleID, &item.EntryID, &item.ArticleURL, &item.RemotePublishedAt, &item.PublishedAt,
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
// UpdateFoloSnapshot records the state both sides last agreed on and pushes the
// mapping to the back of the next batch.
//
// The stamp is in milliseconds: a pass rewrites every row it processes, and with
// one-second resolution a batch of 500 rows produced 500 identical stamps, so the
// "oldest first" order could keep picking the same rows and never reach the rest.
// Rows written by older builds carry seconds, which sort before milliseconds, so
// the first pass after upgrading rewrites them and the units converge.
func (db *DB) UpdateFoloSnapshot(articleID int64, read, favorite bool) error {
	db.WaitForReady()
	_, err := db.Exec(
		"UPDATE folo_entry_map SET last_read = ?, last_favorite = ?, updated_at = ? WHERE article_id = ?",
		boolToInt(read), boolToInt(favorite), time.Now().UnixMilli(), articleID)
	return err
}

// UpdateFoloSnapshotPreserveOrder updates a recheck result without moving the
// mapping in the push rotation. A recheck must not make a pending local change
// look newer than untouched mappings.
func (db *DB) UpdateFoloSnapshotPreserveOrder(articleID int64, read, favorite bool) error {
	db.WaitForReady()
	_, err := db.Exec(
		"UPDATE folo_entry_map SET last_read = ?, last_favorite = ? WHERE article_id = ?",
		boolToInt(read), boolToInt(favorite), articleID)
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
