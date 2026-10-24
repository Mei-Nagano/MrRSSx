package folo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeFolo answers the three better-auth paths the integration uses, with each
// one configurable so a test can pin down exactly which call was allowed to
// happen.
type fakeFolo struct {
	mu     sync.Mutex
	counts map[string]int

	applyMode  string // session, notFound, reject, error
	verifyMode string // session, notFound, reject, error
	// liveSession is the token /get-session accepts; empty means anonymous.
	liveSession   string
	sessionCookie string

	server *httptest.Server
}

func newFakeFolo(t *testing.T) *fakeFolo {
	t.Helper()
	f := &fakeFolo{
		counts:    map[string]int{},
		applyMode: "reject",
	}
	mux := http.NewServeMux()
	mux.HandleFunc(pathApplyToken, func(w http.ResponseWriter, r *http.Request) { f.exchange(w, r, pathApplyToken) })
	mux.HandleFunc(pathVerifyToken, func(w http.ResponseWriter, r *http.Request) { f.exchange(w, r, pathVerifyToken) })
	mux.HandleFunc(pathSession, func(w http.ResponseWriter, r *http.Request) {
		f.hit(pathSession)
		var token string
		f.mu.Lock()
		token = f.liveSession
		f.mu.Unlock()
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != "" && bearer == token {
			_, _ = w.Write([]byte(`{"user":{"id":"u1","name":"Ada","email":"ada@example.com"}}`))
			return
		}
		_, _ = w.Write([]byte("null"))
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeFolo) hit(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[path]++
}

func (f *fakeFolo) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[path]
}

func (f *fakeFolo) setMode(which, mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch which {
	case "apply":
		f.applyMode = mode
	case "verify":
		f.verifyMode = mode
	case "session":
		f.liveSession = mode
	}
}

func (f *fakeFolo) mode(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch path {
	case pathApplyToken:
		return f.applyMode
	case pathVerifyToken:
		return f.verifyMode
	}
	return "reject"
}

func (f *fakeFolo) exchange(w http.ResponseWriter, r *http.Request, which string) {
	f.hit(which)
	switch f.mode(which) {
	case "notFound":
		w.WriteHeader(http.StatusNotFound)
	case "error":
		w.WriteHeader(http.StatusInternalServerError)
	case "session":
		var body struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Token != "one-time" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Invalid token"}`))
			return
		}
		var token string
		f.mu.Lock()
		token = f.sessionCookie
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/"})
		_, _ = w.Write([]byte(`{"session":{"token":"` + token + `"}}`))
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Invalid token"}`))
	}
}

func TestExchangeLoginTokenAppliesOneTimeToken(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "session")
	f.sessionCookie = "SESSION-1"

	token, err := ExchangeLoginToken(context.Background(), f.server.URL, "one-time", nil)
	if err != nil {
		t.Fatalf("ExchangeLoginToken: %v", err)
	}
	if token != "SESSION-1" {
		t.Errorf("token = %q, want the exchanged session token", token)
	}
	if f.count(pathVerifyToken) != 0 {
		t.Error("verify must not be called when apply succeeds")
	}
}

func TestExchangeLoginTokenFallsBackToVerify(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "notFound")
	f.setMode("verify", "session")
	f.sessionCookie = "SESSION-VERIFY"

	token, err := ExchangeLoginToken(context.Background(), f.server.URL, "one-time", nil)
	if err != nil {
		t.Fatalf("ExchangeLoginToken: %v", err)
	}
	if token != "SESSION-VERIFY" {
		t.Errorf("token = %q", token)
	}
	if f.count(pathVerifyToken) != 1 {
		t.Errorf("verify called %d times", f.count(pathVerifyToken))
	}
}

func TestExchangeLoginTokenAcceptsPastedSessionToken(t *testing.T) {
	f := newFakeFolo(t)
	// apply and verify both reject the value, but it is a live session token.
	f.setMode("session", "pasted-session")

	token, err := ExchangeLoginToken(context.Background(), f.server.URL, " pasted-session ", nil)
	if err != nil {
		t.Fatalf("a pasted session token should be accepted: %v", err)
	}
	if token != "pasted-session" {
		t.Errorf("token = %q", token)
	}
}

