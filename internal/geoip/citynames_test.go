package geoip

import "testing"

func TestPostprocessLocationNormalizesKnownCityAliases(t *testing.T) {
	tests := []struct {
		country, city, wantCountry, wantCity string
	}{
		{"CN", "上海", "CN", "Shanghai"},
		{"CN", "SHANGHAI CITY", "CN", "Shanghai"},
		{"CN", " 上海市 ", "CN", "Shanghai"},
		{"CN", "喀什地区", "CN", "Kashgar"},
		{"CN", "Kashi Prefecture", "CN", "Kashgar"},
		{"CN", "乌鲁木齐", "CN", "Urumqi"},
		{"CN", "Ürümqi City", "CN", "Urumqi"},
		{"CN", "Xi'an", "CN", "Xian"},
		{"HK", "香港特别行政区", "CN", "Hong Kong"},
		{"HK", "Hong Kong SAR", "CN", "Hong Kong"},
		{"CN", "HONG-KONG SPECIAL ADMINISTRATIVE REGION", "CN", "Hong Kong"},
		{"CN", "Hong Kong SAR, China", "CN", "Hong Kong"},
		{"CN", "ＨＯＮＧ　ＫＯＮＧ S.A.R.", "CN", "Hong Kong"},
		{"CN", "香港特別行政區", "CN", "Hong Kong"},
		{"MO", "Macao SAR", "CN", "Macau"},
		{"CN", "澳门特别行政区", "CN", "Macau"},
		{"CN", "澳門特別行政區", "CN", "Macau"},
		{"CN", "Macao Special Administrative Region of the People's Republic of China", "CN", "Macau"},
		{"TW", "臺北市", "CN", "Taipei"},
		{"US", "New York City", "US", "New York City"},
		{"HK", "Lo So Shing", "CN", "Lo So Shing"},
		{"US", "San Francisco", "US", "San Francisco"},
	}
	for _, test := range tests {
		got := PostprocessLocation(Location{CountryCode: test.country, City: test.city})
		if got.CountryCode != test.wantCountry || got.City != test.wantCity {
			t.Errorf("PostprocessLocation(%q, %q) = %q, %q", test.country, test.city, got.CountryCode, got.City)
		}
		if normalized := NormalizeCityEN(got.City); normalized != got.City {
			t.Errorf("NormalizeCityEN(%q) is not idempotent: %q", got.City, normalized)
		}
	}
}

func TestCityAliasesResolveToKnownCanonicalNames(t *testing.T) {
	for alias, canonical := range cityNamesEN {
		if got := NormalizeCityEN(alias); got != canonical {
			t.Errorf("known name %q normalized to %q, want %q", alias, got, canonical)
		}
	}
	for alias, canonical := range cityAlternateNames {
		if NormalizeCityEN(alias) != canonical || NormalizeCityEN(canonical) != canonical {
			t.Errorf("city alias %q does not resolve consistently to %q", alias, canonical)
		}
	}
}

func TestAmbiguousCityAliasIsNotMerged(t *testing.T) {
	index := buildCityCanonicalNames(map[string]string{
		"Shared": "First City",
		"shared": "Second City",
	}, nil)
	if canonical := index["shared"]; canonical != "" {
		t.Fatalf("ambiguous city alias resolved to %q", canonical)
	}
}
