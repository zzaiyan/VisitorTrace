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
	if !s.authorizeOperation(w, r) {
		return
	}
	if s.Config.GeoIPUpdate == "disabled" {
		s.redirectWithError(w, r, "/admin", "GeoIP 自动更新已在配置中关闭。")
		return
	}
	runner := geoipupdate.New(s.Config, s.Store, s.logger)
	runner.Activate = func(path string) error {
		resolver, err := geoip.Open(path)
		if err != nil {
			return err
		}
		s.SetGeoIP(resolver)
		return nil
	}
	result, err := runner.RunOnce(r.Context(), false)
	if err != nil {
		s.redirectWithError(w, r, "/admin", "GeoIP 更新失败："+err.Error())
		return
	}
	value := "geoip-current"
	if result.Updated {
		value = "geoip"
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
