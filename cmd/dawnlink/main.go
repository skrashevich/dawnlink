package main

import (
	"embed"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/skrashevich/dawnlink/internal/config"
	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/github"
	"github.com/skrashevich/dawnlink/internal/handlers"
	"github.com/skrashevich/dawnlink/internal/i18n"
	"github.com/skrashevich/dawnlink/internal/render"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	bundle, err := i18n.Load(cfg.DefaultLocale, cfg.PrimaryDomain())
	if err != nil {
		log.Fatal(err)
	}
	if !bundle.Has(cfg.DefaultLocale) {
		log.Fatalf("DEFAULT_LOCALE %q is not available", cfg.DefaultLocale)
	}
	eng, err := render.New(bundle, cfg.BaseURL)
	if err != nil {
		log.Fatal(err)
	}
	ghApp, err := github.NewAppAuth(cfg.GitHubAppID, cfg.GitHubPEMPath)
	if err != nil {
		log.Fatal(err)
	}
	store, err := db.Open(cfg.DatabaseFile, cfg.AppSecret, ghApp, cfg.FallbackInstallationID)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if cfg.DownloadAnalyticsCollect && cfg.DownloadAnalyticsRetention > 0 {
		if n, err := store.PurgeDownloadEventsOlderThan(cfg.DownloadAnalyticsRetention); err != nil {
			log.Printf("download analytics: purge: %v", err)
		} else if n > 0 {
			log.Printf("download analytics: purged %d old events", n)
		}
	}

	srv := handlers.New(cfg, store, ghApp, eng, bundle)
	mux := http.NewServeMux()
	mux.HandleFunc("/style.css", staticHandler("style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/logo.svg", logoHandler(cfg.PrimaryDomain()))
	mux.Handle("/", srv)

	addr := ":" + cfg.Port
	log.Printf("dawnlink listening on %s (default locale: %s)", addr, cfg.DefaultLocale)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func logoHandler(domain string) http.HandlerFunc {
	name, suffix := domain, ""
	if i := strings.LastIndex(domain, "."); i >= 0 {
		name, suffix = domain[:i], domain[i:]
	}
	width := 48 + len([]rune(domain))*9
	if width < 168 {
		width = 168
	}
	replacements := map[string]string{
		"{{width}}":         fmt.Sprint(width),
		"{{domain_name}}":   html.EscapeString(name),
		"{{domain_suffix}}": html.EscapeString(suffix),
	}
	return staticHandlerWithReplacements("logo.svg", "image/svg+xml", replacements)
}

func staticHandler(name, contentType string) http.HandlerFunc {
	return staticHandlerWithReplacements(name, contentType, nil)
}

func staticHandlerWithReplacements(name, contentType string, replacements map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(staticFS, "static/"+name)
		if err != nil {
			if path := os.Getenv("STATIC_DIR"); path != "" {
				data, err = os.ReadFile(strings.TrimRight(path, "/") + "/" + name)
			}
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		content := string(data)
		for placeholder, replacement := range replacements {
			content = strings.ReplaceAll(content, placeholder, replacement)
		}
		w.Header().Set("Cache-Control", "max-age=8640000")
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(content))
	}
}
