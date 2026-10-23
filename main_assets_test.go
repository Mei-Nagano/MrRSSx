package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAssetProbe verifies that the handler handed to Wails serves the embedded
// frontend for the exact paths the Android WebViewAssetLoader asks for.
//
// The Android host rewrites the initial "/" navigation to "/index.html" and
// rejects any non-200 reply, so every path below must come back as 200.
func TestAssetProbe(t *testing.T) {
	sub, err := fs.Sub(frontendFiles, "frontend/dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	h := &CombinedHandler{
		apiMux:     http.NewServeMux(),
		fileServer: newFrontendFileServer(sub),
	}

	paths := []string{"/", "/index.html", "/manifest.webmanifest", "/sw.js"}
	// A real hashed asset, as referenced from index.html.
	entries, err := fs.ReadDir(sub, "assets")
	if err != nil {
		t.Fatalf("read assets dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			paths = append(paths, "/assets/"+e.Name())
			break
		}
	}

	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, "https://wails.localhost"+p, nil)
		req.Body = http.NoBody
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %-28s -> %d (want 200)", p, rec.Code)
			continue
		}
		t.Logf("GET %-28s -> %d  %6d bytes  ct=%q", p, rec.Code, rec.Body.Len(), rec.Header().Get("Content-Type"))
	}
}
