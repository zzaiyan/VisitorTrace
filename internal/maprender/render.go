package maprender

import (
	"embed"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zzaiyan/VisitorTrace/internal/store"
)

//go:embed assets/world.path
var assets embed.FS

func Render(data store.PublicMapData, options Options) ([]byte, error) {
	pathData, err := assets.ReadFile("assets/world.path")
	if err != nil {
		return nil, fmt.Errorf("read world basemap: %w", err)
	}
	mapHeight := options.Height
	if options.Show["pv"] || options.Show["uv"] {
		mapHeight -= 24
	}
	if mapHeight < 1 {
		mapHeight = 1
	}
	var output strings.Builder
	output.Grow(len(pathData) + 2048)
	output.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
	fmt.Fprintf(&output, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\" role=\"img\">", options.Width, options.Height, options.Width, options.Height)
	fmt.Fprintf(&output, "<title>%s</title>", html.EscapeString(mapTitle(data, options)))
	fmt.Fprintf(&output, "<rect width=\"100%%\" height=\"100%%\" fill=\"#%s\"/>", options.BG)
	fmt.Fprintf(&output, "<g transform=\"scale(%s %s)\"><path d=\"%s\" fill=\"#%s\" stroke=\"#%s\" stroke-width=\"0.7\" vector-effect=\"non-scaling-stroke\"/></g>",
		format(float64(options.Width)/1000), format(float64(mapHeight)/500), pathData, options.Land, options.Border)
	if options.Show["title"] {
		value := options.Title
		if value == "" {
			value = data.SiteName
		}
		if value != "" {
			fmt.Fprintf(&output, "<rect x=\"0\" y=\"0\" width=\"100%%\" height=\"%d\" fill=\"#%s\" fill-opacity=\"0.82\"/>", options.FontSize+10, options.BG)
			fmt.Fprintf(&output, "<text x=\"10\" y=\"%d\" fill=\"#%s\" font-family=\"system-ui,sans-serif\" font-size=\"%d\" font-weight=\"600\"%s>%s</text>", options.FontSize+4, options.Text, options.FontSize, fitTextAttributes(value, options.FontSize, options.Width), html.EscapeString(value))
		}
	}
	maxMetric := int64(0)
	for _, point := range data.Points {
		value := point.Pageviews
		if options.Metric == "uv" {
			value = point.UniqueVisitors
		}
		if value > maxMetric {
			maxMetric = value
		}
	}
	for _, point := range data.Points {
		value := point.Pageviews
		if options.Metric == "uv" {
			value = point.UniqueVisitors
		}
		radius := 2.0
		if maxMetric > 0 && value > 0 {
			radius += 4 * math.Sqrt(float64(value)/float64(maxMetric))
		}
		x := (point.Longitude + 180) / 360 * float64(options.Width)
		y := (90 - point.Latitude) / 180 * float64(mapHeight)
		name := point.City
		if name == "" {
			name = point.CountryCode
		}
		tooltip := fmt.Sprintf("%s: %s Pageviews, %s Unique Visitors", name, formatCount(point.Pageviews), formatCount(point.UniqueVisitors))
		fmt.Fprintf(&output, "<circle cx=\"%s\" cy=\"%s\" r=\"%s\" fill=\"#%s\" fill-opacity=\"0.78\" stroke=\"#ffffff\" stroke-width=\"0.6\"><title>%s</title></circle>", format(x), format(y), format(radius), options.Marker, html.EscapeString(tooltip))
	}
	if options.Show["pv"] || options.Show["uv"] {
		stats := make([]string, 0, 2)
		if options.Show["pv"] {
			stats = append(stats, fmt.Sprintf("%s: %s", labelOrDefault(options.PVLabel, "Total Pageviews"), formatCount(data.Pageviews)))
		}
		if options.Show["uv"] {
			stats = append(stats, fmt.Sprintf("%s: %s", labelOrDefault(options.UVLabel, "Unique Visitors"), formatCount(data.UniqueVisitors)))
		}
		statsText := strings.Join(stats, "  ·  ")
		fmt.Fprintf(&output, "<text x=\"10\" y=\"%d\" fill=\"#%s\" font-family=\"system-ui,sans-serif\" font-size=\"%d\"%s>%s</text>", options.Height-8, options.Text, options.FontSize, fitTextAttributes(statsText, options.FontSize, options.Width), html.EscapeString(statsText))
	}
	output.WriteString("</svg>")
	return []byte(output.String()), nil
}

func mapTitle(data store.PublicMapData, options Options) string {
	if options.Title != "" {
		return options.Title
	}
	if data.SiteName != "" {
		return data.SiteName
	}
	return "VisitorTrace Public Map"
}

func labelOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func formatCount(value int64) string {
	return strconv.FormatInt(value, 10)
}

func format(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func fitTextAttributes(value string, fontSize, width int) string {
	estimatedWidth := float64(utf8.RuneCountInString(value)) * float64(fontSize) * 0.62
	if estimatedWidth <= float64(width-20) {
		return ""
	}
	return fmt.Sprintf(" textLength=\"%d\" lengthAdjust=\"spacingAndGlyphs\"", width-20)
}
