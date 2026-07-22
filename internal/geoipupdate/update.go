package geoipupdate

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

var updateMu sync.Mutex

const (
	checkInterval      = 24 * time.Hour
	maxCompressedBytes = int64(1 << 30)
	maxDatabaseBytes   = int64(2 << 30)
)

type Result struct {
	Updated        bool
	Source         string
	SHA256         string
	CompressedSize int64
}

type Runner struct {
	Config   config.Config
	Store    *store.Store
	Logger   *slog.Logger
	Client   *http.Client
	Now      func() time.Time
	Validate func(string) error
	Probe    func(string) error
	Activate func(string) error
}

func New(cfg config.Config, st *store.Store, logger *slog.Logger) *Runner {
	client := &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateRemoteURL(request.URL)
		},
	}
	return &Runner{
		Config: cfg, Store: st, Logger: logger, Client: client, Now: time.Now,
		Validate: func(path string) error {
			return geoip.ValidateWithProvider(cfg.GeoIPProvider, path)
		},
		Probe: func(path string) error {
			resolver, err := geoip.OpenWithProvider(cfg.GeoIPProvider, path)
			if err != nil {
				return err
			}
			return resolver.Close()
		},
	}
}

func (r *Runner) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.runLogged(ctx, false)
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runLogged(ctx, false)
			}
		}
	}()
	return done
}

func (r *Runner) RunOnce(ctx context.Context, force bool) (Result, error) {
	if !updateMu.TryLock() {
		return Result{}, fmt.Errorf("a GeoIP update is already running")
	}
	defer updateMu.Unlock()
	if r.Config.GeoIPUpdate == "disabled" {
		return Result{}, nil
	}
	now := r.Now().UTC()
	if !force && r.currentDatabaseIsFresh(now) {
		return Result{}, nil
	}
	if err := r.Store.StartOperation(ctx, "geoip_update", now); err != nil {
		return Result{}, err
	}
	result, runErr := r.downloadAndInstall(ctx, now)
	summary := "not updated"
	if result.Updated {
		summary = fmt.Sprintf("source=%s sha256=%s bytes=%d", result.Source, result.SHA256, result.CompressedSize)
	}
	if runErr != nil {
		summary = "error=" + runErr.Error()
	}
	if err := r.Store.FinishOperation(ctx, "geoip_update", r.Now().UTC(), runErr == nil, summary); err != nil && runErr == nil {
		runErr = err
	}
	return result, runErr
}

func (r *Runner) currentDatabaseIsFresh(now time.Time) bool {
	info, err := os.Stat(r.Config.GeoIPPath)
	if err != nil || info.IsDir() {
		return false
	}
	modified := info.ModTime().UTC()
	if modified.Year() != now.Year() || modified.Month() != now.Month() {
		return false
	}
	return r.Probe(r.Config.GeoIPPath) == nil
}

func (r *Runner) downloadAndInstall(ctx context.Context, now time.Time) (Result, error) {
	source := expandTemplate(r.Config.GeoIPUpdateURL, now)
	parsed, err := url.Parse(source)
	if err != nil {
		return Result{}, fmt.Errorf("parse GeoIP update URL: %w", err)
	}
	if err := validateRemoteURL(parsed); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(r.Config.GeoIPPath), 0o700); err != nil {
		return Result{}, fmt.Errorf("create GeoIP directory: %w", err)
	}
	workDir, err := os.MkdirTemp(filepath.Dir(r.Config.GeoIPPath), ".geoip-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("create GeoIP update workspace: %w", err)
	}
	defer os.RemoveAll(workDir)
	compressedPath := filepath.Join(workDir, "download")
	checksum, size, err := r.download(ctx, source, compressedPath, maxCompressedBytes)
	if err != nil {
		return Result{}, err
	}
	if r.Config.GeoIPChecksumURL != "" {
		checksumURL := expandTemplate(r.Config.GeoIPChecksumURL, now)
		want, err := r.downloadChecksum(ctx, checksumURL)
		if err != nil {
			return Result{}, err
		}
		if !strings.EqualFold(want, checksum) {
			return Result{}, fmt.Errorf("GeoIP download SHA-256 mismatch")
		}
	}
	candidate := filepath.Join(workDir, "geoip.mmdb")
	if err := unpack(compressedPath, candidate); err != nil {
		return Result{}, err
	}
	if err := r.Validate(candidate); err != nil {
		return Result{}, fmt.Errorf("validate downloaded GeoIP database: %w", err)
	}
	if err := install(candidate, r.Config.GeoIPPath, r.Activate); err != nil {
		return Result{}, err
	}
	return Result{Updated: true, Source: source, SHA256: checksum, CompressedSize: size}, nil
}

