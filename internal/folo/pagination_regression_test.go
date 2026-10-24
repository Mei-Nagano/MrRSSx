package folo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"MrRSS/internal/database"
	"MrRSS/internal/models"
)

func TestSyncMapsAVisibleBoundaryBandOnce(t *testing.T) {
	boundary := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	entries := make([]Entry, 0, 102)
	for i := 0; i < 99; i++ {
		entries = append(entries, f04RegressionEntry("new", i, boundary.Add(time.Minute)))
	}
	for i := 0; i < 2; i++ {
		entries = append(entries, f04RegressionEntry("boundary", i, boundary))
	}
	entries = append(entries, f04RegressionEntry("old", 0, boundary.Add(-time.Hour)))

	db, syncer, feed := newF04MappedFixture(t, entries)
	feed.set(entries)
	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertF04MappedResult(t, db, result, len(entries), false)
}

func TestSyncMapsBoundaryAcrossMultipleFullPages(t *testing.T) {
	boundary := time.Date(2026, 10, 24, 11, 0, 0, 0, time.UTC)
	entries := make([]Entry, 0, 203)
	for i := 0; i < 97; i++ {
		entries = append(entries, f04RegressionEntry("new", i, boundary.Add(time.Minute)))
	}
	for i := 0; i < 5; i++ {
		entries = append(entries, f04RegressionEntry("boundary", i, boundary))
	}
	for i := 0; i < 101; i++ {
		entries = append(entries, f04RegressionEntry("old", i, boundary.Add(-time.Duration(i+1)*time.Minute)))
	}

	db, syncer, feed := newF04MappedFixture(t, entries)
	feed.set(entries)
	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertF04MappedResult(t, db, result, len(entries), false)
}

func TestSyncMapsSubMillisecondSuccessor(t *testing.T) {
	base := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	entries := make([]Entry, 0, 101)
	for i := 0; i < 100; i++ {
		entries = append(entries, f04RegressionEntry("regular", i, base.Add(-time.Duration(i)*time.Second)))
	}
	entries = append(entries, f04RegressionEntry("half-ms", 0, base.Add(-99*time.Second-500*time.Microsecond)))

	db, syncer, feed := newF04MappedFixture(t, entries)
	feed.set(entries)
	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertF04MappedResult(t, db, result, len(entries), false)
}

