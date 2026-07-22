package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	backupservice "github.com/zzaiyan/VisitorTrace/internal/backup"
	"github.com/zzaiyan/VisitorTrace/internal/buildinfo"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const (
	manifestLimit = int64(1 << 20)
	updateLockAge = 2 * time.Hour
)

type CandidateInfo struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildTime     string `json:"build_time"`
	SchemaVersion int    `json:"schema_version"`
}

type CheckResult struct {
	Manifest Manifest
	Asset    Asset
	Current  bool
}

type PrepareResult struct {
	Version        string
	Current        bool
	BinaryPath     string
	BackupPath     string
	StablePath     string
	PreviousTarget string
}

type Manager struct {
	Config         config.Config
	ConfigPath     string
	Store          *store.Store
	PublicKey      []byte
	CurrentVersion string
	Platform       string
	HTTPClient     *http.Client
	ExecutablePath string
	Args0          string
	Now            func() time.Time
	RunCommand     func(context.Context, string, ...string) ([]byte, error)
}

func New(cfg config.Config, configPath string, st *store.Store) *Manager {
	publicKey, _ := DecodePublicKey(buildinfo.UpdatePublicKey)
	client := &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateDownloadURL(request.URL)
		},
	}
	executable, _ := os.Executable()
	return &Manager{
		Config: cfg, ConfigPath: configPath, Store: st, PublicKey: publicKey,
		CurrentVersion: buildinfo.Version, Platform: runtime.GOOS + "-" + runtime.GOARCH,
		HTTPClient: client, ExecutablePath: executable, Args0: os.Args[0], Now: time.Now,
		RunCommand: runCommand,
	}
}

func (m *Manager) Check(ctx context.Context) (CheckResult, error) {
	manifestURL, err := url.Parse(m.Config.UpdateManifestURL)
	if err != nil || manifestURL.Host == "" {
		return CheckResult{}, fmt.Errorf("parse update manifest URL")
	}
	if err := validateDownloadURL(manifestURL); err != nil {
		return CheckResult{}, err
	}
	data, err := m.downloadBytes(ctx, manifestURL.String(), manifestLimit)
	if err != nil {
		return CheckResult{}, fmt.Errorf("download release manifest: %w", err)
	}
	manifest, err := DecodeManifest(data)
	if err != nil {
		return CheckResult{}, err
	}
	if err := VerifyManifest(manifest, m.PublicKey); err != nil {
		return CheckResult{}, err
	}
	asset, ok := manifest.Assets[m.Platform]
	if !ok {
		return CheckResult{}, fmt.Errorf("release %s has no asset for %s", manifest.Version, m.Platform)
	}
	comparison, err := compareSemanticVersions(manifest.Version, m.CurrentVersion)
	if err != nil {
		return CheckResult{}, fmt.Errorf("compare release versions: %w", err)
	}
	if comparison < 0 {
		return CheckResult{}, fmt.Errorf("release manifest version %s is older than current version %s", manifest.Version, m.CurrentVersion)
	}
	return CheckResult{Manifest: manifest, Asset: asset, Current: comparison == 0}, nil
}

