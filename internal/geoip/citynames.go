package geoip

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"unicode"
)

// A suffix is removed only when the remaining name resolves to a known city.
// This avoids turning unknown localities such as "New York City" into guesses.
var cityAdministrativeSuffixes = []string{
	" of the peoples republic of china", " special administrative region", " prefecture level city",
	" autonomous region", " municipality", " prefecture", " province",
	" region", " city", " sar", " china",
	"特别行政区", "特別行政區", "维吾尔自治区", "壮族自治区", "回族自治区", "自治区",
	"自治州", "地区", "省级", "省", "市", "盟",
}

type cityAlias struct {
	CountryCode string
	City        string
	RegionCode  string
	RegionName  string
}

type regionAlias struct {
	CountryCode string
	RegionCode  string
	RegionName  string
}

var (
	cityAliases   map[string][]cityAlias
	regionAliases map[string][]regionAlias
)

func init() {
	cityAliases = make(map[string][]cityAlias)
	regionAliases = make(map[string][]regionAlias)
	reader, err := gzip.NewReader(bytes.NewReader(placeNamesData))
	if err != nil {
		panic(fmt.Sprintf("open embedded place names: %v", err))
	}
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		switch {
		case len(fields) == 6 && fields[0] == "city":
			cityAliases[fields[1]] = append(cityAliases[fields[1]], cityAlias{
				CountryCode: fields[2], City: fields[3], RegionCode: fields[4], RegionName: fields[5],
			})
		case len(fields) == 5 && fields[0] == "region":
			regionAliases[fields[1]] = append(regionAliases[fields[1]], regionAlias{
				CountryCode: fields[2], RegionCode: fields[3], RegionName: fields[4],
			})
		}
	}
	if err := scanner.Err(); err != nil {
		panic(fmt.Sprintf("read embedded place names: %v", err))
	}
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

func cityAliasFor(countryCode, value string) (cityAlias, bool) {
	value = strings.TrimSpace(value)
	key := cityNameKey(value)
	for range 4 {
		targets, known := cityAliases[key]
		if known {
			country := NormalizeCountryCode(countryCode)
			candidates := make([]cityAlias, 0, len(targets))
			for _, target := range targets {
				if cityAliasCountryMatches(country, target.CountryCode) {
					candidates = append(candidates, target)
				}
			}
			if country != "" {
				for _, candidate := range candidates {
					if candidate.City != "" {
						return candidate, true
					}
				}
			} else {
				candidates = targets
				seen := make(map[string]struct{}, len(candidates))
				for _, candidate := range candidates {
					if candidate.City != "" {
						seen[candidate.City] = struct{}{}
					}
				}
				if len(seen) == 1 {
					for _, candidate := range candidates {
						if candidate.City != "" {
							return candidate, true
						}
					}
				}
			}
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
	return cityAlias{}, false
}

func cityAliasCountryMatches(search, target string) bool {
	if search == target {
		return true
	}
	return search == "CN" && (target == "HK" || target == "MO" || target == "TW")
}

// NormalizeCityForCountry returns a known canonical English city name across
// Chinese and Latin provider labels. Unknown names are unchanged.
func NormalizeCityForCountry(countryCode, value string) string {
	target, ok := cityAliasFor(countryCode, value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return target.City
}

func normalizeRegion(countryCode, value string) (code, name string, ok bool) {
	key := cityNameKey(value)
	country := NormalizeCountryCode(countryCode)
	for range 4 {
		for _, target := range regionAliases[key] {
			if target.CountryCode == country {
				return target.RegionCode, target.RegionName, true
			}
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
	return "", "", false
}
