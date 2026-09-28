package server

import (
	"net/http"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
)

func (s *Server) adminRestartService(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, translate(adminLanguage(r), "err_csrf"))
		return
	}
	if !s.authorizeStepUp(w, r, session) {
		return
	}
	saved, err := s.savedSettings()
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings/maintenance", err.Error())
		return
	}
	reconnectURL := s.requestOrigin(r) + config.BasePath(saved.BaseURL) + "/admin/settings"
	s.renderPage(w, r, "settings-restarting", settingsRestartData{
		pageLayout:   s.adminLayout(r, session, translate(adminLanguage(r), "service_restarting"), "settings"),
		ReconnectURL: reconnectURL,
		Eyebrow:      translate(adminLanguage(r), "settings"),
		Message:      translate(adminLanguage(r), "restart_manual_help"),
	})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.RequestRestart()
	}()
}
