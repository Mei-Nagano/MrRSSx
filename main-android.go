//go:build android

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"MrRSS/internal/ai"
	"MrRSS/internal/database"
	"MrRSS/internal/feed"
	handlers "MrRSS/internal/handlers/core"
	"MrRSS/internal/network"
	"MrRSS/internal/routes"
	"MrRSS/internal/translation"
	"MrRSS/internal/utils/fileutil"
)

// mobileDataDir returns the app-private directory the platform provides for
// persistent files. Android hands out a path that always exists and is wiped
// only when the user clears the app data or uninstalls it.
func mobileDataDir() string {
	return application.Android.StoragePath()
}

// main runs inside the Android process after the native bridge is initialised.
// Desktop-only concerns (tray, single instance, startup registration, window
// state, the loopback integration API) have no meaning here and are skipped.
func main() {
	// Route every data file into the app sandbox before anything opens a path.
	if dir := mobileDataDir(); dir != "" {
		if err := fileutil.ConfigureDataDir(dir); err != nil {
			log.Printf("Warning: could not use the app data directory %q: %v", dir, err)
		}
	} else {
		log.Println("Warning: the platform did not report an app data directory")
	}

	logPath, err := fileutil.GetLogPath()
	if err != nil {
		log.Printf("Warning: could not get log path: %v", err)
	} else if f, openErr := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666); openErr == nil {
		// Keep stdout for logcat and mirror into the data directory for support.
		log.SetOutput(io.MultiWriter(os.Stdout, f))
		// The Go runtime writes fatal-signal tracebacks straight to fd 2, which
		// Android points at /dev/null for app processes. Redirect it to the log
		// so a native crash leaves something readable behind.
		_ = syscall.Dup3(int(f.Fd()), 2, 0)
	}

	log.Println("Starting application in Android mode...")

	dbPath, err := fileutil.GetDBPath()
	if err != nil {
		log.Fatalf("resolve database path: %v", err)
	}
	debugLog("Database path: %s", dbPath)

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := db.Init(); err != nil {
		log.Fatalf("initialise database schema: %v", err)
	}
	log.Println("Database initialized successfully")

	profileProvider := ai.NewProfileProvider(db)
	translator := translation.NewDynamicTranslatorWithCache(db, db)
	translator.SetProfileProvider(profileProvider)

	fetcher := feed.NewFetcher(db)
	h := handlers.NewHandler(db, fetcher, translator, profileProvider)
	configureAdvertisements(h)

	apiMux := http.NewServeMux()
	routes.RegisterAPIRoutes(apiMux, h)

	frontendFS, err := fs.Sub(frontendFiles, "frontend/dist")
	if err != nil {
		log.Fatalf("read embedded frontend: %v", err)
	}

	combinedHandler := &CombinedHandler{
		apiMux:     apiMux,
		fileServer: newFrontendFileServer(frontendFS),
	}

	// Match the desktop background so the first paint does not flash white.
	backgroundColour := application.NewRGB(30, 30, 30)
	if theme, err := db.GetSetting("theme"); err == nil && theme == "light" {
		backgroundColour = application.NewRGB(255, 255, 255)
	}

	app := application.New(application.Options{
		Name:        "MrRSS",
		Description: "A modern, privacy-focused RSS reader",
		LogLevel:    slog.LevelError,
		Assets: application.AssetOptions{
			Handler:    combinedHandler,
			Middleware: APIMiddleware(combinedHandler),
		},
	})

	h.SetApp(app)

	// The WebView cannot hand request bodies to the asset loader, so the
	// frontend routes /api/ calls through this service instead (see api_proxy.go
	// and frontend/src/utils/apiBridge.ts).
	app.RegisterService(application.NewService(&APIProxy{mux: apiMux}))

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "MrRSS-main-window",
		Title:            "MrRSS",
		URL:              "/",
		BackgroundColour: backgroundColour,
	})

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	go h.StartBackgroundScheduler(bgCtx)

	// Android is free to kill a backgrounded process, which would stop the
	// scheduler above. A foreground service — the one Wails ships — keeps the
	// process alive while the user is elsewhere, at the cost of an ongoing
	// notification. It is raised only while the app is actually in the
	// background and only when automatic refresh is configured, so a user who
	// turned refresh off never sees the notification.
	go watchBackgroundRefresh(app, db)

	go func() {
		detector := network.NewDetector(&http.Client{Timeout: 10 * time.Second})
		detectCtx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
		defer cancel()

		result := detector.DetectSpeed(detectCtx)
		if result.DetectionSuccess {
			db.SetSetting("network_speed", string(result.SpeedLevel))
			db.SetSetting("network_bandwidth_mbps", fmt.Sprintf("%.2f", result.BandwidthMbps))
		}
	}()

	log.Println("Starting Wails on Android...")
	if err := app.Run(); err != nil {
		log.Printf("Application stopped: %v", err)
	}

	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
}

// backgroundRefreshNotification returns the notification title and text in the
// language the reader is using, so the ongoing notification does not switch
// language relative to the interface.
func backgroundRefreshNotification(db *database.DB) (string, string) {
	if language, err := db.GetSetting("language"); err == nil && strings.HasPrefix(language, "zh") {
		return "MrRSS", "正在后台刷新订阅"
	}
	return "MrRSS", "Refreshing feeds in the background"
}

// watchBackgroundRefresh raises a foreground service while the app is in the
// background so the scheduler keeps running, and drops it when the app comes
// back. Modes that never refresh on their own do not get the service at all:
// keeping a process alive for no work would drain the battery for nothing.
//
// Starting and stopping are both idempotent on the platform side (a second
// start reaches the same service, stopping an absent one is a no-op), so this
// does not track state. That matters because the start can be deferred: the
// service may wait for the notification permission, and a guessed "running"
// flag would then disagree with the platform.
func watchBackgroundRefresh(app *application.App, db *database.DB) {
	if app == nil {
		return
	}

	refreshEnabled := func() bool {
		mode, err := db.GetSetting("refresh_mode")
		if err != nil || mode == "" {
			return true // default scheduler mode
		}
		return mode != "never"
	}

	app.Event.OnApplicationEvent(events.Android.ActivityStopped, func(*application.ApplicationEvent) {
		if !refreshEnabled() {
			return
		}
		title, text := backgroundRefreshNotification(db)
		payload, err := json.Marshal(map[string]string{"title": title, "text": text})
		if err != nil {
			log.Printf("Background refresh: could not encode notification: %v", err)
			return
		}
		application.Android.StartForegroundService(string(payload))
		log.Println("Background refresh: keeping the process alive while backgrounded")
	})
	app.Event.OnApplicationEvent(events.Android.ActivityStarted, func(*application.ApplicationEvent) {
		application.Android.StopForegroundService()
	})
}