func (r *Runner) download(ctx context.Context, source, destination string, limit int64) (string, int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create GeoIP download request: %w", err)
	}
	request.Header.Set("User-Agent", "VisitorTrace GeoIP Updater")
	response, err := r.Client.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("download GeoIP database: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download GeoIP database: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return "", 0, fmt.Errorf("GeoIP download exceeds %d bytes", limit)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("create GeoIP download: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", 0, fmt.Errorf("write GeoIP download: %w", copyErr)
	}
	if closeErr != nil {
		return "", 0, fmt.Errorf("close GeoIP download: %w", closeErr)
	}
	if written > limit {
		return "", 0, fmt.Errorf("GeoIP download exceeds %d bytes", limit)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func (r *Runner) downloadChecksum(ctx context.Context, source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse GeoIP checksum URL: %w", err)
	}
	if err := validateRemoteURL(parsed); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	response, err := r.Client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download GeoIP checksum: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download GeoIP checksum: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(data) > 4096 {
		return "", fmt.Errorf("read GeoIP checksum response")
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", fmt.Errorf("GeoIP SHA-256 response is invalid")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("GeoIP SHA-256 response is invalid")
	}
	return strings.ToLower(fields[0]), nil
}

func unpack(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open GeoIP download: %w", err)
	}
	defer file.Close()
	buffered := bufio.NewReader(file)
	header, err := buffered.Peek(2)
	if err != nil {
		return fmt.Errorf("read GeoIP download header: %w", err)
	}
	var reader io.Reader = buffered
	var zipped *gzip.Reader
	if header[0] == 0x1f && header[1] == 0x8b {
		zipped, err = gzip.NewReader(buffered)
		if err != nil {
			return fmt.Errorf("open compressed GeoIP database: %w", err)
		}
		defer zipped.Close()
		reader = zipped
	}
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create GeoIP candidate: %w", err)
	}
	written, copyErr := io.Copy(target, io.LimitReader(reader, maxDatabaseBytes+1))
	if copyErr == nil {
		copyErr = target.Sync()
	}
	closeErr := target.Close()
	if copyErr != nil {
		return fmt.Errorf("unpack GeoIP database: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close GeoIP candidate: %w", closeErr)
	}
	if written > maxDatabaseBytes {
		return fmt.Errorf("unpacked GeoIP database exceeds %d bytes", maxDatabaseBytes)
	}
	return nil
}

func install(candidate, target string, activate func(string) error) error {
	previous := target + ".previous"
	_ = os.Remove(previous)
	hadPrevious := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, previous); err != nil {
			return fmt.Errorf("stage previous GeoIP database: %w", err)
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check current GeoIP database: %w", err)
	}
	if err := os.Rename(candidate, target); err != nil {
		if hadPrevious {
			_ = os.Rename(previous, target)
		}
		return fmt.Errorf("activate GeoIP database file: %w", err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		_ = rollback(target, previous, hadPrevious, activate)
		return fmt.Errorf("protect GeoIP database: %w", err)
	}
	if activate != nil {
		if err := activate(target); err != nil {
			_ = rollback(target, previous, hadPrevious, activate)
			return fmt.Errorf("activate GeoIP database: %w", err)
		}
	}
	return nil
}

func rollback(target, previous string, hadPrevious bool, activate func(string) error) error {
	_ = os.Remove(target)
	if !hadPrevious {
		return nil
	}
	if err := os.Rename(previous, target); err != nil {
		return err
	}
	if activate != nil {
		return activate(target)
	}
	return nil
}

func expandTemplate(value string, now time.Time) string {
	return strings.ReplaceAll(value, "{YYYY-MM}", now.UTC().Format("2006-01"))
}

func validateRemoteURL(value *url.URL) error {
	if value == nil || value.Host == "" {
		return fmt.Errorf("GeoIP update URL must be absolute")
	}
	if value.Scheme == "https" {
		return nil
	}
	host := strings.ToLower(value.Hostname())
	if value.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return nil
	}
	return fmt.Errorf("GeoIP update URL must use HTTPS except on loopback")
}

func (r *Runner) runLogged(ctx context.Context, force bool) {
	result, err := r.RunOnce(ctx, force)
	if err != nil {
		if ctx.Err() == nil {
			r.Logger.Error("GeoIP update failed", "error", err)
		}
		return
	}
	if result.Updated {
		r.Logger.Info("GeoIP database updated", "source", result.Source, "sha256", result.SHA256, "bytes", result.CompressedSize)
	}
}
