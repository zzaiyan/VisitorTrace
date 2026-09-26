package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/geoiponline"
)

const CurrentVersion = 1

// OnlineServiceConfig holds the credentials for one online lookup service.
type OnlineServiceConfig struct {
	Key string `json:"key"`
	SK  string `json:"sk,omitempty"`
}

type GeoIPDatasetSource struct {
	Provider    string `json:"provider"`
	URL         string `json:"url,omitempty"`
	ChecksumURL string `json:"checksum_url,omitempty"`
}

// GeoIPPresetFromProvider derives the preset name from a legacy provider
// configuration so existing installs migrate transparently.
func GeoIPPresetFromProvider(provider string) string {
	switch provider {
	case string(geoip.ProviderIP2Region):
		return "recommended"
	default:
		return "basic"
	}
}

type Config struct {
	Version              int                            `json:"version"`
	DataDir              string                         `json:"data_dir"`
	DatabasePath         string                         `json:"database_path"`
	GeoIPPath            string                         `json:"geoip_path"`
	GeoIPPreset          string                         `json:"geoip_preset,omitempty"`
	GeoIPProvider        string                         `json:"geoip_provider,omitempty"`
	GeoIPBasicBackend    string                         `json:"geoip_basic_backend,omitempty"`
	GeoIPDomesticOffline string                         `json:"geoip_domestic_offline,omitempty"`
	GeoIPDomesticOnline  string                         `json:"geoip_domestic_online,omitempty"`
	GeoIPForeignOffline  string                         `json:"geoip_foreign_offline,omitempty"`
	GeoIPForeignOnline   string                         `json:"geoip_foreign_online,omitempty"`
	GeoIPBackups         []string                       `json:"geoip_backups,omitempty"`
	GeoIPUpdate          string                         `json:"geoip_update,omitempty"`
	GeoIPUpdateURL       string                         `json:"geoip_update_url,omitempty"`
	GeoIPChecksumURL     string                         `json:"geoip_checksum_url,omitempty"`
	GeoIPDatasetSources  map[string]GeoIPDatasetSource  `json:"geoip_dataset_sources,omitempty"`
	MaxMindAccountID     string                         `json:"maxmind_account_id,omitempty"`
	MaxMindLicenseKey    string                         `json:"maxmind_license_key,omitempty"`
	IP2LocationToken     string                         `json:"ip2location_download_token,omitempty"`
	OnlineServices       map[string]OnlineServiceConfig `json:"online_services,omitempty"`
	GeoIPForeignPath     string                         `json:"geoip_foreign_path,omitempty"`
	BackupDir            string                         `json:"backup_dir,omitempty"`
	UpdateManifestURL    string                         `json:"update_manifest_url,omitempty"`
	Listen               string                         `json:"listen"`
	BaseURL              string                         `json:"base_url,omitempty"`
	TrustedProxies       []string                       `json:"trusted_proxies,omitempty"`
}

func DefaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "visitortrace", "config.json")
}

func DefaultDataDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = os.Getenv("HOME")
	}
	return filepath.Join(dir, ".local", "share", "visitortrace")
}

func Default(dataDir string) Config {
	profile, _ := geoip.UpdateProfileForProvider(string(geoip.ProviderDBIP))
	return Config{
		Version:           CurrentVersion,
		DataDir:           dataDir,
		DatabasePath:      filepath.Join(dataDir, "visitortrace.sqlite3"),
		GeoIPPath:         filepath.Join(dataDir, "geoip.mmdb"),
		GeoIPProvider:     string(geoip.ProviderDBIP),
		GeoIPUpdate:       "automatic",
		GeoIPUpdateURL:    profile.URL,
		BackupDir:         filepath.Join(dataDir, "backups"),
		UpdateManifestURL: "https://github.com/zzaiyan/VisitorTrace/releases/latest/download/manifest.json",
		Listen:            "127.0.0.1:8790",
	}
}

