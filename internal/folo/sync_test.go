package folo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"MrRSS/internal/database"
	"MrRSS/internal/models"
)

type syncRecorder struct {
	mu           sync.Mutex
	entries      []map[string]any
	entryQueries []map[string]any
	readIDs      [][]string
	unreadIDs    []string
	collected    []string
	uncollected  []string
	failReads    bool
}

func (r *syncRecorder) setEntries(entries []map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = entries
}

func newSyncFolo(t *testing.T) (*httptest.Server, *syncRecorder) {
	t.Helper()
	recorder := &syncRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc(pathEntries, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		recorder.mu.Lock()
		recorder.entryQueries = append(recorder.entryQueries, body)
		entries := recorder.entries
		recorder.mu.Unlock()
		if entries == nil {
			entries = []map[string]any{}
		}
		payload, _ := json.Marshal(map[string]any{"code": 0, "data": entries})
		_, _ = w.Write(payload)
	})
	mux.HandleFunc(pathReads, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch r.Method {
		case http.MethodPost:
			recorder.mu.Lock()
			fail := recorder.failReads
			recorder.mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var body struct {
				EntryIDs []string `json:"entryIds"`
			}
			_ = json.Unmarshal(raw, &body)
			recorder.mu.Lock()
			recorder.readIDs = append(recorder.readIDs, body.EntryIDs)
			recorder.mu.Unlock()
		case http.MethodDelete:
			var body struct {
				EntryID string `json:"entryId"`
			}
			_ = json.Unmarshal(raw, &body)
			recorder.mu.Lock()
			recorder.unreadIDs = append(recorder.unreadIDs, body.EntryID)
			recorder.mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	mux.HandleFunc(pathCollections, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			EntryID string `json:"entryId"`
		}
		_ = json.Unmarshal(raw, &body)
		recorder.mu.Lock()
		if r.Method == http.MethodPost {
			recorder.collected = append(recorder.collected, body.EntryID)
		} else {
			recorder.uncollected = append(recorder.uncollected, body.EntryID)
		}
		recorder.mu.Unlock()
		_, _ = w.Write([]byte(`{"code":0}`))
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, recorder
}

const syncArticleURL = "https://example.com/articles/1"

func newSyncFixture(t *testing.T) (*database.DB, *Syncer, *syncRecorder, int64) {
	t.Helper()
	db, err := database.NewDB("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	feedID, err := db.AddFeed(&models.Feed{Title: "Example", URL: "http://127.0.0.1:1/feed.xml"})
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}
	article := &models.Article{
		FeedID:      feedID,
		Title:       "Article",
		URL:         syncArticleURL,
		PublishedAt: time.Now(),
	}
	if err := db.SaveArticle(article); err != nil {
		t.Fatalf("save article: %v", err)
	}
	stored, err := db.GetArticleByURL(syncArticleURL)
	if err != nil {
		t.Fatalf("load article: %v", err)
	}

	server, recorder := newSyncFolo(t)
	client, err := NewClientWithHTTPClient(server.URL, "session", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return db, NewSyncer(client, db), recorder, stored.ID
}

func remoteEntry(id string, read, collected bool, publishedAt string) map[string]any {
	entry := map[string]any{
		"id":   "row-" + id,
		"read": read,
		"entries": map[string]any{
			"id":          id,
			"url":         syncArticleURL,
			"publishedAt": publishedAt,
		},
	}
	if collected {
		entry["collections"] = map[string]any{"createdAt": publishedAt}
	}
	return entry
}

func loadArticle(t *testing.T, db *database.DB, id int64) *models.Article {
	t.Helper()
	article, err := db.GetArticleByID(id)
	if err != nil {
		t.Fatalf("load article: %v", err)
	}
	return article
}

func TestSyncMapsArticleAndAdoptsRemoteState(t *testing.T) {
	db, syncer, recorder, articleID := newSyncFixture(t)
	recorder.setEntries([]map[string]any{remoteEntry("E1", true, true, "2026-10-05T12:00:00Z")})

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	article := loadArticle(t, db, articleID)
	if !article.IsRead || !article.IsFavorite {
		t.Errorf("article state = read:%v fav:%v, want both true", article.IsRead, article.IsFavorite)
	}
	entryID, snapRead, snapFav, mapped, err := db.GetFoloMapping(articleID)
	if err != nil || !mapped {
		t.Fatalf("mapping missing (err=%v)", err)
	}
	if entryID != "E1" || !snapRead || !snapFav {
		t.Errorf("mapping = %s %v %v", entryID, snapRead, snapFav)
	}
	if result.Matched != 1 || result.Pulled != 2 || result.Mapped != 1 {
		t.Errorf("result = %+v", result)
	}
	if len(recorder.readIDs) != 0 || len(recorder.collected) != 0 {
		t.Error("adopting remote state must not write back to Folo")
	}
	cursor, _ := db.GetSetting(SyncCursorKey)
	if cursor != "2026-10-05T12:00:00Z" {
		t.Errorf("cursor = %q", cursor)
	}
	if result.LastSync == "" {
		t.Error("last sync time should record the wall-clock time of the pass")
	}
}

func TestSyncPushesLocalReadAndIgnoresStaleTimeline(t *testing.T) {
	db, syncer, recorder, articleID := newSyncFixture(t)
	// Both sides were in sync at "unread", then the user read it locally.
	if err := db.UpsertFoloEntry(articleID, "E1", syncArticleURL, "", false, false); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkArticleRead(articleID, true); err != nil {
		t.Fatal(err)
	}
	// The timeline the pull sees still reports the entry unread.
	recorder.setEntries([]map[string]any{remoteEntry("E1", false, false, "2026-10-05T12:00:00Z")})

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(recorder.readIDs) != 1 || len(recorder.readIDs[0]) != 1 || recorder.readIDs[0][0] != "E1" {
		t.Errorf("read pushes = %v", recorder.readIDs)
	}
	if article := loadArticle(t, db, articleID); !article.IsRead {
		t.Error("the stale timeline must not undo the local change")
	}
	_, snapRead, _, _, _ := db.GetFoloMapping(articleID)
	if !snapRead {
		t.Error("snapshot should record that both sides are read")
	}
}

func TestSyncKeepsChangePendingWhenPushFails(t *testing.T) {
	db, syncer, recorder, articleID := newSyncFixture(t)
	if err := db.UpsertFoloEntry(articleID, "E1", syncArticleURL, "", false, false); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkArticleRead(articleID, true); err != nil {
		t.Fatal(err)
	}
	recorder.failReads = true
	recorder.setEntries([]map[string]any{remoteEntry("E1", false, false, "2026-10-05T12:00:00Z")})

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("a failed push must not abort the pass: %v", err)
	}
	if result.Pushed != 0 {
		t.Errorf("pushed = %d, want 0", result.Pushed)
	}
	if article := loadArticle(t, db, articleID); !article.IsRead {
		t.Error("a failed push must not clear the local state")
	}
	_, snapRead, _, _, _ := db.GetFoloMapping(articleID)
	if snapRead {
		t.Error("snapshot must stay behind so the next pass retries")
	}

	// The next pass, with Folo healthy, delivers the change.
	recorder.failReads = false
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(recorder.readIDs) != 1 {
		t.Errorf("read pushes = %v", recorder.readIDs)
	}
	_, snapRead, _, _, _ = db.GetFoloMapping(articleID)
	if !snapRead {
		t.Error("snapshot should advance after the successful push")
	}
}

func TestSyncAppliesRemoteUnfavorite(t *testing.T) {
	db, syncer, recorder, articleID := newSyncFixture(t)
	// Both sides had the article saved; then it left the Folo collection.
	if err := db.UpsertFoloEntry(articleID, "E1", syncArticleURL, "", false, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetArticleFavorite(articleID, true); err != nil {
		t.Fatal(err)
	}
	recorder.setEntries([]map[string]any{remoteEntry("E1", false, false, "2026-10-05T12:00:00Z")})

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if article := loadArticle(t, db, articleID); article.IsFavorite {
		t.Error("remote removal should clear the local favorite")
	}
	if len(recorder.uncollected) != 0 {
		t.Errorf("no collection write expected, got %v", recorder.uncollected)
	}
}

func TestSyncPushesLocalFavorite(t *testing.T) {
	_, syncer, recorder, articleID := newSyncFixture(t)
	if err := recorderSetFavorite(syncer, articleID); err != nil {
		t.Fatal(err)
	}
	recorder.setEntries(nil)

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(recorder.collected) != 1 || recorder.collected[0] != "E1" {
		t.Errorf("collection pushes = %v", recorder.collected)
	}
	if result.Pushed != 1 {
		t.Errorf("pushed = %d, want 1", result.Pushed)
	}
}

// recorderSetFavorite seeds a mapping whose snapshot says "not saved" while the
// local article is saved.
func recorderSetFavorite(syncer *Syncer, articleID int64) error {
	if err := syncer.db.UpsertFoloEntry(articleID, "E1", syncArticleURL, "", false, false); err != nil {
		return err
	}
	return syncer.db.SetArticleFavorite(articleID, true)
}

func TestSyncSkipsUnmatchedAndURLlessEntries(t *testing.T) {
	db, syncer, recorder, _ := newSyncFixture(t)
	unmatched := remoteEntry("E2", true, false, "2026-10-05T12:00:00Z")
	unmatched["entries"].(map[string]any)["url"] = "https://unknown.example/x"
	urlless := remoteEntry("E3", true, false, "2026-10-05T12:01:00Z")
	urlless["entries"].(map[string]any)["url"] = nil
	recorder.setEntries([]map[string]any{unmatched, urlless})

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Skipped != 2 || result.Matched != 0 {
		t.Errorf("result = %+v", result)
	}
	if count, _ := db.CountFoloMappings(); count != 0 {
		t.Errorf("mappings = %d, want 0", count)
	}
}

func TestSyncRequiresAuthentication(t *testing.T) {
	db, _, recorder, _ := newSyncFixture(t)
	recorder.setEntries(nil)
	server, _ := newSyncFolo(t)
	client, err := NewClientWithHTTPClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewSyncer(client, db).Sync(context.Background()); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("Sync = %v, want ErrNotAuthenticated", err)
	}
}

type timelineQuery struct {
	limit      int
	newerThan  string
	olderThan  string
	sortOrder  string
	collection bool
}

type timelineFeed struct {
	mu      sync.Mutex
	entries []Entry
	queries []timelineQuery
}

func (f *timelineFeed) set(entries []Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = entries
}

func (f *timelineFeed) calls() []timelineQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]timelineQuery, len(f.queries))
	copy(out, f.queries)
	return out
}

