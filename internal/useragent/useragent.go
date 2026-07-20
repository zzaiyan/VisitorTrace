package useragent

import "strings"

type Classification struct {
	Browser         string
	OperatingSystem string
	Bot             bool
}

func Classify(value string) Classification {
	lower := strings.ToLower(value)
	result := Classification{Browser: "Other", OperatingSystem: "Other"}
	for _, marker := range []string{"bot", "crawler", "spider", "headless", "preview", "slurp"} {
		if strings.Contains(lower, marker) {
			result.Bot = true
			break
		}
	}
	switch {
	case strings.Contains(lower, "edg/") || strings.Contains(lower, "edgios/") || strings.Contains(lower, "edga/"):
		result.Browser = "Edge"
	case strings.Contains(lower, "opr/") || strings.Contains(lower, "opera"):
		result.Browser = "Opera"
	case strings.Contains(lower, "chrome/") || strings.Contains(lower, "crios/"):
		result.Browser = "Chrome"
	case strings.Contains(lower, "firefox/") || strings.Contains(lower, "fxios/"):
		result.Browser = "Firefox"
	case strings.Contains(lower, "safari/"):
		result.Browser = "Safari"
	case strings.Contains(lower, "msie ") || strings.Contains(lower, "trident/"):
		result.Browser = "Internet Explorer"
	}
	switch {
	case strings.Contains(lower, "android"):
		result.OperatingSystem = "Android"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") || strings.Contains(lower, "ipod"):
		result.OperatingSystem = "iOS"
	case strings.Contains(lower, "windows"):
		result.OperatingSystem = "Windows"
	case strings.Contains(lower, "mac os x") || strings.Contains(lower, "macintosh"):
		result.OperatingSystem = "macOS"
	case strings.Contains(lower, "cros"):
		result.OperatingSystem = "ChromeOS"
	case strings.Contains(lower, "linux"):
		result.OperatingSystem = "Linux"
	}
	return result
}
