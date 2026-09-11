//go:build geoip_integration

package geoipupdate

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

type integrationConfig struct {
	Providers map[string]integrationProvider `json:"providers"`
}

type integrationProvider struct {
	DatabasePath string           `json:"database_path"`
	IP           string           `json:"ip"`
	Download     bool             `json:"download"`
	AccountID    string           `json:"account_id"`
	LicenseKey   string           `json:"license_key"`
	Token        string           `json:"token"`
	Expected     expectedLocation `json:"expected"`
}

type expectedLocation struct {
	CountryCode string `json:"country_code"`
	RegionCode  string `json:"region_code"`
	City        string `json:"city"`
}

func TestConfiguredGeoIPProviders(t *testing.T) {
	path := os.Getenv("VISITORTRACE_GEOIP_TEST_CONFIG")
	if path == "" {
		path = ".geoip-test.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read GeoIP integration config %q: %v", path, err)
	}
	var cfg integrationConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatalf("decode GeoIP integration config: %v", err)
	}
	if len(cfg.Providers) == 0 {
		t.Fatal("GeoIP integration config has no providers")
	}
	for provider := range cfg.Providers {
		if provider != "dbip" && provider != "maxmind" && provider != "ip2location" {
			t.Errorf("unsupported provider %q in integration config", provider)
		}
	}
	for _, provider := range []string{"dbip", "maxmind", "ip2location"} {
		fixture, ok := cfg.Providers[provider]
		if !ok {
			continue
		}
		t.Run(provider, func(t *testing.T) {
			if fixture.DatabasePath != "" {
				t.Run("existing_database", func(t *testing.T) {
					validateConfiguredDatabase(t, provider, fixture)
				})
			} else if !fixture.Download {
				t.Fatal("database_path is required when download is false")
			}
			if fixture.Download {
				t.Run("official_update", func(t *testing.T) {
					validateConfiguredDownload(t, provider, fixture)
				})
			}
		})
	}
}

func validateConfiguredDatabase(t *testing.T, provider string, fixture integrationProvider) {
	t.Helper()
	if err := geoip.ValidateWithProvider(provider, fixture.DatabasePath); err != nil {
		t.Fatalf("ValidateWithProvider(%s) error = %v", provider, err)
	}
	resolver, err := geoip.OpenWithProvider(provider, fixture.DatabasePath)
	if err != nil {
		t.Fatalf("OpenWithProvider(%s) error = %v", provider, err)
	}
	defer resolver.Close()
	address, err := netip.ParseAddr(fixture.IP)
	if err != nil {
		t.Fatalf("parse test IP %q: %v", fixture.IP, err)
	}
	location := resolver.Lookup(address)
	if location.CountryCode == "" && location.City == "" && location.Latitude == nil {
		t.Fatalf("Lookup(%s, %s) returned an empty location", provider, fixture.IP)
	}
	assertExpectedLocation(t, location, fixture.Expected)
}

func validateConfiguredDownload(t *testing.T, provider string, fixture integrationProvider) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default(dir)
	cfg.GeoIPProvider = provider
	cfg.GeoIPUpdate = "automatic"
	cfg.GeoIPPath = filepath.Join(dir, "geoip.mmdb")
	profile, err := geoip.UpdateProfileForProvider(provider)
	if err != nil {
		t.Fatalf("provider profile %s: %v", provider, err)
	}
	cfg.GeoIPUpdateURL = profile.URL
	cfg.GeoIPChecksumURL = ""
	cfg.MaxMindAccountID = fixture.AccountID
	cfg.MaxMindLicenseKey = fixture.LicenseKey
	cfg.IP2LocationToken = fixture.Token
	if err := cfg.Validate(); err != nil {
		t.Fatalf("provider configuration is invalid: %v", err)
	}
	st, err := store.Initialize(context.Background(), cfg.DatabasePath, "integration-test")
	if err != nil {
		t.Fatalf("initialize integration store: %v", err)
	}
	defer st.Close()
	runner := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := runner.RunOnce(context.Background(), true)
	if err != nil {
		t.Fatalf("download and activate %s: %v", provider, err)
	}
	if !result.Updated {
		t.Fatal("download completed without activating a database")
	}
	if err := geoip.ValidateWithProvider(provider, cfg.GeoIPPath); err != nil {
		t.Fatalf("validate activated %s database: %v", provider, err)
	}
	resolver, err := geoip.OpenWithProvider(provider, cfg.GeoIPPath)
	if err != nil {
		t.Fatalf("open activated %s database: %v", provider, err)
	}
	defer resolver.Close()
	address, err := netip.ParseAddr(fixture.IP)
	if err != nil {
		t.Fatalf("parse test IP %q: %v", fixture.IP, err)
	}
	location := resolver.Lookup(address)
	if location.CountryCode == "" && location.City == "" && location.Latitude == nil {
		t.Fatalf("activated %s database returned an empty location", provider)
	}
	assertExpectedLocation(t, location, fixture.Expected)
}

func assertExpectedLocation(t *testing.T, got geoip.Location, want expectedLocation) {
	t.Helper()
	if want.CountryCode != "" && !strings.EqualFold(got.CountryCode, want.CountryCode) {
		t.Errorf("country_code = %q, want %q", got.CountryCode, want.CountryCode)
	}
	if want.RegionCode != "" && !strings.EqualFold(got.RegionCode, want.RegionCode) {
		t.Errorf("region_code = %q, want %q", got.RegionCode, want.RegionCode)
	}
	if want.City != "" && got.City != want.City {
		t.Errorf("city = %q, want %q", got.City, want.City)
	}
}
