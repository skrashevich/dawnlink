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

const (
	analyticsScopeInstance = "instance"
	analyticsScopeUser     = "user"
)

func (s *Server) downloadAnalyticsURL(r *http.Request) string {
	if !s.cfg.DownloadAnalyticsView || !s.cfg.DownloadAnalyticsCollect {
		return ""
	}
	if s.cfg.GitHubClientID == "" || s.cfg.GitHubClientSecret == "" {
		return ""
	}
	return s.abs(r, "/analytics/downloads")
}

func (s *Server) analyticsAdminToken(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return r.Header.Get("X-Download-Analytics-Admin-Token")
}

func (s *Server) analyticsAdminAuth(r *http.Request) (bool, error) {
	secret := s.cfg.DownloadAnalyticsAdminSecret
	if secret == "" {
		return false, nil
	}
	token := s.analyticsAdminToken(r)
	if token == "" {
		return false, nil
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		return false, &httpError{status: 404, message: "not found"}
	}
	return true, nil
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
	if !s.cfg.DownloadAnalyticsView || !s.cfg.DownloadAnalyticsCollect {
		return &httpError{status: 404, message: "not found"}
	}

	scope := analyticsScopeUser
	var refs []db.RepoRef
	var err error
	canonical := s.abs(r, "/analytics/downloads")

	if admin, err := s.analyticsAdminAuth(r); err != nil {
		return err
	} else if admin {
		scope = analyticsScopeInstance
		refs, err = s.store.AllInstanceRepoRefs()
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("token", s.analyticsAdminToken(r))
		canonical += "?" + q.Encode()
	} else {
		userTok, err := s.resolveUserSession(w, r, "/analytics/downloads")
		if err != nil {
			return err
		}
		refs, err = s.userRepoRefs(userTok)
		if err != nil {
			return err
		}
	}

	summary, err := s.store.DownloadAnalyticsSummary(refs, 75)
	if err != nil {
		return err
	}

	leadKey := "analytics_lead_user"
	if scope == analyticsScopeInstance {
		leadKey = "analytics_lead_instance"
	}

	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
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
			"AnalyticsScope": scope,
			"AnalyticsLead":  s.t(r, leadKey),
			"ScopedRepos":    len(refs),
			"TotalAll":       summary.TotalAll,
			"Total24h":       summary.Total24h,
			"Total7d":        summary.Total7d,
			"Total30d":       summary.Total30d,
			"UniqueRepos":    summary.UniqueRepos,
			"UniqueClients":  summary.UniqueClients,
			"PrivateCount":   summary.PrivateCount,
			"PublicCount":    summary.PublicCount,
			"PrivatePct":     fmt.Sprintf("%.1f", privatePct),
			"PublicPct":      fmt.Sprintf("%.1f", 100-privatePct),
			"ByDay":          summary.ByDay,
			"ByHour":         summary.ByHour,
			"TopRepos":       summary.TopRepos,
			"TopArtifacts":   summary.TopArtifacts,
			"TopWorkflows":   summary.TopWorkflows,
			"TopBranches":    summary.TopBranches,
			"TopRouteKinds":  summary.TopRouteKinds,
			"TopReferers":    summary.TopReferers,
			"Recent":         summary.Recent,
			"MaxDayCount":    maxDay,
			"MaxHourCount":   maxHour,
		},
	})
}
