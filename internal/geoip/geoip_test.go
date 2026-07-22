package geoip

import (
	"net/netip"
	"testing"
)

func TestOpenRejectsMissingDatabase(t *testing.T) {
	if _, err := Open(t.TempDir() + "/missing.mmdb"); err == nil {
		t.Fatal("Open() accepted a missing GeoIP database")
	}
}

func TestLookupUnknownResolverIsSafe(t *testing.T) {
	var resolver *Resolver
	got := resolver.Lookup(netip.MustParseAddr("192.0.2.1"))
	if got.CountryCode != "" || got.Latitude != nil {
		t.Fatalf("Lookup() = %#v", got)
	}
}

func TestCityLevelName(t *testing.T) {
	tests := []struct {
		country string
		city    string
		region  string
		want    string
	}{
		{"CN", "Shenzhen (Bantian Residential District)", "Guangdong", "Shenzhen"},
		{"CN", "Qinghe (Qinghe Subdistrict, Beijing)", "Beijing", "Beijing"},
		{"CN", "Jinrongjie (Xicheng District)", "Beijing", "Beijing"},
		{"CN", "Longgang District", "Guangdong", ""},
		{"CN", "Dali Old Town", "Yunnan", "Dali"},
		{"CN", "Guangzhou", "Guangdong", "Guangzhou"},
		{"US", "Washington (District)", "District of Columbia", "Washington (District)"},
	}
	for _, test := range tests {
		if got := CityLevelName(test.country, test.city, test.region); got != test.want {
			t.Errorf("CityLevelName(%q, %q, %q) = %q, want %q", test.country, test.city, test.region, got, test.want)
		}
	}
}
