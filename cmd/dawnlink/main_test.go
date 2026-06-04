package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogoHandlerUsesConfiguredDomain(t *testing.T) {
	rec := httptest.NewRecorder()
	logoHandler("downloads.example").ServeHTTP(rec, httptest.NewRequest("GET", "/logo.svg", nil))

	body := rec.Body.String()
	if strings.Contains(body, "{{") {
		t.Fatalf("logo still contains a placeholder: %s", body)
	}
	if !strings.Contains(body, ">downloads</tspan>") || !strings.Contains(body, ">.example</tspan>") {
		t.Fatalf("logo does not contain configured domain: %s", body)
	}
}
