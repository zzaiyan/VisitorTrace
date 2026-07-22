package maprender

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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
	return ParseOptionsWithDefaults(values, DefaultOptions())
}

func ParseOptionsWithDefaults(values url.Values, defaults Options) (Options, error) {
	options := cloneOptions(defaults)
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
			var parsed string
			if key == "bg" && value[0] == "transparent" {
				parsed = "transparent"
			} else {
				parsed, err = color(value[0])
			}
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

type presetJSON struct {
	Width    *int            `json:"w,omitempty"`
	Height   *int            `json:"h,omitempty"`
	Title    *string         `json:"title,omitempty"`
	PVLabel  *string         `json:"pv_label,omitempty"`
	UVLabel  *string         `json:"uv_label,omitempty"`
	FontSize *int            `json:"fs,omitempty"`
	BG       *string         `json:"bg,omitempty"`
	Land     *string         `json:"land,omitempty"`
	Border   *string         `json:"border,omitempty"`
	Text     *string         `json:"text,omitempty"`
	Marker   *string         `json:"marker,omitempty"`
	Metric   *string         `json:"metric,omitempty"`
	Show     map[string]bool `json:"show"`
}

func ParsePresetJSON(value string) (Options, error) {
	options := DefaultOptions()
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "{}" {
		return options, nil
	}
	var preset presetJSON
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&preset); err != nil {
		return Options{}, fmt.Errorf("decode Map Preset: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Options{}, fmt.Errorf("decode Map Preset: trailing content")
	}
	if preset.Width != nil {
		options.Width = *preset.Width
	}
	if preset.Height != nil {
		options.Height = *preset.Height
	}
	if preset.Title != nil {
		options.Title = *preset.Title
	}
	if preset.PVLabel != nil {
		options.PVLabel = *preset.PVLabel
	}
	if preset.UVLabel != nil {
		options.UVLabel = *preset.UVLabel
	}
	if preset.FontSize != nil {
		options.FontSize = *preset.FontSize
	}
	if preset.BG != nil {
		options.BG = *preset.BG
	}
	if preset.Land != nil {
		options.Land = *preset.Land
	}
	if preset.Border != nil {
		options.Border = *preset.Border
	}
	if preset.Text != nil {
		options.Text = *preset.Text
	}
	if preset.Marker != nil {
		options.Marker = *preset.Marker
	}
	if preset.Metric != nil {
		options.Metric = *preset.Metric
	}
	if preset.Show != nil {
		options.Show = preset.Show
	}
	if err := validateOptions(options); err != nil {
		return Options{}, err
	}
	return options, nil
}

func PresetJSON(options Options) (string, error) {
	if err := validateOptions(options); err != nil {
		return "", err
	}
	show := make(map[string]bool, len(options.Show))
	for key, value := range options.Show {
		if value {
			show[key] = true
		}
	}
	value, err := json.Marshal(presetJSON{
		Width: &options.Width, Height: &options.Height, Title: &options.Title,
		PVLabel: &options.PVLabel, UVLabel: &options.UVLabel, FontSize: &options.FontSize,
		BG: &options.BG, Land: &options.Land, Border: &options.Border, Text: &options.Text,
		Marker: &options.Marker, Metric: &options.Metric, Show: show,
	})
	if err != nil {
		return "", fmt.Errorf("encode Map Preset: %w", err)
	}
	return string(value), nil
}

func cloneOptions(value Options) Options {
	if value.Show == nil {
		value.Show = make(map[string]bool)
	}
	return value
}

func validateOptions(options Options) error {
	if options.Width < 160 || options.Width > 1200 || options.Height < 90 || options.Height > 800 {
		return fmt.Errorf("Map Preset dimensions are out of range")
	}
	if options.FontSize < 8 || options.FontSize > 32 {
		return fmt.Errorf("Map Preset font size must be between 8 and 32")
	}
	for key, value := range map[string]string{"title": options.Title, "pv_label": options.PVLabel, "uv_label": options.UVLabel} {
		if _, err := label(value); err != nil {
			return fmt.Errorf("Map Preset %s: %w", key, err)
		}
	}
	for key, value := range map[string]string{"land": options.Land, "border": options.Border, "text": options.Text, "marker": options.Marker} {
		if _, err := color(value); err != nil {
			return fmt.Errorf("Map Preset %s: %w", key, err)
		}
	}
	if options.BG != "transparent" {
		if _, err := color(options.BG); err != nil {
			return fmt.Errorf("Map Preset bg: %w", err)
		}
	}
	if options.Metric != "pv" && options.Metric != "uv" {
		return fmt.Errorf("Map Preset metric must be pv or uv")
	}
	for key, enabled := range options.Show {
		if enabled && key != "title" && key != "pv" && key != "uv" {
			return fmt.Errorf("Map Preset show contains unsupported value %q", key)
		}
	}
	return nil
}

func (o Options) CacheKey() string {
	show := make([]string, 0, len(o.Show))
	for key, enabled := range o.Show {
		if enabled {
			show = append(show, key)
		}
	}
	sort.Strings(show)
	canonical, _ := json.Marshal(struct {
		Width, Height, FontSize                int
		Title, PVLabel, UVLabel                string
		BG, Land, Border, Text, Marker, Metric string
		Show                                   []string
	}{
		Width: o.Width, Height: o.Height, FontSize: o.FontSize,
		Title: o.Title, PVLabel: o.PVLabel, UVLabel: o.UVLabel,
		BG: o.BG, Land: o.Land, Border: o.Border, Text: o.Text, Marker: o.Marker, Metric: o.Metric, Show: show,
	})
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", digest)
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
