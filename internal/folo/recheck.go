package folo

import (
	"context"
	"fmt"
	"time"
)

// findRecheckEntry uses a cheap narrow lookup first, then one larger bounded
// lookup when the target is behind other entries with the same publish time.
// The entry endpoint has no offset or ID cursor, so a target still absent from
// the page cannot be confirmed; callers report that explicitly and leave its
// snapshot unchanged.
func (s *Syncer) findRecheckEntry(
	ctx context.Context,
	entryID, at string,
	slack time.Duration,
	initialLimit int,
) (Entry, bool, error) {
	batch, err := s.listRecheckWindow(ctx, at, slack, initialLimit)
	if err != nil {
		return Entry{}, false, fmt.Errorf("recheck Folo entry %s: %w", entryID, err)
	}
	if entry, found := findRecheckEntryByID(batch, entryID); found {
		return entry, true, nil
	}

	// A timestamp band larger than the normal ten-row probe needs one page-sized
	// retry. This covers dense but enumerable bands (including the 11th row)
	// without allowing an unbounded scan.
	if initialLimit < syncPageSize {
		batch, err = s.listRecheckWindow(ctx, at, slack, syncPageSize)
		if err != nil {
			return Entry{}, false, fmt.Errorf("expand recheck for Folo entry %s: %w", entryID, err)
		}
		if entry, found := findRecheckEntryByID(batch, entryID); found {
			return entry, true, nil
		}
	}
	return Entry{}, false, nil
}

func (s *Syncer) listRecheckWindow(
	ctx context.Context,
	at string,
	slack time.Duration,
	limit int,
) ([]Entry, error) {
	return s.client.ListEntries(ctx, EntryListParams{
		Limit:     limit,
		NewerThan: shiftTime(at, -slack),
		OlderThan: shiftTime(at, slack),
	})
}

func findRecheckEntryByID(entries []Entry, id string) (Entry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}
