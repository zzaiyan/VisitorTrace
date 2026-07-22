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

func TestProviders(t *testing.T) {
	for _, provider := range []string{"dbip", "maxmind", "ip2location", "MAXMIND"} {
		if _, err := NormalizeProvider(provider); err != nil {
			t.Errorf("NormalizeProvider(%q) error = %v", provider, err)
		}
	}
	if _, err := NormalizeProvider("unknown"); err == nil {
		t.Fatal("NormalizeProvider accepted an unknown provider")
	}
	for _, provider := range []string{"dbip", "maxmind", "ip2location"} {
		if _, err := OpenWithProvider(provider, t.TempDir()+"/missing.mmdb"); err == nil {
			t.Errorf("OpenWithProvider(%q) accepted a missing database", provider)
		}
	}
}

func TestLookupUnknownResolverIsSafe(t *testing.T) {
	var resolver *Resolver
	got := resolver.Lookup(netip.MustParseAddr("192.0.2.1"))
	if got.CountryCode != "" || got.Latitude != nil {
		t.Fatalf("Lookup() = %#v", got)
	}
}

func TestIP2LocationRecordMapsToLocation(t *testing.T) {
	latitude := 31.2304
	longitude := 121.4737
	got := locationFromIP2LocationRecord(ip2LocationRecord{
		CountryCode: "CN", CountryName: "China", RegionName: "Shanghai", CityName: "Shanghai",
		Latitude: latitude, Longitude: longitude,
	})
	if got.CountryCode != "CN" || got.CountryName != "China" || got.RegionName != "Shanghai" || got.City != "Shanghai" {
		t.Fatalf("location fields = %#v", got)
	}
	if got.Latitude == nil || *got.Latitude != latitude || got.Longitude == nil || *got.Longitude != longitude {
		t.Fatalf("location coordinates = %#v", got)
	}
}

func TestNestedRecordMapsToLocationWithoutDBIPNormalization(t *testing.T) {
	record := mmdbRecord{}
	record.Country.ISOCode = "CN"
	record.Country.Names = map[string]string{"en": "China"}
	record.Subdivisions = []struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	}{{ISOCode: "BJ", Names: map[string]string{"en": "Beijing"}}}
	record.City.Names = map[string]string{"en": "Jinrongjie (Xicheng District)"}
	record.Location.Latitude = 39.9042
	record.Location.Longitude = 116.4074

	got := locationFromMMDBRecord(record, false)
	if got.City != "Jinrongjie (Xicheng District)" || got.RegionCode != "BJ" {
		t.Fatalf("nested location = %#v", got)
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
