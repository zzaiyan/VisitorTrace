package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	backupservice "github.com/zzaiyan/VisitorTrace/internal/backup"
	"github.com/zzaiyan/VisitorTrace/internal/buildinfo"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const pendingFormatVersion = 1

type PendingUpdate struct {
	FormatVersion  int       `json:"format_version"`
	Version        string    `json:"version"`
	PreviousTarget string    `json:"previous_target"`
	NewTarget      string    `json:"new_target"`
	BackupPath     string    `json:"backup_path"`
	SchemaBefore   int       `json:"schema_before"`
	SchemaAfter    int       `json:"schema_after"`
	CreatedAt      time.Time `json:"created_at"`
	Attempts       int       `json:"attempts"`
}

func HasPending(dataDir string) bool {
	_, err := os.Stat(pendingPath(dataDir))
	return err == nil
}

// RegisterStartup records a startup attempt. On the third failed restart it
// restores the pre-update snapshot and switches the stable link back.
func RegisterStartup(ctx context.Context, cfg config.Config, runningVersions ...string) (bool, error) {
	pending, err := readPending(cfg.DataDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	root := filepath.Join(cfg.DataDir, "releases")
	currentTarget, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil {
		return false, fmt.Errorf("read pending update release link: %w", err)
	}
	if currentTarget == pending.PreviousTarget {
		if current, openErr := store.OpenExisting(ctx, cfg.DatabasePath); openErr == nil {
			_ = current.FinishOperation(ctx, "self_update", time.Now().UTC(), false, "activation interrupted before release switch")
			_ = current.Close()
		}
		if err := os.Remove(pendingPath(cfg.DataDir)); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("clear interrupted update state: %w", err)
		}
		return false, nil
	}
	runningVersion := buildinfo.Version
	if len(runningVersions) > 0 {
		runningVersion = runningVersions[0]
	}
	if runningVersion != pending.Version {
		return false, fmt.Errorf("pending update expects version %s but version %s started", pending.Version, runningVersion)
	}
	if currentTarget != pending.NewTarget {
		return false, fmt.Errorf("pending update target does not match the current release link")
	}
	pending.Attempts++
	if pending.Attempts < 3 {
		if err := writePending(cfg.DataDir, pending); err != nil {
			return false, err
		}
		return false, nil
	}
	if _, err := backupservice.RestoreForRollback(ctx, pending.BackupPath, cfg.DatabasePath); err != nil {
		return false, fmt.Errorf("restore pre-update backup: %w", err)
	}
	if err := switchCurrentRelease(root, pending.PreviousTarget); err != nil {
		return false, fmt.Errorf("switch back to previous release: %w", err)
	}
	if restored, err := store.OpenExisting(ctx, cfg.DatabasePath); err == nil {
		_ = restored.FinishOperation(ctx, "self_update", time.Now().UTC(), false, "rolled back after three failed startup attempts")
		_ = restored.Close()
	}
	if err := os.Remove(pendingPath(cfg.DataDir)); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("clear rolled-back update state: %w", err)
	}
	return true, nil
}

func CompletePending(ctx context.Context, cfg config.Config, st *store.Store, now time.Time) error {
	pending, err := readPending(cfg.DataDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := st.FinishOperation(ctx, "self_update", now.UTC(), true, "activated version="+pending.Version); err != nil {
		return err
	}
	if err := os.Remove(pendingPath(cfg.DataDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear completed update state: %w", err)
	}
	pruneReleases(filepath.Join(cfg.DataDir, "releases"), pending.NewTarget, pending.PreviousTarget)
	return nil
}

func pendingPath(dataDir string) string {
	return filepath.Join(dataDir, ".update-pending.json")
}

func writePending(dataDir string, pending PendingUpdate) error {
	if err := validatePending(dataDir, pending); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pending update: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create update state directory: %w", err)
	}
	temporary, err := os.CreateTemp(dataDir, ".update-pending-*.tmp")
	if err != nil {
		return fmt.Errorf("create pending update state: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write pending update state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync pending update state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close pending update state: %w", err)
	}
	if err := os.Rename(name, pendingPath(dataDir)); err != nil {
		return fmt.Errorf("activate pending update state: %w", err)
	}
	return nil
}

func readPending(dataDir string) (PendingUpdate, error) {
	data, err := os.ReadFile(pendingPath(dataDir))
	if err != nil {
		return PendingUpdate{}, err
	}
	var pending PendingUpdate
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pending); err != nil {
		return PendingUpdate{}, fmt.Errorf("decode pending update: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return PendingUpdate{}, fmt.Errorf("decode pending update: trailing content")
	}
	if err := validatePending(dataDir, pending); err != nil {
		return PendingUpdate{}, err
	}
	return pending, nil
}

func validatePending(dataDir string, pending PendingUpdate) error {
	if pending.FormatVersion != pendingFormatVersion {
		return fmt.Errorf("unsupported pending update format %d", pending.FormatVersion)
	}
	if _, err := parseSemanticVersion(pending.Version); err != nil {
		return fmt.Errorf("pending update has invalid version: %w", err)
	}
	for _, target := range []string{pending.PreviousTarget, pending.NewTarget} {
		if filepath.Base(target) != target || target == "." || target == ".." {
			return fmt.Errorf("pending update has unsafe release target")
		}
	}
	backupRoot, _ := filepath.Abs(filepath.Join(dataDir, "backups", "pre-update"))
	backupPath, _ := filepath.Abs(pending.BackupPath)
	relative, err := filepath.Rel(backupRoot, backupPath)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("pending update has unsafe backup path")
	}
	if pending.SchemaBefore < 1 || pending.SchemaAfter < pending.SchemaBefore || pending.Attempts < 0 {
		return fmt.Errorf("pending update has invalid schema or attempt state")
	}
	return nil
}

func pruneReleases(root, current, previous string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type candidate struct {
		name string
		time time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == current || entry.Name() == previous {
			continue
		}
		if _, err := parseSemanticVersion(entry.Name()); err != nil {
			continue
		}
		if info, err := entry.Info(); err == nil {
			candidates = append(candidates, candidate{name: entry.Name(), time: info.ModTime()})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].time.After(candidates[j].time) })
	if len(candidates) <= 1 {
		return
	}
	for _, item := range candidates[1:] {
		_ = os.RemoveAll(filepath.Join(root, item.name))
	}
}
