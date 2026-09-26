package geoip

import (
	"net/netip"
	"os"
	"testing"
)

// TestIP2RegionLocalDatabase runs only when a real ip2region v4 xdb file is
// placed at the path below, mirroring the opt-in GeoIP integration checks.
func TestIP2RegionLocalDatabase(t *testing.T) {
	const databasePath = "/tmp/ip2region.xdb"
	if _, err := os.Stat(databasePath); err != nil {
		t.Skipf("local ip2region database not present: %v", err)
	}
	resolver, err := OpenWithProvider(string(ProviderIP2Region), databasePath)
	if err != nil {
		t.Fatalf("OpenWithProvider(ip2region) error: %v", err)
	}
	defer resolver.Close()
	if err := ValidateWithProvider(string(ProviderIP2Region), databasePath); err != nil {
		t.Fatalf("ValidateWithProvider(ip2region) error: %v", err)
	}
	expected := map[string]Location{
		"114.114.114.114": {CountryCode: "CN", CountryName: "中国", RegionName: "江苏省", City: "南京"},
		"223.5.5.5":       {CountryCode: "CN", CountryName: "中国", RegionName: "浙江省", City: "杭州"},
		"8.8.8.8":         {CountryCode: "US", CountryName: "United States", RegionName: "California"},
		"220.181.38.148":  {CountryCode: "CN", CountryName: "中国", RegionName: "北京市", City: "北京"},
	}
	for address, want := range expected {
		parsed := netip.MustParseAddr(address)
		got := resolver.Lookup(parsed)
		if got.CountryCode != want.CountryCode || got.CountryName != want.CountryName || got.RegionName != want.RegionName || got.City != want.City {
			t.Errorf("Lookup(%s) = %+v, want %+v", address, got, want)
		}
	}
}
