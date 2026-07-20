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
