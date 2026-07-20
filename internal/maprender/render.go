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

const (
	mapMinLatitude = -60.0
	mapMaxLatitude = 90.0
)

//go:embed assets/world.path
var assets embed.FS

func Render(data store.PublicMapData, options Options) ([]byte, error) {
	pathData, err := assets.ReadFile("assets/world.path")
	if err != nil {
		return nil, fmt.Errorf("read world basemap: %w", err)
	}
	title := ""
	if options.Show["title"] {
		title = options.Title
		if title == "" {
			title = data.SiteName
		}
	}
	titleHeight := 0
	if title != "" {
		titleHeight = options.FontSize + 10
	}
	stats := mapStats(data, options)
	statLines := layoutStatLines(stats, options.Width, options.FontSize)
	footerHeight := 0
	if len(statLines) > 0 {
		footerHeight = len(statLines)*(options.FontSize+4) + 8
	}
	mapHeight := options.Height - titleHeight - footerHeight
	if mapHeight < 1 {
		mapHeight = 1
	}
	var output strings.Builder
	output.Grow(len(pathData) + 2048)
	output.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
	fmt.Fprintf(&output, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\" role=\"img\">", options.Width, options.Height, options.Width, options.Height)
	fmt.Fprintf(&output, "<title>%s</title>", html.EscapeString(mapTitle(data, options)))
	backgroundFill := "#" + options.BG
	if options.BG == "transparent" {
		backgroundFill = "none"
	}
	fmt.Fprintf(&output, "<rect width=\"100%%\" height=\"100%%\" fill=\"%s\"/>", backgroundFill)
	if title != "" {
		fmt.Fprintf(&output, "<text class=\"visitortrace-title\" x=\"%s\" y=\"%d\" text-anchor=\"middle\" fill=\"#%s\" font-family=\"system-ui,sans-serif\" font-size=\"%d\" font-weight=\"600\">%s</text>", format(float64(options.Width)/2), options.FontSize+3, options.Text, options.FontSize, html.EscapeString(title))
	}
	fmt.Fprintf(&output, "<g class=\"visitortrace-map\" transform=\"translate(0 %d) scale(%s %s)\"><path d=\"%s\" fill=\"#%s\" stroke=\"#%s\" stroke-width=\"0.7\" vector-effect=\"non-scaling-stroke\"/></g>",
		titleHeight, format(float64(options.Width)/1000), format(float64(mapHeight)/500), pathData, options.Land, options.Border)
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
		y := float64(titleHeight) + (mapMaxLatitude-point.Latitude)/(mapMaxLatitude-mapMinLatitude)*float64(mapHeight)
		name := point.City
		if name == "" {
			name = point.CountryCode
		}
		tooltip := fmt.Sprintf("%s: %s Pageviews, %s Unique Visitors", name, formatCount(point.Pageviews), formatCount(point.UniqueVisitors))
		fmt.Fprintf(&output, "<g class=\"visitortrace-marker\"><title>%s</title><circle cx=\"%s\" cy=\"%s\" r=\"%s\" fill=\"#%s\" fill-opacity=\"0.78\" stroke=\"#ffffff\" stroke-width=\"0.6\"/></g>", html.EscapeString(tooltip), format(x), format(y), format(radius), options.Marker)
	}
	footerTop := options.Height - footerHeight
	for index, line := range statLines {
		y := footerTop + options.FontSize + 4 + index*(options.FontSize+4)
		fmt.Fprintf(&output, "<text class=\"visitortrace-stat\" x=\"%s\" y=\"%d\" text-anchor=\"middle\" fill=\"#%s\" font-family=\"system-ui,sans-serif\" font-size=\"%d\">%s</text>", format(float64(options.Width)/2), y, options.Text, options.FontSize, html.EscapeString(line))
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

func mapStats(data store.PublicMapData, options Options) []string {
	stats := make([]string, 0, 2)
	if options.Show["pv"] {
		stats = append(stats, fmt.Sprintf("%s: %s", labelOrDefault(options.PVLabel, "Total Pageviews"), formatCount(data.Pageviews)))
	}
	if options.Show["uv"] {
		stats = append(stats, fmt.Sprintf("%s: %s", labelOrDefault(options.UVLabel, "Unique Visitors"), formatCount(data.UniqueVisitors)))
	}
	return stats
}

func layoutStatLines(stats []string, width, fontSize int) []string {
	if len(stats) == 0 {
		return nil
	}
	joined := strings.Join(stats, "  ·  ")
	if len(stats) == 2 && estimatedTextWidth(joined, fontSize) > float64(width-20) {
		return stats
	}
	return []string{joined}
}

func estimatedTextWidth(value string, fontSize int) float64 {
	estimatedWidth := float64(utf8.RuneCountInString(value)) * float64(fontSize) * 0.62
	return estimatedWidth
}
