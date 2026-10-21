package folo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientRejectsUnusableURLs(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"ftp://folo.example/feed",
		"https://user:pass@api.folo.is",
		"https://",
	}
	for _, raw := range cases {
		if _, err := NewClientWithHTTPClient(raw, "token", http.DefaultClient); err == nil {
			t.Errorf("NewClient(%q) accepted an unsafe URL", raw)
		}
	}
	client, err := NewClientWithHTTPClient("https://api.folo.is/", "", http.DefaultClient)
	if err != nil {
		t.Fatalf("default-like URL rejected: %v", err)
	}
	if client.baseURL != "https://api.folo.is" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", client.baseURL)
	}
}

func TestSessionCookieHeaderKeepsSecureNameOnlyForRemoteHTTPS(t *testing.T) {
	if got := sessionCookieHeader("https://api.folo.is", "T"); got !=
		"__Secure-better-auth.session_token=T; better-auth.session_token=T" {
		t.Errorf("remote https cookie = %q", got)
	}
	if got := sessionCookieHeader("http://localhost:3000", "T"); got != "better-auth.session_token=T" {
		t.Errorf("localhost cookie = %q", got)
	}
	if got := sessionCookieHeader("http://api.folo.is", "T"); got != "better-auth.session_token=T" {
		t.Errorf("plain http cookie = %q", got)
	}
}

func TestWhoamiSendsCredentialsAndResolvesAccount(t *testing.T) {
	var auth, cookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathSession {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		cookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"u1","name":"Ada","email":"ada@example.com"}}`))
	}))
	defer server.Close()

	client, err := NewClientWithHTTPClient(server.URL, "session-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	user, err := client.Whoami(context.Background())
	if err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if user.Email != "ada@example.com" || user.Name != "Ada" {
		t.Errorf("unexpected user %+v", user)
	}
	if auth != "Bearer session-token" {
		t.Errorf("Authorization = %q", auth)
	}
	if !strings.Contains(cookie, "better-auth.session_token=session-token") {
		t.Errorf("Cookie = %q", cookie)
	}
}

func TestWhoamiTreatsAnonymousAndUnauthorizedAsSignedOut(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"anonymous null": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("null"))
		},
		"unauthorized": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
		"no user in payload": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"session":{"token":"x"}}`))
		},
	} {
		server := httptest.NewServer(handler)
		client, err := NewClientWithHTTPClient(server.URL, "token", server.Client())
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		if _, err := client.Whoami(context.Background()); err == nil || err != ErrNotAuthenticated {
			t.Errorf("%s: Whoami error = %v, want ErrNotAuthenticated", name, err)
		}
		server.Close()
	}
}

func TestWhoamiWithoutTokenMakesNoRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
	}))
	defer server.Close()

	client, err := NewClientWithHTTPClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Whoami(context.Background()); err != ErrNotAuthenticated {
		t.Errorf("Whoami without token = %v", err)
	}
	if requests != 0 {
		t.Errorf("expected no request, made %d", requests)
	}
}

func TestExportOPMLOverridesFolderModeAndRSSHub(t *testing.T) {
	var query, path string
	document := `<?xml version="1.0"?><opml version="2.0"><body><outline text="b" type="rss" xmlUrl="https://example.com/feed"/></body></opml>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(document))
	}))
	defer server.Close()

	client, err := NewClientWithHTTPClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ExportOPML(context.Background(), "view", "https://rsshub.mynet")
	if err != nil {
		t.Fatalf("ExportOPML: %v", err)
	}
	if string(got) != document {
		t.Errorf("document mismatch: %q", got)
	}
	if path != pathSubscriptionExp {
		t.Errorf("path = %s", path)
	}
	for _, want := range []string{"folderMode=view", "RSSHubURL=https%3A%2F%2Frsshub.mynet"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q missing %q", query, want)
		}
	}
}

func TestExportOPMLDefaultsFolderModeAndRefusesNonOPML(t *testing.T) {
	var folderMode string
	body := "<html><body>Just a moment...</body></html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		folderMode = r.URL.Query().Get("folderMode")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client, err := NewClientWithHTTPClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ExportOPML(context.Background(), "nonsense", ""); err == nil {
		t.Fatal("an HTML interstitial must not pass as a subscription list")
	}
	if folderMode != "category" {
		t.Errorf("folderMode = %q, want the category default", folderMode)
	}
	if _, err := client.ExportOPML(context.Background(), "category", ""); !strings.Contains(err.Error(), "OPML") {
		t.Errorf("error = %v, should name the OPML problem", err)
	}
}

func TestExportOPMLUnauthorizedMapsToSignedOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewClientWithHTTPClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ExportOPML(context.Background(), "category", ""); err != ErrNotAuthenticated {
		t.Errorf("ExportOPML = %v, want ErrNotAuthenticated", err)
	}
}

func TestClientHonoursContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewClientWithHTTPClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Whoami(ctx); err == nil {
		t.Error("a cancelled context must not report success")
	}
}
