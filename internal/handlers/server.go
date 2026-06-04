package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/skrashevich/dawnlink/internal/cache"
	"github.com/skrashevich/dawnlink/internal/config"
	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/github"
	"github.com/skrashevich/dawnlink/internal/i18n"
	"github.com/skrashevich/dawnlink/internal/render"
)

type Server struct {
	cfg    config.Config
	store  *db.Store
	ghApp  *github.AppAuth
	render *render.Engine
	i18n   *i18n.Bundle

	zipCache     *cache.TTLCache[string, string]
	logsCache    *cache.TTLCache[string, string]
	badgeCache   *cache.TTLCache[string, badgeInfo]
	exampleCache sync.Map
}

type ArtifactLink struct {
	URL   string
	Title string
	Ext   bool
	H     string
}

func New(cfg config.Config, store *db.Store, ghApp *github.AppAuth, eng *render.Engine, bundle *i18n.Bundle) *Server {
	return &Server{
		cfg:        cfg,
		store:      store,
		ghApp:      ghApp,
		render:     eng,
		i18n:       bundle,
		zipCache:   cache.NewTTLCache[string, string](1000, 50*time.Second),
		logsCache:  cache.NewTTLCache[string, string](1000, 50*time.Second),
		badgeCache: cache.NewTTLCache[string, badgeInfo](2000, 5*time.Minute),
	}
}

func (s *Server) baseURL(r *http.Request) string {
	return s.cfg.BaseURLForRequest(r)
}

func (s *Server) abs(r *http.Request, path string) string {
	return render.AbsURL(s.baseURL(r), path)
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, name string, data render.PageData) error {
	data.BaseURL = s.baseURL(r)
	return s.render.Page(w, r, name, data)
}

func (s *Server) locale(r *http.Request) string {
	return s.i18n.LocaleFromRequest(r)
}

func (s *Server) t(r *http.Request, key string, args ...any) string {
	return s.i18n.T(s.locale(r), key, args...)
}

func (s *Server) setLangCookie(w http.ResponseWriter, r *http.Request) {
	if lang := r.URL.Query().Get("lang"); lang != "" && s.i18n.Has(lang) {
		http.SetCookie(w, &http.Cookie{
			Name:     "lang",
			Value:    lang,
			Path:     "/",
			MaxAge:   365 * 24 * 3600,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		})
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("h") != "" {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		s.serveError(w, r, &httpError{status: http.StatusMethodNotAllowed, message: "method not allowed"})
		return
	}
	s.setLangCookie(w, r)
	if err := s.route(w, r); err != nil {
		s.serveError(w, r, err)
	}
}

type httpError struct {
	status  int
	message string
	headers http.Header
}

func (e *httpError) Error() string { return e.message }

func redirect(loc string, status int) *httpError {
	h := make(http.Header)
	h.Set("Location", loc)
	return &httpError{status: status, headers: h}
}

func (s *Server) serveError(w http.ResponseWriter, r *http.Request, err error) {
	he, ok := err.(*httpError)
	if !ok {
		he = &httpError{status: 500, message: err.Error()}
	}
	if he.headers != nil {
		for k, v := range he.headers {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
	}
	if he.status >= 300 && he.status < 400 {
		w.WriteHeader(he.status)
		return
	}
	msg := template.HTMLEscapeString(he.message)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(he.status)
	_ = s.renderPage(w, r, "error_page.html", render.PageData{
		Title:     s.t(r, "error_not_found"),
		PageBlock: "error_body",
		Content:   template.HTML(msg),
	})
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) error {
	path := r.URL.Path
	if path == "/" || path == "" {
		return s.index(w, r)
	}
	if path == "/dashboard" {
		return s.dashboard(w, r)
	}
	if path == "/setup" {
		return s.setup(w, r)
	}

	return s.serveArtifactRoutes(w, r)
}

var ghImportPatterns = []struct {
	re  *regexp.Regexp
	gen func(m []string) string
}{
	{
		regexp.MustCompile(`^/([^/]+)/([^/]+)/(blob|tree|raw|blame|commits)/(.+)/\.github/workflows/([^/]+\.ya?ml)$`),
		func(m []string) string {
			wf := strings.TrimSuffix(m[5], ".yml")
			wf = strings.TrimSuffix(wf, ".yaml")
			return fmt.Sprintf("/%s/%s/workflows/%s/%s?preview", m[1], m[2], url.PathEscape(wf), url.PathEscape(m[4]))
		},
	},
	{
		regexp.MustCompile(`^/([^/]+)/([^/]+)/actions/runs/([0-9]+)$`),
		func(m []string) string {
			return fmt.Sprintf("/%s/%s/actions/runs/%s", m[1], m[2], m[3])
		},
	},
	{
		regexp.MustCompile(`^/([^/]+)/([^/]+)/runs/([0-9]+)$`),
		func(m []string) string {
			return fmt.Sprintf("/%s/%s/runs/%s", m[1], m[2], m[3])
		},
	},
	{
		regexp.MustCompile(`^/([^/]+)/([^/]+)/suites/([0-9]+)/artifacts/([0-9]+)$`),
		func(m []string) string {
			return fmt.Sprintf("/%s/%s/actions/artifacts/%s", m[1], m[2], m[4])
		},
	},
	{
		regexp.MustCompile(`^/([^/]+)/([^/]+)/commit/([0-9a-fA-F]{40})/checks/([0-9]+)/logs$`),
		func(m []string) string {
			return fmt.Sprintf("/%s/%s/runs/%s.txt", m[1], m[2], m[4])
		},
	},
}

func matchGitHubImport(path string) (string, bool) {
	for _, p := range ghImportPatterns {
		if m := p.re.FindStringSubmatch(path); m != nil {
			return p.gen(m), true
		}
	}
	return "", false
}
