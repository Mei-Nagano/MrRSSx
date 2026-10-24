package folo

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A page can end inside one publish time while older entries exist below it. The
// next page is then short but not empty, which used to look like a complete walk
// and reported success while one entry at the boundary was never listed.
func TestSyncReportsTruncationWhenOlderEntriesExistBelowTheBand(t *testing.T) {
	boundary := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	older := boundary.Add(-time.Hour)

	entries := make([]Entry, 0, 106)
	for i := 0; i < 101; i++ {
		entries = append(entries, Entry{
			ID:          fmt.Sprintf("B%03d", i),
			URL:         fmt.Sprintf("https://example.com/band/%d", i),
			PublishedAt: boundary.Format(time.RFC3339),
		})
	}
	for i := 0; i < 5; i++ {
		entries = append(entries, Entry{
			ID:          fmt.Sprintf("O%03d", i),
			URL:         fmt.Sprintf("https://example.com/older/%d", i),
			PublishedAt: older.Format(time.RFC3339),
		})
	}

	_, syncer, _ := newTimelineFixture(t, entries)

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Skipped != 105 {
		t.Errorf("skipped = %d, want 105 (one full page plus the older rows)", result.Skipped)
	}
	if !result.Truncated {
		t.Fatal("a page ending inside one publish time with rows below it must be reported as truncated")
	}
	if result.TruncatedAt != boundary.Format(time.RFC3339) {
		t.Errorf("truncatedAt = %q, want the boundary %q", result.TruncatedAt, boundary.Format(time.RFC3339))
	}
	// The marker is sticky: a later pass cannot recover the gap either.
	second, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if !second.Truncated {
		t.Error("the gap stopped being reported on the next pass")
	}
}

// A page can end inside a publish time that holds only a few entries: the band is
// enumerable, so nothing is actually lost. Reporting it as truncated was a false
// positive, and the rows the page cut were never picked up either.
func TestSyncRecoversTheBandWhenItCanBeEnumerated(t *testing.T) {
	boundary := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	newer := boundary.Add(time.Minute)

	entries := make([]Entry, 0, 102)
	for i := 0; i < 97; i++ {
		entries = append(entries, Entry{
			ID:          fmt.Sprintf("N%03d", i),
			URL:         fmt.Sprintf("https://example.com/newer/%d", i),
			PublishedAt: newer.Format(time.RFC3339),
		})
	}
	for i := 0; i < 5; i++ {
		entries = append(entries, Entry{
			ID:          fmt.Sprintf("B%03d", i),
			URL:         fmt.Sprintf("https://example.com/band/%d", i),
			PublishedAt: boundary.Format(time.RFC3339),
		})
	}

	_, syncer, _ := newTimelineFixture(t, entries)

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// 100 rows in the first page, 2 on the next one, and the 2 the page cut are
	// fetched back from the band. Without the recovery this stays at 102.
	if result.Skipped != 104 {
		t.Errorf("skipped = %d, want 104 (the cut rows must be fetched back)", result.Skipped)
	}
	if result.Truncated {
		t.Errorf("a band of five that fits in one page is not a gap (truncatedAt=%q)", result.TruncatedAt)
	}
}
