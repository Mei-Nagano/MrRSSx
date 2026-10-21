// Package folo exposes the Folo integration over HTTP. It is available in both
// desktop and server builds: the browser login hands its token back to a
// loopback listener owned by the Go process, and a pasted credential covers the
// headless case where no browser can reach this machine.
package folo

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"MrRSS/internal/folo"
	"MrRSS/internal/handlers/core"
	opmlhandler "MrRSS/internal/handlers/opml"
	"MrRSS/internal/handlers/response"
	opmlparser "MrRSS/internal/opml"
)

// The session token and the import timestamp are written by the backend only.
// They stay out of the settings schema on purpose: the settings autosave posts
// every schema key it knows about, so a backend-owned secret stored there would
// be overwritten by the stale value the UI loaded before the login completed.
const (
	sessionTokenKey = "folo_session_token"
	lastImportKey   = "folo_last_import_time"
)

// Stable codes the frontend translates. Neither a Folo response body nor a token
// is ever echoed into them.
const (
	codeInvalidRequest   = "invalid_request"
	codeNotAuthenticated = "not_authenticated"
	codeLoginFailed      = "login_failed"
	codeImportFailed     = "import_failed"
	codeSyncFailed       = "sync_failed"
	codeSyncBusy         = "sync_busy"
)

// syncTimeout bounds one reconciliation pass; the syncer also caps how many
// entries and pages it will fetch.
const syncTimeout = 2 * time.Minute

func writeFailure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "code": code})
}

func writeError(w http.ResponseWriter, err error, code string) {
	if errors.Is(err, folo.ErrNotAuthenticated) {
		writeFailure(w, http.StatusUnauthorized, codeNotAuthenticated)
		return
	}
	log.Printf("folo: %v", err)
	writeFailure(w, http.StatusBadGateway, code)
}

type integrationConfig struct {
	apiURL     string
	webURL     string
	folderMode string
	token      string
}

// loadConfig reads the integration settings, falling back to the hosted Folo
// endpoints. h.DB doubles as the proxy-settings provider so every Folo request
// honours the application's global proxy.
func loadConfig(h *core.Handler) (integrationConfig, error) {
	cfg := integrationConfig{
		apiURL:     folo.DefaultAPIURL,
		webURL:     folo.DefaultWebURL,
		folderMode: "category",
	}
	read := func(key string, into *string) {
		value, err := h.DB.GetSetting(key)
		if err != nil {
			log.Printf("folo: read setting %s: %v", key, err)
			return
		}
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			*into = trimmed
		}
	}
	read("folo_api_url", &cfg.apiURL)
	read("folo_web_url", &cfg.webURL)
	read("folo_folder_mode", &cfg.folderMode)

	token, err := h.DB.GetEncryptedSetting(sessionTokenKey)
	switch {
	case err == nil:
		cfg.token = strings.TrimSpace(token)
	case errors.Is(err, sql.ErrNoRows):
		// The token lives outside the seeded settings keys, so a signed-out app
		// simply has no row yet.
	default:
		return cfg, err
	}
	return cfg, nil
}

func (c integrationConfig) client(h *core.Handler) (*folo.Client, error) {
	return folo.NewClient(c.apiURL, c.token, h.DB)
}

func (c integrationConfig) clientWithToken(h *core.Handler, token string) (*folo.Client, error) {
	return folo.NewClient(c.apiURL, token, h.DB)
}

// HandleAccount reports the signed-in Folo account.
//
//	@Summary		Folo account status
//	@Description	Return whether a Folo session is active and which account it belongs to
//	@Tags			folo
//	@Produce		json
//	@Success		200		{object}	object{connected=bool,name=string,email=string}	"Folo account"
//	@Success		401		{object}	object{success=bool,code=string}					"Not signed in"
//	@Router			/folo/account [get]
func HandleAccount(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}

	cfg, err := loadConfig(h)
	if err != nil {
		writeError(w, err, codeImportFailed)
		return
	}
	if cfg.token == "" {
		writeFailure(w, http.StatusUnauthorized, codeNotAuthenticated)
		return
	}
	client, err := cfg.client(h)
	if err != nil {
		writeError(w, err, codeImportFailed)
		return
	}

	user, err := client.Whoami(r.Context())
	if err != nil {
		writeError(w, err, codeNotAuthenticated)
		return
	}
	lastImport, _ := h.DB.GetSetting(lastImportKey)
	lastSync, _ := h.DB.GetSetting(folo.LastSyncKey)
	mapped, _ := h.DB.CountFoloMappings()

	response.JSON(w, map[string]interface{}{
		"connected":        true,
		"name":             user.Name,
		"email":            user.Email,
		"image":            user.Image,
		"last_import_time": lastImport,
		"last_sync_time":   lastSync,
		"mapped_count":     mapped,
		"folder_mode":      cfg.folderMode,
	})
}

