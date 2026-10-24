package folo

import (
	"context"
	"testing"
	"time"

	"MrRSS/internal/database"
	"MrRSS/internal/models"
)

// A state change made to an entry long after it was published never enters a live
// window: the pull only reads entries newer than the cursor. F01 covers exactly
// that, so the syncer re-reads older mappings through a narrow window around each
// entry's own publish time and applies what it finds through the normal
// local-change-wins path.
func TestSyncRechecksOldEntryReadState(t *testing.T) {
	publishedTime := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	published := publishedTime.Format(time.RFC3339)
	db, syncer, feed := newTimelineFixture(t, []Entry{{
		ID: "E1", URL: syncArticleURL, PublishedAt: published, Read: true, Collected: true,
	}, {
		// Moves the cursor past the old entry, so only the recheck can reach it again.
		ID: "E2", URL: syncArticleURL + "/newer", PublishedAt: publishedTime.Add(time.Hour).Format(time.RFC3339), Read: true,
	}})

	feedID, err := db.AddFeed(&models.Feed{Title: "Example", URL: "http://127.0.0.1:1/feed.xml"})
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}
	if err := db.SaveArticle(&models.Article{
		Title: "Example", URL: syncArticleURL, FeedID: feedID, PublishedAt: publishedTime,
	}); err != nil {
		t.Fatalf("save article: %v", err)
	}

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if read, fav := recheckState(t, db); !read || !fav {
		t.Fatalf("after the first pull: read=%v fav=%v, want both true", read, fav)
	}

	// The user marks the old entry unread again, long after publication.
	feed.set([]Entry{{ID: "E1", URL: syncArticleURL, PublishedAt: published}})

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if read, fav := recheckState(t, db); read || fav {
		t.Errorf("an old entry changed remotely stayed stale locally: read=%v fav=%v", read, fav)
	}

	// The change can only have arrived through the narrow re-read window; the live
	// pull is bounded by the cursor at the entry's own publish time.
	window := shiftTime(published, time.Millisecond)
	rechecked := false
	for _, call := range feed.calls() {
		if call.limit == syncRecheckWindow && call.olderThan == window {
			rechecked = true
			break
		}
	}
	if !rechecked {
		t.Errorf("no recheck window (%q) was queried for the old entry", window)
	}
}

func recheckState(t *testing.T, db *database.DB) (bool, bool) {
	t.Helper()
	var read, fav int
	if err := db.QueryRow("SELECT is_read, is_favorite FROM articles WHERE url = ?", syncArticleURL).Scan(&read, &fav); err != nil {
		t.Fatalf("read local state: %v", err)
	}
	return read != 0, fav != 0
}

// The entry and the article can disagree on the publication instant; matching is
// by URL. Aiming the re-read window at the article's time missed the entry
// entirely, so the mapping has to carry the entry's own time.
func TestRecheckUsesTheEntriesOwnPublishTime(t *testing.T) {
	remoteTime := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	localTime := remoteTime.Add(2 * time.Hour)
	db, syncer, feed := newTimelineFixture(t, []Entry{{
		ID: "E1", URL: syncArticleURL, PublishedAt: remoteTime.Format(time.RFC3339), Read: true,
	}})

	feedID, err := db.AddFeed(&models.Feed{Title: "Example", URL: "http://127.0.0.1:1/feed.xml"})
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}
	if err := db.SaveArticle(&models.Article{
		Title: "Example", URL: syncArticleURL, FeedID: feedID, PublishedAt: localTime,
	}); err != nil {
		t.Fatalf("save article: %v", err)
	}

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	// Move the cursor past the entry so only the recheck can reach it.
	feed.set([]Entry{
		{ID: "E1", URL: syncArticleURL, PublishedAt: remoteTime.Format(time.RFC3339)},
		{ID: "E2", URL: syncArticleURL + "/newer",
			PublishedAt: localTime.Add(time.Hour).Format(time.RFC3339), Read: true},
	})

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if read := recheckRead(t, db); read {
		t.Error("an entry whose own time differs from the article's was never re-read")
	}
	found := false
	for _, call := range feed.calls() {
		if call.olderThan == shiftTime(remoteTime.Format(time.RFC3339), time.Millisecond) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no window was opened around the entry's own time %s", remoteTime)
	}
}

// A full recheck page advanced the watermark to its last id; with no more rows the
// next pass queried past the end and rechecking stopped permanently.
func TestRecheckRewindsAtTheEndOfTheTable(t *testing.T) {
	// A watermark left past the last row by a full page used to be terminal: every
	// later pass queried above it, found nothing and returned, so rechecking stopped
	// for good. An empty result must rewind the sweep instead.
	db, syncer, _ := newTimelineFixture(t, nil)
	if err := db.SetSetting(SyncRecheckKey, "5"); err != nil {
		t.Fatalf("set cursor: %v", err)
	}

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	cursor, _ := db.GetSetting(SyncRecheckKey)
	if cursor != "0" {
		t.Errorf("recheck cursor = %q, want it rewound to 0 so the sweep runs again", cursor)
	}
}
func recheckRead(t *testing.T, db *database.DB) bool {
	t.Helper()
	var read int
	if err := db.QueryRow("SELECT is_read FROM articles WHERE url = ?", syncArticleURL).Scan(&read); err != nil {
		t.Fatalf("read local state: %v", err)
	}
	return read != 0
}
