package server

import (
	"net/http/httptest"
	"testing"
)

func TestLanguageResolution(t *testing.T) {
	admin := httptest.NewRequest("GET", "/admin", nil)
	if got := adminLanguage(admin); got != "zh-CN" {
		t.Fatalf("adminLanguage() = %q", got)
	}
	automatic := httptest.NewRequest("GET", "/public/site/analytics", nil)
	automatic.Header.Set("Accept-Language", "en-US,en;q=0.8")
	if got := publicLanguage(automatic, "auto"); got != "en" {
		t.Fatalf("publicLanguage(auto) = %q", got)
	}
	japanese := httptest.NewRequest("GET", "/public/site/analytics", nil)
	japanese.Header.Set("Accept-Language", "ja-JP,ja;q=0.9,en;q=0.8")
	if got := publicLanguage(japanese, "auto"); got != "ja" {
		t.Fatalf("publicLanguage(japanese auto) = %q", got)
	}
	fixed := httptest.NewRequest("GET", "/public/site/analytics", nil)
	fixed.Header.Set("Accept-Language", "zh-CN")
	if got := publicLanguage(fixed, "en"); got != "en" {
		t.Fatalf("publicLanguage(fixed) = %q", got)
	}
	override := httptest.NewRequest("GET", "/public/site/analytics?range=30d&lang=zh-CN", nil)
	if got := publicLanguage(override, "en"); got != "zh-CN" {
		t.Fatalf("publicLanguage(override) = %q", got)
	}
	if got := languageURL(override, "en"); got != "/public/site/analytics?lang=en&range=30d" {
		t.Fatalf("languageURL() = %q", got)
	}
	if !validLanguage("ja") {
		t.Fatal("Japanese is not accepted as a valid language")
	}
}

func TestJapaneseTranslationsAreComplete(t *testing.T) {
	for key := range messages["en"] {
		if japaneseMessages[key] == "" {
			t.Errorf("Japanese translation is missing key %q", key)
		}
	}
	for key := range japaneseMessages {
		if messages["en"][key] == "" {
			t.Errorf("Japanese translation has unknown key %q", key)
		}
	}
}
