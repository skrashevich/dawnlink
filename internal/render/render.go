package render

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/skrashevich/dawnlink/internal/i18n"
)

//go:embed templates/*.html
var templateFS embed.FS

type Engine struct {
	templates *template.Template
	i18n      *i18n.Bundle
	baseURL   string
}

type PageData struct {
	Locale        string
	Tr            map[string]string
	Title         string
	Canonical     string
	Popup         bool
	Messages      []string
	Content       template.HTML
	LanguageLinks []LanguageLink
	BaseURL       string
	PageBlock     string
	Extra         map[string]any
}

type LanguageLink struct {
	Language string
	URL      string
}

func New(bundle *i18n.Bundle, baseURL string) (*Engine, error) {
	funcs := template.FuncMap{
		"index": func(m map[string]any, key string) any {
			if m == nil {
				return nil
			}
			return m[key]
		},
		"tr": func(m map[string]string, key string) string {
			if m == nil {
				return key
			}
			if s, ok := m[key]; ok {
				return s
			}
			return key
		},
		"part": func(v any, i int) string {
			if s, ok := v.([]string); ok && i >= 0 && i < len(s) {
				return s[i]
			}
			return ""
		},
	}
	t, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Engine{templates: t, i18n: bundle, baseURL: baseURL}, nil
}

func (e *Engine) Page(w http.ResponseWriter, r *http.Request, name string, data PageData) error {
	locale := e.i18n.LocaleFromRequest(r)
	if data.Locale == "" {
		data.Locale = locale
	}
	lang := data.Locale
	data.Tr = e.i18n.Messages(lang)
	for _, language := range e.i18n.Languages() {
		u := *r.URL
		q := u.Query()
		q.Set("lang", language)
		u.RawQuery = q.Encode()
		data.LanguageLinks = append(data.LanguageLinks, LanguageLink{Language: language, URL: u.String()})
	}
	data.BaseURL = e.baseURL
	if data.Extra == nil {
		data.Extra = make(map[string]any)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return e.templates.ExecuteTemplate(w, name, data)
}

func AbsURL(base, path string) string {
	if strings.HasPrefix(path, "http") {
		return path
	}
	u := strings.TrimRight(base, "/")
	p := strings.TrimPrefix(path, "/")
	return u + "/" + p
}
