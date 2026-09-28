package server

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

const maxConfigurationSettingsBody = 40 * 1024

func (s *Server) adminUpdateGeoIPConfiguration(w http.ResponseWriter, r *http.Request) {
	s.saveGeoIPConfiguration(w, r)
}

func (s *Server) adminUpdateServiceConfiguration(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, translate(adminLanguage(r), "err_csrf"))
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin/settings/service", translate(adminLanguage(r), "err_config_path"))
		return
	}
	if !s.authorizeStepUp(w, r, session) {
		return
	}
	baseURL, err := config.NormalizeBaseURL(r.FormValue("base_url"))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings/service", err.Error())
		return
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	current, err := s.savedSettings()
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings/service", err.Error())
		return
	}
	updated := current
	updated.BaseURL = baseURL
	if sameSettings(current, updated) {
		s.redirect(w, r, "/admin/settings/service?saved=no-change", http.StatusSeeOther)
		return
	}
	if err := config.Save(s.ConfigPath, updated); err != nil {
		message := fmt.Sprintf(translate(adminLanguage(r), "configuration_save_failed"), err, filepath.Dir(s.ConfigPath))
		s.redirectWithError(w, r, "/admin/settings/service", message)
		return
	}
	s.applyHotSettings(updated)
	s.redirect(w, r, "/admin/settings/service?saved=service", http.StatusSeeOther)
}

func (s *Server) saveGeoIPConfiguration(w http.ResponseWriter, r *http.Request) {
	target := "/admin/settings/geoip"
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigurationSettingsBody)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, translate(adminLanguage(r), "err_csrf"))
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_config_path"))
		return
	}
	if !s.authorizeStepUp(w, r, session) {
		return
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	current, err := s.savedSettings()
	if err != nil {
		s.redirectWithError(w, r, target, err.Error())
		return
	}
	provider, err := geoip.NormalizeProvider(r.FormValue("geoip_provider"))
	if err != nil {
		s.redirectWithError(w, r, target, err.Error())
		return
	}
	updateMode := r.FormValue("geoip_update")
	if updateMode != "automatic" && updateMode != "disabled" {
		s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_mode"))
		return
	}
	updated := current
	updated.GeoIPProvider = provider
	defaultMMDB := filepath.Join(updated.DataDir, "geoip.mmdb")
	defaultXDB := filepath.Join(updated.DataDir, "geoip.xdb")
	if provider == string(geoip.ProviderIP2Region) && updated.GeoIPPath == defaultMMDB {
		updated.GeoIPPath = defaultXDB
	} else if provider != string(geoip.ProviderIP2Region) && updated.GeoIPPath == defaultXDB {
		updated.GeoIPPath = defaultMMDB
	}
	updated.GeoIPUpdate = updateMode
	updated.MaxMindAccountID, updated.MaxMindLicenseKey, err = updatedMaxMindCredentials(r, updated)
	if err != nil {
		s.redirectWithError(w, r, target, err.Error())
		return
	}
	updated.IP2LocationToken, err = updatedSecret(r.FormValue("ip2location_token"), r.FormValue("clear_ip2location_token") == "1", updated.IP2LocationToken, "IP2Location Token", adminLanguage(r))
	if err != nil {
		s.redirectWithError(w, r, target, err.Error())
		return
	}
	// GeoIP preset and per-branch backend selection.
	updated.GeoIPPreset = r.FormValue("geoip_preset")
	if basic := r.FormValue("geoip_basic_provider"); basic != "" {
		updated.GeoIPBasicBackend = basic
	}
	updated.GeoIPDomesticOffline = r.FormValue("geoip_domestic_offline")
	updated.GeoIPDomesticOnline = r.FormValue("geoip_domestic_online")
	updated.GeoIPForeignOffline = r.FormValue("geoip_foreign_offline")
	updated.GeoIPForeignOnline = r.FormValue("geoip_foreign_online")
	if updated.GeoIPPreset == "precise" {
		if err := r.ParseForm(); err != nil {
			s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_source"))
			return
		}
		updated.GeoIPBackups = append([]string(nil), r.PostForm["geoip_backups"]...)
	}
	// Parse online service credentials from whichever inputs are visible.
	updated.OnlineServices = make(map[string]config.OnlineServiceConfig)
	for _, service := range []string{"ipinfo", "tencent", "amap", "bigdatacloud"} {
		key := strings.TrimSpace(r.FormValue("online_" + service + "_key"))
		sk := strings.TrimSpace(r.FormValue("online_" + service + "_sk"))
		if key == "" && sk == "" {
			if existing, ok := current.OnlineServices[service]; ok && existing.Key != "" {
				updated.OnlineServices[service] = existing
			}
			continue
		}
		if key == "" {
			if existing, ok := current.OnlineServices[service]; ok {
				key = existing.Key
			}
		}
		if sk == "" {
			if existing, ok := current.OnlineServices[service]; ok {
				sk = existing.SK
			}
		}
		if key != "" {
			entry := config.OnlineServiceConfig{Key: key}
			if sk != "" {
				entry.SK = sk
			}
			updated.OnlineServices[service] = entry
		}
	}
	updated.GeoIPDatasetSources = make(map[string]config.GeoIPDatasetSource, len(current.GeoIPDatasetSources))
	for id, source := range current.GeoIPDatasetSources {
		updated.GeoIPDatasetSources[id] = source
	}
	for _, dataset := range updated.SelectedGeoIPDatasets() {
		if dataset.Online {
			continue
		}
		mode := r.FormValue("geoip_source_" + dataset.ID)
		if mode == "" {
			mode = "official"
		}
		profile, _ := geoip.UpdateProfileForProvider(dataset.Provider)
		updateURL := profile.URL
		if mode == "custom" {
			updateURL = strings.TrimSpace(r.FormValue("geoip_update_url_" + dataset.ID))
			if updateURL == "" {
				s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_url_required"))
				return
			}
		} else if mode != "official" {
			s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_source"))
			return
		}
		checksumURL := strings.TrimSpace(r.FormValue("geoip_checksum_url_" + dataset.ID))
		if len(updateURL) > 4096 || len(checksumURL) > 4096 {
			s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_url_long"))
			return
		}
		if dataset.ID == "primary" {
			updated.GeoIPUpdateURL = updateURL
			updated.GeoIPChecksumURL = checksumURL
		} else if mode == "custom" || checksumURL != "" {
			source := config.GeoIPDatasetSource{Provider: dataset.Provider, ChecksumURL: checksumURL}
			if mode == "custom" {
				source.URL = updateURL
			}
			updated.GeoIPDatasetSources[dataset.ID] = source
		} else {
			delete(updated.GeoIPDatasetSources, dataset.ID)
		}
	}
	if sameSettings(current, updated) {
		s.redirect(w, r, target+"?saved=no-change", http.StatusSeeOther)
		return
	}
	if err := config.Save(s.ConfigPath, updated); err != nil {
		message := fmt.Sprintf(translate(adminLanguage(r), "configuration_save_failed"), err, filepath.Dir(s.ConfigPath))
		s.redirectWithError(w, r, target, message)
		return
	}
	s.applyHotSettings(updated)
	s.redirect(w, r, target+"?saved=geoip-settings", http.StatusSeeOther)
}