func Load(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, fmt.Errorf("stat config: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return Config{}, fmt.Errorf("config permissions %o are too broad; want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("decode config: trailing content")
	}
	cfg.applyDefaults()
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.applyDefaults()
	if err := cfg.normalize(); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := ensureMode(directory, 0o700); err != nil {
		return fmt.Errorf("protect config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect temporary config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("activate config: %w", err)
	}
	if err := ensureMode(path, 0o600); err != nil {
		return fmt.Errorf("protect config file: %w", err)
	}
	return nil
}

func ensureMode(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() == mode.Perm() {
		return nil
	}
	return os.Chmod(path, mode)
}

func (c Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.DataDir == "" || c.DatabasePath == "" || c.GeoIPPath == "" || c.BackupDir == "" {
		return errors.New("data_dir, database_path, geoip_path, and backup_dir are required")
	}
	provider, err := geoip.NormalizeProvider(c.GeoIPProvider)
	if err != nil {
		return err
	}
	if c.GeoIPProvider != provider {
		return fmt.Errorf("geoip_provider must be one of dbip, maxmind, or ip2location")
	}
	if c.Listen == "" {
		return errors.New("listen is required")
	}
	if _, err := NormalizeBaseURL(c.BaseURL); err != nil {
		return err
	}
	for _, value := range c.TrustedProxies {
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", value)
		}
	}
	if c.GeoIPUpdate != "automatic" && c.GeoIPUpdate != "disabled" {
		return fmt.Errorf("geoip_update must be automatic or disabled")
	}
	if c.GeoIPPreset == "" {
		c.GeoIPPreset = GeoIPPresetFromProvider(c.GeoIPProvider)
	}
	switch c.GeoIPPreset {
	case "basic", "recommended", "precise":
	case "":
		c.GeoIPPreset = "basic"
	default:
		return fmt.Errorf("geoip_preset must be basic, recommended, or precise (got %q)", c.GeoIPPreset)
	}
	validateBackend := func(role, name string) error {
		if _, err := geoip.NormalizeProvider(name); err == nil {
			return nil
		}
		if _, err := geoiponline.NormalizeProvider(name); err != nil {
			return fmt.Errorf("%s: %w", role, err)
		}
		if c.OnlineServices[name].Key == "" {
			return fmt.Errorf("online service %s needs a key", name)
		}
		return nil
	}
	if c.GeoIPPreset == "basic" {
		basic := c.GeoIPBasicBackend
		if basic == "" {
			basic = c.GeoIPProvider
		}
		if basic == "ip2region" {
			return errors.New("ip2region cannot be the only GeoIP database because it has no coordinates")
		}
		if err := validateBackend("basic primary", basic); err != nil {
			return err
		}
	}
	if c.GeoIPPreset == "recommended" {
		domestic := c.GeoIPDomesticOffline
		if domestic == "" {
			domestic = "ipinfo"
		}
		if domestic == "ip2region" {
			return errors.New("ip2region cannot be the sole domestic GeoIP primary because it has no coordinates")
		}
		foreign := c.GeoIPForeignOffline
		if foreign == "" {
			foreign = "ip2location"
		}
		if foreign == "ip2region" {
			return errors.New("ip2region cannot be the sole foreign GeoIP primary because it has no coordinates")
		}
		if domestic == foreign {
			return errors.New("domestic and foreign primaries must use different GeoIP backends")
		}
		for role, name := range map[string]string{"domestic primary": domestic, "foreign primary": foreign} {
			if err := validateBackend(role, name); err != nil {
				return err
			}
		}
	}
	if c.GeoIPPreset == "precise" {
		if c.GeoIPDomesticOnline == "" || c.GeoIPForeignOnline == "" {
			return errors.New("precise GeoIP requires two primary databases in each branch")
		}
		if len(c.GeoIPBackups) == 0 {
			return errors.New("precise GeoIP requires at least one backup database")
		}
		domestic := c.GeoIPDomesticOffline
		if domestic == "" {
			domestic = "ip2region"
		}
		foreign := c.GeoIPForeignOffline
		if foreign == "" {
			foreign = "ip2location"
		}
		if domestic == c.GeoIPDomesticOnline || foreign == c.GeoIPForeignOnline {
			return errors.New("each GeoIP branch needs two different primary backends")
		}
		primaries := map[string]bool{}
		for role, name := range map[string]string{"domestic primary A": domestic, "domestic primary B": c.GeoIPDomesticOnline, "foreign primary A": foreign, "foreign primary B": c.GeoIPForeignOnline} {
			_, offlineErr := geoip.NormalizeProvider(name)
			if primaries[name] && offlineErr == nil {
				return fmt.Errorf("GeoIP primary %s is selected more than once", name)
			}
			primaries[name] = true
			if err := validateBackend(role, name); err != nil {
				return err
			}
		}
	}
	primaries := map[string]bool{}
	if c.GeoIPPreset == "precise" {
		domestic := c.GeoIPDomesticOffline
		if domestic == "" {
			domestic = "ip2region"
		}
		foreign := c.GeoIPForeignOffline
		if foreign == "" {
			foreign = "ip2location"
		}
		for _, name := range []string{domestic, c.GeoIPDomesticOnline, foreign, c.GeoIPForeignOnline} {
			if name != "" {
				primaries[name] = true
			}
		}
	}
	backups := map[string]bool{}
	for _, name := range c.GeoIPBackups {
		switch name {
		case "dbip", "maxmind", "ip2location", "ip2region", "bigdatacloud", "amap", "tencent", "ipinfo":
		default:
			return fmt.Errorf("invalid GeoIP backup %q", name)
		}
		if backups[name] || primaries[name] {
			return fmt.Errorf("GeoIP backup %q is duplicated or already selected as a primary", name)
		}
		backups[name] = true
	}
	for name, svc := range c.OnlineServices {
		if strings.ContainsAny(svc.Key, "\r\n") || strings.ContainsAny(svc.SK, "\r\n") {
			return fmt.Errorf("online_services[%s] must not contain line breaks", name)
		}
	}
	for id, source := range c.GeoIPDatasetSources {
		if id != "foreign" && id != "domestic_b" && id != "foreign_b" && id != "backup_"+source.Provider {
			return fmt.Errorf("invalid GeoIP dataset source %q", id)
		}
		if _, err := geoip.NormalizeProvider(source.Provider); err != nil {
			return fmt.Errorf("invalid GeoIP dataset source %q: %w", id, err)
		}
	}
	for _, dataset := range c.SelectedGeoIPDatasets() {
		if dataset.Online {
			if c.OnlineServices[dataset.Provider].Key == "" {
				return fmt.Errorf("online service %s needs a key", dataset.Provider)
			}
			continue
		}
		if c.GeoIPUpdate == "automatic" {
			if strings.TrimSpace(dataset.UpdateURL) == "" {
				return fmt.Errorf("update URL is required for %s", dataset.Provider)
			}
			profile, _ := geoip.UpdateProfileForProvider(dataset.Provider)
			if dataset.UpdateURL != profile.URL {
				continue
			}
			switch dataset.Provider {
			case "maxmind":
				if c.MaxMindAccountID == "" || c.MaxMindLicenseKey == "" {
					return errors.New("maxmind_account_id and maxmind_license_key are required for the official MaxMind update source")
				}
			case "ip2location":
				if c.IP2LocationToken == "" {
					return errors.New("ip2location_download_token is required for the official IP2Location update source")
				}
			}
		}
	}
	for name, value := range map[string]string{"maxmind_account_id": c.MaxMindAccountID, "maxmind_license_key": c.MaxMindLicenseKey, "ip2location_download_token": c.IP2LocationToken} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s must not contain line breaks", name)
		}
	}
	for name, value := range map[string]string{"geoip_update_url": c.GeoIPUpdateURL, "geoip_checksum_url": c.GeoIPChecksumURL, "update_manifest_url": c.UpdateManifestURL} {
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return fmt.Errorf("%s must be an absolute URL", name)
		}
		if parsed.User != nil {
			return fmt.Errorf("%s must not contain credentials", name)
		}
		host := strings.ToLower(parsed.Hostname())
		loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
			return fmt.Errorf("%s must use HTTPS except on loopback", name)
		}
	}
	for id, source := range c.GeoIPDatasetSources {
		for field, value := range map[string]string{"url": source.URL, "checksum_url": source.ChecksumURL} {
			if value == "" {
				continue
			}
			parsed, err := url.Parse(value)
			if err != nil || parsed.Host == "" || parsed.User != nil {
				return fmt.Errorf("geoip_dataset_sources[%s].%s must be an absolute URL without credentials", id, field)
			}
			host := strings.ToLower(parsed.Hostname())
			loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
			if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
				return fmt.Errorf("geoip_dataset_sources[%s].%s must use HTTPS except on loopback", id, field)
			}
		}
	}
	return nil
}

