package server

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"

	"github.com/zzaiyan/VisitorTrace/internal/config"
)

// savedSettings loads the configuration that the next process start will use.
// The active server configuration remains immutable until a supervised restart.
func (s *Server) savedSettings() (config.Config, error) {
	if s.ConfigPath == "" {
		return s.Config, nil
	}
	return config.Load(s.ConfigPath)
}

func (s *Server) effectiveSettings() config.Config {
	if value := s.runtimeConfig.Load(); value != nil {
		return *value
	}
	return s.Config
}

func sameSettings(a, b config.Config) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func sameGeoIPStructure(a, b config.Config) bool {
	return a.GeoIPPreset == b.GeoIPPreset && a.GeoIPProvider == b.GeoIPProvider &&
		a.GeoIPBasicBackend == b.GeoIPBasicBackend && a.GeoIPDomesticOffline == b.GeoIPDomesticOffline &&
		a.GeoIPDomesticOnline == b.GeoIPDomesticOnline && a.GeoIPForeignOffline == b.GeoIPForeignOffline &&
		a.GeoIPForeignOnline == b.GeoIPForeignOnline && reflect.DeepEqual(a.GeoIPBackups, b.GeoIPBackups) &&
		a.GeoIPPath == b.GeoIPPath && a.GeoIPForeignPath == b.GeoIPForeignPath &&
		a.GeoIPUpdate == b.GeoIPUpdate
}

func sameGeoIPSettings(a, b config.Config) bool {
	return sameGeoIPStructure(a, b) &&
		a.GeoIPUpdateURL == b.GeoIPUpdateURL && a.GeoIPChecksumURL == b.GeoIPChecksumURL &&
		a.MaxMindAccountID == b.MaxMindAccountID && a.MaxMindLicenseKey == b.MaxMindLicenseKey &&
		a.IP2LocationToken == b.IP2LocationToken &&
		maps.Equal(a.GeoIPDatasetSources, b.GeoIPDatasetSources) &&
		maps.Equal(a.OnlineServices, b.OnlineServices)
}

// applyHotSettings only updates values that current requests can consume
// safely. Startup-created GeoIP clients, chains, and automatic runners stay
// on the active snapshot until an explicit restart.
func (s *Server) applyHotSettings(saved config.Config) {
	effective := s.effectiveSettings()
	changed := false
	if config.BasePath(effective.BaseURL) == config.BasePath(saved.BaseURL) && effective.BaseURL != saved.BaseURL {
		effective.BaseURL = saved.BaseURL
		changed = true
	}
	if effective.GeoIPUpdate == "disabled" && saved.GeoIPUpdate == "disabled" && sameGeoIPStructure(effective, saved) {
		if effective.GeoIPUpdateURL != saved.GeoIPUpdateURL || effective.GeoIPChecksumURL != saved.GeoIPChecksumURL ||
			effective.MaxMindAccountID != saved.MaxMindAccountID || effective.MaxMindLicenseKey != saved.MaxMindLicenseKey ||
			effective.IP2LocationToken != saved.IP2LocationToken || !maps.Equal(effective.GeoIPDatasetSources, saved.GeoIPDatasetSources) {
			effective.GeoIPUpdateURL = saved.GeoIPUpdateURL
			effective.GeoIPChecksumURL = saved.GeoIPChecksumURL
			effective.MaxMindAccountID = saved.MaxMindAccountID
			effective.MaxMindLicenseKey = saved.MaxMindLicenseKey
			effective.IP2LocationToken = saved.IP2LocationToken
			effective.GeoIPDatasetSources = saved.GeoIPDatasetSources
			changed = true
		}
	}
	if changed {
		s.runtimeConfig.Store(&effective)
	}
}

func (s *Server) settingsRestartPending() bool {
	saved, err := s.savedSettings()
	if err != nil {
		return false
	}
	effective := s.effectiveSettings()
	return effective.BaseURL != saved.BaseURL || !sameGeoIPSettings(effective, saved)
}
