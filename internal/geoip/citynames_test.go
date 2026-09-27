package geoip

import "testing"

func TestPostprocessLocationNormalizesChineseAndHongKongNames(t *testing.T) {
	tests := []struct {
		country, city, wantCountry, wantCity string
	}{
		{"CN", "上海", "CN", "Shanghai"},
		{"CN", "喀什地区", "CN", "Kashgar"},
		{"HK", "香港特别行政区", "CN", "Hong Kong"},
		{"HK", "Hong Kong SAR", "CN", "Hong Kong"},
		{"US", "San Francisco", "US", "San Francisco"},
	}
	for _, test := range tests {
		got := PostprocessLocation(Location{CountryCode: test.country, City: test.city})
		if got.CountryCode != test.wantCountry || got.City != test.wantCity {
			t.Errorf("PostprocessLocation(%q, %q) = %q, %q", test.country, test.city, got.CountryCode, got.City)
		}
	}
}
