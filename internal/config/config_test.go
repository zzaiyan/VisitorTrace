package config

import (
	"os"
	"path/filepath"
	"reflect"
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
	if got.GeoIPPreset != "basic" {
		t.Fatalf("legacy GeoIP preset = %q, want basic", got.GeoIPPreset)
	}
	if got.GeoIPUpdate != "automatic" || !strings.Contains(got.GeoIPUpdateURL, "{YYYY-MM}") {
		t.Fatalf("GeoIP update defaults = %q, %q", got.GeoIPUpdate, got.GeoIPUpdateURL)
	}
	if !strings.Contains(got.UpdateManifestURL, "VisitorTrace/releases/latest") {
		t.Fatalf("UpdateManifestURL = %q", got.UpdateManifestURL)
	}
}

func TestPrecisePresetRequiresCredentialsForSelectedOnlineBackend(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "precise"
	cfg.IP2LocationToken = "download-token"
	cfg.GeoIPDomesticOnline = "ipinfo"
	cfg.GeoIPForeignOnline = "ipinfo"
	cfg.GeoIPBackups = []string{"dbip"}
	cfg.OnlineServices = map[string]OnlineServiceConfig{"amap": {Key: "amap-key"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an unconfigured selected online backend")
	}
	cfg.OnlineServices["ipinfo"] = OnlineServiceConfig{Key: "ipinfo-key"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected configured online backend: %v", err)
	}
}

func TestPreciseBackupListRoundTripAndValidation(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "precise"
	cfg.GeoIPDomesticOffline = "dbip"
	cfg.GeoIPUpdate = "disabled"
	cfg.GeoIPDomesticOnline = "tencent"
	cfg.GeoIPForeignOnline = "ipinfo"
	cfg.OnlineServices = map[string]OnlineServiceConfig{"tencent": {Key: "key", SK: "secret"}, "ipinfo": {Key: "key"}, "amap": {Key: "key", SK: "secret"}, "bigdatacloud": {Key: "key"}}
	cfg.GeoIPBackups = []string{"ip2region", "amap", "maxmind", "bigdatacloud"}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if !reflect.DeepEqual(loaded.GeoIPBackups, cfg.GeoIPBackups) {
		t.Fatalf("backup order = %v", loaded.GeoIPBackups)
	}
	for _, backups := range [][]string{{"dbip"}, {"ip2location"}, {"tencent"}, {"amap", "amap"}, {"unknown"}} {
		cfg.GeoIPBackups = backups
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() accepted backups %v", backups)
		}
	}
	cfg.GeoIPBackups = []string{"maxmind"}
	cfg.GeoIPUpdate = "automatic"
	cfg.IP2LocationToken = "download-token"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted MaxMind backup without credentials")
	}
	cfg.MaxMindAccountID, cfg.MaxMindLicenseKey = "account", "license"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected credentialed backup: %v", err)
	}
}

func TestBasicPresetRejectsIP2RegionAsOnlyDatabase(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "basic"
	cfg.GeoIPProvider = "ip2region"
	cfg.GeoIPUpdate = "disabled"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted ip2region as the only database")
	}
}

func TestRecommendedForeignDatabaseRequiresUpdateCredentials(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "recommended"
	cfg.OnlineServices = map[string]OnlineServiceConfig{"ipinfo": {Key: "token"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted the default IP2Location foreign database without a token")
	}
	cfg.IP2LocationToken = "download-token"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected credentialed foreign database: %v", err)
	}
}

