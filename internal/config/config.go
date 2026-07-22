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
	"path/filepath"
	"runtime"
	"strings"
)

const CurrentVersion = 1

type Config struct {
	Version           int      `json:"version"`
	DataDir           string   `json:"data_dir"`
	DatabasePath      string   `json:"database_path"`
	GeoIPPath         string   `json:"geoip_path"`
	GeoIPUpdate       string   `json:"geoip_update,omitempty"`
	GeoIPUpdateURL    string   `json:"geoip_update_url,omitempty"`
	GeoIPChecksumURL  string   `json:"geoip_checksum_url,omitempty"`
	BackupDir         string   `json:"backup_dir,omitempty"`
	UpdateManifestURL string   `json:"update_manifest_url,omitempty"`
	Listen            string   `json:"listen"`
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
	return Config{
		Version:           CurrentVersion,
		DataDir:           dataDir,
		DatabasePath:      filepath.Join(dataDir, "visitortrace.sqlite3"),
		GeoIPPath:         filepath.Join(dataDir, "geoip.mmdb"),
		GeoIPUpdate:       "monthly",
		GeoIPUpdateURL:    "https://download.db-ip.com/free/dbip-city-lite-{YYYY-MM}.mmdb.gz",
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
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.applyDefaults()
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
	if c.Listen == "" {
		return errors.New("listen is required")
	}
	for _, value := range c.TrustedProxies {
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", value)
		}
	}
	if c.GeoIPUpdate != "monthly" && c.GeoIPUpdate != "disabled" {
		return fmt.Errorf("geoip_update must be monthly or disabled")
	}
	if c.GeoIPUpdate == "monthly" && strings.TrimSpace(c.GeoIPUpdateURL) == "" {
		return errors.New("geoip_update_url is required when GeoIP updates are enabled")
	}
	for name, value := range map[string]string{"geoip_update_url": c.GeoIPUpdateURL, "geoip_checksum_url": c.GeoIPChecksumURL, "update_manifest_url": c.UpdateManifestURL} {
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return fmt.Errorf("%s must be an absolute URL", name)
		}
		host := strings.ToLower(parsed.Hostname())
		loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
			return fmt.Errorf("%s must use HTTPS except on loopback", name)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.BackupDir == "" && c.DataDir != "" {
		c.BackupDir = filepath.Join(c.DataDir, "backups")
	}
	if c.GeoIPUpdate == "" {
		c.GeoIPUpdate = "monthly"
	}
	if c.GeoIPUpdateURL == "" {
		c.GeoIPUpdateURL = "https://download.db-ip.com/free/dbip-city-lite-{YYYY-MM}.mmdb.gz"
	}
	if c.UpdateManifestURL == "" {
		c.UpdateManifestURL = "https://github.com/zzaiyan/VisitorTrace/releases/latest/download/manifest.json"
	}
}
