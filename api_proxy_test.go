package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestAPIProxyReplaysMethodBodyAndQuery(t *testing.T) {
	var gotMethod, gotQuery, gotContentType string
	var gotBody []byte

	mux := http.NewServeMux()
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "999") // hop-by-hop-ish, must be dropped
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	})

	proxy := &APIProxy{mux: mux}
	res, err := proxy.Do(APIProxyRequest{
		Method:  http.MethodPost,
		Path:    "/api/echo?limit=20&unread=true",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    base64.StdEncoding.EncodeToString([]byte(`{"title":"你好"}`)),
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotQuery != "limit=20&unread=true" {
		t.Errorf("query = %q, want limit=20&unread=true", gotQuery)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q", gotContentType)
	}
	if string(gotBody) != `{"title":"你好"}` {
		t.Errorf("body = %q, want the JSON payload", string(gotBody))
	}
	if res.Status != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.Status)
	}
	if _, ok := res.Headers["Content-Length"]; ok {
		t.Errorf("Content-Length should be stripped, got %v", res.Headers)
	}

	decoded, err := base64.StdEncoding.DecodeString(res.Body)
	if err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if string(decoded) != "{\"ok\":\"yes\"}\n" {
		t.Errorf("response body = %q", string(decoded))
	}
}

func TestAPIProxyRejectsForeignPaths(t *testing.T) {
	proxy := &APIProxy{mux: http.NewServeMux()}
	if _, err := proxy.Do(APIProxyRequest{Method: http.MethodGet, Path: "/etc/passwd"}); err == nil {
		t.Fatal("expected an error for a non-/api/ path")
	}
}
