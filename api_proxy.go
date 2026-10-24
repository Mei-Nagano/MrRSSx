package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// APIProxy replays REST calls coming from the mobile WebView against the same
// http.ServeMux the desktop and server builds use.
//
// It exists because of a hard WebView limitation: when a request is served
// through the Wails asset loader (shouldInterceptRequest), the request body is
// not delivered to Go, and query strings are stripped on the way. Every write
// endpoint of the reader would arrive empty. The frontend therefore hands the
// full request to this service over the Wails runtime transport, which does
// carry the payload, and gets the response back as data.
type APIProxy struct {
	mux *http.ServeMux

	// Requests are cancellable through request_id. The frontend passes an id with
	// each call and cancels it when its AbortSignal fires, so an abandoned request
	// stops inside Go instead of finishing and doing further upstream work.
	mu        sync.Mutex
	cancels   map[string]context.CancelFunc
	cancelled map[string]struct{}
}

// APIProxyRequest is the payload sent by the frontend shim. Body is base64 so
// binary uploads survive the JSON round trip.
type APIProxyRequest struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	RequestID string            `json:"request_id"`
}

// APIProxyResponse mirrors the pieces of an HTTP response the frontend needs.
type APIProxyResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// hopByHopHeaders are not meaningful to a fetch() Response and can make the
// browser reject the constructed response.
var hopByHopHeaders = map[string]bool{
	"content-length":    true,
	"transfer-encoding": true,
	"connection":        true,
	"keep-alive":        true,
	"trailer":           true,
	"upgrade":           true,
}

// Do executes the request against the API mux and returns the response.
func (p *APIProxy) Do(req APIProxyRequest) (*APIProxyResponse, error) {
	if p == nil || p.mux == nil {
		return nil, fmt.Errorf("api proxy is not configured")
	}

	path := req.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Only the reader's own API is reachable; this is not a general proxy.
	if !strings.HasPrefix(path, "/api/") {
		return nil, fmt.Errorf("refusing to proxy %q: only /api/ paths are allowed", path)
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	var body []byte
	if req.Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.Body)
		if err != nil {
			return nil, fmt.Errorf("decode request body: %w", err)
		}
		body = decoded
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	ctx, release, err := p.beginRequest(req.RequestID)
	if err != nil {
		return nil, err
	}
	defer release()

	httpReq := httptest.NewRequest(method, "http://mrrss.local"+path, bodyReader)
	httpReq = httpReq.WithContext(ctx)
	for name, value := range req.Headers {
		if hopByHopHeaders[strings.ToLower(name)] {
			continue
		}
		httpReq.Header.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	p.mux.ServeHTTP(recorder, httpReq)

	result := recorder.Result()
	defer result.Body.Close()

	headers := make(map[string]string, len(result.Header))
	for name, values := range result.Header {
		if hopByHopHeaders[strings.ToLower(name)] {
			continue
		}
		headers[name] = strings.Join(values, ", ")
	}

	return &APIProxyResponse{
		Status:  result.StatusCode,
		Headers: headers,
		Body:    base64.StdEncoding.EncodeToString(recorder.Body.Bytes()),
	}, nil
}

// errAPIProxyCancelled reports that the frontend abandoned the request before it
// reached the mux.
var errAPIProxyCancelled = errors.New("the request was cancelled")

// maxPendingCancels bounds the ids remembered for cancels that arrived before
// their request. The intent only matters for milliseconds, so dropping the whole
// set when it grows is safe.
const maxPendingCancels = 256

// beginRequest registers a cancellable context for id. An empty id keeps the
// old behaviour so non-cancellable callers are unaffected.
func (p *APIProxy) beginRequest(id string) (context.Context, func(), error) {
	if id == "" {
		return context.Background(), func() {}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	if p.cancels == nil {
		p.cancels = map[string]context.CancelFunc{}
	}
	if p.cancelled == nil {
		p.cancelled = map[string]struct{}{}
	}
	_, alreadyCancelled := p.cancelled[id]
	delete(p.cancelled, id)
	p.cancels[id] = cancel
	p.mu.Unlock()

	if alreadyCancelled {
		cancel()
		p.mu.Lock()
		delete(p.cancels, id)
		p.mu.Unlock()
		return nil, nil, errAPIProxyCancelled
	}

	release := func() {
		p.mu.Lock()
		delete(p.cancels, id)
		p.mu.Unlock()
		cancel()
	}
	return ctx, release, nil
}

// Cancel aborts an in-flight request. Calling it before Do registers the id is
// allowed: the intent is remembered so the request never starts.
func (p *APIProxy) Cancel(requestID string) error {
	if requestID == "" {
		return fmt.Errorf("cancel needs a request id")
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if cancel, ok := p.cancels[requestID]; ok {
		cancel()
		return nil
	}
	if p.cancelled == nil || len(p.cancelled) >= maxPendingCancels {
		p.cancelled = map[string]struct{}{}
	}
	p.cancelled[requestID] = struct{}{}
	return nil
}
