package maprender

import (
	"net/url"
	"strings"
	"testing"

	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestParseOptions(t *testing.T) {
	options, err := ParseOptions(url.Values{
		"w":      {"640"},
		"h":      {"360"},
		"show":   {"title,pv"},
		"metric": {"uv"},
		"bg":     {"#FFFFFF"},
	})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if options.Width != 640 || options.Height != 360 || options.Metric != "uv" || options.BG != "ffffff" || !options.Show["title"] || options.Show["uv"] {
		t.Fatalf("ParseOptions() = %#v", options)
	}
}

func TestParseOptionsRejectsUnknownParameter(t *testing.T) {
	if _, err := ParseOptions(url.Values{"unknown": {"value"}}); err == nil {
		t.Fatal("ParseOptions() accepted an unknown parameter")
	}
}

func TestTransparentBackground(t *testing.T) {
	options, err := ParseOptions(url.Values{"bg": {"transparent"}})
	if err != nil {
		t.Fatalf("ParseOptions() transparent error = %v", err)
	}
	if options.BG != "transparent" {
		t.Fatalf("transparent background = %q", options.BG)
	}
	data := store.PublicMapData{SiteName: "Transparent", Pageviews: 1, UniqueVisitors: 1}
	result, err := Render(data, options)
	if err != nil {
		t.Fatalf("Render() transparent error = %v", err)
	}
	if !strings.Contains(string(result), `<rect width="100%" height="100%" fill="none"/>`) {
		t.Fatalf("transparent background was not rendered as none")
	}
	if strings.Contains(string(result), "#transparent") {
		t.Fatal("transparent background produced an invalid SVG color")
	}
}

func TestRenderEscapesLabelsAndIncludesMap(t *testing.T) {
	options := DefaultOptions()
	options.Title = "A <Site>"
	data := store.PublicMapData{
		SiteName:       "A <Site>",
		Pageviews:      12,
		UniqueVisitors: 8,
		Points: []store.MapPoint{{
			City:           "Wuhan",
			CountryCode:    "CN",
			Latitude:       30.59,
			Longitude:      114.30,
			Pageviews:      12,
			UniqueVisitors: 8,
		}},
	}
	result, err := Render(data, options)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	value := string(result)
	if !strings.Contains(value, "&lt;Site&gt;") || !strings.Contains(value, "<path") || !strings.Contains(value, "Wuhan") {
		t.Fatalf("rendered SVG is missing escaped title, basemap, or marker")
	}
	if strings.Contains(value, "DB-IP") {
		t.Fatal("provider attribution was drawn inside SVG")
	}
}

func TestRenderUsesCenteredNonStretchingTextAndSeparateTitleBand(t *testing.T) {
	options := DefaultOptions()
	options.Width = 240
	options.Height = 168
	options.Title = "VisitorTrace"
	options.PVLabel = "A deliberately long Pageview label"
	options.UVLabel = "A deliberately long Visitor label"
	data := store.PublicMapData{SiteName: "Demo", Pageviews: 1234, UniqueVisitors: 456}
	result, err := Render(data, options)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	value := string(result)
	if strings.Contains(value, "textLength=") || strings.Contains(value, "lengthAdjust=") {
		t.Fatal("rendered text still uses SVG font stretching")
	}
	titleIndex := strings.Index(value, `class="visitortrace-title"`)
	mapIndex := strings.Index(value, `<g class="visitortrace-map"`)
	if titleIndex < 0 || mapIndex < 0 || titleIndex > mapIndex {
		t.Fatal("title is not rendered before the map band")
	}
	if !strings.Contains(value, `transform="translate(0 22`) || !strings.Contains(value, `text-anchor="middle"`) {
		t.Fatal("title/map layout is not separated and centered")
	}
	if strings.Count(value, `class="visitortrace-stat"`) != 2 {
		t.Fatalf("statistics were not split into two centered lines: %d", strings.Count(value, `class="visitortrace-stat"`))
	}
}
