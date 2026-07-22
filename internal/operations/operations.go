package operations

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/buildinfo"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

type FileStatus struct {
	Exists     bool
	Name       string
	Size       int64
	ModifiedAt time.Time
	Stale      bool
}

type TaskStatus struct {
	Operation   string
	State       string
	StartedAt   time.Time
	CompletedAt *time.Time
	Summary     string
}

type Snapshot struct {
	Version       string
	Commit        string
	BuildTime     string
	StartedAt     time.Time
	Uptime        time.Duration
	SQLiteVersion string
	SchemaVersion int
	DatabaseSize  int64
	DiskAvailable uint64
	DiskTotal     uint64
	DiskLow       bool
	GeoIP         FileStatus
	Backup        FileStatus
	Tasks         []TaskStatus
	Warnings      []string
}

func Collect(ctx context.Context, cfg config.Config, st *store.Store, startedAt, now time.Time) Snapshot {
	now = now.UTC()
	result := Snapshot{
		Version: buildinfo.Version, Commit: buildinfo.Commit, BuildTime: buildinfo.BuildTime,
		StartedAt: startedAt.UTC(), Uptime: now.Sub(startedAt.UTC()),
	}
	result.DatabaseSize = sqliteFilesSize(cfg.DatabasePath)
	result.SQLiteVersion, _ = st.SQLiteVersion(ctx)
	result.SchemaVersion, _ = st.SchemaVersion(ctx)
	result.DiskAvailable, result.DiskTotal, _ = diskSpace(cfg.DataDir)
	result.DiskLow = result.DiskTotal > 0 && (result.DiskAvailable < 1<<30 || result.DiskAvailable*100/result.DiskTotal < 5)
	if result.DiskLow {
		result.Warnings = append(result.Warnings, "disk_low")
	}
	result.GeoIP = fileStatus(cfg.GeoIPPath)
	result.GeoIP.Stale = result.GeoIP.Exists && now.Sub(result.GeoIP.ModifiedAt) > 35*24*time.Hour
	if !result.GeoIP.Exists {
		result.Warnings = append(result.Warnings, "geoip_missing")
	} else if result.GeoIP.Stale {
		result.Warnings = append(result.Warnings, "geoip_stale")
	}
	result.Backup = latestBackup(cfg.BackupDir)
	result.Backup.Stale = result.Backup.Exists && now.Sub(result.Backup.ModifiedAt) > 48*time.Hour
	if !result.Backup.Exists {
		result.Warnings = append(result.Warnings, "backup_missing")
	} else if result.Backup.Stale {
		result.Warnings = append(result.Warnings, "backup_stale")
	}
	statuses, err := st.OperationStatuses(ctx)
	if err == nil {
		for _, item := range statuses {
			state := "running"
			if item.Succeeded != nil {
				if *item.Succeeded {
					state = "success"
				} else {
					state = "failed"
					result.Warnings = append(result.Warnings, item.Operation+"_failed")
				}
			}
			result.Tasks = append(result.Tasks, TaskStatus{
				Operation: item.Operation, State: state, StartedAt: item.StartedAt,
				CompletedAt: item.CompletedAt, Summary: item.Summary,
			})
		}
	}
	if now.Sub(startedAt.UTC()) > 5*time.Minute && cleanupIsStale(statuses, now) {
		result.Warnings = append(result.Warnings, "cleanup_stale")
	}
	return result
}

func sqliteFilesSize(path string) int64 {
	var size int64
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			size += info.Size()
		}
	}
	return size
}

func fileStatus(path string) FileStatus {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return FileStatus{Name: filepath.Base(path)}
	}
	return FileStatus{Exists: true, Name: info.Name(), Size: info.Size(), ModifiedAt: info.ModTime().UTC()}
}

func latestBackup(directory string) FileStatus {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return FileStatus{}
	}
	var candidates []FileStatus
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".vtbackup") {
			continue
		}
		info, err := entry.Info()
		if err == nil {
			candidates = append(candidates, FileStatus{Exists: true, Name: entry.Name(), Size: info.Size(), ModifiedAt: info.ModTime().UTC()})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ModifiedAt.After(candidates[j].ModifiedAt) })
	if len(candidates) == 0 {
		return FileStatus{}
	}
	return candidates[0]
}

func cleanupIsStale(statuses []store.OperationStatus, now time.Time) bool {
	for _, item := range statuses {
		if item.Operation != "cleanup" {
			continue
		}
		if item.CompletedAt == nil {
			return now.Sub(item.StartedAt) > 2*time.Hour
		}
		return now.Sub(*item.CompletedAt) > 2*time.Hour
	}
	return true
}
