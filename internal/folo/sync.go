package folo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"MrRSS/internal/database"
)

const (
	// SyncCursorKey stores how far the last pull reached and LastSyncKey the
	// wall-clock time of the last pass. Both are backend-owned and therefore not
	// part of the settings schema.
	SyncCursorKey = "folo_sync_cursor"
	LastSyncKey   = "folo_last_sync_time"
	// SyncTopKey is the newest timeline entry already processed. Live pulls only
	// read above it, so an unfinished backfill does not force a rescan.
	SyncTopKey = "folo_sync_top"
	// SyncBackfillKey marks a lower bound that a pass ran out of budget before
	// reaching, so the next pass continues downwards instead of skipping it.
	SyncBackfillKey = "folo_sync_backfill"

	syncMappedLimit = 500
	syncPageSize    = 100
	syncPageBudget  = 5
	// syncFavoriteLimit bounds the extra pass that catches entries saved to the
	// collection but published before the timeline cursor.
	syncFavoriteLimit = 100
	// initialLookback covers first-run and recovery pulls; later runs continue from
	// the last cursor.
	initialLookback = 7 * 24 * time.Hour
)

// SyncResult reports what a synchronization pass changed.
type SyncResult struct {
	Pushed   int    `json:"pushed"`
	Pulled   int    `json:"pulled"`
	Matched  int    `json:"matched"`
	Skipped  int    `json:"skipped"`
	Mapped   int    `json:"mapped"`
	LastSync string `json:"last_sync_time"`
}

// Syncer reconciles read and favorite state between MrRSS and Folo.
//
// Local articles are matched to Folo entries by URL. A mapping row stores the
// state both sides last agreed on, so the next pass can tell whether the local
// side changed; local changes win when both sides moved.
type Syncer struct {
	client *Client
	db     *database.DB
}

func NewSyncer(client *Client, db *database.DB) *Syncer {
	return &Syncer{client: client, db: db}
}

// Sync runs one pass: pull remote state, then push local changes. Pulling first
// means an entry matched during this pass can have its local state pushed in the
// same pass instead of waiting for the next one.
func (s *Syncer) Sync(ctx context.Context) (SyncResult, error) {
	var result SyncResult

	if _, err := s.db.DeleteOrphanedFoloEntries(); err != nil {
		log.Printf("folo sync: clean orphaned mappings: %v", err)
	}

	pulled, matched, skipped, next, err := s.pullRemote(ctx)
	result.Pulled = pulled
	result.Matched = matched
	result.Skipped = skipped
	if err != nil {
		return result, err
	}

	if next.cursor != "" {
		if err := s.db.SetSetting(SyncCursorKey, next.cursor); err != nil {
			log.Printf("folo sync: record sync cursor: %v", err)
		}
	}
	if next.top != "" {
		if err := s.db.SetSetting(SyncTopKey, next.top); err != nil {
			log.Printf("folo sync: record sync top: %v", err)
		}
	}
	if err := s.db.SetSetting(SyncBackfillKey, next.backfill); err != nil {
		log.Printf("folo sync: record backfill cursor: %v", err)
	}

	pushed, err := s.pushLocal(ctx)
	result.Pushed = pushed
	if err != nil {
		return result, err
	}

	completed := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.SetSetting(LastSyncKey, completed); err != nil {
		log.Printf("folo sync: record sync time: %v", err)
	} else {
		result.LastSync = completed
	}

	if mapped, countErr := s.db.CountFoloMappings(); countErr == nil {
		result.Mapped = mapped
	}
	return result, nil
}