// NormalizeBaseURL validates and canonicalizes the public application URL.
// Its optional path is also the HTTP route prefix used by the server.
func NormalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Opaque != "" {
		return "", errors.New("base_url must be an absolute HTTP or HTTPS URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base_url must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("base_url must not contain credentials, a query, or a fragment")
	}
	if strings.ContainsAny(parsed.Path, "\\\x00") {
		return "", errors.New("base_url contains an invalid path")
	}
	cleanPath := path.Clean("/" + strings.TrimPrefix(parsed.Path, "/"))
	if cleanPath == "/" {
		cleanPath = ""
	}
	parsed.Path = cleanPath
	parsed.RawPath = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func BasePath(baseURL string) string {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil || normalized == "" {
		return ""
	}
	parsed, _ := url.Parse(normalized)
	return parsed.Path
}

func (c *Config) normalize() error {
	provider, err := geoip.NormalizeProvider(c.GeoIPProvider)
	if err != nil {
		return err
	}
	c.GeoIPProvider = provider
	if c.GeoIPUpdate == "monthly" {
		c.GeoIPUpdate = "automatic"
	}
	c.MaxMindAccountID = strings.TrimSpace(c.MaxMindAccountID)
	c.MaxMindLicenseKey = strings.TrimSpace(c.MaxMindLicenseKey)
	c.IP2LocationToken = strings.TrimSpace(c.IP2LocationToken)
	baseURL, err := NormalizeBaseURL(c.BaseURL)
	if err != nil {
		return err
	}
	c.BaseURL = baseURL
	return nil
}

func (c *Config) applyDefaults() {
	if c.GeoIPPreset == "" {
		c.GeoIPPreset = GeoIPPresetFromProvider(c.GeoIPProvider)
	}
	if strings.TrimSpace(c.GeoIPProvider) == "" {
		c.GeoIPProvider = string(geoip.ProviderDBIP)
	}
	if c.GeoIPPreset == "basic" && c.GeoIPBasicBackend == "" {
		c.GeoIPBasicBackend = c.GeoIPProvider
	}
	if c.GeoIPPreset == "basic" {
		if basic, err := geoip.NormalizeProvider(c.GeoIPBasicBackend); err == nil {
			c.GeoIPProvider = basic
			if basic == string(geoip.ProviderIP2Region) && c.GeoIPPath == filepath.Join(c.DataDir, "geoip.mmdb") {
				c.GeoIPPath = filepath.Join(c.DataDir, "geoip.xdb")
			} else if basic != string(geoip.ProviderIP2Region) && c.GeoIPPath == filepath.Join(c.DataDir, "geoip.xdb") {
				c.GeoIPPath = filepath.Join(c.DataDir, "geoip.mmdb")
			}
		}
	}
	if c.GeoIPPreset == "recommended" || c.GeoIPPreset == "precise" {
		if c.GeoIPDomesticOffline == "" {
			if c.GeoIPPreset == "recommended" {
				c.GeoIPDomesticOffline = "ipinfo"
			} else {
				c.GeoIPDomesticOffline = string(geoip.ProviderIP2Region)
			}
		}
		if c.GeoIPForeignOffline == "" {
			c.GeoIPForeignOffline = string(geoip.ProviderIP2Location)
		}
		if domestic, err := geoip.NormalizeProvider(c.GeoIPDomesticOffline); err == nil {
			c.GeoIPProvider = domestic
			if domestic == string(geoip.ProviderIP2Region) && c.GeoIPPath == filepath.Join(c.DataDir, "geoip.mmdb") {
				c.GeoIPPath = filepath.Join(c.DataDir, "geoip.xdb")
			} else if domestic != string(geoip.ProviderIP2Region) && c.GeoIPPath == filepath.Join(c.DataDir, "geoip.xdb") {
				c.GeoIPPath = filepath.Join(c.DataDir, "geoip.mmdb")
			}
		}
	}
	if c.BackupDir == "" && c.DataDir != "" {
		c.BackupDir = filepath.Join(c.DataDir, "backups")
	}
	if c.GeoIPUpdate == "" {
		c.GeoIPUpdate = "automatic"
	}
	profile, err := geoip.UpdateProfileForProvider(c.GeoIPProvider)
	if err == nil && (c.GeoIPUpdateURL == "" || geoip.IsDefaultUpdateURL(c.GeoIPUpdateURL)) {
		c.GeoIPUpdateURL = profile.URL
	}
	if c.UpdateManifestURL == "" {
		c.UpdateManifestURL = "https://github.com/zzaiyan/VisitorTrace/releases/latest/download/manifest.json"
	}
}