func (m *Manager) PrepareAndActivate(ctx context.Context) (result PrepareResult, returnErr error) {
	check, err := m.Check(ctx)
	if err != nil {
		return PrepareResult{}, err
	}
	if check.Current {
		return PrepareResult{Version: check.Manifest.Version, Current: true, StablePath: m.StableBinaryPath()}, nil
	}
	lock, err := acquireUpdateLock(m.Config.DataDir, m.Now())
	if err != nil {
		return PrepareResult{}, err
	}
	defer lock.release()
	started := m.Now().UTC()
	if err := m.Store.StartOperation(ctx, "self_update", started); err != nil {
		return PrepareResult{}, err
	}
	defer func() {
		if returnErr != nil {
			_ = m.Store.FinishOperation(context.Background(), "self_update", m.Now().UTC(), false, "error="+returnErr.Error())
		}
	}()
	currentSchema, err := m.Store.SchemaVersion(ctx)
	if err != nil {
		return PrepareResult{}, err
	}
	if check.Manifest.SchemaVersion < currentSchema {
		return PrepareResult{}, fmt.Errorf("release schema %d is older than database schema %d", check.Manifest.SchemaVersion, currentSchema)
	}
	previousTarget, err := m.currentReleaseTarget()
	if err != nil {
		return PrepareResult{}, err
	}
	versionName, err := releaseDirectoryName(check.Manifest.Version)
	if err != nil {
		return PrepareResult{}, err
	}
	releasesRoot := m.ReleasesRoot()
	if err := os.MkdirAll(releasesRoot, 0o700); err != nil {
		return PrepareResult{}, fmt.Errorf("create releases directory: %w", err)
	}
	targetDirectory := filepath.Join(releasesRoot, versionName)
	targetBinary := filepath.Join(targetDirectory, "visitortrace")
	if err := m.installCandidate(ctx, check.Asset, targetDirectory, targetBinary); err != nil {
		return PrepareResult{}, err
	}
	candidateCtx, candidateCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer candidateCancel()
	if err := m.inspectCandidate(candidateCtx, targetBinary, check.Manifest); err != nil {
		return PrepareResult{}, err
	}
	if _, err := m.RunCommand(candidateCtx, targetBinary, "doctor", "--config", m.ConfigPath, "--upgrade-check"); err != nil {
		return PrepareResult{}, fmt.Errorf("candidate doctor failed: %w", err)
	}
	backupDirectory := filepath.Join(m.Config.DataDir, "backups", "pre-update")
	backup, err := backupservice.Create(ctx, m.Store, m.ConfigPath, backupDirectory, 2, m.Now())
	if err != nil {
		return PrepareResult{}, fmt.Errorf("create pre-update backup: %w", err)
	}
	pending := PendingUpdate{
		FormatVersion: 1, Version: check.Manifest.Version, PreviousTarget: previousTarget,
		NewTarget: versionName, BackupPath: backup.Path, SchemaBefore: currentSchema,
		SchemaAfter: check.Manifest.SchemaVersion, CreatedAt: m.Now().UTC(), Attempts: 0,
	}
	if err := writePending(m.Config.DataDir, pending); err != nil {
		return PrepareResult{}, err
	}
	if err := switchCurrentRelease(releasesRoot, versionName); err != nil {
		_ = os.Remove(pendingPath(m.Config.DataDir))
		return PrepareResult{}, err
	}
	return PrepareResult{
		Version: check.Manifest.Version, BinaryPath: targetBinary, BackupPath: backup.Path,
		StablePath: m.StableBinaryPath(), PreviousTarget: previousTarget,
	}, nil
}

func (m *Manager) Bootstrap() (string, error) {
	versionName, err := releaseDirectoryName(m.CurrentVersion)
	if err != nil {
		return "", err
	}
	root := m.ReleasesRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create releases directory: %w", err)
	}
	current := filepath.Join(root, "current")
	if info, err := os.Lstat(current); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("release current path exists and is not a symbolic link")
		}
		return m.StableBinaryPath(), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect release current link: %w", err)
	}
	targetDirectory := filepath.Join(root, versionName)
	if err := os.MkdirAll(targetDirectory, 0o700); err != nil {
		return "", fmt.Errorf("create current release directory: %w", err)
	}
	targetBinary := filepath.Join(targetDirectory, "visitortrace")
	if _, err := os.Stat(targetBinary); os.IsNotExist(err) {
		if err := copyExecutable(m.ExecutablePath, targetBinary); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", fmt.Errorf("inspect bootstrapped executable: %w", err)
	} else {
		sourceChecksum, sourceSize, sourceErr := fileChecksum(m.ExecutablePath)
		targetChecksum, targetSize, targetErr := fileChecksum(targetBinary)
		if sourceErr != nil || targetErr != nil || sourceSize != targetSize || sourceChecksum != targetChecksum {
			return "", fmt.Errorf("bootstrap release directory contains a different executable")
		}
	}
	if err := switchCurrentRelease(root, versionName); err != nil {
		return "", err
	}
	return m.StableBinaryPath(), nil
}

func (m *Manager) RunningFromStablePath() bool {
	stable, err := filepath.Abs(m.StableBinaryPath())
	if err != nil {
		return false
	}
	invoked, err := filepath.Abs(m.Args0)
	return err == nil && filepath.Clean(invoked) == filepath.Clean(stable)
}

func (m *Manager) ReleasesRoot() string {
	return filepath.Join(m.Config.DataDir, "releases")
}

func (m *Manager) StableBinaryPath() string {
	return filepath.Join(m.ReleasesRoot(), "current", "visitortrace")
}

func (m *Manager) currentReleaseTarget() (string, error) {
	current := filepath.Join(m.ReleasesRoot(), "current")
	info, err := os.Lstat(current)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("self-update layout is not initialized; run visitortrace update bootstrap first")
		}
		return "", fmt.Errorf("inspect current release link: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("release current path is not a symbolic link")
	}
	target, err := os.Readlink(current)
	if err != nil {
		return "", fmt.Errorf("read current release link: %w", err)
	}
	if filepath.Base(target) != target || target == "." || target == ".." {
		return "", fmt.Errorf("release current link has an unsafe target")
	}
	if _, err := os.Stat(filepath.Join(m.ReleasesRoot(), target, "visitortrace")); err != nil {
		return "", fmt.Errorf("current release executable is unavailable: %w", err)
	}
	return target, nil
}