// pushLocal sends state changes made in MrRSS to Folo for already-mapped
// articles, and advances the snapshot only for the dimensions that succeeded.
func (s *Syncer) pushLocal(ctx context.Context) (int, error) {
	mapped, err := s.db.ListFoloMappedArticles(syncMappedLimit)
	if err != nil {
		return 0, fmt.Errorf("list Folo mappings: %w", err)
	}

	toRead := make([]string, 0, len(mapped))
	toUnread := make([]string, 0, len(mapped))
	for _, item := range mapped {
		if item.LocalRead == item.SnapshotRead {
			continue
		}
		if item.LocalRead {
			toRead = append(toRead, item.EntryID)
		} else {
			toUnread = append(toUnread, item.EntryID)
		}
	}

	readDone := make(map[string]bool, len(toRead)+len(toUnread))
	if err := s.client.MarkEntriesRead(ctx, toRead); err != nil {
		if errors.Is(err, ErrNotAuthenticated) {
			return 0, err
		}
		log.Printf("folo sync: mark %d entries read: %v", len(toRead), err)
	} else {
		for _, id := range toRead {
			readDone[id] = true
		}
	}
	for _, id := range toUnread {
		if err := s.client.MarkEntryUnread(ctx, id); err != nil {
			if errors.Is(err, ErrNotAuthenticated) {
				return 0, err
			}
			log.Printf("folo sync: mark entry %s unread: %v", id, err)
			continue
		}
		readDone[id] = true
	}

	favoriteDone := make(map[string]bool, len(mapped))
	pushed := 0
	for _, item := range mapped {
		if item.LocalFav != item.SnapshotFav {
			var favErr error
			if item.LocalFav {
				favErr = s.client.AddToCollection(ctx, item.EntryID)
			} else {
				favErr = s.client.RemoveFromCollection(ctx, item.EntryID)
			}
			if favErr != nil {
				if errors.Is(favErr, ErrNotAuthenticated) {
					return 0, favErr
				}
				log.Printf("folo sync: update collection for entry %s: %v", item.EntryID, favErr)
			} else {
				favoriteDone[item.EntryID] = true
			}
		}

		readChanged := item.LocalRead != item.SnapshotRead
		favChanged := item.LocalFav != item.SnapshotFav
		if !readChanged && !favChanged {
			continue
		}
		// Only advance a dimension that actually reached Folo; otherwise the change
		// stays pending for the next pass.
		snapshotRead := item.SnapshotRead
		if readDone[item.EntryID] {
			snapshotRead = item.LocalRead
			pushed++
		}
		snapshotFav := item.SnapshotFav
		if favoriteDone[item.EntryID] {
			snapshotFav = item.LocalFav
			pushed++
		}
		if err := s.db.UpdateFoloSnapshot(item.ArticleID, snapshotRead, snapshotFav); err != nil {
			log.Printf("folo sync: update snapshot for article %d: %v", item.ArticleID, err)
		}
	}
	return pushed, nil
}

// pullCursor is where the next pull should start.
type pullCursor struct {
	// cursor is the sweep's current bottom, meaningful while a backfill runs.
	cursor string
	// top is the newest entry processed so far.
	top string
	// backfill is a lower bound this pass ran out of budget before reaching.
	backfill string
}

