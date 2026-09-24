package server

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

const maxConfigurationSettingsBody = 40 * 1024

func (s *Server) adminUpdateConfiguration(w http.ResponseWriter, r *http.Request) {
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
		s.redirectWithError(w, r, "/admin/settings#configuration", translate(adminLanguage(r), "err_config_path"))
		return
	}
	if !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin/settings#configuration", translate(adminLanguage(r), "err_admin_password"))
		return
	}
	baseURL, err := config.NormalizeBaseURL(r.FormValue("base_url"))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#configuration", err.Error())
		return
	}

	provider, err := geoip.NormalizeProvider(r.FormValue("geoip_provider"))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#configuration", err.Error())
		return
	}
	updateMode := r.FormValue("geoip_update")
	if updateMode != "automatic" && updateMode != "disabled" {
		s.redirectWithError(w, r, "/admin/settings#configuration", translate(adminLanguage(r), "err_geoip_mode"))
		return
	}
	profile, _ := geoip.UpdateProfileForProvider(provider)
	updateURL := profile.URL
	if r.FormValue("geoip_source") == "custom" {
		updateURL = strings.TrimSpace(r.FormValue("geoip_update_url"))
		if updateURL == "" {
			s.redirectWithError(w, r, "/admin/settings#configuration", translate(adminLanguage(r), "err_geoip_url_required"))
			return
		}
	} else if r.FormValue("geoip_source") != "official" {
		s.redirectWithError(w, r, "/admin/settings#configuration", translate(adminLanguage(r), "err_geoip_source"))
		return
	}
	checksumURL := strings.TrimSpace(r.FormValue("geoip_checksum_url"))
	if len(updateURL) > 4096 || len(checksumURL) > 4096 {
		s.redirectWithError(w, r, "/admin/settings#configuration", translate(adminLanguage(r), "err_geoip_url_long"))
		return
	}

	updated := s.Config
	updated.BaseURL = baseURL
	updated.GeoIPProvider = provider
	updated.GeoIPUpdate = updateMode
	updated.GeoIPUpdateURL = updateURL
	updated.GeoIPChecksumURL = checksumURL
	updated.MaxMindAccountID, updated.MaxMindLicenseKey, err = updatedMaxMindCredentials(r, updated)
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#configuration", err.Error())
		return
	}
	updated.IP2LocationToken, err = updatedSecret(r.FormValue("ip2location_token"), r.FormValue("clear_ip2location_token") == "1", updated.IP2LocationToken, "IP2Location Token", adminLanguage(r))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#configuration", err.Error())
		return
	}
	if err := config.Save(s.ConfigPath, updated); err != nil {
		message := fmt.Sprintf(translate(adminLanguage(r), "configuration_save_failed"), err, filepath.Dir(s.ConfigPath))
		s.redirectWithError(w, r, "/admin/settings#configuration", message)
		return
	}

	layout := s.adminLayout(r, session, translate(adminLanguage(r), "service_restarting"), "settings")
	reconnectURL := s.requestOrigin(r) + "/admin/settings#configuration"
	if baseURL != "" {
		reconnectURL = strings.TrimSuffix(baseURL, "/") + "/admin/settings#configuration"
	}
	s.renderPage(w, r, "settings-restarting", settingsRestartData{
		pageLayout: layout, ReconnectURL: reconnectURL, Eyebrow: "Configuration",
		Message: translate(layout.Lang, "configuration_saved"),
	})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.RequestRestart()
	}()
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
