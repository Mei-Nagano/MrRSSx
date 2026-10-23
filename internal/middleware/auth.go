package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuthCookieName is the cookie that carries a validated access token.
const AuthCookieName = "mrrss_access"

// authCookieMaxAge controls how long a browser stays signed in after entering
// the access token.
const authCookieMaxAge = 30 * 24 * time.Hour

// loginTemplate is intentionally self-contained: the login page has to render
// before any authenticated asset is available.
var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>MrRSS</title>
<style>
  :root { color-scheme: light dark; }
  body {
    margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
    background: #f8f9fa; color: #212529;
    font-family: system-ui, -apple-system, "Segoe UI", "Noto Sans CJK SC", sans-serif;
  }
  .card {
    width: min(22rem, calc(100vw - 2rem)); background: #fff; border: 1px solid #dee2e6;
    border-radius: 0.75rem; padding: 1.75rem; box-shadow: 0 12px 32px rgba(0, 0, 0, 0.08);
  }
  h1 { margin: 0 0 0.25rem; font-size: 1.25rem; }
  p { margin: 0 0 1.25rem; font-size: 0.875rem; color: #6c757d; line-height: 1.5; }
  label { display: block; font-size: 0.8125rem; font-weight: 600; margin-bottom: 0.375rem; }
  input {
    width: 100%; box-sizing: border-box; padding: 0.625rem 0.75rem; font-size: 1rem;
    border: 1px solid #dee2e6; border-radius: 0.5rem; background: #fff; color: inherit;
  }
  input:focus { outline: 2px solid #007bff; outline-offset: 1px; border-color: #007bff; }
  button {
    margin-top: 1rem; width: 100%; padding: 0.625rem 0.75rem; font-size: 0.9375rem; font-weight: 600;
    color: #fff; background: #007bff; border: 0; border-radius: 0.5rem; cursor: pointer;
  }
  button:hover { background: #0056b3; }
  .error {
    margin: 0 0 1rem; padding: 0.625rem 0.75rem; border-radius: 0.5rem;
    background: #fdecec; color: #a12622; font-size: 0.8125rem;
  }
  @media (prefers-color-scheme: dark) {
    body { background: #1e1e1e; color: #e0e0e0; }
    .card { background: #252526; border-color: #3e3e42; }
    p { color: #a0a0a0; }
    input { background: #1e1e1e; border-color: #3e3e42; }
    .error { background: #3a1f1f; color: #f5a9a6; }
  }
</style>
</head>
<body>
  <form class="card" method="post" action="/login">
    <h1>MrRSS</h1>
    <p>Enter the access token to open this reader.<br>请输入访问令牌以打开这个阅读器。</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <input type="hidden" name="next" value="{{.Next}}">
    <label for="token">Access token / 访问令牌</label>
    <input id="token" name="token" type="password" autocomplete="current-password" autofocus required>
    <button type="submit">Continue / 继续</button>
  </form>
</body>
</html>
`))

type loginPageData struct {
	Error string
	Next  string
}

// RequireToken protects next with a shared access token. Requests authenticate
// with the cookie set by /login; API requests receive 401 instead of a redirect
// so clients can tell "not signed in" apart from a server error.
//
// An empty token disables the check, which keeps existing deployments working.
func RequireToken(token string, next http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		switch r.URL.Path {
		case "/login":
			handleLogin(w, r, token)
			return
		case "/logout":
			handleLogout(w, r)
			return
		}

		if hasValidToken(r, token) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer realm="MrRSS"`)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
			return
		}

		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
	})
}

// isPublicPath lists assets that must load before the user is authenticated:
// the installable app shell, its icons and the login page itself. None of them
// expose reader data, which stays behind /api/.
func isPublicPath(path string) bool {
	switch path {
	case "/manifest.webmanifest", "/sw.js", "/favicon.ico":
		return true
	}
	return strings.HasPrefix(path, "/icons/") || strings.HasPrefix(path, "/assets/")
}

func hasValidToken(r *http.Request, token string) bool {
	cookie, err := r.Cookie(AuthCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func handleLogin(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodPost {
		renderLogin(w, http.StatusOK, loginPageData{Next: safeNextPath(r.URL.Query().Get("next"))})
		return
	}

	if err := r.ParseForm(); err != nil {
		renderLogin(w, http.StatusBadRequest, loginPageData{Error: "Malformed form submission"})
		return
	}

	nextPath := safeNextPath(r.FormValue("next"))
	if subtle.ConstantTimeCompare([]byte(r.FormValue("token")), []byte(token)) != 1 {
		renderLogin(w, http.StatusUnauthorized, loginPageData{
			Error: "Incorrect access token. 访问令牌不正确。",
			Next:  nextPath,
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(authCookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, nextPath, http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func renderLogin(w http.ResponseWriter, status int, data loginPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := loginTemplate.Execute(w, data); err != nil {
		return
	}
}

// safeNextPath only accepts same-origin absolute paths, which keeps the login
// redirect from becoming an open redirect.
func safeNextPath(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	// Browsers normalise a backslash to "/" in URLs, so a value like
	// "/\evil.example" reaches the client as "//evil.example" and leaves the
	// host. Control characters are no part of a path either.
	if strings.ContainsAny(raw, "\\\r\n\t") {
		return "/"
	}
	if parsed, err := url.Parse(raw); err != nil || parsed.Host != "" {
		return "/"
	}
	return raw
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
