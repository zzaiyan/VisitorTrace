package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectedGeoIPDatasets(t *testing.T) {
	cfg := Default(t.TempDir())
	base := cfg.SelectedGeoIPDatasets()
	if len(base) != 1 || base[0].ID != "primary" || base[0].Provider != "dbip" || base[0].Path != cfg.GeoIPPath {
		t.Fatalf("basic datasets = %#v", base)
	}
	cfg.GeoIPPreset = "precise"
	cfg.GeoIPDomesticOffline = "dbip"
	cfg.GeoIPDomesticOnline = "ipinfo"
	cfg.GeoIPForeignOnline = "ipinfo"
	cfg.GeoIPBackups = []string{"ip2region", "amap", "maxmind"}
	cfg.GeoIPDatasetSources = map[string]GeoIPDatasetSource{
		"foreign":          {Provider: "ip2location", URL: "https://mirror.example.com/foreign.mmdb"},
		"backup_ip2region": {Provider: "ip2region", ChecksumURL: "https://mirror.example.com/ip2region.sha256"},
	}
	got := cfg.SelectedGeoIPDatasets()
	var ids []string
	for _, item := range got {
		ids = append(ids, item.ID)
	}
	want := []string{"primary", "online_ipinfo", "foreign", "backup_ip2region", "online_amap", "backup_maxmind"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("dataset IDs = %v, want %v", ids, want)
	}
	if got[3].Path != filepath.Join(cfg.DataDir, "geoip-backup-ip2region.xdb") || got[3].Operation != "geoip_backup_ip2region" {
		t.Fatalf("ip2region backup = %#v", got[3])
	}
	if got[2].UpdateURL != "https://mirror.example.com/foreign.mmdb" || got[3].ChecksumURL != "https://mirror.example.com/ip2region.sha256" {
		t.Fatalf("dataset sources = %#v, %#v", got[2], got[3])
	}
	if !reflect.DeepEqual(got[1].Roles, []string{"domestic_b", "foreign_b"}) {
		t.Fatalf("shared online roles = %v", got[1].Roles)
	}
}