func updatedMaxMindCredentials(r *http.Request, current config.Config) (string, string, error) {
	clear := r.FormValue("clear_maxmind_credentials") == "1"
	accountID := strings.TrimSpace(r.FormValue("maxmind_account_id"))
	licenseKey := strings.TrimSpace(r.FormValue("maxmind_license_key"))
	if clear && (accountID != "" || licenseKey != "") {
		return "", "", errors.New(translate(adminLanguage(r), "err_credential_conflict_maxmind"))
	}
	if clear {
		return "", "", nil
	}
	if len(accountID) > 512 || len(licenseKey) > 512 {
		return "", "", errors.New(translate(adminLanguage(r), "err_credential_long_maxmind"))
	}
	if accountID == "" {
		accountID = current.MaxMindAccountID
	}
	if licenseKey == "" {
		licenseKey = current.MaxMindLicenseKey
	}
	return accountID, licenseKey, nil
}

func updatedSecret(input string, clear bool, current, label, lang string) (string, error) {
	input = strings.TrimSpace(input)
	if clear && input != "" {
		return "", fmt.Errorf(translate(lang, "err_credential_conflict"), label)
	}
	if clear {
		return "", nil
	}
	if len(input) > 512 {
		return "", fmt.Errorf(translate(lang, "err_credential_long"), label)
	}
	if input == "" {
		return current, nil
	}
	return input, nil
}