func (f *timelineFeed) page(body struct {
	Limit           int    `json:"limit"`
	PublishedBefore string `json:"publishedBefore"`
	PublishedAfter  string `json:"publishedAfter"`
	SortOrder       string `json:"sortOrder"`
	IsCollection    *bool  `json:"isCollection"`
}) []Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	visible := make([]Entry, 0, len(f.entries))
	for _, entry := range f.entries {
		if body.IsCollection != nil && *body.IsCollection {
			continue
		}
		if body.PublishedBefore != "" && !timestampAfter(entry.PublishedAt, body.PublishedBefore) {
			continue
		}
		if body.PublishedAfter != "" && timestampAfter(entry.PublishedAt, body.PublishedAfter) {
			continue
		}
		visible = append(visible, entry)
	}
	if body.Limit > 0 && len(visible) > body.Limit {
		visible = visible[:body.Limit]
	}
	return visible
}

// newTimelineFolo serves a mutable timeline using the semantics verified against
// the live API: publishedBefore is a strict lower bound, publishedAfter a strict
// upper bound, and rows are always newest first.
func newTimelineFolo(t *testing.T, entries []Entry) (*httptest.Server, *timelineFeed) {
	t.Helper()
	feed := &timelineFeed{entries: entries}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != pathEntries {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Limit           int    `json:"limit"`
			PublishedBefore string `json:"publishedBefore"`
			PublishedAfter  string `json:"publishedAfter"`
			SortOrder       string `json:"sortOrder"`
			IsCollection    *bool  `json:"isCollection"`
		}
		_ = json.Unmarshal(raw, &body)
		feed.mu.Lock()
		feed.queries = append(feed.queries, timelineQuery{
			limit:      body.Limit,
			newerThan:  body.PublishedBefore,
			olderThan:  body.PublishedAfter,
			sortOrder:  body.SortOrder,
			collection: body.IsCollection != nil && *body.IsCollection,
		})
		feed.mu.Unlock()

		visible := feed.page(body)
		rows := make([]map[string]any, 0, len(visible))
		for _, entry := range visible {
			row := map[string]any{
				"id":   "row-" + entry.ID,
				"read": entry.Read,
				"entries": map[string]any{
					"id":          entry.ID,
					"url":         entry.URL,
					"publishedAt": entry.PublishedAt,
				},
			}
			if entry.Collected {
				// The real API reports collection membership as an object; without
				// it a collected entry looks uncollected to the client.
				row["collections"] = map[string]any{"createdAt": entry.PublishedAt}
			}
			rows = append(rows, row)
		}
		payload, _ := json.Marshal(map[string]any{"code": 0, "data": rows})
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	return server, feed
}