func TestExchangeLoginTokenRejectsUnusableCredential(t *testing.T) {
	f := newFakeFolo(t)
	if _, err := ExchangeLoginToken(context.Background(), f.server.URL, "junk", nil); err == nil {
		t.Fatal("an unusable credential must not exchange")
	}
}

func TestExchangeLoginTokenDoesNotProbeSessionOnServerError(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "error")

	if _, err := ExchangeLoginToken(context.Background(), f.server.URL, "one-time", nil); err == nil {
		t.Fatal("a Folo outage must surface as an error")
	}
	if f.count(pathSession) != 0 {
		t.Error("a Folo outage is not evidence that the credential is a session token")
	}
}

func TestExchangeLoginTokenRejectsEmptyCredential(t *testing.T) {
	f := newFakeFolo(t)
	if _, err := ExchangeLoginToken(context.Background(), f.server.URL, "   ", nil); err == nil {
		t.Error("an empty credential must be refused before any request")
	}
	if f.count(pathApplyToken) != 0 {
		t.Error("an empty credential must not reach Folo")
	}
}

func TestBuildLoginURLCarriesLoopbackCallback(t *testing.T) {
	built, err := buildLoginURL("https://app.folo.is/", "http://127.0.0.1:44321/folo-login/abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://app.folo.is/login?", "cli_callback=http"} {
		if !strings.Contains(built, want) {
			t.Errorf("login URL %q missing %q", built, want)
		}
	}
	if _, err := buildLoginURL("ftp://app.folo.is", "http://127.0.0.1:1/c"); err == nil {
		t.Error("an unsafe web URL must be refused")
	}
}

func TestStartLoginCallbackExchangesAndPersistsToken(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "session")
	f.sessionCookie = "SESSION-CALLBACK"

	persisted := make(chan string, 1)
	pending, err := StartLogin(f.server.URL, f.server.URL, nil, func(_ context.Context, token string) error {
		persisted <- token
		return nil
	})
	if err != nil {
		t.Fatalf("StartLogin: %v", err)
	}
	t.Cleanup(CancelActiveLogin)

	if !strings.HasPrefix(pending.CallbackURL, "http://127.0.0.1:") {
		t.Errorf("callback must be loopback, got %q", pending.CallbackURL)
	}
	if time.Until(pending.ExpiresAt) <= 0 {
		t.Error("the attempt must carry a deadline")
	}

	page := get(t, pending.CallbackURL+"?token=one-time")

	select {
	case token := <-persisted:
		if token != "SESSION-CALLBACK" {
			t.Errorf("persisted = %q, want the exchanged session token", token)
		}
	default:
		t.Error("the callback did not persist a session token")
	}
	// Neither credential may be reflected into the page the browser renders.
	for _, secret := range []string{"one-time", "SESSION-CALLBACK"} {
		if strings.Contains(page, secret) {
			t.Errorf("callback page leaked %q: %s", secret, page)
		}
	}
	if status := ActiveLoginStatus(); status.State != LoginStateConnected {
		t.Errorf("status after callback = %+v", status)
	}
}

func TestStartLoginCallbackRejectsUnknownPathAndMissingToken(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "session")
	f.sessionCookie = "SESSION-X"

	persisted := make(chan string, 1)
	pending, err := StartLogin(f.server.URL, f.server.URL, nil, func(_ context.Context, token string) error {
		persisted <- token
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(CancelActiveLogin)

	resp, err := http.Get(strings.Replace(pending.CallbackURL, pending.path, "/folo-login/guessed", 1))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("guessed path returned %d, want 404", resp.StatusCode)
	}

	get(t, pending.CallbackURL)
	select {
	case token := <-persisted:
		t.Errorf("a callback without a token persisted %q", token)
	default:
	}
	if status := ActiveLoginStatus(); status.State != LoginStateFailed {
		t.Errorf("a callback without a token should fail the attempt, got %+v", status)
	}
}

