package folo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
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
	// SyncTruncatedKey remembers a publish time whose entries could not all be
	// listed. The entry API has no cursor, so the gap cannot be retried; keeping
	// it here makes every later pass report incomplete coverage instead of
	// quietly counting the missing entries as synced.
	SyncTruncatedKey = "folo_sync_truncated_at"
	// SyncRecheckKey is the article_id watermark for re-reading already-mapped
	// entries. It is deliberately independent of the push window: a recheck must
	// not stamp updated_at, or it would push pending local changes further back.
	SyncRecheckKey = "folo_sync_recheck_id"

	syncMappedLimit = 500
	syncPageSize    = 100
	syncPageBudget  = 5
	// syncRecheckLimit bounds how many mapped entries are re-read per pass, and
	// syncRecheckWindow the page size for one narrow time window.
	syncRecheckLimit  = 8
	syncRecheckWindow = 10
	// Mappings created before the remote publish time was stored fall back to the
	// article's time, which can be hours or days off, so that window is wider.
	syncRecheckFallbackSlack  = 24 * time.Hour
	syncRecheckFallbackWindow = 50
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
	// Truncated reports that one publish timestamp held more entries than a page
	// and the upstream API offers no cursor to page inside it, so this pass did
	// not cover everything. The gap is surfaced instead of being reported as a
	// complete sync.
	Truncated   bool   `json:"truncated"`
	TruncatedAt string `json:"truncated_at,omitempty"`
	// RecheckIncomplete reports that a mapped entry could not be confirmed
	// within the bounded publish-time lookup. Its local snapshot is left alone
	// so a pending local change remains eligible for the push pass.
	RecheckIncomplete   bool   `json:"recheck_incomplete"`
	RecheckIncompleteAt string `json:"recheck_incomplete_at,omitempty"`
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
	// A gap found here is sticky: the API cannot page inside one publish time, so
	// those entries stay missing on later passes. Keeping the marker makes every
	// pass report incomplete coverage instead of implying a complete sync.
	truncatedAt, truncatedErr := s.db.GetSetting(SyncTruncatedKey)
	if truncatedErr != nil {
		log.Printf("folo sync: read truncated coverage: %v", truncatedErr)
	}
	if next.truncated && next.truncatedAt != "" {
		truncatedAt = next.truncatedAt
		if err := s.db.SetSetting(SyncTruncatedKey, truncatedAt); err != nil {
			log.Printf("folo sync: record truncated coverage: %v", err)
		}
	}
	result.Truncated = truncatedAt != ""
	result.TruncatedAt = truncatedAt
	if result.Truncated {
		log.Printf("folo sync: coverage stops at %s: the upstream API cannot page inside one publish time", truncatedAt)
	}

	// Re-read a bounded slice of older mappings: their remote state can change
	// long after publication, and no live window will ever include them again.
	if rechecked, incompleteAt, recheckErr := s.recheckRemote(ctx); recheckErr != nil {
		log.Printf("folo sync: recheck mapped entries: %v", recheckErr)
		result.RecheckIncomplete = true
		result.RecheckIncompleteAt = incompleteAt
		// Any failed confirmation must reach the existing truncated warning path;
		// a transport or database error may not have a timestamp to attach.
		if !result.Truncated {
			result.Truncated = true
		}
		if incompleteAt != "" {
			result.TruncatedAt = incompleteAt
		}
	} else {
		result.Pulled += rechecked
		if incompleteAt != "" {
			result.RecheckIncomplete = true
			result.RecheckIncompleteAt = incompleteAt
			result.Truncated = true
			if incompleteAt != "" {
				result.TruncatedAt = incompleteAt
			}
			log.Printf("folo sync: could not confirm a mapped entry around %s within the bounded recheck window", incompleteAt)
		}
	}
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
			// Nothing to push, but the row still has to rotate: the batch is the
			// oldest mappings, so a row that never gets stamped keeps its place at
			// the front and a pending change behind it is never reached.
			if err := s.db.UpdateFoloSnapshot(item.ArticleID, item.SnapshotRead, item.SnapshotFav); err != nil {
				log.Printf("folo sync: rotate mapping %d: %v", item.ArticleID, err)
			}
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
	// truncated reports that a page boundary fell inside a single publish time.
	// The entry API only takes publishedBefore/publishedAfter bounds, so a
	// timestamp holding more entries than one page cannot be split; the gap is
	// recorded rather than reported as complete coverage or retried forever.
	truncated bool
	// truncatedAt is the timestamp whose remaining entries could not be listed.
	truncatedAt string
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
	// Once a dense timestamp has been reported, do not spend the next pass
	// repeatedly asking for that same boundary. A strict live bound leaves the
	// budget for the older continuation while still accepting newer entries.
	truncatedAt, _ := s.db.GetSetting(SyncTruncatedKey)
	skipLiveBoundary := truncatedAt != "" && backfill != ""

	seen := make(map[string]bool)
	entries := make([]Entry, 0, syncPageSize)
	pagesUsed := 0

	liveFrom, err := s.livePullFrom(since)
	if err != nil {
		return updated, matched, skipped, next, err
	}
	newest, oldestLive := "", ""
	liveCaughtUp, liveProgressed := false, true
	liveTruncated, liveTruncatedAt := false, ""
	for pagesUsed < syncPageBudget {
		params := EntryListParams{Limit: syncPageSize}
		if pagesUsed == 0 {
			params.NewerThan = shiftTime(liveFrom, -time.Millisecond)
			if skipLiveBoundary {
				params.NewerThan = liveFrom
			}
		} else if oldestLive != "" {
			// Include the boundary itself and remove IDs already collected. A
			// millisecond offset drops a legitimate sub-millisecond successor.
			params.OlderThan = oldestLive
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
			// A full page with nothing new means the boundary timestamp holds at
			// least one page of entries and the API has no ID cursor to split it.
			// Record the gap, then make one bounded request strictly below the band
			// so older, independently enumerable entries are not hidden behind it.
			if oldestLive != "" {
				liveTruncated, liveTruncatedAt = true, oldestLive
			}
			liveProgressed = false
			if oldestLive == "" || pagesUsed >= syncPageBudget {
				break
			}
			pastBand, pastErr := s.client.ListEntries(ctx, EntryListParams{
				Limit:     syncPageSize,
				OlderThan: shiftTime(oldestLive, -time.Nanosecond),
			})
			if pastErr != nil {
				return updated, matched, skipped, next, fmt.Errorf("list Folo entries below %s: %w", oldestLive, pastErr)
			}
			pagesUsed++
			added, _, pastOldest := appendEntries(seen, &entries, pastBand)
			oldestLive = earlier(oldestLive, pastOldest)
			if added == 0 || len(pastBand) < syncPageSize {
				liveCaughtUp = true
				break
			}
		}
	}

	oldestBackfill := ""
	backfillPages := 0
	backfillDone, backfillProgressed := false, true
	backfillTruncated, backfillTruncatedAt := false, ""
	for backfill != "" && pagesUsed < syncPageBudget {
		params := EntryListParams{Limit: syncPageSize}
		if backfillPages == 0 {
			// Continue the gap from its top edge, moving downwards.
			params.OlderThan = since
		} else if oldestBackfill != "" {
			params.OlderThan = oldestBackfill
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
			if oldestBackfill != "" {
				backfillTruncated, backfillTruncatedAt = true, oldestBackfill
			}
			backfillProgressed = false
			if oldestBackfill == "" || pagesUsed >= syncPageBudget {
				break
			}
			pastBand, pastErr := s.client.ListEntries(ctx, EntryListParams{
				Limit:     syncPageSize,
				OlderThan: shiftTime(oldestBackfill, -time.Nanosecond),
			})
			if pastErr != nil {
				return updated, matched, skipped, next, fmt.Errorf("list Folo entries below %s: %w", oldestBackfill, pastErr)
			}
			pagesUsed++
			added, _, pastOldest := appendEntries(seen, &entries, pastBand)
			oldestBackfill = earlier(oldestBackfill, pastOldest)
			if added > 0 {
				backfillProgressed = true
			}
			if added == 0 || len(pastBand) < syncPageSize {
				backfillDone = true
				break
			}
		}
		if oldestBackfill != "" && !timestampAfter(oldestBackfill, backfill) {
			backfillDone = true
			break
		}
	}

	switch {
	case liveTruncated && oldestLive != "":
		// Coverage stops at a timestamp the API cannot split. Advancing the cursor
		// past it keeps the same page from being re-read on every sync, and the
		// truncation is reported so the gap is never counted as a complete sync.
		next.cursor = oldestLive
		if !backfillDone {
			next.backfill = backfill
		}
		if backfill == "" {
			// The first truncated live walk has no pending backfill yet. Carry its
			// original lower bound forward so a later pass can resume below the
			// bounded recovery page instead of restarting at the top forever.
			next.backfill = liveFrom
		}
		next.truncated = true
		next.truncatedAt = liveTruncatedAt
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
		if backfillTruncated {
			next.truncated = true
			next.truncatedAt = backfillTruncatedAt
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
	// A live pull can encounter a local change that is waiting in the push
	// window. Updating its snapshot must not move that row behind the next 500
	// mappings before pushLocal gets a chance to send it.
	return s.applyEntryWithRotation(articleID, entry, false)
}

// Rechecks update the agreement snapshot but do not rotate push ordering: a
// pending local change must stay inside the next bounded push batch.
func (s *Syncer) applyRecheckedEntry(articleID int64, entry Entry) (int, error) {
	return s.applyEntryWithRotation(articleID, entry, false)
}

func (s *Syncer) applyEntryWithRotation(articleID int64, entry Entry, rotate bool) (int, error) {
	_, snapshotRead, snapshotFav, mapped, err := s.db.GetFoloMapping(articleID)
	if err != nil {
		return 0, fmt.Errorf("read Folo mapping: %w", err)
	}

	// Existing mappings also need the entry's own publish time: without it a later
	// recheck aims at the article's time, which is not where the entry lives.
	if mapped {
		if err := s.db.SetFoloEntryPublishedAt(articleID, entry.PublishedAt); err != nil {
			log.Printf("folo sync: record entry publish time: %v", err)
		}
	}
	if !mapped {
		// The snapshot keeps Folo's value on purpose: local state that differs is
		// then pushed in the same pass instead of being silently cleared.
		if err := s.db.UpsertFoloEntry(articleID, entry.ID, entry.URL, entry.PublishedAt, entry.Read, entry.Collected); err != nil {
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
	var snapshotErr error
	if rotate {
		snapshotErr = s.db.UpdateFoloSnapshot(articleID, nextRead, nextFav)
	} else {
		snapshotErr = s.db.UpdateFoloSnapshotPreserveOrder(articleID, nextRead, nextFav)
	}
	if snapshotErr != nil {
		return 0, fmt.Errorf("advance Folo snapshot: %w", snapshotErr)
	}
	return changed, nil
}

// recheckCursor reports where the rotating re-read of mapped entries stopped.
func (s *Syncer) recheckCursor() int64 {
	value, err := s.db.GetSetting(SyncRecheckKey)
	if err != nil || value == "" {
		return 0
	}
	id, parseErr := strconv.ParseInt(value, 10, 64)
	if parseErr != nil {
		return 0
	}
	return id
}

// recheckRemote re-reads a bounded, rotating slice of already-mapped entries.
//
// The live pull only reads entries newer than the cursor, so a change made to an
// older entry — marked read, or removed from the collection long after it was
// published — never enters a window again. The entry API can only be queried by
// publish time, so each mapping is re-read through a narrow window around its own
// timestamp, which returns both its read flag and its collection state.
func (s *Syncer) recheckRemote(ctx context.Context) (int, string, error) {
	after := s.recheckCursor()
	rows, err := s.db.ListFoloMappingsForRecheck(after, syncRecheckLimit)
	if err != nil {
		return 0, "", fmt.Errorf("list mappings for recheck: %w", err)
	}
	if len(rows) == 0 {
		// A full page followed by an empty one means the sweep reached the end.
		// Without rewinding, the watermark stayed on the last id and every later
		// pass queried past it, so rechecking stopped for good once the table held
		// exactly one page of mappings.
		if after != 0 {
			if err := s.db.SetSetting(SyncRecheckKey, "0"); err != nil {
				log.Printf("folo sync: rewind recheck cursor: %v", err)
			}
		}
		return 0, "", nil
	}

	changed := 0
	incompleteAt := ""
	for _, item := range rows {
		// The entry's own publish time is the only window that reliably contains it:
		// matching is by URL, so the article's time can differ. Mappings created
		// before that time was recorded fall back to the article's, with a window
		// wide enough to absorb the disagreement.
		at, slack, limit := item.RemotePublishedAt, time.Millisecond, syncRecheckWindow
		if at == "" {
			at, slack, limit = item.PublishedAt, syncRecheckFallbackSlack, syncRecheckFallbackWindow
		}
		if at == "" {
			if incompleteAt == "" {
				incompleteAt = item.EntryID
			}
			continue
		}
		entry, found, listErr := s.findRecheckEntry(ctx, item.EntryID, at, slack, limit)
		if listErr != nil {
			if incompleteAt == "" {
				incompleteAt = at
			}
			return changed, incompleteAt, listErr
		}
		if !found {
			if incompleteAt == "" {
				incompleteAt = at
			}
			continue
		}
		applied, applyErr := s.applyRecheckedEntry(item.ArticleID, entry)
		if applyErr != nil {
			return changed, incompleteAt, applyErr
		}
		changed += applied
	}

	next := rows[len(rows)-1].ArticleID
	if len(rows) < syncRecheckLimit {
		// A short page ends the sweep; the next pass starts from the top again.
		next = 0
	}
	if err := s.db.SetSetting(SyncRecheckKey, strconv.FormatInt(next, 10)); err != nil {
		log.Printf("folo sync: record recheck cursor: %v", err)
	}
	return changed, incompleteAt, nil
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
//
// published_at reaches this from several paths (feed parsing, imports, older
// rows), so more than one layout has to parse: returning the value unchanged
// would make both bounds equal and collapse a recheck window to nothing.
func shiftTime(value string, delta time.Duration) string {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		// What the article store actually holds: the SQLite driver writes a
		// time.Time through String(), not RFC3339.
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Add(delta).UTC().Format(time.RFC3339Nano)
		}
	}
	return value
}

// countPublishedAt counts entries sharing one publish time. A page that ends on a
// repeated timestamp was cut inside a band the API cannot page through, which is
// how a pull detects that its coverage stops there.
func countPublishedAt(entries []Entry, at string) int {
	if at == "" {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.PublishedAt == at {
			count++
		}
	}
	return count
}