// timelineEntries builds entries newest first, one minute apart, ending at the
// given start time. They carry no URL so the pull counts them as skipped.
func timelineEntries(count int, start time.Time) []Entry {
	entries := make([]Entry, 0, count)
	for i := 0; i < count; i++ {
		entries = append(entries, Entry{
			ID:          fmt.Sprintf("E%03d", i),
			PublishedAt: start.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	return entries
}

func newTimelineFixture(t *testing.T, entries []Entry) (*database.DB, *Syncer, *timelineFeed) {
	t.Helper()
	db, err := database.NewDB("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	// Pin the start of the lookback so the test does not depend on the clock.
	if err := db.SetSetting(SyncCursorKey, "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	server, feed := newTimelineFolo(t, entries)
	client, err := NewClientWithHTTPClient(server.URL, "session", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return db, NewSyncer(client, db), feed
}

func TestSyncPullsNewestFirstAndAdvancesCursor(t *testing.T) {
	entries := timelineEntries(250, time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	db, syncer, feed := newTimelineFixture(t, entries)

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Skipped != 250 {
		t.Errorf("skipped = %d, want 250", result.Skipped)
	}

	calls := feed.calls()
	// Three timeline pages plus the collection pass.
	if len(calls) != 4 {
		t.Fatalf("timeline calls = %d, want 4: %+v", len(calls), calls)
	}
	if calls[0].sortOrder != "desc" || calls[0].newerThan == "" || calls[0].olderThan != "" {
		t.Errorf("first call = %+v, want a newest-first page above the cursor", calls[0])
	}
	if want := entries[99].PublishedAt; calls[1].olderThan != want {
		t.Errorf("second call olderThan = %q, want %q", calls[1].olderThan, want)
	}
	if !calls[3].collection || calls[3].newerThan != "" || calls[3].olderThan != "" {
		t.Errorf("collection call = %+v", calls[3])
	}

	cursor, _ := db.GetSetting(SyncCursorKey)
	if cursor != entries[0].PublishedAt {
		t.Errorf("cursor = %q, want the newest entry %q", cursor, entries[0].PublishedAt)
	}
	if top, _ := db.GetSetting(SyncTopKey); top != entries[0].PublishedAt {
		t.Errorf("top = %q, want the newest entry %q", top, entries[0].PublishedAt)
	}
	if backfill, _ := db.GetSetting(SyncBackfillKey); backfill != "" {
		t.Errorf("backfill = %q, want none", backfill)
	}
}

func TestSyncBackfillsWhenPageBudgetRunsOut(t *testing.T) {
	entries := timelineEntries(700, time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	db, syncer, feed := newTimelineFixture(t, entries)

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	cursor, _ := db.GetSetting(SyncCursorKey)
	if cursor != entries[495].PublishedAt {
		t.Fatalf("cursor = %q, want the oldest entry the budget covered %q", cursor, entries[495].PublishedAt)
	}
	if backfill, _ := db.GetSetting(SyncBackfillKey); backfill != "2026-10-01T00:00:00Z" {
		t.Errorf("backfill = %q, want the previous cursor", backfill)
	}
	for i, call := range feed.calls() {
		if call.collection {
			continue
		}
		if call.sortOrder != "desc" {
			t.Errorf("call %d sortOrder = %q", i, call.sortOrder)
		}
	}

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	calls := feed.calls()
	// The live window only asks for entries above the stored top...
	if got, want := calls[6].newerThan, shiftTime(entries[0].PublishedAt, -time.Millisecond); got != want {
		t.Errorf("live call newerThan = %q, want %q", got, want)
	}
	// ...and the rest of the budget continues the backfill below the cursor.
	if got, want := calls[7].olderThan, entries[495].PublishedAt; got != want {
		t.Errorf("resume call olderThan = %q, want %q", got, want)
	}
	if got := calls[7].newerThan; got != "" {
		t.Errorf("resume call newerThan = %q, want none", got)
	}

	cursor, _ = db.GetSetting(SyncCursorKey)
	if cursor != "2026-10-01T00:00:00Z" {
		t.Errorf("cursor = %q, want the gap to be closed back to the previous cursor", cursor)
	}
	if backfill, _ := db.GetSetting(SyncBackfillKey); backfill != "" {
		t.Errorf("backfill = %q, want it cleared once the gap is closed", backfill)
	}
}

func TestSyncMapsAndPushesNewlyReadArticleInOnePass(t *testing.T) {
	db, syncer, recorder, articleID := newSyncFixture(t)
	// The user read this article before Folo ever matched it.
	if err := db.MarkArticleRead(articleID, true); err != nil {
		t.Fatal(err)
	}
	recorder.setEntries([]map[string]any{remoteEntry("E1", false, false, "2026-10-05T12:00:00Z")})

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Matched != 1 || result.Pushed != 1 {
		t.Errorf("result = %+v, want one match and one push", result)
	}
	if len(recorder.readIDs) != 1 || len(recorder.readIDs[0]) != 1 || recorder.readIDs[0][0] != "E1" {
		t.Errorf("read pushes = %v, want E1 marked read", recorder.readIDs)
	}
	_, snapRead, _, mapped, err := db.GetFoloMapping(articleID)
	if err != nil || !mapped || !snapRead {
		t.Errorf("mapping = %v %v (err %v), want read snapshot", mapped, snapRead, err)
	}
}

func TestSyncPullsNewEntriesWhileBackfilling(t *testing.T) {
	base := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	entries := timelineEntries(700, base)
	db, syncer, feed := newTimelineFixture(t, entries)

	// First pass: budget covers the newest 500 and leaves a backfill behind.
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	// Three entries arrive above the stored top while the backfill is pending.
	fresh := []Entry{
		{ID: "N1", PublishedAt: base.Add(3 * time.Minute).Format(time.RFC3339)},
		{ID: "N2", PublishedAt: base.Add(2 * time.Minute).Format(time.RFC3339)},
		{ID: "N3", PublishedAt: base.Add(time.Minute).Format(time.RFC3339)},
	}
	feed.set(append(fresh, entries...))

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	calls := feed.calls()
	if got, want := calls[6].newerThan, shiftTime(entries[0].PublishedAt, -time.Millisecond); got != want {
		t.Errorf("live call newerThan = %q, want the stored top %q", got, want)
	}
	if calls[7].olderThan == "" || calls[7].newerThan != "" {
		t.Errorf("call after the live window = %+v, want the backfill below the cursor", calls[7])
	}
	// The live window re-reads the boundary entry plus the 3 fresh ones, then the
	// backfill reaches the remaining 200 older entries.
	if result.Skipped != 209 {
		t.Errorf("skipped = %d, want 209", result.Skipped)
	}
	if top, _ := db.GetSetting(SyncTopKey); top != fresh[0].PublishedAt {
		t.Errorf("top = %q, want the newest fresh entry %q", top, fresh[0].PublishedAt)
	}
}

// One publish time holding more entries than a page cannot be paged: the entry
// API only takes time bounds. The pull must record that gap instead of quietly
// reporting a complete sync, and must still advance so the same page is not
// re-read inside one pass.
func TestSyncReportsTruncatedCoverageForOnePublishTimeOverflow(t *testing.T) {
	published := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	entries := make([]Entry, 0, 101)
	for i := 0; i < 101; i++ {
		entries = append(entries, Entry{
			ID:          fmt.Sprintf("E%03d", i),
			PublishedAt: published,
		})
	}
	db, syncer, feed := newTimelineFixture(t, entries)

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Skipped != syncPageSize {
		t.Errorf("skipped = %d, want one page (%d)", result.Skipped, syncPageSize)
	}
	if !result.Truncated {
		t.Fatal("a publish time holding more entries than a page must be reported as truncated")
	}
	if result.TruncatedAt != published {
		t.Errorf("truncatedAt = %q, want %q", result.TruncatedAt, published)
	}
	// Two timeline pages, one bounded request below the unsplittable publish time,
	// then the collection pass: the walk stops instead of looping on that page.
	calls := feed.calls()
	if len(calls) != 4 {
		t.Fatalf("timeline calls = %d, want 4: %+v", len(calls), calls)
	}
	band := calls[2]
	if band.newerThan != "" || band.olderThan != shiftTime(published, -time.Nanosecond) {
		t.Errorf("third call is not the bounded request below the band: %+v", band)
	}
	// The cursor still advances to the boundary so a later pass does not walk the
	// same page again from the top.
	if cursor, _ := db.GetSetting(SyncCursorKey); cursor != published {
		t.Errorf("cursor = %q, want the boundary %q", cursor, published)
	}

	// The gap cannot be recovered by a later pass, so it stays reported rather
	// than turning into an implied complete sync.
	second, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if !second.Truncated || second.TruncatedAt != published {
		t.Errorf("second pass = %+v, want the same truncation", second)
	}
}
