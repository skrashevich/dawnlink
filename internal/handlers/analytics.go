package handlers

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/render"
)

type downloadAnalyticsMeta struct {
	RouteKind    string
	Owner        string
	Repo         string
	ArtifactName string
	Workflow     string
	Branch       string
	RunID        int64
	ArtifactID   int64
	StatusFilter string
	RunEvent     string
	PrivateLink  bool
}

func (s *Server) downloadAnalyticsURL(r *http.Request) string {
	if !s.cfg.DownloadAnalyticsView || !s.cfg.DownloadAnalyticsCollect || s.cfg.DownloadAnalyticsSecret == "" {
		return ""
	}
	v := url.Values{}
	v.Set("token", s.cfg.DownloadAnalyticsSecret)
	return s.abs(r, "/analytics/downloads") + "?" + v.Encode()
}

func (s *Server) maybeRecordDownload(r *http.Request, meta downloadAnalyticsMeta) {
	if !s.cfg.DownloadAnalyticsCollect {
		return
	}
	rec := db.DownloadRecord{
		RouteKind:    meta.RouteKind,
		Owner:        meta.Owner,
		Repo:         meta.Repo,
		ArtifactName: meta.ArtifactName,
		Workflow:     meta.Workflow,
		Branch:       meta.Branch,
		RunID:        meta.RunID,
		ArtifactID:   meta.ArtifactID,
		StatusFilter: meta.StatusFilter,
		RunEvent:     meta.RunEvent,
		PrivateLink:  meta.PrivateLink,
		RequestHost:  requestHost(r),
		UserAgent:    r.UserAgent(),
		Referer:      r.Referer(),
		ClientIPHash: db.HashClientIP(s.cfg.AppSecret, clientIP(r)),
		Path:         r.URL.Path,
	}
	go func() {
		if err := s.store.RecordDownload(rec); err != nil {
			log.Printf("download analytics: record: %v", err)
		}
	}()
}

func requestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return host
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) downloadAnalytics(w http.ResponseWriter, r *http.Request) error {
	if !s.cfg.DownloadAnalyticsView {
		return &httpError{status: 404, message: "not found"}
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("X-Download-Analytics-Token")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.DownloadAnalyticsSecret)) != 1 {
		return &httpError{status: 404, message: "not found"}
	}
	summary, err := s.store.DownloadAnalyticsSummary(75)
	if err != nil {
		return err
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	canonical := s.abs(r, "/analytics/downloads")
	q := r.URL.Query()
	q.Set("token", token)
	canonical += "?" + q.Encode()
	privatePct := 0.0
	if summary.TotalAll > 0 {
		privatePct = float64(summary.PrivateCount) * 100 / float64(summary.TotalAll)
	}
	var maxDay, maxHour int64
	for _, d := range summary.ByDay {
		if d.Count > maxDay {
			maxDay = d.Count
		}
	}
	for _, d := range summary.ByHour {
		if d.Count > maxHour {
			maxHour = d.Count
		}
	}
	return s.renderPage(w, r, "download_analytics.html", render.PageData{
		Title:     s.t(r, "analytics_title"),
		Canonical: canonical,
		PageBlock: "download_analytics_body",
		Extra: map[string]any{
			"TotalAll":         summary.TotalAll,
			"Total24h":         summary.Total24h,
			"Total7d":          summary.Total7d,
			"Total30d":         summary.Total30d,
			"UniqueRepos":      summary.UniqueRepos,
			"UniqueClients":    summary.UniqueClients,
			"PrivateCount":     summary.PrivateCount,
			"PublicCount":      summary.PublicCount,
			"PrivatePct":       fmt.Sprintf("%.1f", privatePct),
			"PublicPct":        fmt.Sprintf("%.1f", 100-privatePct),
			"ByDay":            summary.ByDay,
			"ByHour":           summary.ByHour,
			"TopRepos":         summary.TopRepos,
			"TopArtifacts":     summary.TopArtifacts,
			"TopWorkflows":     summary.TopWorkflows,
			"TopBranches":      summary.TopBranches,
			"TopRouteKinds":    summary.TopRouteKinds,
			"TopReferers":      summary.TopReferers,
			"Recent":           summary.Recent,
			"MaxDayCount":      maxDay,
			"MaxHourCount":     maxHour,
			"AnalyticsToken":   token,
		},
	})
}
