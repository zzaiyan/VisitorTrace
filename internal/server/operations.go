package server

import (
	"net/http"
	"time"

	backupservice "github.com/zzaiyan/VisitorTrace/internal/backup"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/geoipupdate"
	"github.com/zzaiyan/VisitorTrace/internal/maintenance"
)

func (s *Server) adminRunBackup(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeOperation(w, r) {
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin", "服务配置路径不可用。")
		return
	}
	_, err := backupservice.CreateTracked(r.Context(), s.Store, s.ConfigPath, s.Config.BackupDir, 3, time.Now())
	if err != nil {
		s.redirectWithError(w, r, "/admin", "备份失败："+err.Error())
		return
	}
	s.redirect(w, r, "/admin?saved=backup", http.StatusSeeOther)
}

func (s *Server) adminRunCleanup(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeOperation(w, r) {
		return
	}
	runner := maintenance.New(s.Store, s.logger)
	if _, err := runner.RunOnce(r.Context()); err != nil {
		s.redirectWithError(w, r, "/admin", "清理失败："+err.Error())
		return
	}
	s.redirect(w, r, "/admin?saved=cleanup", http.StatusSeeOther)
}

func (s *Server) adminRunGeoIPUpdate(w http.ResponseWriter, r *http.Request) {
	s.runGeoIPUpdate(w, r, false)
}

func (s *Server) adminRunGeoIPUpdateFromSettings(w http.ResponseWriter, r *http.Request) {
	s.runGeoIPUpdate(w, r, true)
}

func (s *Server) runGeoIPUpdate(w http.ResponseWriter, r *http.Request, fromSettings bool) {
	if !s.authorizeOperation(w, r) {
		return
	}
	target := "/admin"
	if fromSettings {
		target = "/admin/settings#geoip"
	}
	cfg := s.Config
	if cfg.GeoIPUpdate == "disabled" && !fromSettings {
		s.redirectWithError(w, r, "/admin", "GeoIP 自动更新已在配置中关闭。")
		return
	}
	if fromSettings {
		cfg.GeoIPUpdate = "automatic"
	}
	if err := cfg.Validate(); err != nil {
		s.redirectWithError(w, r, target, "GeoIP 更新设置无效："+err.Error())
		return
	}
	runner := geoipupdate.New(cfg, s.Store, s.logger)
	runner.Activate = func(path string) error {
		resolver, err := geoip.OpenWithProvider(cfg.GeoIPProvider, path)
		if err != nil {
			return err
		}
		s.SetGeoIP(resolver)
		return nil
	}
	force := fromSettings && r.FormValue("force") == "1"
	result, err := runner.RunOnce(r.Context(), force)
	if err != nil {
		s.redirectWithError(w, r, target, "GeoIP 更新失败："+err.Error())
		return
	}
	value := "geoip-current"
	if result.Updated {
		value = "geoip"
	}
	if fromSettings {
		s.redirect(w, r, "/admin/settings?saved="+value+"#geoip", http.StatusSeeOther)
		return
	}
	s.redirect(w, r, "/admin?saved="+value, http.StatusSeeOther)
}

func (s *Server) authorizeOperation(w http.ResponseWriter, r *http.Request) bool {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return false
	}
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return false
	}
	return true
}