func TestStartLoginReplacesPreviousAttempt(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "session")
	f.sessionCookie = "SESSION-REPLACE"

	persisted := make(chan string, 4)
	first, err := StartLogin(f.server.URL, f.server.URL, nil, func(_ context.Context, token string) error {
		persisted <- token
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := StartLogin(f.server.URL, f.server.URL, nil, func(_ context.Context, token string) error {
		persisted <- token
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(CancelActiveLogin)

	if first.CallbackURL == second.CallbackURL {
		t.Error("the replaced attempt should not keep the same listener")
	}
	// The first listener is closed, so its callback can no longer sign anything in.
	if resp, err := http.Get(first.CallbackURL + "?token=one-time"); err == nil {
		resp.Body.Close()
		t.Error("the replaced listener should be gone")
	}
	select {
	case token := <-persisted:
		t.Errorf("the replaced listener persisted %q", token)
	default:
	}

	get(t, second.CallbackURL+"?token=one-time")
	if token := <-persisted; token != "SESSION-REPLACE" {
		t.Errorf("current listener persisted %q", token)
	}
}

func TestCancelActiveLoginClearsRegistry(t *testing.T) {
	f := newFakeFolo(t)
	f.setMode("apply", "session")

	if _, err := StartLogin(f.server.URL, f.server.URL, nil, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	CancelActiveLogin()
	if status := ActiveLoginStatus(); status.State != LoginStateIdle {
		t.Errorf("status after cancel = %+v, want idle", status)
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A sign-out that lands while the callback is exchanging must not let the token
// reach storage afterwards: that is exactly the credential a cancelled sign-in
// used to write back.
func TestCancelledLoginDoesNotPersistInFlightToken(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathApplyToken {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		enteredOnce.Do(func() { close(entered) })
		<-release // hold the exchange open until the test cancels
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "SESSION-LATE"})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]string{"token": "SESSION-LATE"},
		})
	}))
	defer api.Close()

	persisted := make(chan string, 1)
	pending, err := StartLogin(api.URL, api.URL, nil, func(_ context.Context, token string) error {
		persisted <- token
		return nil
	})
	if err != nil {
		t.Fatalf("StartLogin: %v", err)
	}
	t.Cleanup(CancelActiveLogin)

	callbackDone := make(chan struct{})
	go func() {
		defer close(callbackDone)
		resp, getErr := http.Get(pending.CallbackURL + "?token=one-time")
		if getErr == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the callback never reached the token exchange")
	}

	CancelActiveLogin()
	unblock()

	select {
	case <-callbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the callback did not finish after cancellation")
	}

	select {
	case token := <-persisted:
		t.Errorf("a cancelled sign-in stored %q", token)
	default:
	}
	if status := ActiveLoginStatus(); status.State == LoginStateConnected {
		t.Errorf("a cancelled attempt reported connected: %+v", status)
	}
}

// The commit gate itself: an attempt that was cancelled or replaced may never
// write a token, while the current attempt still can.
func TestPersistIfCurrentRefusesAbandonedOrReplacedAttempt(t *testing.T) {
	loginMu.Lock()
	previous := activeLogin
	loginMu.Unlock()
	t.Cleanup(func() {
		loginMu.Lock()
		activeLogin = previous
		loginMu.Unlock()
	})

	stored := make(chan string, 4)
	newAttempt := func() *PendingLogin {
		return &PendingLogin{
			done: make(chan struct{}),
			persist: func(_ context.Context, token string) error {
				stored <- token
				return nil
			},
		}
	}

	cancelled := newAttempt()
	loginMu.Lock()
	activeLogin = cancelled
	loginMu.Unlock()
	cancelled.Cancel()
	if err := cancelled.persistIfCurrent(context.Background(), "SESSION-CANCELLED"); !errors.Is(err, errLoginAbandoned) {
		t.Errorf("cancelled attempt returned %v, want errLoginAbandoned", err)
	}

	older := newAttempt()
	newer := newAttempt()
	loginMu.Lock()
	activeLogin = newer
	loginMu.Unlock()

	if err := older.persistIfCurrent(context.Background(), "SESSION-OLDER"); !errors.Is(err, errLoginAbandoned) {
		t.Errorf("replaced attempt returned %v, want errLoginAbandoned", err)
	}
	if err := newer.persistIfCurrent(context.Background(), "SESSION-CURRENT"); err != nil {
		t.Errorf("current attempt should be allowed to persist: %v", err)
	}

	close(stored)
	var got []string
	for token := range stored {
		got = append(got, token)
	}
	if len(got) != 1 || got[0] != "SESSION-CURRENT" {
		t.Errorf("persisted %v, want only the current attempt's token", got)
	}
}
