package i18n

import (
	"net/http/httptest"
	"testing"
)

func TestUnknownLocaleFallsBackToEnglish(t *testing.T) {
	bundle, err := Load("ru")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := bundle.T("unknown", "site_title"), "dawnl.ink for GitHub"; got != want {
		t.Fatalf("T() = %q, want %q", got, want)
	}
}

func TestLanguagesAreStable(t *testing.T) {
	bundle, err := Load("ru")
	if err != nil {
		t.Fatal(err)
	}
	langs := bundle.Languages()
	if len(langs) != 2 || langs[0] != "en" || langs[1] != "ru" {
		t.Fatalf("Languages() = %#v", langs)
	}
}

func TestLocaleFromRequestUsesEnglishFallbackAndRussianPreference(t *testing.T) {
	bundle, err := Load("en")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Accept-Language", "de-DE, ru;q=0.8, en;q=0.9")
	if got := bundle.LocaleFromRequest(request); got != "en" {
		t.Fatalf("LocaleFromRequest() = %q, want en", got)
	}

	request.Header.Set("Accept-Language", "ru-RU, en;q=0.8")
	if got := bundle.LocaleFromRequest(request); got != "ru" {
		t.Fatalf("LocaleFromRequest() = %q, want ru", got)
	}

	request.Header.Set("Accept-Language", "de-DE")
	if got := bundle.LocaleFromRequest(request); got != "en" {
		t.Fatalf("LocaleFromRequest() = %q, want en fallback", got)
	}
}
