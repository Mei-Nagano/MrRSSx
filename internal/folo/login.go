package folo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"MrRSS/internal/utils/httputil"
)

const (
	// LoginLifetime bounds how long an unfinished browser login stays valid.
	LoginLifetime = 3 * time.Minute

	callbackPathPrefix = "/folo-login/"
	shutdownTimeout    = 2 * time.Second
)

// Login states reported to the UI.
const (
	LoginStateIdle      = "idle"
	LoginStatePending   = "pending"
	LoginStateConnected = "connected"
	LoginStateFailed    = "failed"
	LoginStateExpired   = "expired"
)

// PersistFunc stores an exchanged session token.
type PersistFunc func(ctx context.Context, sessionToken string) error

// LoginStatus describes the outcome of the most recent login attempt. The
// internal failure detail stays server-side: it can carry text from Folo, which
// is useful in the log but not worth echoing to the browser.
type LoginStatus struct {
	State string `json:"state"`
}

var (
	errNoSessionToken = errors.New("Folo did not return a session token")
	errNotFound       = errors.New("folo: endpoint not found")
	// errRejectedCredential marks a answer that says "this is not a usable login
	// credential", which is what makes the pasted-session-token fallback worth trying.
	errRejectedCredential = errors.New("Folo rejected the credential")

	loginMu     sync.Mutex
	activeLogin *PendingLogin
)

// ExchangeLoginToken turns whatever the login page handed out into a usable
// session token. Folo issues a one-time token, but users may also paste a raw
// session token, so an unusable value is finally tried as a token itself.
func ExchangeLoginToken(ctx context.Context, apiURL, credential string, settings httputil.ProxySettingsProvider) (string, error) {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return "", errors.New("Folo login credential is empty")
	}

	client, err := NewClient(apiURL, "", settings)
	if err != nil {
		return "", err
	}

	sessionToken, err := client.requestSessionToken(ctx, pathApplyToken, credential)
	if errors.Is(err, errNotFound) {
		sessionToken, err = client.requestSessionToken(ctx, pathVerifyToken, credential)
	}
	if err == nil {
		return sessionToken, nil
	}
	if !isCredentialProblem(err) {
		// A transport or server failure says nothing about the credential already
		// being a session token, so do not add a second request on top of it.
		return "", err
	}

	// Fallback for pasted session tokens: accept the value if it has a live session.
	bearer := &Client{baseURL: client.baseURL, token: credential, http: client.http}
	if _, whoamiErr := bearer.Whoami(ctx); whoamiErr == nil {
		return credential, nil
	}
	return "", err
}

// isCredentialProblem reports whether Folo refused the credential itself rather
// than failing for transport or server reasons.
func isCredentialProblem(err error) bool {
	return errors.Is(err, errNoSessionToken) ||
		errors.Is(err, errNotFound) ||
		errors.Is(err, errRejectedCredential)
}

// requestSessionToken posts a one-time token and reads the session token back
// from the better-auth cookie or the JSON body.
func (c *Client) requestSessionToken(ctx context.Context, path, oneTimeToken string) (string, error) {
	payload, err := json.Marshal(map[string]string{"token": oneTimeToken})
	if err != nil {
		return "", fmt.Errorf("encode Folo login request: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, nil, bytes.NewReader(payload), "application/json")
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("call Folo %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", errNotFound
	}
	data, readErr := readCapped(resp.Body)
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", fmt.Errorf("%w (%s): %s", errRejectedCredential, resp.Status, sanitizeMessage(data))
		}
		return "", fmt.Errorf("Folo %s returned %s: %s", path, resp.Status, sanitizeMessage(data))
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == sessionCookieName || cookie.Name == secureSessionCookieName {
			if value := strings.TrimSpace(cookie.Value); value != "" {
				return value, nil
			}
		}
	}
	var body struct {
		Session struct {
			Token string `json:"token"`
		} `json:"session"`
	}
	if err := json.Unmarshal(data, &body); err == nil {
		if token := strings.TrimSpace(body.Session.Token); token != "" {
			return token, nil
		}
	}
	return "", errNoSessionToken
}

