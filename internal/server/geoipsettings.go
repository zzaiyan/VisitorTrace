package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
)

const maxGeoIPSettingsBody = 32 * 1024

func (s *Server) adminUpdateGeoIPSettings(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxGeoIPSettingsBody)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin/settings#geoip", "服务配置路径不可用。")
		return
	}
	if !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin/settings#geoip", "管理员密码不正确。")
		return
	}

	provider, err := geoip.NormalizeProvider(r.FormValue("geoip_provider"))
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#geoip", err.Error())
		return
	}
	updateMode := r.FormValue("geoip_update")
	if updateMode != "automatic" && updateMode != "disabled" {
		s.redirectWithError(w, r, "/admin/settings#geoip", "GeoIP 更新模式无效。")
		return
	}
	profile, _ := geoip.UpdateProfileForProvider(provider)
	updateURL := profile.URL
	if r.FormValue("geoip_source") == "custom" {
		updateURL = strings.TrimSpace(r.FormValue("geoip_update_url"))
		if updateURL == "" {
			s.redirectWithError(w, r, "/admin/settings#geoip", "自定义 GeoIP 下载地址不能为空。")
			return
		}
	} else if r.FormValue("geoip_source") != "official" {
		s.redirectWithError(w, r, "/admin/settings#geoip", "GeoIP 下载源类型无效。")
		return
	}
	checksumURL := strings.TrimSpace(r.FormValue("geoip_checksum_url"))
	if len(updateURL) > 4096 || len(checksumURL) > 4096 {
		s.redirectWithError(w, r, "/admin/settings#geoip", "GeoIP 下载地址过长。")
		return
	}

	updated := s.Config
	updated.GeoIPProvider = provider
	updated.GeoIPUpdate = updateMode
	updated.GeoIPUpdateURL = updateURL
	updated.GeoIPChecksumURL = checksumURL
	updated.MaxMindAccountID, updated.MaxMindLicenseKey, err = updatedMaxMindCredentials(r, updated)
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#geoip", err.Error())
		return
	}
	updated.IP2LocationToken, err = updatedSecret(r.FormValue("ip2location_token"), r.FormValue("clear_ip2location_token") == "1", updated.IP2LocationToken, "IP2Location Token")
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#geoip", err.Error())
		return
	}
	if err := config.Save(s.ConfigPath, updated); err != nil {
		s.redirectWithError(w, r, "/admin/settings#geoip", "无法保存 GeoIP 设置："+err.Error())
		return
	}

	layout := s.adminLayout(r, session, "正在重启", "settings")
	reconnectURL := strings.TrimSuffix(s.externalBaseURL(r), "/") + "/admin/settings#geoip"
	s.renderPage(w, r, "settings-restarting", settingsRestartData{
		pageLayout: layout, ReconnectURL: reconnectURL, Eyebrow: "GEOIP",
		Message: translate(layout.Lang, "geoip_settings_saved"),
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
		return "", "", fmt.Errorf("不能同时清除和替换 MaxMind 凭证")
	}
	if clear {
		return "", "", nil
	}
	if len(accountID) > 512 || len(licenseKey) > 512 {
		return "", "", fmt.Errorf("MaxMind 凭证过长")
	}
	if accountID == "" {
		accountID = current.MaxMindAccountID
	}
	if licenseKey == "" {
		licenseKey = current.MaxMindLicenseKey
	}
	return accountID, licenseKey, nil
}

func updatedSecret(input string, clear bool, current, label string) (string, error) {
	input = strings.TrimSpace(input)
	if clear && input != "" {
		return "", fmt.Errorf("不能同时清除和替换 %s", label)
	}
	if clear {
		return "", nil
	}
	if len(input) > 512 {
		return "", fmt.Errorf("%s 过长", label)
	}
	if input == "" {
		return current, nil
	}
	return input, nil
}
