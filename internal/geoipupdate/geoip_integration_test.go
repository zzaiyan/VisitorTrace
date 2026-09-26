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
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/geoiponline"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

// The integration configuration keeps credentials and test addresses apart:
// every configured provider is paired with every address in the shared list,
// so one run compares how each backend geolocates the same inputs. Providers
// and services whose credentials are empty are skipped, so the file can carry
// the full matrix at all times.
type integrationConfig struct {
	IPs       []integrationAddress        `json:"ips"`
	Providers map[string]integrationProvider `json:"providers"`
	Online    map[string]integrationOnline   `json:"online"`
}

type integrationAddress struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code"`
	RegionCode  string `json:"region_code"`
	City        string `json:"city"`
}

type integrationProvider struct {
	Download    bool   `json:"download"`
	DownloadURL string `json:"download_url"`
	DatabasePath string `json:"database_path"`
	AccountID   string `json:"account_id"`
	LicenseKey  string `json:"license_key"`
	Token       string `json:"token"`
}

type integrationOnline struct {
	Key string   `json:"key"`
	SK  string   `json:"sk"`
	IPs []string `json:"ips"`
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
	if len(cfg.IPs) == 0 {
		t.Fatal("GeoIP integration config has no test addresses")
	}
	addresses := make([]netip.Addr, len(cfg.IPs))
	for index, entry := range cfg.IPs {
		parsed, err := netip.ParseAddr(entry.IP)
		if err != nil {
			t.Fatalf("parse test IP %q: %v", entry.IP, err)
		}
		addresses[index] = parsed
	}
	if len(cfg.Providers) == 0 && len(cfg.Online) == 0 {
		t.Fatal("GeoIP integration config has no providers and no online services")
	}
	for provider := range cfg.Providers {
		if provider != "dbip" && provider != "maxmind" && provider != "ip2location" && provider != "ip2region" {
			t.Errorf("unsupported provider %q in integration config", provider)
		}
	}
	for service := range cfg.Online {
		if _, err := geoiponline.NormalizeProvider(service); err != nil {
			t.Errorf("unsupported online service %q in integration config", service)
		}
	}
	for _, provider := range []string{"dbip", "maxmind", "ip2location", "ip2region"} {
		fixture, ok := cfg.Providers[provider]
		if !ok {
			continue
		}
		if fixture.Download && missingProviderCredential(provider, fixture) {
			t.Run(provider, func(t *testing.T) {
				t.Skipf("provider %s has no credentials configured", provider)
			})
			continue
		}
		t.Run(provider, func(t *testing.T) {
			databasePath := strings.TrimSpace(fixture.DatabasePath)
			if databasePath == "" && fixture.Download {
				directory := t.TempDir()
				t.Run("official_update", func(t *testing.T) {
					databasePath = downloadConfiguredDatabase(t, directory, provider, fixture)
				})
			}
			if databasePath == "" {
				t.Skipf("provider %s has no database: enable download or set database_path", provider)
			}
			resolver, err := geoip.OpenWithProvider(provider, databasePath)
			if err != nil {
				t.Fatalf("open %s database: %v", provider, err)
			}
			defer resolver.Close()
			for index, entry := range cfg.IPs {
				address := entry
				t.Run(address.IP, func(t *testing.T) {
					location := resolver.Lookup(addresses[index])
					if location.CountryCode == "" && location.City == "" && location.Latitude == nil {
						t.Skipf("%s returned no location for %s", provider, address.IP)
					}
					t.Logf("location = %+v", location)
					assertExpectedLocation(t, location, address)
				})
			}
		})
	}
	for _, service := range []string{"tencent", "amap", "ipinfo", "bigdatacloud"} {
		fixture, ok := cfg.Online[service]
		if !ok {
			continue
		}
		if strings.TrimSpace(fixture.Key) == "" {
			t.Run("online/"+service, func(t *testing.T) {
				t.Skipf("online service %s has no key configured", service)
			})
			continue
		}
		t.Run("online/"+service, func(t *testing.T) {
			credential := fixture.Key
			if fixture.SK != "" {
				credential = fixture.Key + ":" + fixture.SK
			}
			client, err := geoiponline.New(service, credential, 10*time.Second)
			if err != nil {
				t.Fatalf("create online client %s: %v", service, err)
			}
			for index, entry := range cfg.IPs {
				address := entry
				if len(fixture.IPs) > 0 && !containsAddress(fixture.IPs, address.IP) {
					continue
				}
				if service == "tencent" || service == "amap" {
					time.Sleep(350 * time.Millisecond)
				}
				t.Run(address.IP, func(t *testing.T) {
					location, err := client.Lookup(context.Background(), addresses[index])
					if err != nil {
						t.Fatalf("online lookup %s(%s): %v", service, address.IP, err)
					}
					if location.CountryCode == "" && location.City == "" && location.Latitude == nil {
						t.Skipf("online lookup %s(%s) returned no location", service, address.IP)
					}
					t.Logf("location = %+v", location)
					assertExpectedLocation(t, location, address)
				})
			}
		})
	}
}

// missingProviderCredential reports whether a provider entry cannot run
// because its required download credentials have not been filled in yet.
func missingProviderCredential(provider string, fixture integrationProvider) bool {
	switch provider {
	case "maxmind":
		return fixture.AccountID == "" || fixture.LicenseKey == ""
	case "ip2location":
		return fixture.Token == ""
	default:
		return false
	}
}

// downloadConfiguredDatabase runs one official update into the given
// directory and returns the activated database path. The directory must
// outlive the download subtest it runs in.
func downloadConfiguredDatabase(t *testing.T, dir string, provider string, fixture integrationProvider) string {
	t.Helper()
	cfg := config.Default(dir)
	cfg.GeoIPProvider = provider
	cfg.GeoIPUpdate = "automatic"
	cfg.GeoIPPath = filepath.Join(dir, "geoip.db")
	profile, err := geoip.UpdateProfileForProvider(provider)
	if err != nil {
		t.Fatalf("provider profile %s: %v", provider, err)
	}
	cfg.GeoIPUpdateURL = profile.URL
	if strings.TrimSpace(fixture.DownloadURL) != "" {
		cfg.GeoIPUpdateURL = strings.TrimSpace(fixture.DownloadURL)
	}
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
	return cfg.GeoIPPath
}

func containsAddress(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertExpectedLocation(t *testing.T, got geoip.Location, want integrationAddress) {
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
