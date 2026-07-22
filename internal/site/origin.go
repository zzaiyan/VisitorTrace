package site

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("origin is empty")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid origin %q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("origin scheme must be http or https")
	}
	if u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("origin must not contain credentials, path, query, or fragment")
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}

func NormalizeOrigins(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized, err := NormalizeOrigin(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one allowed origin is required")
	}
	sort.Strings(result)
	return result, nil
}

func HostnameFromOrigin(raw string) (string, error) {
	normalized, err := NormalizeOrigin(raw)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parse normalized origin: %w", err)
	}
	hostname := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if hostname == "" {
		return "", fmt.Errorf("origin hostname is empty")
	}
	return hostname, nil
}