// HandleLoginStart opens a loopback callback and returns Folo's sign-in URL.
//
//	@Summary		Start Folo browser sign-in
//	@Description	Create a short-lived loopback listener and return the Folo login URL that hands its token back to it
//	@Tags			folo
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	object{success=bool,login_url=string}	"Sign-in can begin"
//	@Failure		502	{object}	object{success=bool,code=string}		"Could not start sign-in"
//	@Router			/folo/login/start [post]
func HandleLoginStart(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}

	cfg, err := loadConfig(h)
	if err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}
	pending, err := folo.StartLogin(cfg.apiURL, cfg.webURL, h.DB, func(ctx context.Context, sessionToken string) error {
		return h.DB.SetEncryptedSetting(sessionTokenKey, sessionToken)
	})
	if err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}

	response.JSON(w, map[string]interface{}{
		"success":    true,
		"login_url":  pending.LoginURL,
		"expires_at": pending.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// HandleLoginStatus reports the in-flight sign-in attempt.
//
//	@Summary		Folo sign-in status
//	@Description	Return the state of the browser sign-in attempt so the UI can stop waiting on failure
//	@Tags			folo
//	@Produce		json
//	@Success		200	{object}	object{state=string}	"Sign-in state"
//	@Router			/folo/login/status [get]
func HandleLoginStatus(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}
	response.JSON(w, map[string]interface{}{"state": folo.ActiveLoginStatus().State})
}

// HandleLoginComplete stores a credential pasted by the user, which is the only
// sign-in path when no browser can reach this machine (remote server mode).
//
//	@Summary		Complete Folo sign-in manually
//	@Description	Exchange a one-time token or accept a pasted Folo session token and store it encrypted
//	@Tags			folo
//	@Accept			json
//	@Produce		json
//	@Param			request	body		object{credential=string}	true	"One-time token or session token"
//	@Success		200		{object}	object{success=bool,name=string,email=string}
//	@Failure		502		{object}	object{success=bool,code=string}
//	@Router			/folo/login/complete [post]
func HandleLoginComplete(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Credential string `json:"credential"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, codeInvalidRequest)
		return
	}
	if strings.TrimSpace(req.Credential) == "" {
		writeFailure(w, http.StatusBadRequest, codeInvalidRequest)
		return
	}

	cfg, err := loadConfig(h)
	if err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}
	sessionToken, err := folo.ExchangeLoginToken(r.Context(), cfg.apiURL, req.Credential, h.DB)
	if err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}
	if err := h.DB.SetEncryptedSetting(sessionTokenKey, sessionToken); err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}

	client, err := cfg.clientWithToken(h, sessionToken)
	if err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}
	user, err := client.Whoami(r.Context())
	if err != nil {
		// The exchanged token does not hold a session, so do not keep it.
		if clearErr := h.DB.SetEncryptedSetting(sessionTokenKey, ""); clearErr != nil {
			log.Printf("folo: clear unusable session token: %v", clearErr)
		}
		writeError(w, err, codeNotAuthenticated)
		return
	}

	response.JSON(w, map[string]interface{}{
		"success": true,
		"name":    user.Name,
		"email":   user.Email,
	})
}

// HandleLogout forgets the stored session token.
//
//	@Summary		Sign out of Folo
//	@Description	Clear the stored Folo session token; imported subscriptions stay local
//	@Tags			folo
//	@Produce		json
//	@Success		200	{object}	object{success=bool}	"Signed out"
//	@Router			/folo/logout [post]
func HandleLogout(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}
	folo.CancelActiveLogin()
	if err := h.DB.SetEncryptedSetting(sessionTokenKey, ""); err != nil {
		writeError(w, err, codeLoginFailed)
		return
	}
	// A later sign-in may reach a different account, so mappings taken against
	// this session's entries must not be reused.
	if _, err := h.DB.ClearFoloMappings(); err != nil {
		log.Printf("folo: clear mappings on sign-out: %v", err)
	}
	for _, key := range []string{folo.SyncCursorKey, folo.SyncTopKey, folo.SyncBackfillKey, folo.LastSyncKey, lastImportKey} {
		if err := h.DB.SetSetting(key, ""); err != nil {
			log.Printf("folo: clear %s on sign-out: %v", key, err)
		}
	}
	response.JSON(w, map[string]interface{}{"success": true})
}

// HandleImport pulls the account's subscriptions and adds them as local feeds.
//
//	@Summary		Import Folo subscriptions
//	@Description	Download the Folo subscription list as OPML and add it as ordinary local subscriptions
//	@Tags			folo
//	@Produce		json
//	@Success		200	{object}	object{success=bool,imported=int,total=int}	"Import finished"
//	@Failure		401	{object}	object{success=bool,code=string}				"Not signed in"
//	@Router			/folo/import [post]
func HandleImport(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}

	cfg, err := loadConfig(h)
	if err != nil {
		writeError(w, err, codeImportFailed)
		return
	}
	client, err := cfg.client(h)
	if err != nil {
		writeError(w, err, codeImportFailed)
		return
	}

	document, err := client.ExportOPML(r.Context(), cfg.folderMode, rsshubEndpoint(h))
	if err != nil {
		writeError(w, err, codeImportFailed)
		return
	}
	feeds, err := opmlparser.Parse(bytes.NewReader(document))
	if err != nil {
		writeError(w, err, codeImportFailed)
		return
	}

	imported := opmlhandler.ImportFeeds(r.Context(), h, feeds)
	if err := h.DB.SetSetting(lastImportKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
		log.Printf("folo: record import time: %v", err)
	}

	response.JSON(w, map[string]interface{}{
		"success":  true,
		"imported": len(imported),
		"total":    len(feeds),
	})
}

// syncRunning guards a single in-flight state sync: two passes would fight over
// the same snapshots and could push each other's intermediate state.
var syncRunning atomic.Bool

type syncResponse struct {
	Success bool `json:"success"`
	folo.SyncResult
}

// HandleSync reconciles read and favorite state with the signed-in account.
//
//	@Summary		Sync read and favorite state with Folo
//	@Description	Push local read/favorite changes and apply Folo changes to matched local articles
//	@Tags			folo
//	@Produce		json
//	@Success		200	{object}	object{success=bool,pushed=int,pulled=int,matched=int,skipped=int,mapped=int}	"Sync finished"
//	@Failure		401	{object}	object{success=bool,code=string}													"Not signed in"
//	@Failure		409	{object}	object{success=bool,code=string}													"Sync already running"
//	@Failure		502	{object}	object{success=bool,code=string}													"Sync failed"
//	@Router			/folo/sync [post]
func HandleSync(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}

	cfg, err := loadConfig(h)
	if err != nil {
		writeError(w, err, codeSyncFailed)
		return
	}
	client, err := cfg.client(h)
	if err != nil {
		writeError(w, err, codeSyncFailed)
		return
	}
	if !client.HasSession() {
		writeFailure(w, http.StatusUnauthorized, codeNotAuthenticated)
		return
	}
	if !syncRunning.CompareAndSwap(false, true) {
		writeFailure(w, http.StatusConflict, codeSyncBusy)
		return
	}
	defer syncRunning.Store(false)

	ctx, cancel := context.WithTimeout(r.Context(), syncTimeout)
	defer cancel()

	result, err := folo.NewSyncer(client, h.DB).Sync(ctx)
	if err != nil {
		writeError(w, err, codeSyncFailed)
		return
	}
	response.JSON(w, syncResponse{Success: true, SyncResult: result})
}

// rsshubEndpoint lets Folo's rsshub:// subscriptions resolve against the user's
// own instance when one is configured, instead of the public relay.
func rsshubEndpoint(h *core.Handler) string {
	enabled, err := h.DB.GetSetting("rsshub_enabled")
	if err != nil || enabled != "true" {
		return ""
	}
	endpoint, err := h.DB.GetSetting("rsshub_endpoint")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(endpoint)
}
