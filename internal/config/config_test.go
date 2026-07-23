package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visitortrace.json")
	want := Default(filepath.Join(dir, "data"))
	want.Listen = "127.0.0.1:9876"
	want.TrustedProxies = []string{"127.0.0.1/32"}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != want.Version || got.DataDir != want.DataDir || got.Listen != want.Listen {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, want)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat config: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestSaveProtectsConfigDirectoryAndFileOnlyWhenNeeded(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "config")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "visitortrace.json")
	cfg := Default(filepath.Join(root, "data"))
	if err := Save(path, cfg); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if info, err := os.Stat(directory); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o700 {
		t.Fatalf("config directory permissions = %o, want 700", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visitortrace.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"data_dir":"/tmp/data","database_path":"/tmp/data.db","geoip_path":"/tmp/geoip.mmdb","listen":"127.0.0.1:8790","unexpected":true}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted an unknown field")
	}
}

func TestLoadDefaultsBackupDirectoryForExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visitortrace.json")
	dataDir := filepath.Join(t.TempDir(), "data")
	value := `{"version":1,"data_dir":"` + dataDir + `","database_path":"` + filepath.Join(dataDir, "visitortrace.sqlite3") + `","geoip_path":"` + filepath.Join(dataDir, "geoip.mmdb") + `","listen":"127.0.0.1:8790"}`
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.BackupDir != filepath.Join(dataDir, "backups") {
		t.Fatalf("BackupDir = %q", got.BackupDir)
	}
	if got.GeoIPUpdate != "automatic" || !strings.Contains(got.GeoIPUpdateURL, "{YYYY-MM}") {
		t.Fatalf("GeoIP update defaults = %q, %q", got.GeoIPUpdate, got.GeoIPUpdateURL)
	}
	if !strings.Contains(got.UpdateManifestURL, "VisitorTrace/releases/latest") {
		t.Fatalf("UpdateManifestURL = %q", got.UpdateManifestURL)
	}
}

func TestProvidersDefaultToAutomaticOfficialUpdates(t *testing.T) {
	for _, provider := range []string{"dbip", "maxmind", "ip2location"} {
		t.Run(provider, func(t *testing.T) {
			cfg := Default(filepath.Join(t.TempDir(), "data"))
			cfg.GeoIPProvider = provider
			cfg.GeoIPUpdate = ""
			cfg.MaxMindAccountID = "account"
			cfg.MaxMindLicenseKey = "license"
			cfg.IP2LocationToken = "token"
			path := filepath.Join(t.TempDir(), "visitortrace.json")
			if err := Save(path, cfg); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			profile, _ := geoip.UpdateProfileForProvider(provider)
			if got.GeoIPProvider != provider || got.GeoIPUpdate != "automatic" || got.GeoIPUpdateURL != profile.URL {
				t.Fatalf("provider defaults = %#v", got)
			}
		})
	}
}

func TestOfficialProviderUpdatesRequireCredentials(t *testing.T) {
	for _, provider := range []string{"maxmind", "ip2location"} {
		t.Run(provider, func(t *testing.T) {
			cfg := Default(t.TempDir())
			cfg.GeoIPProvider = provider
			cfg.GeoIPUpdateURL = ""
			cfg.applyDefaults()
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted an official source without credentials")
			}

			cfg.GeoIPUpdateURL = "https://mirror.example.com/city.mmdb"
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected a credential-free custom mirror: %v", err)
			}
		})
	}
}

func TestLegacyMonthlyUpdateModeMigratesToAutomatic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visitortrace.json")
	cfg := Default(filepath.Join(dir, "data"))
	cfg.GeoIPUpdate = "monthly"
	data := `{"version":1,"data_dir":"` + cfg.DataDir + `","database_path":"` + cfg.DatabasePath + `","geoip_path":"` + cfg.GeoIPPath + `","geoip_provider":"dbip","geoip_update":"monthly","listen":"127.0.0.1:8790"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.GeoIPUpdate != "automatic" {
		t.Fatalf("GeoIPUpdate = %q", got.GeoIPUpdate)
	}
}

func TestValidateRejectsUnknownGeoIPProvider(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPProvider = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an unknown GeoIP provider")
	}
}

func TestValidateRejectsInsecureRemoteGeoIPSource(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPUpdateURL = "http://example.com/geoip.mmdb.gz"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an insecure remote GeoIP source")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{" https://stats.example.com/visitortrace/ ", "https://stats.example.com/visitortrace"},
		{"HTTP://localhost", "http://localhost"},
		{"https://stats.example.com/a/../visitortrace", "https://stats.example.com/visitortrace"},
	}
	for _, test := range tests {
		got, err := NormalizeBaseURL(test.input)
		if err != nil {
			t.Errorf("NormalizeBaseURL(%q) error = %v", test.input, err)
			continue
		}
		if got != test.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	if got := BasePath("https://stats.example.com/visitortrace"); got != "/visitortrace" {
		t.Fatalf("BasePath() = %q", got)
	}
}

func TestNormalizeBaseURLRejectsUnsupportedComponents(t *testing.T) {
	for _, input := range []string{
		"stats.example.com/visitortrace",
		"ftp://stats.example.com/visitortrace",
		"https://user:pass@stats.example.com/visitortrace",
		"https://stats.example.com/visitortrace?debug=1",
		"https://stats.example.com/visitortrace#section",
	} {
		if _, err := NormalizeBaseURL(input); err == nil {
			t.Errorf("NormalizeBaseURL(%q) accepted an invalid URL", input)
		}
	}
}
