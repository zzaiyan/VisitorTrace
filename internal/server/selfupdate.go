package server

import (
	"net/http"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/selfupdate"
)

type updateRestartData struct {
	pageLayout
	Version string
}

func (s *Server) adminRunSelfUpdate(w http.ResponseWriter, r *http.Request) {
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
		s.redirectWithError(w, r, "/admin/settings", "服务配置路径不可用。")
		return
	}
	if !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin/settings", "管理员密码不正确。")
		return
	}
	verifiedAt := time.Now().UTC()
	if err := s.Store.MarkAdministratorPasswordVerified(r.Context(), session.TokenDigest, verifiedAt); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法记录密码验证状态。")
		return
	}
	manager := selfupdate.New(s.Config, s.ConfigPath, s.Store)
	if len(manager.PublicKey) == 0 {
		s.redirectWithError(w, r, "/admin/settings", "当前构建未嵌入自更新签名公钥。")
		return
	}
	if !manager.RunningFromStablePath() {
		s.redirectWithError(w, r, "/admin/settings", "服务未从稳定更新路径启动，请先运行 update bootstrap 并调整进程管理器。")
		return
	}
	result, err := manager.PrepareAndActivate(r.Context())
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings", "更新失败："+err.Error())
		return
	}
	if result.Current {
		s.redirect(w, r, "/admin/settings?saved=update-current", http.StatusSeeOther)
		return
	}
	s.renderPage(w, r, "update-restarting", updateRestartData{
		pageLayout: s.adminLayout(r, session, "正在重启", "settings"), Version: result.Version,
	})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.RequestRestart()
	}()
}