func TestSyncMapsBoundaryWhileBackfilling(t *testing.T) {
	boundary := time.Date(2026, 10, 24, 13, 0, 0, 0, time.UTC)
	entries := make([]Entry, 0, 100)
	for i := 0; i < 99; i++ {
		entries = append(entries, f04RegressionEntry("backfill-band", i, boundary))
	}
	entries = append(entries, f04RegressionEntry("backfill-old", 0, boundary.Add(-time.Hour)))

	db, syncer, feed := newF04MappedFixture(t, entries)
	feed.set(entries)
	future := boundary.Add(time.Hour).Format(time.RFC3339)
	if err := db.SetSetting(SyncCursorKey, future); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(SyncTopKey, future); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(SyncBackfillKey, "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertF04MappedResult(t, db, result, len(entries), false)
	if cursor, _ := db.GetSetting(SyncBackfillKey); cursor != "" {
		t.Errorf("backfill cursor = %q, want it cleared after the complete band", cursor)
	}
}

func TestSyncReportsUnenumerableBackfillBand(t *testing.T) {
	boundary := time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)
	entries := make([]Entry, 0, 102)
	for i := 0; i < 101; i++ {
		entries = append(entries, f04RegressionEntry("backfill-overflow", i, boundary))
	}
	entries = append(entries, f04RegressionEntry("backfill-tail", 0, boundary.Add(-time.Hour)))

	db, syncer, feed := newF04MappedFixture(t, entries)
	feed.set(entries)
	future := boundary.Add(time.Hour).Format(time.RFC3339)
	if err := db.SetSetting(SyncCursorKey, future); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(SyncTopKey, future); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(SyncBackfillKey, "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	count, err := db.CountFoloMappings()
	if err != nil {
		t.Fatal(err)
	}
	if count != 101 || result.Matched != 101 || result.Skipped != 0 {
		t.Fatalf("result = %+v, mappings = %d; want 101 mapped and no skipped entries", result, count)
	}
	if !result.Truncated || result.TruncatedAt != boundary.Format(time.RFC3339) {
		t.Fatalf("result = %+v, want explicit truncation at the dense backfill band", result)
	}
}

func TestSyncContinuesBelowLiveTruncatedBand(t *testing.T) {
	boundary := time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC)
	entries := make([]Entry, 0, 701)
	for i := 0; i < 101; i++ {
		entries = append(entries, f04RegressionEntry("live-overflow", i, boundary))
	}
	for i := 0; i < 600; i++ {
		entries = append(entries, f04RegressionEntry("live-tail", i, boundary.Add(-time.Duration(i+1)*time.Minute)))
	}

	db, syncer, feed := newF04MappedFixture(t, entries)
	feed.set(entries)
	first, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	firstCount, err := db.CountFoloMappings()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || firstCount >= len(entries) {
		t.Fatalf("first result = %+v, mappings = %d; want a bounded partial walk", first, firstCount)
	}
	if cursor, _ := db.GetSetting(SyncBackfillKey); cursor == "" {
		t.Fatal("first truncated live walk did not preserve a continuation cursor")
	}

	second, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	secondCount, err := db.CountFoloMappings()
	if err != nil {
		t.Fatal(err)
	}
	// One of the 101 rows in the unsplittable band remains unknowable, but every
	// older row must still be reached by the continuation pass.
	if secondCount != len(entries)-1 || secondCount <= firstCount {
		t.Fatalf("second result = %+v, mappings = %d (first %d); want the old tail fully resumed", second, secondCount, firstCount)
	}
	oldest, err := db.GetArticleByURL(entries[len(entries)-1].URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, mapped, err := db.GetFoloMapping(oldest.ID); err != nil || !mapped {
		t.Fatalf("oldest tail mapping = %v (err %v), want the continuation to reach the tail", mapped, err)
	}
	if cursor, _ := db.GetSetting(SyncBackfillKey); cursor != "" {
		t.Errorf("backfill cursor = %q, want it cleared after the tail", cursor)
	}
}

func f04RegressionEntry(prefix string, index int, published time.Time) Entry {
	return Entry{
		ID:          fmt.Sprintf("%s-%03d", prefix, index),
		URL:         fmt.Sprintf("https://example.com/%s/%03d", prefix, index),
		PublishedAt: published.Format(time.RFC3339Nano),
	}
}

func newF04MappedFixture(t *testing.T, entries []Entry) (*database.DB, *Syncer, *timelineFeed) {
	t.Helper()
	db, syncer, feed := newTimelineFixture(t, nil)
	feedID, err := db.AddFeed(&models.Feed{Title: "Mapped", URL: "http://127.0.0.1:1/mapped"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		published, err := time.Parse(time.RFC3339Nano, entry.PublishedAt)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.ID, err)
		}
		article := &models.Article{
			FeedID:      feedID,
			Title:       entry.ID,
			URL:         entry.URL,
			PublishedAt: published,
		}
		if err := db.SaveArticle(article); err != nil {
			t.Fatalf("save %s: %v", entry.ID, err)
		}
	}
	return db, syncer, feed
}

func assertF04MappedResult(t *testing.T, db *database.DB, result SyncResult, want int, truncated bool) {
	t.Helper()
	count, err := db.CountFoloMappings()
	if err != nil {
		t.Fatal(err)
	}
	if count != want || result.Matched != want || result.Skipped != 0 {
		t.Fatalf("result = %+v, mappings = %d; want %d mapped, no skipped entries", result, count, want)
	}
	if result.Truncated != truncated {
		t.Fatalf("truncated = %v, want %v (result %+v)", result.Truncated, truncated, result)
	}
}
