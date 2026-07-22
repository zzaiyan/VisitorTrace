package operations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestCollectReportsFilesAndTaskOutcomes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Default(dir)
	st, err := store.Initialize(ctx, cfg.DatabasePath, "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := os.WriteFile(cfg.GeoIPPath, []byte("geoip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.BackupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(cfg.BackupDir, "visitortrace-test.vtbackup")
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	for _, path := range []string{cfg.GeoIPPath, backupPath} {
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.StartOperation(ctx, "cleanup", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishOperation(ctx, "cleanup", now, true, "pageviews=2"); err != nil {
		t.Fatal(err)
	}
	snapshot := Collect(ctx, cfg, st, now.Add(-time.Hour), now)
	if snapshot.DatabaseSize == 0 || !snapshot.GeoIP.Exists || !snapshot.Backup.Exists || snapshot.Backup.Name != filepath.Base(backupPath) {
		t.Fatalf("Collect() = %#v", snapshot)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].State != "success" {
		t.Fatalf("task statuses = %#v", snapshot.Tasks)
	}
	for _, warning := range snapshot.Warnings {
		if warning == "geoip_missing" || warning == "backup_missing" || warning == "cleanup_stale" {
			t.Fatalf("unexpected warning %q in %#v", warning, snapshot.Warnings)
		}
	}
}
