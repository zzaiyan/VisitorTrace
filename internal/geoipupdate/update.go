package geoipupdate

import (
	"archive/tar"
	"archive/zip"
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
	Profile  geoip.UpdateProfile
	Store    *store.Store
	Logger   *slog.Logger
	Client   *http.Client
	Now      func() time.Time
	Validate func(string) error
	Probe    func(string) error
	Activate func(string) error
}

func New(cfg config.Config, st *store.Store, logger *slog.Logger) *Runner {
	profile, _ := geoip.UpdateProfileForProvider(cfg.GeoIPProvider)
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
		Config: cfg, Profile: profile, Store: st, Logger: logger, Client: client, Now: time.Now,
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
	if r.Profile.CalendarMonthly {
		if modified.Year() != now.Year() || modified.Month() != now.Month() {
			return false
		}
	} else if r.Profile.FreshFor > 0 {
		age := now.Sub(modified)
		if age > r.Profile.FreshFor {
			return false
		}
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
	return Result{Updated: true, Source: publicSource(source), SHA256: checksum, CompressedSize: size}, nil
}

func (r *Runner) download(ctx context.Context, source, destination string, limit int64) (string, int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create GeoIP download request: %w", err)
	}
	request.Header.Set("User-Agent", "VisitorTrace GeoIP Updater")
	r.authorize(request)
	response, err := r.Client.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("download GeoIP database: %s", r.redactError(err))
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
	request.Header.Set("User-Agent", "VisitorTrace GeoIP Updater")
	r.authorize(request)
	response, err := r.Client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download GeoIP checksum: %s", r.redactError(err))
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
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat GeoIP download: %w", err)
	}
	header := make([]byte, 4)
	n, readErr := io.ReadFull(file, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return fmt.Errorf("read GeoIP download header: %w", readErr)
	}
	header = header[:n]
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind GeoIP download: %w", err)
	}

	if isZipHeader(header) {
		archive, err := zip.NewReader(file, info.Size())
		if err != nil {
			return fmt.Errorf("open GeoIP ZIP archive: %w", err)
		}
		return unpackZip(archive, destination)
	}
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		zipped, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("open compressed GeoIP database: %w", err)
		}
		defer zipped.Close()
		buffered := bufio.NewReader(zipped)
		tarHeader, peekErr := buffered.Peek(512)
		if peekErr != nil && peekErr != io.EOF {
			return fmt.Errorf("inspect compressed GeoIP database: %w", peekErr)
		}
		if isTarHeader(tarHeader) {
			return unpackTar(tar.NewReader(buffered), destination)
		}
		return writeDatabase(buffered, destination)
	}
	return writeDatabase(file, destination)
}

func isZipHeader(header []byte) bool {
	if len(header) < 4 || header[0] != 'P' || header[1] != 'K' {
		return false
	}
	return (header[2] == 3 && header[3] == 4) || (header[2] == 5 && header[3] == 6) || (header[2] == 7 && header[3] == 8)
}

func isTarHeader(header []byte) bool {
	return len(header) >= 265 && string(header[257:262]) == "ustar"
}

func unpackZip(archive *zip.Reader, destination string) error {
	var candidate *zip.File
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(entry.Name), ".mmdb") {
			continue
		}
		if candidate != nil {
			return fmt.Errorf("GeoIP ZIP archive contains multiple MMDB files")
		}
		candidate = entry
	}
	if candidate == nil {
		return fmt.Errorf("GeoIP ZIP archive contains no MMDB file")
	}
	reader, err := candidate.Open()
	if err != nil {
		return fmt.Errorf("open MMDB in GeoIP ZIP archive: %w", err)
	}
	defer reader.Close()
	return writeDatabase(reader, destination)
}

func unpackTar(archive *tar.Reader, destination string) error {
	found := false
	candidateWritten := false
	defer func() {
		if candidateWritten {
			_ = os.Remove(destination)
		}
	}()
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read GeoIP tar archive: %w", err)
		}
		if (entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeRegA) || !strings.EqualFold(filepath.Ext(entry.Name), ".mmdb") {
			continue
		}
		if found {
			return fmt.Errorf("GeoIP tar archive contains multiple MMDB files")
		}
		if entry.Size > maxDatabaseBytes {
			return fmt.Errorf("unpacked GeoIP database exceeds %d bytes", maxDatabaseBytes)
		}
		if err := writeDatabase(archive, destination); err != nil {
			return err
		}
		found = true
		candidateWritten = true
	}
	if !found {
		return fmt.Errorf("GeoIP tar archive contains no MMDB file")
	}
	candidateWritten = false
	return nil
}

func writeDatabase(reader io.Reader, destination string) error {
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create GeoIP candidate: %w", err)
	}
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(destination)
		}
	}()
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
	remove = false
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

func (r *Runner) authorize(request *http.Request) {
	if request == nil || !strings.EqualFold(request.URL.Hostname(), r.Profile.OfficialHost) {
		return
	}
	switch geoip.Provider(r.Config.GeoIPProvider) {
	case geoip.ProviderMaxMind:
		request.SetBasicAuth(r.Config.MaxMindAccountID, r.Config.MaxMindLicenseKey)
	case geoip.ProviderIP2Location:
		query := request.URL.Query()
		query.Set("token", r.Config.IP2LocationToken)
		request.URL.RawQuery = query.Encode()
	}
}

func (r *Runner) redactError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	for _, secret := range []string{r.Config.MaxMindLicenseKey, r.Config.IP2LocationToken} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func publicSource(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	query := parsed.Query()
	for _, key := range []string{"token", "license_key", "key", "password"} {
		if query.Has(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
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
