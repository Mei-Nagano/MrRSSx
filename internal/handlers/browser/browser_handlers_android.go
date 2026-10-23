//go:build android

// Package browser provides HTTP handlers for browser-related operations on
// Android, where links open through the system Intent chooser.
package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/wailsapp/wails/v3/pkg/application"

	handlers "MrRSS/internal/handlers/core"
	"MrRSS/internal/handlers/response"
)

// HandleOpenURL opens a URL in the system browser via an Android VIEW intent.
// The desktop build uses the Wails browser manager, which shells out to a
// platform opener that does not exist on Android.
// @Summary      Open URL in browser
// @Description  Open a URL in the user's default web browser
// @Tags         browser
// @Accept       json
// @Produce      json
// @Param        url       query     string  false  "URL to open (for GET requests)"
// @Param        request  body      object  true  "Open URL request (url) (for POST requests)"
// @Success      200  {object}  map[string]string  "Success status (status)"
// @Failure      400  {object}  map[string]string  "Bad request (invalid URL)"
// @Router       /browser/open [post]
// @Router       /browser/open [get]
func HandleOpenURL(h *handlers.Handler, w http.ResponseWriter, r *http.Request) {
	var targetURL string

	switch r.Method {
	case http.MethodGet:
		targetURL = r.URL.Query().Get("url")
	case http.MethodPost:
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, err, http.StatusBadRequest)
			return
		}
		targetURL = req.URL
	default:
		response.Error(w, nil, http.StatusMethodNotAllowed)
		return
	}

	if targetURL == "" {
		response.Error(w, fmt.Errorf("URL is required"), http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		response.Error(w, fmt.Errorf("invalid URL format"), http.StatusBadRequest)
		return
	}
	// Only allow http and https schemes for security
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		response.Error(w, fmt.Errorf("only HTTP and HTTPS URLs are allowed"), http.StatusBadRequest)
		return
	}

	application.Android.OpenURL(targetURL)
	log.Printf("Opened URL in system browser: %s", targetURL)

	response.JSON(w, map[string]string{"status": "success"})
}
