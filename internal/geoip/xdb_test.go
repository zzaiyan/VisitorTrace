package geoip

import (
	"encoding/binary"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

// buildXDB writes a minimal valid ip2region v3 IPv4 database with one
// segment covering 1.0.0.0-1.255.255.255.
func buildXDB(t *testing.T, region string) string {
	t.Helper()
	vectorIndex := make([]byte, xdbVectorSize)
	dataStart := uint32(xdbHeaderBytes + xdbVectorSize)
	data := []byte(region)
	indexStart := dataStart + uint32(len(data))
	index := make([]byte, xdbIndexBytes)
	// 1.0.0.0 network order = 0x01000000, stored little-endian.
	binary.LittleEndian.PutUint32(index[0:4], 0x01000000)
	binary.LittleEndian.PutUint32(index[4:8], 0x01FFFFFF)
	binary.LittleEndian.PutUint16(index[8:10], uint16(len(data)))
	binary.LittleEndian.PutUint32(index[10:14], dataStart)
	// Vector cell for 1.2.x: row 1, column 2.
	cell := 1*xdbVectorColumns*xdbVectorSpacing + 2*xdbVectorSpacing
	binary.LittleEndian.PutUint32(vectorIndex[cell:], indexStart)
	binary.LittleEndian.PutUint32(vectorIndex[cell+4:], indexStart+xdbIndexBytes)

	header := make([]byte, xdbHeaderBytes)
	binary.LittleEndian.PutUint16(header[0:2], xdbStructureIPv6)
	binary.LittleEndian.PutUint16(header[16:18], 4)
	binary.LittleEndian.PutUint32(header[8:12], indexStart)
	binary.LittleEndian.PutUint32(header[12:16], indexStart)

	file := filepath.Join(t.TempDir(), "test.xdb")
	payload := append(append(append(header, vectorIndex...), data...), index...)
	if err := os.WriteFile(file, payload, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return file
}

func TestXDBReaderLookupAndMiss(t *testing.T) {
	file := buildXDB(t, "Testland|Test Province|Test City|Test ISP|TL")
	resolver, err := OpenWithProvider(string(ProviderIP2Region), file)
	if err != nil {
		t.Fatalf("OpenWithProvider(ip2region) error: %v", err)
	}
	defer resolver.Close()
	location := resolver.Lookup(netip.MustParseAddr("1.2.3.4"))
	if location.CountryCode != "TL" || location.CountryName != "Testland" || location.RegionName != "Test Province" || location.City != "Test City" {
		t.Fatalf("Lookup(1.2.3.4) = %+v", location)
	}
	if missed := resolver.Lookup(netip.MustParseAddr("8.8.8.8")); missed.CountryCode != "" || missed.City != "" {
		t.Fatalf("Lookup(8.8.8.8) = %+v, want a miss", missed)
	}
	if v6 := resolver.Lookup(netip.MustParseAddr("2606:4700::1111")); v6.CountryCode != "" {
		t.Fatalf("IPv6 lookup = %+v, want a miss", v6)
	}
	if err := ValidateWithProvider(string(ProviderIP2Region), file); err != nil {
		t.Fatalf("ValidateWithProvider(ip2region) error: %v", err)
	}
}

func TestXDBReaderRejectsGarbage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "garbage.xdb")
	if err := os.WriteFile(file, []byte("not an xdb database at all"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := OpenWithProvider(string(ProviderIP2Region), file); err == nil {
		t.Fatal("OpenWithProvider(ip2region) accepted garbage")
	}
}
