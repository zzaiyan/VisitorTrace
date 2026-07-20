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
