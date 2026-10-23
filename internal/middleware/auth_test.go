package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testToken = "s3cret-token"

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected"))
	})
}

func TestRequireTokenDisabledWithoutToken(t *testing.T) {
	handler := RequireToken("", okHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without a configured token, got %d", rec.Code)
	}
}

func TestRequireTokenRedirectsBrowserRequests(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?filter=all", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/login?next=") {
		t.Fatalf("expected a login redirect, got %q", location)
	}
	decoded, err := url.QueryUnescape(strings.TrimPrefix(location, "/login?next="))
	if err != nil {
		t.Fatalf("decode redirect target: %v", err)
	}
	if decoded != "/?filter=all" {
		t.Fatalf("expected the original path in the redirect, got %q", decoded)
	}
}

func TestRequireTokenRejectsAPIRequests(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feeds", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated API calls, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "authentication required") {
		t.Fatalf("expected a JSON error body, got %q", rec.Body.String())
	}
}

func TestRequireTokenAcceptsValidCookie(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: testToken})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with a valid cookie, got %d", rec.Code)
	}
}

func TestRequireTokenRejectsWrongCookie(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/feeds", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: "wrong"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with a wrong cookie, got %d", rec.Code)
	}
}

func TestRequireTokenAllowsPublicAssets(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	for _, path := range []string{"/manifest.webmanifest", "/sw.js", "/icons/icon-192.png", "/assets/app.js"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s to load without a token, got %d", path, rec.Code)
		}
	}
}

func TestLoginPageRenders(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Access token") {
		t.Fatalf("expected the login form, got %q", rec.Body.String())
	}
}

func TestLoginWithCorrectTokenSetsCookie(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	form := url.Values{"token": {testToken}, "next": {"/?filter=unread"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/?filter=unread" {
		t.Fatalf("expected the original destination, got %q", location)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != AuthCookieName || cookies[0].Value != testToken {
		t.Fatalf("expected the access cookie to be set, got %+v", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("expected the access cookie to be HttpOnly")
	}
}

func TestLoginWithWrongTokenIsRejected(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	form := url.Values{"token": {"nope"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("expected no cookie for a failed login")
	}
}

func TestLoginRejectsOffsiteRedirect(t *testing.T) {
	handler := RequireToken(testToken, okHandler())

	form := url.Values{"token": {testToken}, "next": {"//evil.example/steal"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if location := rec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected the redirect to stay on this host, got %q", location)
	}
}

func TestSafeNextPath(t *testing.T) {
	cases := map[string]string{
		"":                  "/",
		"/":                 "/",
		"/?filter=all":      "/?filter=all",
		"//evil.example":    "/",
		"https://evil.test": "/",
		"relative/path":     "/",
		// Browsers turn the backslash into "/" and follow "//evil.example".
		"/\\evil.example":     "/",
		"/\\/evil.example":    "/",
		"/path/with\\slash":   "/",
		"/path\nwith-newline": "/",
	}
	for input, want := range cases {
		if got := safeNextPath(input); got != want {
			t.Errorf("safeNextPath(%q) = %q, want %q", input, got, want)
		}
	}
}
