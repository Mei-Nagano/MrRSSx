//go:build server

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"MrRSS/internal/ai"
	"MrRSS/internal/database"
	"MrRSS/internal/feed"
	handlers "MrRSS/internal/handlers/core"
	"MrRSS/internal/middleware"
	"MrRSS/internal/network"
	"MrRSS/internal/routes"
	"MrRSS/internal/translation"
	"MrRSS/internal/utils/fileutil"

	httpSwagger "github.com/swaggo/http-swagger"
)

// @title           MrRSS API
// @version         1.3.41
// @description     MrRSS is a modern, cross-platform desktop RSS reader with auto-translation, smart feed discovery, and AI-powered summarization.

// @contact.name   API Support
// @contact.url    https://github.com/DevXDojojo/MrRSS
// @contact.email  mail@ch3nyang.top

// @license.name  GPL-3.0
// @license.url   https://www.gnu.org/licenses/gpl-3.0.en.html

// @host      localhost:1234
// @BasePath  /api

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// isLoopbackHost reports whether the listen address is only reachable from this
// machine, which decides whether running without an access token is safe.
func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func main() {
	// Parse flags
	flag.BoolFunc("server", "Run in headless server mode", func(s string) error {
		v, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		fileutil.SetServerMode(v)
		return nil
	})
	host := flag.String("host", "0.0.0.0", "Host to listen on in server mode")
	port := flag.String("port", "1234", "Port to listen on in server mode")
	dataDirOption := flag.String("data-dir", "", "Data directory (defaults to MRRSS_DATA_DIR or ./data)")
	authToken := flag.String("auth-token", "", "Require this access token for browser access (defaults to MRRSS_AUTH_TOKEN)")
	flag.Parse()
	if err := fileutil.ConfigureDataDir(*dataDirOption); err != nil {
		log.Fatal(err)
	}

	// Force server mode for this build
	fileutil.SetServerMode(true)

	// Get proper paths for data files
	logPath, err := fileutil.GetLogPath()
	if err != nil {
		log.Printf("Warning: Could not get log path: %v. Using current directory.", err)
		logPath = "debug.log"
	}

	// In server mode, log to both stdout and file
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.SetOutput(os.Stdout) // Fallback
	} else {
		// Note: we don't close f here as it needs to stay open for logging
		// It will be closed by OS on process exit
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	}

	log.Println("Starting application in server mode...")

	// Log portable mode status
	if fileutil.IsPortableMode() {
		log.Println("Running in PORTABLE mode")
	} else {
		log.Println("Running in NORMAL mode")
	}

	log.Printf("Log file: %s", logPath)

	// Get database path
	dbPath, err := fileutil.GetDBPath()
	if err != nil {
		log.Printf("Error getting database path: %v", err)
		log.Fatal(err)
	}
	debugLog("Database path: %s", dbPath)

	// Initialize database
	log.Println("Initializing Database...")
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Printf("Error initializing database: %v", err)
		log.Fatal(err)
	}

	// Run database schema initialization synchronously to ensure it's ready
	log.Println("Running DB migrations...")
	if err := db.Init(); err != nil {
		log.Printf("Error initializing database schema: %v", err)
		log.Fatal(err)
	}
	log.Println("Database initialized successfully")

	// Initialize AI profile provider
	profileProvider := ai.NewProfileProvider(db)
	translator := translation.NewDynamicTranslatorWithCache(db, db)
	translator.SetProfileProvider(profileProvider)

	fetcher := feed.NewFetcher(db)
	h := handlers.NewHandler(db, fetcher, translator, profileProvider)
	configureAdvertisements(h)

	// API Routes
	log.Println("Setting up API routes...")
	apiMux := http.NewServeMux()
	routes.RegisterAPIRoutes(apiMux, h)

	// Swagger Documentation - Serve swagger.json file
	apiMux.HandleFunc("/docs/SERVER_MODE/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/SERVER_MODE/swagger.json")
	})

	apiMux.HandleFunc("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/docs/SERVER_MODE/swagger.json"),
	))

	// Static Files
	log.Println("Setting up static files...")
	frontendFS, err := fs.Sub(frontendFiles, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	// Browsers install the PWA using this manifest; serve it with the
	// registered media type instead of the generic text/plain fallback.
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		log.Printf("Warning: could not register the web manifest media type: %v", err)
	}

	fileServer := newFrontendFileServer(frontendFS)

	combinedHandler := &CombinedHandler{
		apiMux:     apiMux,
		fileServer: fileServer,
	}

	log.Printf("Starting in headless server mode on http://%s:%s", *host, *port)

	// Access control. A token protects every reader route and API call; without
	// one the server keeps its previous "trusted network" behaviour.
	accessToken := strings.TrimSpace(*authToken)
	if accessToken == "" {
		accessToken = strings.TrimSpace(os.Getenv("MRRSS_AUTH_TOKEN"))
	}
	var rootHandler http.Handler = combinedHandler
	if accessToken != "" {
		rootHandler = middleware.RequireToken(accessToken, combinedHandler)
		log.Println("Access token protection enabled")
	} else if !isLoopbackHost(*host) {
		log.Println("WARNING: no access token configured and the server is reachable from the network. " +
			"Anyone who can reach this port can read and change every subscription and setting. " +
			"Start with --auth-token or MRRSS_AUTH_TOKEN to require a token.")
	}

	// Start background scheduler
	// Use a context that we can cancel on shutdown
	bgCtx, bgCancel := context.WithCancel(context.Background())

	log.Println("Starting background scheduler...")
	go h.StartBackgroundScheduler(bgCtx)

	// Start Network Speed Detection (optional but good to have)
	go func() {
		log.Println("Detecting network speed...")
		detector := network.NewDetector(&http.Client{Timeout: 10 * time.Second})
		detectCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		result := detector.DetectSpeed(detectCtx)
		if result.DetectionSuccess {
			db.SetSetting("network_speed", string(result.SpeedLevel))
			db.SetSetting("network_bandwidth_mbps", fmt.Sprintf("%.2f", result.BandwidthMbps))
			log.Printf("Network detection complete: %s", result.SpeedLevel)
		}
	}()

	// Start HTTP Server
	srv := &http.Server{
		Addr:    *host + ":" + *port,
		Handler: rootHandler,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	bgCancel()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close Database
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	} else {
		log.Println("Database closed")
	}

	log.Println("Server exited")
}
