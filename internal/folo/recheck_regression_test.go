package folo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"MrRSS/internal/models"
)

func TestRecheckFindsAnEntryOutsideTheNarrowProbe(t *testing.T) {
	publishedTime := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	published := publishedTime.Format(time.RFC3339)
	entries := make([]Entry, 11)
	for i := range entries {
		entries[i] = Entry{
			ID:          fmt.Sprintf("dense-%02d", i),
			URL:         fmt.Sprintf("https://example.com/dense/%02d", i),
			PublishedAt: published,
			Read:        true,
		}
	}
	entries[len(entries)-2].Read = false
	db, syncer, feed := newTimelineFixture(t, entries)
	feedID, err := db.AddFeed(&models.Feed{Title: "Dense", URL: "http://127.0.0.1:1/dense"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		article := &models.Article{
			FeedID:      feedID,
			Title:       entry.ID,
			URL:         entry.URL,
			PublishedAt: publishedTime,
		}
		if err := db.SaveArticle(article); err != nil {
			t.Fatalf("save %s: %v", entry.ID, err)
		}
		stored, err := db.GetArticleByURL(entry.URL)
		if err != nil {
			t.Fatalf("load %s: %v", entry.ID, err)
		}
		if err := db.UpsertFoloEntry(stored.ID, entry.ID, entry.URL, published, false, false); err != nil {
			t.Fatalf("map %s: %v", entry.ID, err)
		}
		if entry.ID == entries[len(entries)-2].ID {
			if err := db.MarkArticleRead(stored.ID, true); err != nil {
				t.Fatalf("mark local pending %s: %v", entry.ID, err)
			}
		}
	}
	pending, err := db.GetArticleByURL(entries[len(entries)-2].URL)
	if err != nil {
		t.Fatal(err)
	}
	_, pendingSnapshot, _, mapped, err := db.GetFoloMapping(pending.ID)
	if err != nil || !mapped || pendingSnapshot {
		t.Fatalf("pending mapping = mapped:%v snapshot:%v err:%v, want an unread snapshot", mapped, pendingSnapshot, err)
	}
	last, err := db.GetArticleByURL(entries[len(entries)-1].URL)
	if err != nil {
		t.Fatal(err)
	}
	_, lastSnapshot, _, mapped, err := db.GetFoloMapping(last.ID)
	if err != nil || !mapped || lastSnapshot {
		t.Fatalf("expanded target mapping = mapped:%v snapshot:%v err:%v, want an unread snapshot", mapped, lastSnapshot, err)
	}
	future := publishedTime.Add(time.Hour).Format(time.RFC3339)
	if err := db.SetSetting(SyncTopKey, future); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(SyncCursorKey, future); err != nil {
		t.Fatal(err)
	}

	for pass := 0; pass < 2; pass++ {
		result, err := syncer.Sync(context.Background())
		if err != nil {
			t.Fatalf("Sync pass %d: %v", pass, err)
		}
		if result.RecheckIncomplete {
			t.Fatalf("pass %d reported an enumerable dense band as incomplete: %+v", pass, result)
		}
	}
	for _, entry := range entries {
		article, err := db.GetArticleByURL(entry.URL)
		if err != nil {
			t.Fatalf("load final %s: %v", entry.ID, err)
		}
		if !article.IsRead {
			t.Errorf("entry %s stayed unread after bounded expansion", entry.ID)
		}
	}
	last, err = db.GetArticleByURL(entries[len(entries)-1].URL)
	if err != nil {
		t.Fatal(err)
	}
	_, lastSnapshot, _, _, err = db.GetFoloMapping(last.ID)
	if err != nil || !lastSnapshot {
		t.Errorf("expanded target snapshot = %v (err %v), want the remote read state", lastSnapshot, err)
	}
	pending, err = db.GetArticleByURL(entries[len(entries)-2].URL)
	if err != nil {
		t.Fatal(err)
	}
	_, pendingSnapshot, _, _, err = db.GetFoloMapping(pending.ID)
	if err != nil || pendingSnapshot {
		t.Errorf("pending snapshot = %v (err %v), want it left pending after the failed push", pendingSnapshot, err)
	}

	foundNarrow, foundExpanded := false, false
	for _, call := range feed.calls() {
		if call.olderThan != shiftTime(published, time.Millisecond) {
			continue
		}
		if call.limit == syncRecheckWindow {
			foundNarrow = true
		}
		if call.limit == syncPageSize {
			foundExpanded = true
		}
	}
	if !foundNarrow || !foundExpanded {
		t.Fatalf("recheck did not show the bounded 10-row probe and 100-row expansion: narrow=%v expanded=%v calls=%+v", foundNarrow, foundExpanded, feed.calls())
	}
}

func TestRecheckReportsAnUnconfirmableDenseBand(t *testing.T) {
	publishedTime := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	published := publishedTime.Format(time.RFC3339)
	entries := make([]Entry, 101)
	for i := range entries {
		entries[i] = Entry{
			ID:          fmt.Sprintf("overflow-%03d", i),
			PublishedAt: published,
		}
	}
	db, syncer, feed := newTimelineFixture(t, entries)
	feedID, err := db.AddFeed(&models.Feed{Title: "Overflow", URL: "http://127.0.0.1:1/overflow"})
	if err != nil {
		t.Fatal(err)
	}
	article := &models.Article{
		FeedID:      feedID,
		Title:       "overflow",
		URL:         "https://example.com/overflow",
		PublishedAt: publishedTime,
	}
	if err := db.SaveArticle(article); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetArticleByURL(article.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertFoloEntry(stored.ID, entries[100].ID, article.URL, published, false, false); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkArticleRead(stored.ID, true); err != nil {
		t.Fatal(err)
	}
	future := publishedTime.Add(time.Hour).Format(time.RFC3339)
	if err := db.SetSetting(SyncTopKey, future); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(SyncCursorKey, future); err != nil {
		t.Fatal(err)
	}

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !result.RecheckIncomplete || result.RecheckIncompleteAt != published || !result.Truncated || result.TruncatedAt != published {
		t.Fatalf("result = %+v, want explicit bounded recheck incompleteness", result)
	}
	final, err := db.GetArticleByURL(article.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !final.IsRead {
		t.Error("unconfirmed remote state overwrote the pending local read")
	}
	_, snapshotRead, _, mapped, err := db.GetFoloMapping(stored.ID)
	if err != nil || !mapped || snapshotRead {
		t.Errorf("unconfirmed mapping snapshot = mapped:%v read:%v err:%v, want pending read snapshot", mapped, snapshotRead, err)
	}
	if len(feed.calls()) < 3 {
		t.Fatalf("expected narrow and expanded recheck calls, got %+v", feed.calls())
	}
}
