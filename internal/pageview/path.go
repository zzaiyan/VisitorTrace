package pageview

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const MaxPathBytes = 512

func NormalizePath(raw string) (string, error) {
	if raw == "" {
		return "/", nil
	}
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("path must be valid UTF-8")
	}
	if !strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("path must start with a slash")
	}
	if strings.ContainsAny(raw, "?#") {
		return "", fmt.Errorf("path must not contain a query or fragment")
	}
	if hasControl(raw) {
		return "", fmt.Errorf("path must not contain control characters")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("path contains invalid percent encoding")
	}
	if hasControl(decoded) {
		return "", fmt.Errorf("path must not contain encoded control characters")
	}
	normalized := removeDotSegments(decoded)
	if normalized == "" {
		normalized = "/"
	}
	encoded := (&url.URL{Path: normalized}).EscapedPath()
	if len(encoded) > MaxPathBytes {
		return "", fmt.Errorf("normalized path exceeds %d bytes", MaxPathBytes)
	}
	return encoded, nil
}

func removeDotSegments(input string) string {
	var output string
	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"):
			input = strings.TrimPrefix(input, "../")
		case strings.HasPrefix(input, "./"):
			input = strings.TrimPrefix(input, "./")
		case strings.HasPrefix(input, "/./"):
			input = "/" + strings.TrimPrefix(input, "/./")
		case input == "/.":
			input = "/"
		case strings.HasPrefix(input, "/../"):
			input = "/" + strings.TrimPrefix(input, "/../")
			output = removeLastSegment(output)
		case input == "/..":
			input = "/"
			output = removeLastSegment(output)
		case input == "." || input == "..":
			input = ""
		default:
			end := 0
			if strings.HasPrefix(input, "/") {
				end = strings.Index(input[1:], "/")
				if end >= 0 {
					end++
				}
			} else {
				end = strings.Index(input, "/")
			}
			if end < 0 {
				output += input
				input = ""
			} else {
				output += input[:end]
				input = input[end:]
			}
		}
	}
	return output
}

func removeLastSegment(value string) string {
	if index := strings.LastIndex(value, "/"); index >= 0 {
		return value[:index]
	}
	return ""
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
