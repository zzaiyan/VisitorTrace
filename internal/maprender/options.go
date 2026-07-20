package maprender

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Options struct {
	Width    int
	Height   int
	Title    string
	PVLabel  string
	UVLabel  string
	FontSize int
	BG       string
	Land     string
	Border   string
	Text     string
	Marker   string
	Metric   string
	Show     map[string]bool
}

var colorPattern = regexp.MustCompile("^[0-9a-fA-F]{6}$")

func DefaultOptions() Options {
	return Options{
		Width:    300,
		Height:   168,
		FontSize: 12,
		BG:       "f2f3f3",
		Land:     "6f808f",
		Border:   "ffffff",
		Text:     "54606a",
		Marker:   "e34949",
		Metric:   "pv",
		Show:     map[string]bool{"title": true, "pv": true, "uv": true},
	}
}

func ParseOptions(values url.Values) (Options, error) {
	options := DefaultOptions()
	allowed := map[string]bool{
		"w": true, "h": true, "title": true, "pv_label": true, "uv_label": true,
		"show": true, "fs": true, "bg": true, "land": true, "border": true,
		"text": true, "marker": true, "metric": true,
	}
	for key := range values {
		if !allowed[key] {
			return Options{}, fmt.Errorf("unknown map parameter %q", key)
		}
		if len(values[key]) != 1 {
			return Options{}, fmt.Errorf("map parameter %q must occur once", key)
		}
	}
	var err error
	if options.Width, err = integerOverride(values, "w", options.Width, 160, 1200); err != nil {
		return Options{}, err
	}
	if options.Height, err = integerOverride(values, "h", options.Height, 90, 800); err != nil {
		return Options{}, err
	}
	if options.FontSize, err = integerOverride(values, "fs", options.FontSize, 8, 32); err != nil {
		return Options{}, err
	}
	if value, ok := values["title"]; ok {
		options.Title, err = label(value[0])
		if err != nil {
			return Options{}, fmt.Errorf("title: %w", err)
		}
	}
	if value, ok := values["pv_label"]; ok {
		options.PVLabel, err = label(value[0])
		if err != nil {
			return Options{}, fmt.Errorf("pv_label: %w", err)
		}
	}
	if value, ok := values["uv_label"]; ok {
		options.UVLabel, err = label(value[0])
		if err != nil {
			return Options{}, fmt.Errorf("uv_label: %w", err)
		}
	}
	if value, ok := values["show"]; ok {
		options.Show, err = parseShow(value[0])
		if err != nil {
			return Options{}, err
		}
	}
	for key, destination := range map[string]*string{"bg": &options.BG, "land": &options.Land, "border": &options.Border, "text": &options.Text, "marker": &options.Marker} {
		if value, ok := values[key]; ok {
			parsed, err := color(value[0])
			if err != nil {
				return Options{}, fmt.Errorf("%s: %w", key, err)
			}
			*destination = parsed
		}
	}
	if value, ok := values["metric"]; ok {
		if value[0] != "pv" && value[0] != "uv" {
			return Options{}, fmt.Errorf("metric must be pv or uv")
		}
		options.Metric = value[0]
	}
	return options, nil
}

func (o Options) CacheKey() string {
	show := make([]string, 0, len(o.Show))
	for key, enabled := range o.Show {
		if enabled {
			show = append(show, key)
		}
	}
	sort.Strings(show)
	return fmt.Sprintf("%d,%d,%s,%s,%s,%d,%s,%s,%s,%s,%s,%s", o.Width, o.Height, o.Title, o.PVLabel, o.UVLabel, o.FontSize, o.BG, o.Land, o.Border, o.Text, o.Marker, o.Metric+":"+strings.Join(show, ","))
}

func integerOverride(values url.Values, key string, fallback, minimum, maximum int) (int, error) {
	value, ok := values[key]
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value[0])
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum)
	}
	return parsed, nil
}

func label(value string) (string, error) {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 40 {
		return "", fmt.Errorf("must contain at most 40 characters")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("must not contain control characters")
		}
	}
	return value, nil
}

func color(value string) (string, error) {
	value = strings.TrimPrefix(value, "#")
	if !colorPattern.MatchString(value) {
		return "", fmt.Errorf("must be a six-digit hexadecimal color")
	}
	return strings.ToLower(value), nil
}

func parseShow(value string) (map[string]bool, error) {
	result := make(map[string]bool)
	if value == "" || value == "none" {
		return result, nil
	}
	for _, item := range strings.Split(value, ",") {
		if item != "title" && item != "pv" && item != "uv" {
			return nil, fmt.Errorf("show contains unsupported value %q", item)
		}
		result[item] = true
	}
	return result, nil
}
