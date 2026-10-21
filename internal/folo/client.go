// Package folo talks to a Folo (folo.is) account over its HTTP API.
//
// Authentication never touches the user's password: Folo's web login page hands
// out a one-time token that is exchanged for a session token (see login.go).
package folo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"MrRSS/internal/utils/httputil"
)

const (
	DefaultAPIURL = "https://api.folo.is"
	DefaultWebURL = "https://app.folo.is"

	requestTimeout   = 30 * time.Second
	maxResponseBytes = 8 << 20

	sessionCookieName       = "better-auth.session_token"
	secureSessionCookieName = "__Secure-better-auth.session_token"

	pathSession         = "/better-auth/get-session"
	pathApplyToken      = "/better-auth/one-time-token/apply"
	pathVerifyToken     = "/better-auth/one-time-token/verify"
	pathSubscriptionExp = "/subscriptions/export"

	userAgent = "MrRSS"
)

// ErrNotAuthenticated means Folo refused the stored session token. Callers map
// it to a re-login prompt instead of surfacing the HTTP status.
var ErrNotAuthenticated = errors.New("folo: not authenticated")

// AuthUser is the account behind the current session token.
type AuthUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Image string `json:"image"`
}

// Client is a single-account Folo API client. It is safe for concurrent use.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a client honouring the application's global proxy settings,
// which matters because folo.is is unreachable for some users without one.
func NewClient(apiURL, sessionToken string, settings httputil.ProxySettingsProvider) (*Client, error) {
	httpClient, err := httputil.CreateHTTPClientWithProxySettings(settings, requestTimeout)
	if err != nil {
		return nil, fmt.Errorf("create Folo HTTP client: %w", err)
	}
	return NewClientWithHTTPClient(apiURL, sessionToken, httpClient)
}

// NewClientWithHTTPClient lets tests and callers with a shared client build one.
func NewClientWithHTTPClient(apiURL, sessionToken string, httpClient *http.Client) (*Client, error) {
	baseURL, err := normalizeBaseURL(apiURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, errors.New("folo: HTTP client is required")
	}
	return &Client{baseURL: baseURL, token: strings.TrimSpace(sessionToken), http: httpClient}, nil
}

func normalizeBaseURL(apiURL string) (string, error) {
	trimmed := strings.TrimSpace(apiURL)
	if trimmed == "" {
		trimmed = DefaultAPIURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid Folo URL %q: %w", apiURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported Folo URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid Folo URL %q", apiURL)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// HasSession reports whether a session token is available.
func (c *Client) HasSession() bool { return c.token != "" }

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (*http.Request, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("build Folo request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Cookie", sessionCookieHeader(c.baseURL, c.token))
	}
	return req, nil
}

// sessionCookieHeader mirrors what the official Folo client sends: the bearer
// token plus the better-auth cookie, the secure name only for remote https hosts.
func sessionCookieHeader(baseURL, token string) string {
	parsed, err := url.Parse(baseURL)
	if err == nil && (parsed.Scheme != "https" || parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1") {
		return sessionCookieName + "=" + token
	}
	return secureSessionCookieName + "=" + token + "; " + sessionCookieName + "=" + token
}

func readCapped(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Folo response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("Folo response larger than %d bytes", maxResponseBytes)
	}
	return data, nil
}

// statusError maps a rejected request, translating the two unauthorized codes
// into ErrNotAuthenticated so callers can distinguish "log in again" from "Folo
// is unhappy".
func statusError(path string, resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrNotAuthenticated
	}
	return fmt.Errorf("Folo %s returned %s", path, resp.Status)
}

// Whoami resolves the account owning the current session token. A session Folo
// no longer recognises is reported as ErrNotAuthenticated.
func (c *Client) Whoami(ctx context.Context) (*AuthUser, error) {
	if !c.HasSession() {
		return nil, ErrNotAuthenticated
	}
	req, err := c.newRequest(ctx, http.MethodGet, pathSession, nil, nil, "")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Folo session endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(pathSession, resp)
	}
	data, err := readCapped(resp.Body)
	if err != nil {
		return nil, err
	}
	// better-auth answers bare "null" for an anonymous get-session.
	if strings.TrimSpace(string(data)) == "null" {
		return nil, ErrNotAuthenticated
	}
	var payload struct {
		User *AuthUser `json:"user"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode Folo session: %w", err)
	}
	if payload.User == nil {
		return nil, ErrNotAuthenticated
	}
	return payload.User, nil
}

// ExportOPML retrieves the account's subscriptions as an OPML document. Feeds
// built from RSSHub routes are expanded against rsshubURL when one is set.
func (c *Client) ExportOPML(ctx context.Context, folderMode, rsshubURL string) ([]byte, error) {
	if !c.HasSession() {
		return nil, ErrNotAuthenticated
	}
	query := url.Values{}
	switch folderMode {
	case "view", "category":
		query.Set("folderMode", folderMode)
	default:
		query.Set("folderMode", "category")
	}
	if trimmed := strings.TrimSpace(rsshubURL); trimmed != "" {
		query.Set("RSSHubURL", trimmed)
	}

	req, err := c.newRequest(ctx, http.MethodGet, pathSubscriptionExp, query, nil, "")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download Folo subscriptions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(pathSubscriptionExp, resp)
	}
	data, err := readCapped(resp.Body)
	if err != nil {
		return nil, err
	}
	// An interstitial or challenge page would parse as an empty subscription list,
	// so insist on an actual OPML document.
	if !strings.Contains(strings.ToLower(string(data)), "<opml") {
		return nil, errors.New("Folo did not return an OPML document")
	}
	return data, nil
}
