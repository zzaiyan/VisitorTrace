package geoip

import (
	"strings"
	"unicode"
)

// Alternate spellings are data, not branches in the matching logic. Values
// also allow a canonical city to be added without editing the generated map.
var cityAlternateNames = map[string]string{
	"Macao":   "Macau",
	"Kashi":   "Kashgar",
	"Urumchi": "Urumqi",
	"Ürümqi":  "Urumqi",
	"澳門":      "Macau",
	"台北":      "Taipei",
	"臺北":      "Taipei",
	"臺中":      "Taichung",
}

// A suffix is removed only when the remaining name resolves to a known city.
// This avoids turning unknown localities such as "New York City" into guesses.
var cityAdministrativeSuffixes = []string{
	" of the peoples republic of china", " special administrative region", " prefecture level city",
	" autonomous region", " municipality", " prefecture", " province",
	" region", " city", " sar", " china",
	"特别行政区", "特別行政區", "维吾尔自治区", "壮族自治区", "回族自治区", "自治区",
	"自治州", "地区", "省级", "省", "市", "盟",
}

var cityCanonicalNames = buildCityCanonicalNames(cityNamesEN, cityAlternateNames)

func buildCityCanonicalNames(primary, alternate map[string]string) map[string]string {
	result := make(map[string]string, len(primary)*2+len(alternate)*2)
	add := func(alias, canonical string) {
		key := cityNameKey(alias)
		if key == "" {
			return
		}
		if previous, exists := result[key]; exists {
			if previous != canonical {
				// An ambiguous alias must never depend on Go map iteration order.
				result[key] = ""
			}
			return
		}
		result[key] = canonical
	}
	for alias, canonical := range primary {
		add(alias, canonical)
		add(canonical, canonical)
	}
	for alias, canonical := range alternate {
		add(alias, canonical)
		add(canonical, canonical)
	}
	return result
}

// cityNameKey ignores case, spacing, and punctuation and folds full-width
// ASCII. Apostrophes join a word, so Xi'an and Xian match.
func cityNameKey(value string) string {
	var builder strings.Builder
	spaced := false
	for _, char := range strings.TrimSpace(value) {
		if char >= '！' && char <= '～' {
			char -= '！' - '!'
		}
		switch {
		case char == '\'' || char == '’' || char == 'ʼ' || char == '.':
			continue
		case unicode.IsLetter(char) || unicode.IsNumber(char):
			if spaced && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(unicode.ToLower(char))
			spaced = false
		default:
			spaced = true
		}
	}
	return builder.String()
}

// NormalizeCityEN returns a known canonical English name across Chinese and
// Latin provider labels. Unknown or ambiguous names remain unchanged.
func NormalizeCityEN(value string) string {
	value = strings.TrimSpace(value)
	key := cityNameKey(value)
	for range 4 {
		if canonical, known := cityCanonicalNames[key]; known {
			if canonical != "" {
				return canonical
			}
			return value
		}
		stripped := false
		for _, suffix := range cityAdministrativeSuffixes {
			if strings.HasSuffix(key, suffix) && len(key) > len(suffix) {
				key = strings.TrimSpace(strings.TrimSuffix(key, suffix))
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	return value
}