// pullRemote applies Folo state to local articles. It returns the number of
// local updates, matches, skips, and where to resume.
//
// Two windows are read, newest first:
//   - the live window above the newest entry processed so far, which keeps
//     newly published (and newly read) articles flowing while a backlog exists;
//   - the backfill window below the stored cursor, resumed until it reaches the
//     target a previous pass ran out of budget before reaching.
func (s *Syncer) pullRemote(ctx context.Context) (updated, matched, skipped int, next pullCursor, err error) {
	since := s.lastSyncCursor()
	backfill := s.pendingBackfill()

	seen := make(map[string]bool)
	entries := make([]Entry, 0, syncPageSize)
	pagesUsed := 0

	liveFrom, err := s.livePullFrom(since)
	if err != nil {
		return updated, matched, skipped, next, err
	}
	newest, oldestLive := "", ""
	liveCaughtUp, liveProgressed := false, true
	for pagesUsed < syncPageBudget {
		params := EntryListParams{Limit: syncPageSize}
		if pagesUsed == 0 {
			params.NewerThan = shiftTime(liveFrom, -time.Millisecond)
		} else if oldestLive != "" {
			params.OlderThan = shiftTime(oldestLive, -time.Millisecond)
		} else {
			break
		}

		batch, listErr := s.client.ListEntries(ctx, params)
		if listErr != nil {
			return updated, matched, skipped, next, fmt.Errorf("list Folo entries: %w", listErr)
		}
		pagesUsed++
		added, batchNewest, batchOldest := appendEntries(seen, &entries, batch)
		newest = later(newest, batchNewest)
		oldestLive = earlier(oldestLive, batchOldest)
		if len(batch) < syncPageSize {
			liveCaughtUp = true
			break
		}
		if added == 0 {
			liveProgressed = false
			break
		}
	}

	oldestBackfill := ""
	backfillPages := 0
	backfillDone, backfillProgressed := false, true
	for backfill != "" && pagesUsed < syncPageBudget {
		params := EntryListParams{Limit: syncPageSize}
		if backfillPages == 0 {
			// Continue the gap from its top edge, moving downwards.
			params.OlderThan = shiftTime(since, -time.Millisecond)
		} else if oldestBackfill != "" {
			params.OlderThan = shiftTime(oldestBackfill, -time.Millisecond)
		} else {
			break
		}

		batch, listErr := s.client.ListEntries(ctx, params)
		if listErr != nil {
			return updated, matched, skipped, next, fmt.Errorf("list Folo entries: %w", listErr)
		}
		pagesUsed++
		backfillPages++
		added, _, batchOldest := appendEntries(seen, &entries, batch)
		oldestBackfill = earlier(oldestBackfill, batchOldest)
		if len(batch) < syncPageSize {
			backfillDone = true
			break
		}
		if added == 0 {
			backfillProgressed = false
			break
		}
		if oldestBackfill != "" && !timestampAfter(oldestBackfill, backfill) {
			backfillDone = true
			break
		}
	}

	switch {
	case backfill != "":
		switch {
		case backfillDone:
			// Coverage is contiguous down to the target the previous pass set.
			next.cursor = backfill
		case backfillPages > 0 && backfillProgressed:
			next.cursor = oldestBackfill
			next.backfill = backfill
		default:
			next.cursor = since
			next.backfill = backfill
		}
	case liveCaughtUp:
		if newest != "" {
			next.cursor = newest
		} else {
			next.cursor = liveFrom
		}
	case liveProgressed && oldestLive != "":
		// The page budget ran out above the cursor; walk the unread range down
		// from the oldest entry seen until it reaches the old live top.
		next.cursor = oldestLive
		next.backfill = liveFrom
	default:
		next.cursor = liveFrom
	}
	next.top = newest
	if next.top == "" {
		next.top = liveFrom
	}

	// Favorites can point at entries published before the cursor, so they get a
	// bounded extra pass.
	collected := true
	if favorites, listErr := s.client.ListEntries(ctx, EntryListParams{
		Limit:        syncFavoriteLimit,
		IsCollection: &collected,
	}); listErr != nil {
		if errors.Is(listErr, ErrNotAuthenticated) {
			return updated, matched, skipped, next, listErr
		}
		log.Printf("folo sync: list collected entries: %v", listErr)
	} else {
		for _, entry := range favorites {
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			entries = append(entries, entry)
		}
	}

	for _, entry := range entries {
		if entry.URL == "" {
			skipped++
			continue
		}
		article, findErr := s.db.GetArticleByURL(entry.URL)
		if errors.Is(findErr, sql.ErrNoRows) || (findErr == nil && article == nil) {
			skipped++
			continue
		}
		if findErr != nil {
			return updated, matched, skipped, next, fmt.Errorf("find article for Folo entry: %w", findErr)
		}
		matched++

		applied, applyErr := s.applyEntry(article.ID, entry)
		if applyErr != nil {
			return updated, matched, skipped, next, applyErr
		}
		updated += applied
	}
	return updated, matched, skipped, next, nil
}

