package folo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"MrRSS/internal/database"
	"MrRSS/internal/feed"
	folopkg "MrRSS/internal/folo"
	corepkg "MrRSS/internal/handlers/core"
	"MrRSS/internal/models"
)

const (
	fakeApplyPath    = "/better-auth/one-time-token/apply"
	fakeSessionPath  = "/better-auth/get-session"
	fakeExportPath   = "/subscriptions/export"
	fakeSessionToken = "handler-session-token"
)

func newTestHandler(t *testing.T) (*corepkg.Handler, *database.DB) {
	t.Helper()
	db, err := database.NewDB("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	return &corepkg.Handler{DB: db, Fetcher: feed.NewFetcher(db)}, db
}

// fakeFolo stands in for the hosted API. Subscription URLs point at a closed
// port so the background first fetch stays on this machine.
type fakeFoloServer struct {
	*httptest.Server
	mu      sync.Mutex
	entries []map[string]any
	readIDs []string
}

func (f *fakeFoloServer) setEntries(entries []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = entries
}

func (f *fakeFoloServer) pushedReadIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.readIDs...)
}

func fakeFolo(t *testing.T, body string) *fakeFoloServer {
	t.Helper()
	fake := &fakeFoloServer{}
	mux := http.NewServeMux()
	mux.HandleFunc(fakeSessionPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+fakeSessionToken {
			_, _ = w.Write([]byte(`{"user":{"id":"u1","name":"Ada","email":"ada@example.com","image":"https://i"}}`))
			return
		}
		_, _ = w.Write([]byte("null"))
	})
	mux.HandleFunc(fakeApplyPath, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Token != "one-time" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "better-auth.session_token", Value: fakeSessionToken, Path: "/"})
		_, _ = w.Write([]byte(`{"session":{"token":"` + fakeSessionToken + `"}}`))
	})
	mux.HandleFunc(fakeExportPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeSessionToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/entries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fake.mu.Lock()
		entries := fake.entries
		fake.mu.Unlock()
		if entries == nil {
			entries = []map[string]any{}
		}
		payload, _ := json.Marshal(map[string]any{"code": 0, "data": entries})
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/reads", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			EntryIDs []string `json:"entryIds"`
			EntryID  string   `json:"entryId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.Method == http.MethodPost {
			fake.mu.Lock()
			fake.readIDs = append(fake.readIDs, req.EntryIDs...)
			fake.mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	})

	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Server.Close)
	return fake
}

const sampleOPML = `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>Folo subscriptions</title></head><body>
<outline text="Dev" title="Dev">
  <outline type="rss" text="Ada" title="Ada" xmlUrl="http://127.0.0.1:1/ada.xml"/>
  <outline type="rss" text="Blog" title="Blog" xmlUrl="http://127.0.0.1:1/blog.xml"/>
</outline>
</body></opml>`

func signIn(t *testing.T, h *corepkg.Handler, apiURL string) {
	t.Helper()
	if err := h.DB.SetSetting("folo_api_url", apiURL); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.SetEncryptedSetting(sessionTokenKey, fakeSessionToken); err != nil {
		t.Fatal(err)
	}
}

func TestHandleAccountReportsSignedOutWithoutToken(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := httptest.NewRecorder()
	HandleAccount(h, rr, httptest.NewRequest(http.MethodGet, "/api/folo/account", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if code := decodedCode(t, rr.Body.String()); code != codeNotAuthenticated {
		t.Errorf("code = %q", code)
	}
}

func TestHandleAccountResolvesFoloUser(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	signIn(t, h, server.URL)

	rr := httptest.NewRecorder()
	HandleAccount(h, rr, httptest.NewRequest(http.MethodGet, "/api/folo/account", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["connected"] != true || payload["email"] != "ada@example.com" {
		t.Errorf("payload = %+v", payload)
	}
}

func TestHandleAccountTreatsStaleTokenAsSignedOut(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	if err := h.DB.SetSetting("folo_api_url", server.URL); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.SetEncryptedSetting(sessionTokenKey, "expired-token"); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	HandleAccount(h, rr, httptest.NewRequest(http.MethodGet, "/api/folo/account", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a token Folo no longer knows", rr.Code)
	}
}

func TestHandleAccountTreatsUnreadableTokenAsSignedOut(t *testing.T) {
	h, _ := newTestHandler(t)
	// A token encrypted under a different machine ID (hostname/OS changed, database
	// copied) cannot be decrypted. Every endpoint, sign-in included, must stay
	// usable instead of failing with 502.
	if err := h.DB.SetSetting(sessionTokenKey, "MrRSS-v1:not-valid-base64!!"); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	HandleAccount(h, rr, httptest.NewRequest(http.MethodGet, "/api/folo/account", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if code := decodedCode(t, rr.Body.String()); code != codeNotAuthenticated {
		t.Errorf("code = %q", code)
	}
	if stored, err := h.DB.GetSetting(sessionTokenKey); err != nil || stored == "" {
		t.Errorf("stored token = %q, %v; the unreadable row should stay for a re-login to overwrite", stored, err)
	}
}

func TestHandleLoginCompleteStoresExchangedToken(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	if err := h.DB.SetSetting("folo_api_url", server.URL); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"credential": "one-time"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/folo/login/complete", strings.NewReader(string(body)))
	HandleLoginComplete(h, rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	stored, err := h.DB.GetEncryptedSetting(sessionTokenKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored != fakeSessionToken {
		t.Errorf("stored token = %q", stored)
	}
	// The token must never be echoed back to the browser.
	if strings.Contains(rr.Body.String(), fakeSessionToken) {
		t.Errorf("response leaked the session token: %s", rr.Body.String())
	}
}

func TestHandleLoginCompleteRejectsUnusableCredential(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	if err := h.DB.SetSetting("folo_api_url", server.URL); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"credential": "not-a-token"})
	rr := httptest.NewRecorder()
	HandleLoginComplete(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/login/complete", strings.NewReader(string(body))))

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	stored, err := h.DB.GetEncryptedSetting(sessionTokenKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if stored != "" {
		t.Errorf("a rejected credential must not be stored, got %q", stored)
	}
}

func TestHandleLoginCompleteRefusesEmptyAndBadRequests(t *testing.T) {
	h, _ := newTestHandler(t)
	for name, payload := range map[string]string{
		"empty":     `{}`,
		"blank":     `{"credential":"   "}`,
		"malformed": `{`,
	} {
		rr := httptest.NewRecorder()
		HandleLoginComplete(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/login/complete", strings.NewReader(payload)))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rr.Code)
		}
	}
}

func TestHandleLoginStartReturnsLoopbackURL(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	if err := h.DB.SetSetting("folo_api_url", server.URL); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.SetSetting("folo_web_url", server.URL); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	HandleLoginStart(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/login/start", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	loginURL, _ := payload["login_url"].(string)
	if !strings.HasPrefix(loginURL, server.URL+"/login?cli_callback=") {
		t.Errorf("login_url = %q", loginURL)
	}
	if decodedField(t, rr.Body.String(), "expires_at") == "" {
		t.Error("the attempt should carry a deadline")
	}
	if !strings.Contains(rr.Body.String(), "127.0.0.1") {
		t.Errorf("the callback must be loopback: %s", rr.Body.String())
	}
	t.Cleanup(func() {
		rr := httptest.NewRecorder()
		HandleLogout(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/logout", nil))
	})
}

func TestHandleLogoutClearsToken(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, db := newTestHandler(t)
	signIn(t, h, server.URL)
	articleID := seedArticle(t, db, "https://example.com/a")
	if err := db.UpsertFoloEntry(articleID, "E1", "https://example.com/a", "", true, false); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		folopkg.SyncCursorKey:   "2026-10-01T00:00:00Z",
		folopkg.SyncBackfillKey: "2026-09-01T00:00:00Z",
		lastImportKey:           "2026-10-01T00:00:00Z",
	} {
		if err := db.SetSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}

	rr := httptest.NewRecorder()
	HandleLogout(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/logout", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	stored, err := h.DB.GetEncryptedSetting(sessionTokenKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored != "" {
		t.Errorf("token still stored: %q", stored)
	}
	// The next sign-in may reach a different account, so nothing tied to this
	// session's entries may survive.
	if count, err := db.CountFoloMappings(); err != nil || count != 0 {
		t.Errorf("mappings = %d (err %v), want 0", count, err)
	}
	for _, key := range []string{folopkg.SyncCursorKey, folopkg.SyncBackfillKey, folopkg.LastSyncKey, lastImportKey} {
		if value, _ := db.GetSetting(key); value != "" {
			t.Errorf("%s still stored: %q", key, value)
		}
	}
}

func TestHandleImportAddsSubscriptionsAsLocalFeeds(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, db := newTestHandler(t)
	signIn(t, h, server.URL)

	rr := httptest.NewRecorder()
	HandleImport(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/import", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["total"] != float64(2) || payload["imported"] != float64(2) {
		t.Errorf("payload = %+v", payload)
	}

	feeds, err := db.GetFeeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 2 {
		t.Fatalf("stored %d feeds, want 2", len(feeds))
	}
	for _, f := range feeds {
		// Folo imports are ordinary local subscriptions: MrRSS refreshes them
		// itself and keeps them offline-readable.
		if f.IsFreshRSSSource {
			t.Errorf("feed %s marked as remotely managed", f.Title)
		}
		if f.Category != "Dev" {
			t.Errorf("feed %s category = %q, want the Folo folder", f.Title, f.Category)
		}
	}

	stored, err := db.GetSetting(lastImportKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored == "" {
		t.Error("a finished import should be recorded")
	}
}

func TestHandleImportRequiresSession(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	if err := h.DB.SetSetting("folo_api_url", server.URL); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	HandleImport(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/import", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandleImportRejectsNonOPMLAnswer(t *testing.T) {
	server := fakeFolo(t, "<html><body>challenge</body></html>")
	h, _ := newTestHandler(t)
	signIn(t, h, server.URL)

	rr := httptest.NewRecorder()
	HandleImport(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/import", nil))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if code := decodedCode(t, rr.Body.String()); code != codeImportFailed {
		t.Errorf("code = %q", code)
	}
	if strings.Contains(rr.Body.String(), fakeSessionToken) {
		t.Errorf("the response leaked the session token: %s", rr.Body.String())
	}
}

func TestHandlersRejectWrongMethods(t *testing.T) {
	h, _ := newTestHandler(t)
	cases := []struct {
		name    string
		path    string
		method  string
		handler func(*corepkg.Handler, http.ResponseWriter, *http.Request)
	}{
		{"account", "/api/folo/account", http.MethodPost, HandleAccount},
		{"login start", "/api/folo/login/start", http.MethodGet, HandleLoginStart},
		{"login status", "/api/folo/login/status", http.MethodPost, HandleLoginStatus},
		{"logout", "/api/folo/logout", http.MethodGet, HandleLogout},
		{"import", "/api/folo/import", http.MethodGet, HandleImport},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		tc.handler(h, rr, httptest.NewRequest(tc.method, tc.path, nil))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s with %s: status = %d, want 405", tc.name, tc.method, rr.Code)
		}
	}
}

func decodedCode(t *testing.T, body string) string {
	t.Helper()
	return decodedField(t, body, "code")
}

func decodedField(t *testing.T, body, field string) string {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	value, _ := payload[field].(string)
	return value
}

func seedArticle(t *testing.T, db *database.DB, url string) int64 {
	t.Helper()
	feedID, err := db.AddFeed(&models.Feed{Title: "Example", URL: "http://127.0.0.1:1/feed.xml"})
	if err != nil {
		t.Fatalf("add feed: %v", err)
	}
	if err := db.SaveArticle(&models.Article{
		FeedID:      feedID,
		Title:       "Article",
		URL:         url,
		PublishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save article: %v", err)
	}
	article, err := db.GetArticleByURL(url)
	if err != nil {
		t.Fatalf("load article: %v", err)
	}
	return article.ID
}

func TestHandleSyncRequiresSession(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	if err := h.DB.SetSetting("folo_api_url", server.URL); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	HandleSync(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/sync", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandleSyncRejectsConcurrentPass(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, _ := newTestHandler(t)
	signIn(t, h, server.URL)

	syncRunning.Store(true)
	defer syncRunning.Store(false)

	rr := httptest.NewRecorder()
	HandleSync(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/sync", nil))
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
	if code := decodedCode(t, rr.Body.String()); code != codeSyncBusy {
		t.Errorf("code = %q", code)
	}
}

func TestHandleSyncReconcilesMatchedEntry(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, db := newTestHandler(t)
	signIn(t, h, server.URL)
	articleID := seedArticle(t, db, "https://example.com/a")

	server.setEntries([]map[string]any{{
		"id":   "row-1",
		"read": true,
		"entries": map[string]any{
			"id":          "E1",
			"url":         "https://example.com/a",
			"publishedAt": "2026-10-05T12:00:00Z",
		},
	}})

	rr := httptest.NewRecorder()
	HandleSync(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/sync", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]float64{"matched": 1, "pulled": 1, "skipped": 0, "mapped": 1} {
		if payload[field] != want {
			t.Errorf("%s = %v, want %v (payload %+v)", field, payload[field], want, payload)
		}
	}
	syncedAt, err := time.Parse(time.RFC3339, payload["last_sync_time"].(string))
	if err != nil || time.Since(syncedAt) > time.Minute {
		t.Errorf("last_sync_time = %v, want the wall-clock time of this pass", payload["last_sync_time"])
	}
	if cursor, _ := db.GetSetting(folopkg.SyncCursorKey); cursor != "2026-10-05T12:00:00Z" {
		t.Errorf("cursor = %q, want the newest pulled entry", cursor)
	}

	article, err := db.GetArticleByID(articleID)
	if err != nil {
		t.Fatal(err)
	}
	if !article.IsRead {
		t.Error("the matched article should be read")
	}

	// The account view reports the same cursor and mapping count.
	account := httptest.NewRecorder()
	HandleAccount(h, account, httptest.NewRequest(http.MethodGet, "/api/folo/account", nil))
	if account.Code != http.StatusOK {
		t.Fatalf("account status = %d", account.Code)
	}
	var accountPayload map[string]any
	if err := json.Unmarshal(account.Body.Bytes(), &accountPayload); err != nil {
		t.Fatal(err)
	}
	if accountPayload["last_sync_time"] != payload["last_sync_time"] || accountPayload["mapped_count"] != float64(1) {
		t.Errorf("account payload = %+v", accountPayload)
	}
}

func TestHandleSyncPushesLocalChangeToFolo(t *testing.T) {
	server := fakeFolo(t, sampleOPML)
	h, db := newTestHandler(t)
	signIn(t, h, server.URL)
	articleID := seedArticle(t, db, "https://example.com/a")

	// Both sides agreed "unread"; the user read it locally.
	if err := db.UpsertFoloEntry(articleID, "E1", "https://example.com/a", "", false, false); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkArticleRead(articleID, true); err != nil {
		t.Fatal(err)
	}
	server.setEntries(nil)

	rr := httptest.NewRecorder()
	HandleSync(h, rr, httptest.NewRequest(http.MethodPost, "/api/folo/sync", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if ids := server.pushedReadIDs(); len(ids) != 1 || ids[0] != "E1" {
		t.Errorf("pushed reads = %v", ids)
	}
}
