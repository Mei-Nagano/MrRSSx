package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// debugLogging enables verbose logging when MRRSS_DEBUG is set.
var debugLogging = os.Getenv("MRRSS_DEBUG") != ""

func debugLog(format string, args ...interface{}) {
	if debugLogging {
		log.Printf(format, args...)
	}
}

//go:embed frontend/dist
var frontendFiles embed.FS

// indexRewrite strips a trailing "index.html" before handing the request to the
// file server.
//
// net/http's file server answers "/index.html" with a 301 redirect to "./". That
// is harmless in a browser, but the Android host cannot follow it: Wails'
// WailsPathHandler rewrites the initial "/" navigation to "/index.html" and
// treats every non-200 reply as a missing asset, so the redirect turns into a
// blank WebView that falls back to resolving wails.localhost over the network
// ("无法连接"). Serving the file directly keeps the same bytes without the
// redirect.
type indexRewrite struct {
	next http.Handler
}

func (h indexRewrite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/index.html") {
		clone := r.Clone(r.Context())
		clone.URL.Path = strings.TrimSuffix(r.URL.Path, "index.html")
		if clone.URL.Path == "" {
			clone.URL.Path = "/"
		}
		h.next.ServeHTTP(w, clone)
		return
	}
	h.next.ServeHTTP(w, r)
}

// newFrontendFileServer serves the embedded reader interface, avoiding the
// "/index.html" canonical redirect described on indexRewrite.
func newFrontendFileServer(fsys fs.FS) http.Handler {
	return indexRewrite{next: http.FileServer(http.FS(fsys))}
}

// CombinedHandler serves the REST API under /api/ and the embedded reader
// interface for every other path. The desktop, mobile and server builds all
// serve the same frontend through it.
type CombinedHandler struct {
	apiMux     *http.ServeMux
	fileServer http.Handler
}

func (h *CombinedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.apiMux.ServeHTTP(w, r)
		return
	}
	h.fileServer.ServeHTTP(w, r)
}

// APIMiddleware routes API requests to the API handler, and lets Wails handle the rest
func APIMiddleware(combinedHandler *CombinedHandler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Let the /wails route be handled by Wails runtime
			if strings.HasPrefix(r.URL.Path, "/wails") {
				next.ServeHTTP(w, r)
				return
			}
			// Handle API routes and serve static files
			combinedHandler.ServeHTTP(w, r)
		})
	}
}
