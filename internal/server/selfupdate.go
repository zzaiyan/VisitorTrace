package server

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/selfupdate"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const (
	localUpdateFormMemory = int64(2 << 20)
	localUpdateBodyLimit  = selfupdate.MaxManifestBytes + selfupdate.MaxReleaseAssetBytes + localUpdateFormMemory
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
	manager, ok := s.authorizedSelfUpdateManager(w, r, session)
	if !ok {
		return
	}
	result, err := manager.PrepareAndActivate(r.Context())
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#self-update", "更新失败："+err.Error())
		return
	}
	s.finishSelfUpdate(w, r, session, result)
}

func (s *Server) adminRunLocalSelfUpdate(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, localUpdateBodyLimit)
	if err := r.ParseMultipartForm(localUpdateFormMemory); err != nil {
		s.redirectWithError(w, r, "/admin/settings#self-update", "无法读取本地更新文件："+err.Error())
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if !s.validCSRF(r, session) {
		s.renderError(w, r, http.StatusForbidden, "请求令牌无效。")
		return
	}
	manager, ok := s.authorizedSelfUpdateManager(w, r, session)
	if !ok {
		return
	}
	manifestFile, manifestHeader, err := r.FormFile("manifest")
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#self-update", "请选择签名发布清单。")
		return
	}
	defer manifestFile.Close()
	if manifestHeader.Size < 1 || manifestHeader.Size > selfupdate.MaxManifestBytes {
		s.redirectWithError(w, r, "/admin/settings#self-update", fmt.Sprintf("发布清单必须小于 %d 字节。", selfupdate.MaxManifestBytes))
		return
	}
	manifestData, err := io.ReadAll(io.LimitReader(manifestFile, selfupdate.MaxManifestBytes+1))
	if err != nil || int64(len(manifestData)) > selfupdate.MaxManifestBytes {
		s.redirectWithError(w, r, "/admin/settings#self-update", "无法读取签名发布清单。")
		return
	}
	binaryFile, binaryHeader, err := r.FormFile("binary")
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#self-update", "请选择当前平台的发布二进制。")
		return
	}
	defer binaryFile.Close()
	if binaryHeader.Size < 1 || binaryHeader.Size > selfupdate.MaxReleaseAssetBytes {
		s.redirectWithError(w, r, "/admin/settings#self-update", "发布二进制大小超出允许范围。")
		return
	}
	result, err := manager.PrepareAndActivateLocal(r.Context(), manifestData, binaryFile)
	if err != nil {
		s.redirectWithError(w, r, "/admin/settings#self-update", "更新失败："+err.Error())
		return
	}
	s.finishSelfUpdate(w, r, session, result)
}

func (s *Server) authorizedSelfUpdateManager(w http.ResponseWriter, r *http.Request, session store.AdministratorSession) (*selfupdate.Manager, bool) {
	if s.ConfigPath == "" {
		s.redirectWithError(w, r, "/admin/settings#self-update", "服务配置路径不可用。")
		return nil, false
	}
	if !s.administratorPasswordMatches(r.Context(), r.FormValue("password")) {
		s.redirectWithError(w, r, "/admin/settings#self-update", "管理员密码不正确。")
		return nil, false
	}
	verifiedAt := time.Now().UTC()
	if err := s.Store.MarkAdministratorPasswordVerified(r.Context(), session.TokenDigest, verifiedAt); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "无法记录密码验证状态。")
		return nil, false
	}
	manager := selfupdate.New(s.Config, s.ConfigPath, s.Store)
	if len(manager.PublicKey) == 0 {
		s.redirectWithError(w, r, "/admin/settings#self-update", "当前构建未嵌入自更新签名公钥。")
		return nil, false
	}
	if !manager.RunningFromStablePath() {
		s.redirectWithError(w, r, "/admin/settings#self-update", "服务未从稳定更新路径启动，请先运行 update bootstrap 并调整进程管理器。")
		return nil, false
	}
	return manager, true
}

func (s *Server) finishSelfUpdate(w http.ResponseWriter, r *http.Request, session store.AdministratorSession, result selfupdate.PrepareResult) {
	if result.Current {
		s.redirect(w, r, "/admin/settings?saved=update-current#self-update", http.StatusSeeOther)
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