// applyEntry reconciles one matched entry and returns how many local fields it
// changed. A local change made since the last snapshot wins over the remote one.
func (s *Syncer) applyEntry(articleID int64, entry Entry) (int, error) {
	_, snapshotRead, snapshotFav, mapped, err := s.db.GetFoloMapping(articleID)
	if err != nil {
		return 0, fmt.Errorf("read Folo mapping: %w", err)
	}

	if !mapped {
		// The snapshot keeps Folo's value on purpose: local state that differs is
		// then pushed in the same pass instead of being silently cleared.
		if err := s.db.UpsertFoloEntry(articleID, entry.ID, entry.URL, entry.Read, entry.Collected); err != nil {
			return 0, fmt.Errorf("store Folo mapping: %w", err)
		}
		changed := 0
		if entry.Read {
			if err := s.db.MarkArticleRead(articleID, true); err != nil {
				return 0, err
			}
			changed++
		}
		if entry.Collected {
			if err := s.db.SetArticleFavorite(articleID, true); err != nil {
				return 0, err
			}
			changed++
		}
		return changed, nil
	}

	article, err := s.db.GetArticleByID(articleID)
	if err != nil {
		return 0, fmt.Errorf("load article: %w", err)
	}

	localReadPending := article.IsRead != snapshotRead
	localFavPending := article.IsFavorite != snapshotFav

	changed := 0
	if !localReadPending && article.IsRead != entry.Read {
		if err := s.db.MarkArticleRead(articleID, entry.Read); err != nil {
			return 0, err
		}
		changed++
	}
	if !localFavPending && article.IsFavorite != entry.Collected {
		if err := s.db.SetArticleFavorite(articleID, entry.Collected); err != nil {
			return 0, err
		}
		changed++
	}

	// Snapshot advances only for dimensions actually reconciled; a pending local
	// change keeps the old snapshot so the push picks it up.
	nextRead := snapshotRead
	if !localReadPending {
		nextRead = entry.Read
	}
	nextFav := snapshotFav
	if !localFavPending {
		nextFav = entry.Collected
	}
	if err := s.db.UpdateFoloSnapshot(articleID, nextRead, nextFav); err != nil {
		return 0, fmt.Errorf("advance Folo snapshot: %w", err)
	}
	return changed, nil
}

// lastSyncCursor returns where the previous pull stopped.
func (s *Syncer) lastSyncCursor() string {
	value, err := s.db.GetSetting(SyncCursorKey)
	if err != nil || value == "" {
		return time.Now().Add(-initialLookback).UTC().Format(time.RFC3339)
	}
	return value
}

// pendingBackfill returns the lower bound a previous pass did not reach.
func (s *Syncer) pendingBackfill() string {
	value, err := s.db.GetSetting(SyncBackfillKey)
	if err != nil {
		return ""
	}
	return value
}

// livePullFrom is the lower bound for the live window: the newest timeline entry
// already processed. Without one (first run, or upgraded from a version that did
// not store it) it falls back to the cursor, which triggers one bounded rescan
// of the newest entries.
func (s *Syncer) livePullFrom(fallback string) (string, error) {
	value, err := s.db.GetSetting(SyncTopKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read Folo sync top: %w", err)
	}
	if value != "" {
		return value, nil
	}
	return fallback, nil
}

// appendEntries records new entries in order and reports the time range of the
// whole batch, which pagination needs even when every row was already seen.
func appendEntries(seen map[string]bool, dst *[]Entry, batch []Entry) (added int, newest, oldest string) {
	for _, entry := range batch {
		if entry.PublishedAt != "" {
			newest = later(newest, entry.PublishedAt)
			oldest = earlier(oldest, entry.PublishedAt)
		}
		if seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		*dst = append(*dst, entry)
		added++
	}
	return added, newest, oldest
}

func later(a, b string) string {
	if a == "" || (b != "" && timestampAfter(b, a)) {
		return b
	}
	return a
}

func earlier(a, b string) string {
	if a == "" || (b != "" && timestampAfter(a, b)) {
		return b
	}
	return a
}

// timestampAfter reports whether a is later than b, tolerating timestamps that
// differ in fractional-second precision (plain string order gets those wrong).
func timestampAfter(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339Nano, a)
	tb, errB := time.Parse(time.RFC3339Nano, b)
	if errA != nil || errB != nil {
		return a > b
	}
	return ta.After(tb)
}

// shiftTime moves a timestamp bound back so entries sharing the boundary
// timestamp are not dropped. Folo compares strictly and offers no id cursor, so
// an unshifted bound would skip rows that tie with the page's last row.
func shiftTime(value string, delta time.Duration) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.Add(delta).UTC().Format(time.RFC3339Nano)
}
