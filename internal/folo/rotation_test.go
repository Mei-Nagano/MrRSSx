package folo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"MrRSS/internal/database"
	"MrRSS/internal/models"
)

// The push batch is the oldest mappings, so a row with nothing to push must still
// be stamped. Without that it kept its place at the front forever and a pending
// change sitting behind the window was never reached — three full syncs in a row
// pushed nothing.
func TestSyncRotatesUnchangedMappingsIntoThePushWindow(t *testing.T) {
	server, recorder := newSyncFolo(t)
	db, err := database.NewDB("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	client, err := NewClientWithHTTPClient(server.URL, "session", server.Client())
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	syncer := NewSyncer(client, db)

	feedID, err := db.AddFeed(&models.Feed{Title: "f", URL: "http://127.0.0.1:1/f.xml"})
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}

	const total = 501
	pendingEntryID := fmt.Sprintf("E%d", total-1)
	for i := 0; i < total; i++ {
		url := fmt.Sprintf("https://example.com/rot/%d", i)
		article := &models.Article{
			Title: fmt.Sprintf("a%d", i), URL: url, FeedID: feedID,
			UniqueID:    fmt.Sprintf("u%d", i),
			PublishedAt: time.Date(2026, 10, 20, 10, 0, i%60, 0, time.UTC),
		}
		if err := db.SaveArticle(article); err != nil {
			t.Fatalf("save article %d: %v", i, err)
		}
		var id int64
		if err := db.QueryRow("SELECT id FROM articles WHERE url = ?", url).Scan(&id); err != nil {
			t.Fatalf("lookup article %d: %v", i, err)
		}
		if err := db.UpsertFoloEntry(id, fmt.Sprintf("E%d", i), url,
			article.PublishedAt.Format(time.RFC3339), false, false); err != nil {
			t.Fatalf("map %d: %v", i, err)
		}
		// The last mapping carries the only pending local change.
		if i == total-1 {
			if err := db.MarkArticleRead(id, true); err != nil {
				t.Fatalf("mark read: %v", err)
			}
		}
	}

	for pass := 0; pass < 3; pass++ {
		if _, err := syncer.Sync(context.Background()); err != nil {
			t.Fatalf("sync %d: %v", pass, err)
		}
	}

	recorder.mu.Lock()
	pushed := false
	for _, ids := range recorder.readIDs {
		for _, id := range ids {
			if id == pendingEntryID {
				pushed = true
			}
		}
	}
	recorder.mu.Unlock()
	if !pushed {
		t.Errorf("the pending change on %s was never pushed through three syncs", pendingEntryID)
	}
}
