package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	backupservice "github.com/zzaiyan/VisitorTrace/internal/backup"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/geoipupdate"
	"github.com/zzaiyan/VisitorTrace/internal/maintenance"
)

type restoreRestartData struct {
	pageLayout
	ArchiveName string
	Manifest    backupservice.Manifest
}

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

func (s *Server) adminRunRestore(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin", "服务配置路径不可用。")
		return
	}
	if !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin", "管理员密码不正确。")
		return
	}

	s.restoreMu.Lock()
	if s.restoreActive {
		s.restoreMu.Unlock()
		s.redirectWithError(w, r, "/admin", "已有备份恢复任务正在等待服务重启。")
		return
	}
	s.restoreActive = true
	s.restoreMu.Unlock()
	scheduled := false
	defer func() {
		if !scheduled {
			s.restoreMu.Lock()
			s.restoreActive = false
			s.restoreMu.Unlock()
		}
	}()

	archiveName := strings.TrimSpace(r.FormValue("backup"))
	archivePath, err := backupservice.Resolve(s.Config.BackupDir, archiveName)
	if err != nil {
		s.redirectWithError(w, r, "/admin", "备份文件不可用："+err.Error())
		return
	}
	manifest, err := backupservice.ValidateArchive(r.Context(), archivePath)
	if err != nil {
		s.redirectWithError(w, r, "/admin", "备份校验失败："+err.Error())
		return
	}
	preRestoreDir := filepath.Join(s.Config.BackupDir, "pre-restore")
	preRestore, err := backupservice.Create(r.Context(), s.Store, s.ConfigPath, preRestoreDir, 3, time.Now())
	if err != nil {
		s.redirectWithError(w, r, "/admin", "创建恢复前安全备份失败："+err.Error())
		return
	}
	if err := backupservice.ScheduleRestore(s.Config.DataDir, s.Config.BackupDir, archivePath, preRestore.Path, time.Now()); err != nil {
		s.redirectWithError(w, r, "/admin", "安排恢复失败："+err.Error())
		return
	}
	scheduled = true
	s.renderPage(w, r, "restore-restarting", restoreRestartData{
		pageLayout:  s.adminLayout(r, session, translate(adminLanguage(r), "restore_backup"), "dashboard"),
		ArchiveName: archiveName, Manifest: manifest,
	})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.RequestRestart()
	}()
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