func TestDatasetSourcesRequireSafeURLsAndKnownIDs(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPDatasetSources = map[string]GeoIPDatasetSource{"foreign": {Provider: "ip2location", URL: "https://mirror.example.com/city.mmdb"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	cfg.GeoIPDatasetSources["foreign"] = GeoIPDatasetSource{Provider: "ip2location", URL: "http://mirror.example.com/city.mmdb"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("insecure dataset source accepted")
	}
	cfg.GeoIPDatasetSources = map[string]GeoIPDatasetSource{"backup_wrong": {Provider: "dbip", URL: "https://mirror.example.com/city.mmdb"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown dataset source ID accepted")
	}
}

func TestDatasetCustomMirrorDoesNotRequireOfficialCredentials(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "precise"
	cfg.GeoIPDomesticOnline = "ipinfo"
	cfg.GeoIPForeignOnline = "ipinfo"
	cfg.OnlineServices = map[string]OnlineServiceConfig{"ipinfo": {Key: "token"}}
	cfg.GeoIPBackups = []string{"maxmind"}
	cfg.GeoIPDatasetSources = map[string]GeoIPDatasetSource{
		"foreign":        {Provider: "ip2location", URL: "https://mirror.example.com/foreign.mmdb"},
		"backup_maxmind": {Provider: "maxmind", URL: "https://mirror.example.com/maxmind.mmdb"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("custom mirrors rejected without official credentials: %v", err)
	}
	delete(cfg.GeoIPDatasetSources, "foreign")
	if err := cfg.Validate(); err == nil {
		t.Fatal("official foreign source accepted without token")
	}
}

func TestPrecisePresetRequiresFourPrimariesAndBackup(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "precise"
	cfg.GeoIPUpdate = "disabled"
	cfg.GeoIPDomesticOnline = "tencent"
	cfg.GeoIPForeignOnline = "ipinfo"
	cfg.GeoIPBackups = []string{"dbip"}
	cfg.OnlineServices = map[string]OnlineServiceConfig{"tencent": {Key: "key"}, "ipinfo": {Key: "token"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete precise preset rejected: %v", err)
	}
	missingDomestic := cfg
	missingDomestic.GeoIPDomesticOnline = ""
	if err := missingDomestic.Validate(); err == nil {
		t.Fatal("missing domestic primary B accepted")
	}
	missingForeign := cfg
	missingForeign.GeoIPForeignOnline = ""
	if err := missingForeign.Validate(); err == nil {
		t.Fatal("missing foreign primary B accepted")
	}
	missingBackup := cfg
	missingBackup.GeoIPBackups = nil
	if err := missingBackup.Validate(); err == nil {
		t.Fatal("precise preset without a backup accepted")
	}
	duplicateDomestic := cfg
	duplicateDomestic.GeoIPDomesticOffline = "tencent"
	if err := duplicateDomestic.Validate(); err == nil {
		t.Fatal("duplicate domestic primaries accepted")
	}
}

func TestRecommendedPresetRejectsIP2RegionAsSoleDomesticPrimary(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "recommended"
	cfg.GeoIPUpdate = "disabled"
	cfg.GeoIPDomesticOffline = "ip2region"
	if err := cfg.Validate(); err == nil {
		t.Fatal("recommended preset accepted ip2region without a domestic coordinate source")
	}
}

func TestRecommendedPresetDefaultsToCoordinateBearingPair(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "recommended"
	cfg.GeoIPUpdate = "disabled"
	cfg.OnlineServices = map[string]OnlineServiceConfig{"ipinfo": {Key: "token"}}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GeoIPDomesticOffline != "ipinfo" || loaded.GeoIPForeignOffline != "ip2location" || loaded.GeoIPProvider != "dbip" || loaded.GeoIPPath != filepath.Join(cfg.DataDir, "geoip.mmdb") {
		t.Fatalf("recommended defaults = %#v", loaded)
	}
	datasets := loaded.SelectedGeoIPDatasets()
	if len(datasets) != 2 || datasets[0].ID != "online_ipinfo" || datasets[1].Provider != "ip2location" {
		t.Fatalf("recommended datasets = %#v", datasets)
	}
}

func TestBasicPresetSupportsOnlineProvider(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "basic"
	cfg.GeoIPBasicBackend = "ipinfo"
	if err := cfg.Validate(); err == nil {
		t.Fatal("basic online provider accepted without a key")
	}
	cfg.OnlineServices = map[string]OnlineServiceConfig{"ipinfo": {Key: "token"}}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	datasets := loaded.SelectedGeoIPDatasets()
	if loaded.GeoIPBasicBackend != "ipinfo" || len(datasets) != 1 || datasets[0].ID != "online_ipinfo" || !datasets[0].Online {
		t.Fatalf("basic online selection = %#v; datasets = %#v", loaded, datasets)
	}
}

func TestRecommendedPresetSupportsOnlineForeignPrimary(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "recommended"
	cfg.GeoIPUpdate = "disabled"
	cfg.GeoIPDomesticOffline = "dbip"
	cfg.GeoIPForeignOffline = "ipinfo"
	cfg.OnlineServices = map[string]OnlineServiceConfig{"ipinfo": {Key: "token"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("online foreign primary rejected: %v", err)
	}
	datasets := cfg.SelectedGeoIPDatasets()
	if len(datasets) != 2 || datasets[0].Provider != "dbip" || datasets[1].ID != "online_ipinfo" {
		t.Fatalf("recommended datasets = %#v", datasets)
	}
}

func TestPrecisePresetSupportsOfflineSecondaries(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "precise"
	cfg.GeoIPUpdate = "disabled"
	cfg.GeoIPDomesticOffline = "ip2region"
	cfg.GeoIPDomesticOnline = "dbip"
	cfg.GeoIPForeignOffline = "ip2location"
	cfg.GeoIPForeignOnline = "maxmind"
	cfg.GeoIPBackups = []string{"amap"}
	cfg.OnlineServices = map[string]OnlineServiceConfig{"amap": {Key: "key"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("offline secondaries rejected: %v", err)
	}
	datasets := cfg.SelectedGeoIPDatasets()
	if len(datasets) != 5 || datasets[1].ID != "domestic_b" || datasets[1].Online || datasets[1].Path != filepath.Join(cfg.DataDir, "geoip-domestic-b.mmdb") || datasets[3].ID != "foreign_b" || datasets[3].Online {
		t.Fatalf("precise datasets = %#v", datasets)
	}
}

func TestPrecisePresetRequiresSelectedOnlineBackupKey(t *testing.T) {
	cfg := Default(t.TempDir())
	cfg.GeoIPPreset = "precise"
	cfg.GeoIPUpdate = "disabled"
	cfg.GeoIPDomesticOnline = "tencent"
	cfg.GeoIPForeignOnline = "ipinfo"
	cfg.GeoIPBackups = []string{"amap"}
	cfg.OnlineServices = map[string]OnlineServiceConfig{"tencent": {Key: "key"}, "ipinfo": {Key: "token"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("precise preset accepted an unconfigured online backup")
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
