package main

import (
	"embed"
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
	bundle, err := i18n.Load(cfg.DefaultLocale)
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

	srv := handlers.New(cfg, store, ghApp, eng, bundle)
	mux := http.NewServeMux()
	mux.HandleFunc("/style.css", staticHandler("style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/logo.svg", staticHandler("logo.svg", "image/svg+xml"))
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

func staticHandler(name, contentType string) http.HandlerFunc {
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
		w.Header().Set("Cache-Control", "max-age=8640000")
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	}
}