// sanitizeMessage keeps a server message usable without smuggling arbitrary
// HTML or an echoed credential into a response.
func sanitizeMessage(data []byte) string {
	message := strings.TrimSpace(string(data))
	if runes := []rune(message); len(runes) > 200 {
		message = string(runes[:200])
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, message)
}

// StartLogin opens a browser login and returns the URL to visit. Any attempt
// still in flight is replaced. The listener is loopback-only, single use, and
// closes once the login lands, fails, or expires.
func StartLogin(apiURL, webURL string, settings httputil.ProxySettingsProvider, persist PersistFunc) (*PendingLogin, error) {
	loginMu.Lock()
	defer loginMu.Unlock()

	if activeLogin != nil {
		// shutdown only: the registry is already held and replaced below.
		activeLogin.shutdown()
		activeLogin = nil
	}

	client, err := NewClient(apiURL, "", settings)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("open loopback login listener: %w", err)
	}

	secret, err := randomHex(16)
	if err != nil {
		listener.Close()
		return nil, err
	}

	pending := &PendingLogin{
		apiURL:   client.baseURL,
		settings: settings,
		persist:  persist,
		path:     callbackPathPrefix + secret,
		done:     make(chan struct{}),
	}
	pending.CallbackURL = fmt.Sprintf("http://%s%s", listener.Addr().String(), pending.path)
	pending.LoginURL, err = buildLoginURL(webURL, pending.CallbackURL)
	if err != nil {
		listener.Close()
		return nil, err
	}
	pending.ExpiresAt = time.Now().Add(LoginLifetime)

	mux := http.NewServeMux()
	mux.HandleFunc(pending.path, pending.handleCallback)
	pending.listener = listener
	pending.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	serverCtx, cancel := context.WithCancel(context.Background())
	pending.cancel = cancel
	pending.ctx = serverCtx
	go func() {
		if serveErr := pending.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			pending.fail(fmt.Errorf("login listener failed: %w", serveErr))
		}
	}()
	go func() {
		timer := time.NewTimer(LoginLifetime)
		defer timer.Stop()
		select {
		case <-timer.C:
			pending.expire()
		case <-pending.done:
		case <-serverCtx.Done():
		}
	}()

	activeLogin = pending
	return pending, nil
}

