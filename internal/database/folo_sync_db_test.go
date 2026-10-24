package database

import (
	"fmt"
	"testing"

	"MrRSS/internal/models"
)

// The mapping batch is a bounded window over a table that can be larger than it.
// It used to be "the 500 most recently touched", so a mapping older than that
// window could never be reached again and its pending local change never shipped.
// Oldest-first plus a stamp on every processed row rotates the whole table
// through the budget.
func TestListFoloMappedArticlesRotatesOlderMappingsIntoTheBatch(t *testing.T) {
	db, err := NewDB("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	feedID, err := db.AddFeed(&models.Feed{Title: "feed", URL: "https://example.org/feed"})
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}

	const total = 501
	const batch = 500
	for i := 0; i < total; i++ {
		article := &models.Article{
			Title:  fmt.Sprintf("article %d", i),
			URL:    fmt.Sprintf("https://example.org/%d", i),
			FeedID: feedID,
		}
		if err := db.SaveArticle(article); err != nil {
			t.Fatalf("save article %d: %v", i, err)
		}
		entryID := fmt.Sprintf("E%04d", i)
		var articleID int64
		if err := db.QueryRow("SELECT id FROM articles WHERE url = ?", article.URL).Scan(&articleID); err != nil {
			t.Fatalf("lookup article %d: %v", i, err)
		}
		if err := db.UpsertFoloEntry(articleID, entryID, article.URL, "", false, false); err != nil {
			t.Fatalf("map article %d: %v", i, err)
		}
	}

	seen := make(map[int64]bool, total)
	// Two bounded passes must cover all 501: the first stamps the 500 rows it
	// returned, which pushes them behind the row it could not reach.
	for pass := 0; pass < 2; pass++ {
		rows, err := db.ListFoloMappedArticles(batch)
		if err != nil {
			t.Fatalf("list pass %d: %v", pass, err)
		}
		if len(rows) != batch {
			t.Fatalf("pass %d returned %d rows, want %d", pass, len(rows), batch)
		}
		for _, row := range rows {
			seen[row.ArticleID] = true
			if err := db.UpdateFoloSnapshot(row.ArticleID, row.SnapshotRead, row.SnapshotFav); err != nil {
				t.Fatalf("stamp article %d: %v", row.ArticleID, err)
			}
		}
	}

	if len(seen) != total {
		t.Fatalf("two bounded passes covered %d of %d mappings", len(seen), total)
	}
}