func (m *Manager) installCandidate(ctx context.Context, asset Asset, targetDirectory, targetBinary string) error {
	if info, err := os.Stat(targetBinary); err == nil && !info.IsDir() {
		checksum, size, err := fileChecksum(targetBinary)
		if err == nil && strings.EqualFold(checksum, asset.SHA256) && size == asset.Size {
			if err := os.Chmod(targetBinary, 0o700); err != nil {
				return fmt.Errorf("mark existing candidate executable: %w", err)
			}
			return nil
		}
		return fmt.Errorf("release directory already contains a different executable")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect release executable: %w", err)
	}
	if info, err := os.Stat(targetDirectory); err == nil && info.IsDir() {
		if err := os.RemoveAll(targetDirectory); err != nil {
			return fmt.Errorf("remove incomplete release directory: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect release directory: %w", err)
	}
	manifestURL, _ := url.Parse(m.Config.UpdateManifestURL)
	assetURL, err := manifestURL.Parse(asset.URL)
	if err != nil {
		return fmt.Errorf("resolve release asset URL: %w", err)
	}
	if err := validateDownloadURL(assetURL); err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(m.ReleasesRoot(), ".download-*")
	if err != nil {
		return fmt.Errorf("create release download workspace: %w", err)
	}
	defer os.RemoveAll(workspace)
	candidate := filepath.Join(workspace, "visitortrace")
	if err := m.downloadFile(ctx, assetURL.String(), candidate, asset); err != nil {
		return err
	}
	if err := os.Chmod(candidate, 0o700); err != nil {
		return fmt.Errorf("mark candidate executable: %w", err)
	}
	if err := os.Rename(workspace, targetDirectory); err != nil {
		return fmt.Errorf("activate versioned release directory: %w", err)
	}
	return nil
}

func (m *Manager) inspectCandidate(ctx context.Context, binary string, manifest Manifest) error {
	data, err := m.RunCommand(ctx, binary, "version", "--json")
	if err != nil {
		return fmt.Errorf("inspect candidate executable: %w", err)
	}
	var info CandidateInfo
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&info); err != nil {
		return fmt.Errorf("decode candidate version: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode candidate version: trailing content")
	}
	if info.Version != manifest.Version || info.SchemaVersion != manifest.SchemaVersion {
		return fmt.Errorf("candidate identity does not match signed manifest")
	}
	return nil
}

func (m *Manager) downloadBytes(ctx context.Context, source string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "VisitorTrace Self-Updater")
	response, err := m.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

func (m *Manager) downloadFile(ctx context.Context, source, destination string, asset Asset) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "VisitorTrace Self-Updater")
	response, err := m.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("download release executable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download release executable: HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != asset.Size {
		return fmt.Errorf("release executable size does not match manifest")
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create release executable: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, asset.Size+1))
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("write release executable: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close release executable: %w", closeErr)
	}
	if written != asset.Size {
		return fmt.Errorf("release executable size does not match manifest")
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, asset.SHA256) {
		return fmt.Errorf("release executable SHA-256 mismatch")
	}
	return nil
}

func validateDownloadURL(value *url.URL) error {
	if value == nil || value.Host == "" {
		return fmt.Errorf("release URL must be absolute")
	}
	if value.Scheme == "https" {
		return nil
	}
	host := strings.ToLower(value.Hostname())
	if value.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return nil
	}
	return fmt.Errorf("release URL must use HTTPS except on loopback")
}

func releaseDirectoryName(version string) (string, error) {
	if _, err := parseSemanticVersion(version); err != nil {
		return "", err
	}
	name := "v" + strings.TrimPrefix(version, "v")
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("release version is unsafe for a directory name")
	}
	return name, nil
}

func switchCurrentRelease(root, target string) error {
	if filepath.Base(target) != target || target == "." || target == ".." {
		return fmt.Errorf("release target is unsafe")
	}
	temporary := filepath.Join(root, ".current-"+fmt.Sprint(time.Now().UnixNano()))
	if err := os.Symlink(target, temporary); err != nil {
		return fmt.Errorf("create release link: %w", err)
	}
	defer os.Remove(temporary)
	if err := os.Rename(temporary, filepath.Join(root, "current")); err != nil {
		return fmt.Errorf("switch current release: %w", err)
	}
	return nil
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open current executable: %w", err)
	}
	defer input.Close()
	temporary := destination + ".tmp"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return fmt.Errorf("create bootstrapped executable: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("copy current executable: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("close bootstrapped executable: %w", closeErr)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("activate bootstrapped executable: %w", err)
	}
	return nil
}

func fileChecksum(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func runCommand(ctx context.Context, binary string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

type updateLock struct {
	path string
	file *os.File
}

func acquireUpdateLock(dataDir string, now time.Time) (*updateLock, error) {
	path := filepath.Join(dataDir, ".self-update.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		if info, statErr := os.Stat(path); statErr == nil && now.Sub(info.ModTime()) > updateLockAge {
			_ = os.Remove(path)
			file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("another self-update is active: %w", err)
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	return &updateLock{path: path, file: file}, nil
}

func (l *updateLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	_ = os.Remove(l.path)
}
