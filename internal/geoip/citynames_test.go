package geoip

import "testing"

func TestPostprocessLocationNormalizesGeneratedPlaceAliases(t *testing.T) {
	tests := []struct {
		name                           string
		country, city, region          string
		wantCountry, wantCity          string
		wantRegionCode, wantRegionName string
	}{
		{
			name: "known city and inferred province", country: "CN", city: "武汉",
			wantCountry: "CN", wantCity: "Wuhan", wantRegionCode: "HB", wantRegionName: "Hubei",
		},
		{
			name: "new sample without manual patch", country: "CN", city: "酒泉",
			wantCountry: "CN", wantCity: "Jiuquan", wantRegionCode: "GS", wantRegionName: "Gansu",
		},
		{
			name: "suffix falls back to canonical city", country: "CN", city: "辽阳",
			wantCountry: "CN", wantCity: "Liaoyang", wantRegionCode: "LN", wantRegionName: "Liaoning",
		},
		{
			name: "Chinese province label", country: "CN", city: "南京市", region: "江苏省",
			wantCountry: "CN", wantCity: "Nanjing", wantRegionCode: "JS", wantRegionName: "Jiangsu",
		},
		{
			name: "English province suffix", country: "CN", city: "Wuhan", region: "Hubei Province",
			wantCountry: "CN", wantCity: "Wuhan", wantRegionCode: "HB", wantRegionName: "Hubei",
		},
		{
			name: "administrative suffix", country: "CN", city: "香港特别行政区",
			wantCountry: "CN", wantCity: "Hong Kong",
		},
		{
			name: "Latin administrative suffix", country: "CN", city: "SHANGHAI CITY",
			wantCountry: "CN", wantCity: "Shanghai", wantRegionCode: "SH", wantRegionName: "Shanghai",
		},
		{
			name: "known United States city and state", country: "US", city: "New York City",
			wantCountry: "US", wantCity: "New York City", wantRegionCode: "NY", wantRegionName: "New York",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := PostprocessLocation(Location{CountryCode: test.country, City: test.city, RegionName: test.region})
			if got.CountryCode != test.wantCountry || got.City != test.wantCity ||
				got.RegionCode != test.wantRegionCode || got.RegionName != test.wantRegionName {
				t.Fatalf("PostprocessLocation() = %#v", got)
			}
			if normalized := NormalizeCityForCountry(got.CountryCode, got.City); normalized != got.City {
				t.Fatalf("normalization is not idempotent: %q -> %q", got.City, normalized)
			}
		})
	}
}

func TestCountryContextResolvesGeneratedCity(t *testing.T) {
	if got := NormalizeCityForCountry("CN", "梅州"); got != "Meizhou" {
		t.Fatalf("CN Meizhou normalized to %q", got)
	}
}
