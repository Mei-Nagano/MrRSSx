package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
}

// APIProxyRequest is the payload sent by the frontend shim. Body is base64 so
// binary uploads survive the JSON round trip.
type APIProxyRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
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
	httpReq := httptest.NewRequest(method, "http://mrrss.local"+path, bodyReader)
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