func buildLoginURL(webURL, callbackURL string) (string, error) {
	trimmed := strings.TrimSpace(webURL)
	if trimmed == "" {
		trimmed = DefaultWebURL
	}
	base, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid Folo web URL %q: %w", webURL, err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("unsupported Folo web URL scheme %q", base.Scheme)
	}
	if base.Host == "" || base.User != nil {
		return "", fmt.Errorf("invalid Folo web URL %q", webURL)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/login"
	base.RawQuery = ""
	base.Fragment = ""
	query := base.Query()
	query.Set("cli_callback", callbackURL)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

// PendingLogin is an in-flight browser login.
type PendingLogin struct {
	LoginURL    string
	CallbackURL string
	ExpiresAt   time.Time

	apiURL   string
	settings httputil.ProxySettingsProvider
	persist  PersistFunc
	path     string
	listener net.Listener
	server   *http.Server
	done     chan struct{}
	cancel   context.CancelFunc
	// ctx is cancelled when the attempt is abandoned. The callback's token
	// exchange derives from it, so a sign-out mid-exchange aborts the request
	// instead of letting it finish and write the credential back.
	ctx       context.Context
	stateMu   sync.Mutex
	state     string
	closeOnce sync.Once
}

func (p *PendingLogin) State() LoginStatus {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	state := p.state
	if state == "" {
		state = LoginStatePending
	}
	return LoginStatus{State: state}
}

// Cancel abandons an attempt, e.g. when the user closes the settings tab.
func (p *PendingLogin) Cancel() {
	p.record(LoginStateFailed, "cancelled")
}

// ActiveLoginStatus reports the most recent attempt for the UI.
func ActiveLoginStatus() LoginStatus {
	loginMu.Lock()
	defer loginMu.Unlock()
	if activeLogin == nil {
		return LoginStatus{State: LoginStateIdle}
	}
	status := activeLogin.State()
	if status.State == LoginStatePending && time.Now().After(activeLogin.ExpiresAt) {
		return LoginStatus{State: LoginStateExpired}
	}
	return status
}

// CancelActiveLogin abandons an in-flight browser sign-in, e.g. when the user
// signs out or leaves the settings tab.
func CancelActiveLogin() {
	loginMu.Lock()
	pending := activeLogin
	activeLogin = nil
	loginMu.Unlock()
	if pending != nil {
		pending.Cancel()
	}
}

// errLoginAbandoned reports that the attempt ended (cancelled, expired or
// replaced) before its exchanged token could be stored.
var errLoginAbandoned = errors.New("the Folo sign-in was abandoned")

// abandoned reports whether the attempt already ended without a session.
func (p *PendingLogin) abandoned() bool {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.state == LoginStateFailed || p.state == LoginStateExpired
}

// persistIfCurrent stores the exchanged token only while p is still the attempt
// the user is waiting for. The check and the write both happen under loginMu,
// which CancelActiveLogin also takes, so the two cannot interleave: either the
// token is written before sign-out clears it, or it is refused afterwards.
func (p *PendingLogin) persistIfCurrent(ctx context.Context, token string) error {
	loginMu.Lock()
	defer loginMu.Unlock()
	if activeLogin != p || p.abandoned() {
		return errLoginAbandoned
	}
	return p.persist(ctx, token)
}

func (p *PendingLogin) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	credential := strings.TrimSpace(r.URL.Query().Get("token"))
	if credential == "" {
		p.fail(errors.New("the Folo login callback carried no token"))
		writeResultPage(w, false, "MrRSS received no login token. Start the sign-in again.")
		return
	}

	// The callback owns the browser navigation, so the exchange must not inherit a
	// request context the browser could cancel. It still derives from the
	// attempt's own context, so abandoning the attempt aborts the exchange.
	base := p.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, requestTimeout)
	defer cancel()

	sessionToken, err := ExchangeLoginToken(ctx, p.apiURL, credential, p.settings)
	if err != nil {
		p.fail(err)
		writeResultPage(w, false, "MrRSS could not finish the Folo sign-in. Start it again.")
		return
	}
	// The exchange may finish after the user signed out or started a newer
	// attempt. Committing is gated so such a token never writes the credential
	// the user just cleared back into storage.
	if err := p.persistIfCurrent(ctx, sessionToken); err != nil {
		if errors.Is(err, errLoginAbandoned) {
			writeResultPage(w, false, "This sign-in was cancelled. Start it again.")
			return
		}
		p.fail(err)
		writeResultPage(w, false, "MrRSS could not store the Folo session. Try again.")
		return
	}

	p.record(LoginStateConnected, "")
	writeResultPage(w, true, "")
}

func (p *PendingLogin) record(state, message string) {
	p.stateMu.Lock()
	fresh := p.state == "" || p.state == LoginStatePending
	if fresh {
		p.state = state
	}
	p.stateMu.Unlock()

	if fresh && state != LoginStateConnected {
		log.Printf("folo: login %s: %s", state, message)
	}
	// The attempt stays readable after it ends: the UI polls for the outcome, so
	// closing the listener must not erase the result.
	p.shutdown()
}

func (p *PendingLogin) fail(err error) {
	message := "login failed"
	if err != nil {
		message = err.Error()
	}
	p.record(LoginStateFailed, message)
}

func (p *PendingLogin) expire() {
	p.record(LoginStateExpired, "the sign-in attempt expired")
}

// shutdown tears the listener down exactly once. Callers that already hold
// loginMu use it directly; CancelActiveLogin clears the registry around it.
func (p *PendingLogin) shutdown() {
	p.closeOnce.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		if p.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			p.server.Shutdown(ctx)
		}
		if p.listener != nil {
			p.listener.Close()
		}
		close(p.done)
	})
}

func writeResultPage(w http.ResponseWriter, ok bool, reason string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	title, body := "Folo sign-in failed", html.EscapeString(reason)
	if ok {
		title, body = "Folo sign-in complete", "You can close this tab and return to MrRSS."
	}
	// Static text only: the token must never be reflected into the page.
	fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>%s</title>"+
		"</head><body style=\"font-family:system-ui,sans-serif;padding:2rem\"><h1>%s</h1><p>%s</p></body></html>\n",
		html.EscapeString(title), html.EscapeString(title), body)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate login nonce: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
