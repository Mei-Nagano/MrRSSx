package folo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	pathEntries     = "/entries"
	pathReads       = "/reads"
	pathCollections = "/collections"

	// maxEntriesPerPage is Folo's page size for timeline reads; the sync service
	// keeps its own, larger, page budget.
	maxEntriesPerPage = 100
)

// Entry is the part of a Folo timeline entry that read-state sync needs.
type Entry struct {
	ID          string
	URL         string
	PublishedAt string
	Read        bool
	Collected   bool
}

// EntryListParams narrows a timeline query.
type EntryListParams struct {
	Limit int
	// NewerThan and OlderThan are plain time bounds; ListEntries translates them
	// to Folo's date fields.
	NewerThan string
	OlderThan string
	// IsCollection filters to entries saved into the user's collection.
	IsCollection *bool
}

// timelineSortOrder is the only value the timeline endpoint accepts; every other
// spelling ("asc", "ASC", 1, ...) is rejected with 422.
const timelineSortOrder = "desc"

// apiEnvelope is Folo's response wrapper: code 0 carries data, anything else is
// an application-level failure that can arrive with HTTP 200.
type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// callJSON performs an authenticated JSON request and unwraps the envelope.
func (c *Client) callJSON(ctx context.Context, method, path string, body any, out any) error {
	if !c.HasSession() {
		return ErrNotAuthenticated
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Folo %s request: %w", path, err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := c.newRequest(ctx, method, path, nil, payload, "application/json")
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call Folo %s: %w", path, err)
	}
	defer resp.Body.Close()

	data, err := readCapped(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrNotAuthenticated
	}

	var envelope apiEnvelope
	decodeErr := json.Unmarshal(data, &envelope)
	if resp.StatusCode != http.StatusOK {
		detail := sanitizeMessage(data)
		if decodeErr == nil && envelope.Message != "" {
			detail = sanitizeMessage([]byte(envelope.Message))
		}
		return fmt.Errorf("Folo %s returned %s: %s", path, resp.Status, detail)
	}
	if decodeErr != nil {
		return fmt.Errorf("decode Folo %s response: %w", path, decodeErr)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("Folo %s failed with code %d: %s", path, envelope.Code, sanitizeMessage([]byte(envelope.Message)))
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode Folo %s data: %w", path, err)
	}
	return nil
}

// ListEntries reads one page of the account's timeline, newest first.
func (c *Client) ListEntries(ctx context.Context, params EntryListParams) ([]Entry, error) {
	limit := params.Limit
	if limit <= 0 || limit > maxEntriesPerPage {
		limit = maxEntriesPerPage
	}
	body := map[string]any{
		"limit":       limit,
		"sortOrder":   timelineSortOrder,
		"withContent": false,
	}
	// Folo's date filters are inverted relative to their names: publishedBefore
	// is the lower bound and publishedAfter the upper bound. Verified against the
	// live API; keep the translation here so callers can think in plain time order.
	if params.NewerThan != "" {
		body["publishedBefore"] = params.NewerThan
	}
	if params.OlderThan != "" {
		body["publishedAfter"] = params.OlderThan
	}
	if params.IsCollection != nil {
		body["isCollection"] = *params.IsCollection
	}

	var rows []struct {
		ID          string `json:"id"`
		Read        bool   `json:"read"`
		Collections *struct {
			CreatedAt string `json:"createdAt"`
		} `json:"collections"`
		Entries struct {
			ID          string  `json:"id"`
			URL         *string `json:"url"`
			PublishedAt string  `json:"publishedAt"`
		} `json:"entries"`
	}
	if err := c.callJSON(ctx, http.MethodPost, pathEntries, body, &rows); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		id := strings.TrimSpace(row.Entries.ID)
		if id == "" {
			id = strings.TrimSpace(row.ID)
		}
		if id == "" {
			continue
		}
		url := ""
		if row.Entries.URL != nil {
			url = strings.TrimSpace(*row.Entries.URL)
		}
		entries = append(entries, Entry{
			ID:          id,
			URL:         url,
			PublishedAt: row.Entries.PublishedAt,
			Read:        row.Read,
			Collected:   row.Collections != nil,
		})
	}
	return entries, nil
}

// MarkEntriesRead marks the given Folo entries read in one request.
func (c *Client) MarkEntriesRead(ctx context.Context, entryIDs []string) error {
	ids := make([]string, 0, len(entryIDs))
	for _, id := range entryIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return c.callJSON(ctx, http.MethodPost, pathReads, map[string]any{"entryIds": ids}, nil)
}

// MarkEntryUnread marks a single Folo entry unread; the API has no batch form.
func (c *Client) MarkEntryUnread(ctx context.Context, entryID string) error {
	if strings.TrimSpace(entryID) == "" {
		return nil
	}
	return c.callJSON(ctx, http.MethodDelete, pathReads, map[string]string{"entryId": entryID}, nil)
}

// AddToCollection saves a Folo entry, which is what MrRSS calls a favorite.
func (c *Client) AddToCollection(ctx context.Context, entryID string) error {
	if strings.TrimSpace(entryID) == "" {
		return nil
	}
	return c.callJSON(ctx, http.MethodPost, pathCollections, map[string]string{"entryId": entryID}, nil)
}

// RemoveFromCollection removes a saved Folo entry.
func (c *Client) RemoveFromCollection(ctx context.Context, entryID string) error {
	if strings.TrimSpace(entryID) == "" {
		return nil
	}
	return c.callJSON(ctx, http.MethodDelete, pathCollections, map[string]string{"entryId": entryID}, nil)
}
