package folo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type recordedRequest struct {
	method string
	path   string
	body   map[string]any
}

func newStateServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var (
		mu   sync.Mutex
		seen []recordedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		mu.Lock()
		seen = append(seen, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
		mu.Unlock()

		if r.Header.Get("Authorization") != "Bearer session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r, body)
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func newStateClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClientWithHTTPClient(server.URL, "session", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestListEntriesSendsTimelineQueryAndParsesRows(t *testing.T) {
	server, seen := newStateServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"code":0,"data":[
			{"id":"row-1","read":true,"collections":{"createdAt":"2026-01-01"},
			 "entries":{"id":"entry-1","url":"https://example.com/a","publishedAt":"2026-10-01T00:00:00Z"}},
			{"id":"row-2","read":false,
			 "entries":{"id":"entry-2","url":null,"publishedAt":"2026-10-02T00:00:00Z"}},
			{"read":false,"entries":{"url":"https://example.com/no-id"}}
		]}`))
	})
	client := newStateClient(t, server)

	collected := true
	entries, err := client.ListEntries(context.Background(), EntryListParams{
		Limit:        50,
		NewerThan:    "2026-09-30T00:00:00Z",
		IsCollection: &collected,
	})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (a row without an id is dropped)", len(entries))
	}
	if entries[0].ID != "entry-1" || entries[0].URL != "https://example.com/a" || !entries[0].Read || !entries[0].Collected {
		t.Errorf("first entry = %+v", entries[0])
	}
	if entries[1].ID != "entry-2" || entries[1].URL != "" || entries[1].Collected {
		t.Errorf("second entry = %+v", entries[1])
	}

	if len(*seen) != 1 {
		t.Fatalf("requests = %d", len(*seen))
	}
	request := (*seen)[0]
	if request.method != http.MethodPost || request.path != pathEntries {
		t.Errorf("request = %s %s", request.method, request.path)
	}
	if request.body["limit"] != float64(50) || request.body["sortOrder"] != "desc" ||
		request.body["publishedBefore"] != "2026-09-30T00:00:00Z" || request.body["isCollection"] != true {
		t.Errorf("body = %+v", request.body)
	}
	if request.body["withContent"] != false {
		t.Errorf("withContent = %v, want false", request.body["withContent"])
	}
}

func TestListEntriesCapsPageSizeAndOmitsUnsetFilters(t *testing.T) {
	server, seen := newStateServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
	})
	client := newStateClient(t, server)

	if _, err := client.ListEntries(context.Background(), EntryListParams{Limit: 5000}); err != nil {
		t.Fatal(err)
	}
	body := (*seen)[0].body
	if body["limit"] != float64(maxEntriesPerPage) {
		t.Errorf("limit = %v, want %d", body["limit"], maxEntriesPerPage)
	}
	if body["sortOrder"] != "desc" {
		t.Errorf("sortOrder = %v, want desc; Folo rejects every other value with 422", body["sortOrder"])
	}
	for _, key := range []string{"publishedBefore", "publishedAfter", "isCollection"} {
		if _, ok := body[key]; ok {
			t.Errorf("%s should be omitted when unset", key)
		}
	}
}

func TestListEntriesMapsTimeBoundsOntoFoloDateFields(t *testing.T) {
	server, seen := newStateServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
	})
	client := newStateClient(t, server)

	// Folo's publishedBefore is a lower bound and publishedAfter an upper bound,
	// the reverse of their names; callers use plain directions.
	_, err := client.ListEntries(context.Background(), EntryListParams{
		NewerThan: "2026-10-01T00:00:00Z",
		OlderThan: "2026-10-20T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := (*seen)[0].body
	if body["publishedBefore"] != "2026-10-01T00:00:00Z" {
		t.Errorf("publishedBefore = %v, want the NewerThan bound", body["publishedBefore"])
	}
	if body["publishedAfter"] != "2026-10-20T00:00:00Z" {
		t.Errorf("publishedAfter = %v, want the OlderThan bound", body["publishedAfter"])
	}
}

func TestReadAndCollectionCallsUseExpectedMethods(t *testing.T) {
	server, seen := newStateServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	client := newStateClient(t, server)
	ctx := context.Background()

	if err := client.MarkEntriesRead(ctx, []string{"e1", " ", "e2"}); err != nil {
		t.Fatal(err)
	}
	if err := client.MarkEntryUnread(ctx, "e3"); err != nil {
		t.Fatal(err)
	}
	if err := client.AddToCollection(ctx, "e4"); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveFromCollection(ctx, "e5"); err != nil {
		t.Fatal(err)
	}

	want := []struct {
		method, path string
	}{
		{http.MethodPost, pathReads},
		{http.MethodDelete, pathReads},
		{http.MethodPost, pathCollections},
		{http.MethodDelete, pathCollections},
	}
	if len(*seen) != len(want) {
		t.Fatalf("requests = %d, want %d", len(*seen), len(want))
	}
	for i, expected := range want {
		if (*seen)[i].method != expected.method || (*seen)[i].path != expected.path {
			t.Errorf("request %d = %s %s, want %s %s", i, (*seen)[i].method, (*seen)[i].path, expected.method, expected.path)
		}
	}

	readIDs, _ := (*seen)[0].body["entryIds"].([]any)
	if len(readIDs) != 2 || readIDs[0] != "e1" || readIDs[1] != "e2" {
		t.Errorf("entryIds = %v, want the blank id dropped", (*seen)[0].body["entryIds"])
	}
	if (*seen)[1].body["entryId"] != "e3" || (*seen)[2].body["entryId"] != "e4" || (*seen)[3].body["entryId"] != "e5" {
		t.Errorf("single-entry bodies = %v %v %v", (*seen)[1].body, (*seen)[2].body, (*seen)[3].body)
	}
}

func TestEmptyStateCallsMakeNoRequest(t *testing.T) {
	server, seen := newStateServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	client := newStateClient(t, server)
	ctx := context.Background()

	if err := client.MarkEntriesRead(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.MarkEntryUnread(ctx, "  "); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveFromCollection(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 0 {
		t.Errorf("requests = %d, want 0", len(*seen))
	}
}

func TestApiEnvelopeErrorsSurface(t *testing.T) {
	server, _ := newStateServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"code":3,"message":"Unprocessable content"}`))
	})
	client := newStateClient(t, server)

	err := client.MarkEntriesRead(context.Background(), []string{"e1"})
	if err == nil || !strings.Contains(err.Error(), "code 3") {
		t.Fatalf("error = %v, want the envelope code surfaced", err)
	}
	if !strings.Contains(err.Error(), "Unprocessable content") {
		t.Errorf("error = %v, want Folo's message", err)
	}
}

func TestStateCallsMapUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := newStateClient(t, server)

	if err := client.AddToCollection(context.Background(), "e1"); err != ErrNotAuthenticated {
		t.Errorf("error = %v, want ErrNotAuthenticated", err)
	}
}

func TestStateCallsWithoutSessionSkipNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request should be made without a session")
	}))
	defer server.Close()
	client, err := NewClientWithHTTPClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MarkEntriesRead(context.Background(), []string{"e1"}); err != ErrNotAuthenticated {
		t.Errorf("error = %v", err)
	}
}
