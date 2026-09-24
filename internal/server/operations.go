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
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_config_path"))
		return
	}
	_, err := backupservice.CreateTracked(r.Context(), s.Store, s.ConfigPath, s.Config.BackupDir, 3, time.Now())
	if err != nil {
		s.redirectWithError(w, r, "/admin", translate(adminLanguage(r), "err_backup_failed")+err.Error())
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
		s.renderError(w, r, http.StatusForbidden, translate(adminLanguage(r), "err_csrf"))
		return
	}
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin", translate(adminLanguage(r), "err_config_path"))
		return
	}
	if !s.authorizeStepUp(w, r, session) {
		return
	}

	s.restoreMu.Lock()
	if s.restoreActive {
		s.restoreMu.Unlock()
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_restore_pending"))
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
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_backup_missing")+err.Error())
		return
	}
	manifest, err := backupservice.ValidateArchive(r.Context(), archivePath)
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_backup_verify_failed")+err.Error())
		return
	}
	preRestoreDir := filepath.Join(s.Config.BackupDir, "pre-restore")
	preRestore, err := backupservice.Create(r.Context(), s.Store, s.ConfigPath, preRestoreDir, 3, time.Now())
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_restore_snapshot_failed")+err.Error())
		return
	}
	if err := backupservice.ScheduleRestore(s.Config.DataDir, s.Config.BackupDir, archivePath, preRestore.Path, time.Now()); err != nil {
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_restore_schedule_failed")+err.Error())
		return
	}
	scheduled = true
	s.renderPage(w, r, "restore-restarting", restoreRestartData{
		pageLayout:  s.adminLayout(r, session, translate(adminLanguage(r), "restore_backup"), "settings"),
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
		s.redirectWithError(w, r, "/admin", translate(adminLanguage(r), "err_cleanup_failed")+err.Error())
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
		s.redirectWithError(w, r, "/admin", translate(adminLanguage(r), "err_geoip_disabled"))
		return
	}
	if fromSettings {
		cfg.GeoIPUpdate = "automatic"
	}
	if err := cfg.Validate(); err != nil {
		s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_settings_failed")+err.Error())
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
		s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_update_failed")+err.Error())
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
		s.renderError(w, r, http.StatusForbidden, translate(adminLanguage(r), "err_csrf"))
		return false
	}
	return true
}
