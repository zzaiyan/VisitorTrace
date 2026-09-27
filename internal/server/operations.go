package server

import (
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	backupservice "github.com/zzaiyan/VisitorTrace/internal/backup"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/geoiponline"
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
	if r.FormValue("confirm_backup") == "" || r.FormValue("confirm_backup") != r.FormValue("backup") {
		s.redirectWithError(w, r, "/admin/settings#backup", translate(adminLanguage(r), "err_backup_confirmation"))
		return
	}
	if !s.authorizeCriticalAction(w, r, session) {
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
	if err := cfg.Validate(); err != nil {
		s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_settings_failed")+err.Error())
		return
	}
	datasetID := "primary"
	selectedDatasets := cfg.SelectedGeoIPDatasets()
	if fromSettings {
		if r.FormValue("dataset") != "" {
			datasetID = r.FormValue("dataset")
		}
	} else {
		datasetID = ""
		for _, selected := range selectedDatasets {
			if !selected.Online {
				datasetID = selected.ID
				break
			}
		}
		if datasetID == "" && len(selectedDatasets) > 0 {
			datasetID = selectedDatasets[0].ID
		}
	}
	var dataset *config.GeoIPDataset
	for _, selected := range selectedDatasets {
		if selected.ID == datasetID {
			copy := selected
			dataset = &copy
			break
		}
	}
	if dataset == nil {
		s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_dataset_unselected"))
		return
	}
	if dataset.Online {
		if err := s.testOnlineGeoIP(r, *dataset); err != nil {
			s.redirectWithError(w, r, target, translate(adminLanguage(r), "err_geoip_test_failed")+err.Error())
			return
		}
		if fromSettings {
			s.redirect(w, r, "/admin/settings?saved=geoip-current#geoip", http.StatusSeeOther)
		} else {
			s.redirect(w, r, "/admin?saved=geoip-current", http.StatusSeeOther)
		}
		return
	}
	cfg.GeoIPProvider = dataset.Provider
	cfg.GeoIPPath = dataset.Path
	cfg.GeoIPUpdateURL = dataset.UpdateURL
	cfg.GeoIPChecksumURL = dataset.ChecksumURL
	if fromSettings || cfg.GeoIPUpdate == "disabled" {
		cfg.GeoIPUpdate = "automatic"
	}
	runner := geoipupdate.New(cfg, s.Store, s.logger)
	runner.OperationName = dataset.Operation
	runner.Activate = func(path string) error {
		resolver, err := geoip.OpenWithProvider(dataset.Provider, path)
		if err != nil {
			return err
		}
		switch dataset.ID {
		case "primary":
			s.SetGeoIP(resolver)
		case "foreign":
			s.SetGeoIPForeign(resolver)
		default:
			s.SetGeoIPBackup(dataset.Provider, resolver)
		}
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

func (s *Server) testOnlineGeoIP(r *http.Request, dataset config.GeoIPDataset) error {
	if !s.geoProbeMu.TryLock() {
		return fmt.Errorf("%s", translate(adminLanguage(r), "err_geoip_probe_busy"))
	}
	defer s.geoProbeMu.Unlock()
	service := s.Config.OnlineServices[dataset.Provider]
	credential := service.Key
	if service.SK != "" {
		credential += ":" + service.SK
	}
	client, err := geoiponline.New(dataset.Provider, credential, 3*time.Second)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.Store.StartOperation(r.Context(), dataset.Operation, now); err != nil {
		return err
	}
	probeIP := "114.114.114.114"
	if dataset.Provider == "amap" {
		// Amap does not locate the public DNS address used for other services.
		probeIP = "220.181.38.148"
	}
	location, lookupErr := client.Lookup(r.Context(), netip.MustParseAddr(probeIP))
	if lookupErr == nil && location.CountryCode == "" && location.City == "" {
		lookupErr = fmt.Errorf("service returned no location")
	}
	summary := fmt.Sprintf("country=%s city=%s coordinates=%t", location.CountryCode, location.City, location.Latitude != nil && location.Longitude != nil)
	if lookupErr != nil {
		summary = lookupErr.Error()
		for _, secret := range []string{service.Key, service.SK} {
			if secret != "" {
				summary = strings.ReplaceAll(summary, secret, "[redacted]")
				summary = strings.ReplaceAll(summary, url.QueryEscape(secret), "[redacted]")
			}
		}
	}
	if err := s.Store.FinishOperation(r.Context(), dataset.Operation, time.Now().UTC(), lookupErr == nil, summary); err != nil {
		return err
	}
	if lookupErr != nil {
		return fmt.Errorf("%s", summary)
	}
	return nil
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
