package i18n

import (
	"embed"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

type Bundle struct {
	mu       sync.RWMutex
	messages map[string]map[string]string
	defaults string
}

func Load(defaultLocale, domain string) (*Bundle, error) {
	b := &Bundle{
		messages: make(map[string]map[string]string),
		defaults: defaultLocale,
	}
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(e.Name(), ".json")
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		for key, message := range m {
			m[key] = strings.ReplaceAll(message, "{{domain}}", domain)
		}
		b.messages[lang] = m
	}
	return b, nil
}

func (b *Bundle) LocaleFromRequest(r *http.Request) string {
	if q := r.URL.Query().Get("lang"); q != "" {
		if b.has(q) {
			return q
		}
	}
	if c, err := r.Cookie("lang"); err == nil && b.has(c.Value) {
		return c.Value
	}
	al := r.Header.Get("Accept-Language")
	type candidate struct {
		lang  string
		q     float64
		order int
	}
	var candidates []candidate
	order := 0
	for part := range strings.SplitSeq(al, ",") {
		code, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		code = strings.ToLower(code)
		q := 1.0
		for param := range strings.SplitSeq(params, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && key == "q" {
				if parsed, err := strconv.ParseFloat(value, 64); err == nil {
					q = parsed
				}
			}
		}
		short, _, _ := strings.Cut(code, "-")
		if q > 0 && b.has(short) {
			candidates = append(candidates, candidate{lang: short, q: q, order: order})
		}
		order++
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].q > candidates[j].q
	})
	if len(candidates) > 0 {
		return candidates[0].lang
	}
	if b.has(b.defaults) {
		return b.defaults
	}
	return "en"
}

func (b *Bundle) Has(lang string) bool {
	return b.has(lang)
}

func (b *Bundle) has(lang string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.messages[lang]
	return ok
}

func (b *Bundle) Messages(lang string) map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.messages[lang]; ok {
		return m
	}
	return b.messages["en"]
}

func (b *Bundle) T(lang, key string, args ...any) string {
	b.mu.RLock()
	m, ok := b.messages[lang]
	if !ok {
		m = b.messages["en"]
	}
	b.mu.RUnlock()
	s := key
	if m[key] != "" {
		s = m[key]
	}
	if len(args) == 0 {
		return s
	}
	return strings.Replace(s, "%s", fmtArg(args[0]), 1)
}

func fmtArg(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func (b *Bundle) Languages() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var langs []string
	for k := range b.messages {
		langs = append(langs, k)
	}
	sort.Strings(langs)
	return langs
}
