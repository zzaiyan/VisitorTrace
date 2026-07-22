package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zzaiyan/VisitorTrace/internal/password"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const (
	adminSessionCookie = "visitortrace_admin"
	adminSessionAge    = 7 * 24 * time.Hour
)

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentAdmin(r); ok {
		s.redirect(w, r, safeNext(r.URL.Query().Get("next"), "/admin"), http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		s.handleAdminLogin(w, r)
		return
	}
	layout := pageLayout{Title: "管理员登录", Lang: adminLanguage(r)}
	if r.URL.Query().Get("changed") == "1" {
		layout.Flash = "管理员密码已更新，请重新登录。"
	}
	s.renderPage(w, r, "login", loginPageData{pageLayout: layout, Next: safeNext(r.URL.Query().Get("next"), "/admin")})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !s.adminHTTPSAllowed(r) {
		s.renderError(w, r, http.StatusForbidden, "管理员后台需要 HTTPS；本机回环地址可使用 HTTP 预览。")
		return
	}
	if !s.loginLimit.Allow(s.loginClientKey(r)) {
		s.renderLoginError(w, r, "登录尝试过于频繁，请稍后再试。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	if err := r.ParseForm(); err != nil {
		s.renderLoginError(w, r, "登录请求无效。")
		return
	}
	passwordValue := r.FormValue("password")
	hash, err := s.Store.AdministratorPasswordHash(r.Context())
	length := utf8.RuneCountInString(passwordValue)
	if err != nil || !utf8.ValidString(passwordValue) || length < 8 || length > 128 || !password.Verify([]byte(passwordValue), hash) {
		s.renderLoginError(w, r, "密码不正确。")
		return
	}
	token := make([]byte, 32)
	csrf := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法创建登录会话。")
		return
	}
	if _, err := rand.Read(csrf); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法创建安全令牌。")
		return
	}
	now := time.Now().UTC()
	_ = s.Store.DeleteExpiredAdministratorSessions(r.Context(), now)
	if err := s.Store.CreateAdministratorSession(r.Context(), store.HashSessionToken(hex.EncodeToString(token)), csrf, now, now.Add(adminSessionAge)); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法保存登录会话。")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    hex.EncodeToString(token),
		Path:     s.cookiePath(),
		HttpOnly: true,
		Secure:   s.secureAdminCookie(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(adminSessionAge / time.Second),
	})
	s.redirect(w, r, safeNext(r.FormValue("next"), "/admin"), http.StatusSeeOther)
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	session, ok := s.currentAdmin(r)
	if !ok {
		s.redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost || !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	_ = s.Store.DeleteAdministratorSession(r.Context(), session.TokenDigest)
	s.clearAdminCookie(w, r)
	s.redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) currentAdmin(r *http.Request) (store.AdministratorSession, bool) {
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil || len(cookie.Value) != 64 {
		return store.AdministratorSession{}, false
	}
	if _, err := hex.DecodeString(cookie.Value); err != nil {
		return store.AdministratorSession{}, false
	}
	digest := store.HashSessionToken(cookie.Value)
	session, err := s.Store.FindAdministratorSession(r.Context(), digest, time.Now().UTC())
	if err != nil {
		return store.AdministratorSession{}, false
	}
	if time.Since(session.LastSeenAt) > time.Minute {
		_ = s.Store.TouchAdministratorSession(r.Context(), digest, time.Now().UTC())
	}
	session.TokenDigest = digest
	return session, true
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (store.AdministratorSession, bool) {
	session, ok := s.currentAdmin(r)
	if ok {
		return session, true
	}
	next := r.URL.RequestURI()
	s.redirect(w, r, "/admin/login?next="+url.QueryEscape(next), http.StatusSeeOther)
	return store.AdministratorSession{}, false
}

func (s *Server) validCSRF(r *http.Request, session store.AdministratorSession) bool {
	if r.Method != http.MethodPost {
		return false
	}
	if err := r.ParseForm(); err != nil {
		return false
	}
	value, err := hex.DecodeString(r.FormValue("csrf"))
	if err != nil {
		return false
	}
	return len(value) == len(session.CSRFToken) && subtle.ConstantTimeCompare(value, session.CSRFToken) == 1
}

func (s *Server) adminHTTPSAllowed(r *http.Request) bool {
	if r.TLS != nil || s.forwardedHTTPS(r) {
		return true
	}
	host := r.Host
	if hostName, _, err := net.SplitHostPort(host); err == nil {
		host = hostName
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func (s *Server) secureAdminCookie(r *http.Request) bool {
	return !isLoopbackHost(r.Host) || r.TLS != nil || s.forwardedHTTPS(r)
}

func (s *Server) forwardedHTTPS(r *http.Request) bool {
	return s.clientIP != nil && s.clientIP.IsTrustedRemote(r.RemoteAddr) && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func isLoopbackHost(host string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func (s *Server) loginClientKey(r *http.Request) string {
	if s.clientIP != nil {
		if address, err := s.clientIP.Resolve(r); err == nil {
			return address.String()
		}
	}
	value := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

func (s *Server) administratorPasswordMatches(ctx context.Context, value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	length := utf8.RuneCountInString(value)
	if length < 8 || length > 128 {
		return false
	}
	hash, err := s.Store.AdministratorPasswordHash(ctx)
	return err == nil && password.Verify([]byte(value), hash)
}

func (s *Server) clearAdminCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: adminSessionCookie, Value: "", Path: s.cookiePath(), MaxAge: -1, HttpOnly: true, Secure: s.secureAdminCookie(r), SameSite: http.SameSiteStrictMode})
}

func safeNext(value, fallback string) string {
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return fallback
	}
	return value
}

func (s *Server) renderLoginError(w http.ResponseWriter, r *http.Request, message string) {
	s.renderPage(w, r, "login", loginPageData{pageLayout: pageLayout{Title: "管理员登录", Error: message, Lang: adminLanguage(r)}, Next: safeNext(r.FormValue("next"), "/admin")})
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.WriteHeader(status)
	s.renderPage(w, r, "error", errorPageData{pageLayout: pageLayout{Title: "请求未完成", Lang: adminLanguage(r)}, Status: status, Message: message})
}
