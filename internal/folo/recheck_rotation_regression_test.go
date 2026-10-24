package folo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"MrRSS/internal/models"
)

func TestRemoteApplyPreservesPendingPushOrder(t *testing.T) {
	const total = 501
	base := time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC)
	entries := make([]Entry, total)
	for i := range entries {
		entries[i] = Entry{
			ID:          fmt.Sprintf("rotation-%03d", i),
			URL:         fmt.Sprintf("https://example.com/rotation/%03d", i),
			PublishedAt: base.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		}
	}
	db, syncer, _ := newTimelineFixture(t, entries)
	feedID, err := db.AddFeed(&models.Feed{Title: "Rotation", URL: "http://127.0.0.1:1/rotation"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		article := &models.Article{
			FeedID:      feedID,
			Title:       entry.ID,
			URL:         entry.URL,
			PublishedAt: base,
		}
		if err := db.SaveArticle(article); err != nil {
			t.Fatalf("save %s: %v", entry.ID, err)
		}
		stored, err := db.GetArticleByURL(entry.URL)
		if err != nil {
			t.Fatalf("load %s: %v", entry.ID, err)
		}
		if err := db.UpsertFoloEntry(stored.ID, entry.ID, entry.URL, entry.PublishedAt, false, false); err != nil {
			t.Fatalf("map %s: %v", entry.ID, err)
		}
	}
	pending, err := db.GetArticleByURL(entries[7].URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MarkArticleRead(pending.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE folo_entry_map SET updated_at = 1"); err != nil {
		t.Fatal(err)
	}

	assertPendingInPushWindow := func(stage string) {
		t.Helper()
		rows, err := db.ListFoloMappedArticles(syncMappedLimit)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ArticleID == pending.ID {
				if row.SnapshotRead {
					t.Errorf("%s advanced the snapshot over the pending local read", stage)
				}
				return
			}
		}
		t.Errorf("%s left the pending mapping outside the next push batch", stage)
	}

	if _, err := syncer.applyEntry(pending.ID, entries[7]); err != nil {
		t.Fatalf("live pull apply: %v", err)
	}
	assertPendingInPushWindow("live pull")

	if _, _, err := syncer.recheckRemote(context.Background()); err != nil {
		t.Fatalf("recheck: %v", err)
	}
	assertPendingInPushWindow("recheck")
}
