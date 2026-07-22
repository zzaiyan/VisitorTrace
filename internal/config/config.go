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
)

const CurrentVersion = 1

type Config struct {
	Version           int      `json:"version"`
	DataDir           string   `json:"data_dir"`
	DatabasePath      string   `json:"database_path"`
	GeoIPPath         string   `json:"geoip_path"`
	GeoIPProvider     string   `json:"geoip_provider,omitempty"`
	GeoIPUpdate       string   `json:"geoip_update,omitempty"`
	GeoIPUpdateURL    string   `json:"geoip_update_url,omitempty"`
	GeoIPChecksumURL  string   `json:"geoip_checksum_url,omitempty"`
	MaxMindAccountID  string   `json:"maxmind_account_id,omitempty"`
	MaxMindLicenseKey string   `json:"maxmind_license_key,omitempty"`
	IP2LocationToken  string   `json:"ip2location_download_token,omitempty"`
	BackupDir         string   `json:"backup_dir,omitempty"`
	UpdateManifestURL string   `json:"update_manifest_url,omitempty"`
	Listen            string   `json:"listen"`
	BaseURL           string   `json:"base_url,omitempty"`
	TrustedProxies    []string `json:"trusted_proxies,omitempty"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("protect config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
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
	return os.Chmod(path, 0o600)
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
	if c.GeoIPUpdate == "automatic" && strings.TrimSpace(c.GeoIPUpdateURL) == "" {
		return errors.New("geoip_update_url is required when GeoIP updates are enabled")
	}
	profile, err := geoip.UpdateProfileForProvider(c.GeoIPProvider)
	if err != nil {
		return err
	}
	if c.GeoIPUpdate == "automatic" && c.GeoIPUpdateURL == profile.URL {
		switch geoip.Provider(c.GeoIPProvider) {
		case geoip.ProviderMaxMind:
			if c.MaxMindAccountID == "" || c.MaxMindLicenseKey == "" {
				return errors.New("maxmind_account_id and maxmind_license_key are required for the official MaxMind update source")
			}
		case geoip.ProviderIP2Location:
			if c.IP2LocationToken == "" {
				return errors.New("ip2location_download_token is required for the official IP2Location update source")
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
	if strings.TrimSpace(c.GeoIPProvider) == "" {
		c.GeoIPProvider = string(geoip.ProviderDBIP)
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
