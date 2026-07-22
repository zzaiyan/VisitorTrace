package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestCreateAndRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfg := config.Default(dataDir)
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	st, err := store.Initialize(ctx, cfg.DatabasePath, "original-password-hash")
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	site, err := st.CreateSite(ctx, store.CreateSiteParams{Name: "Before backup", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	now := time.Date(2026, time.July, 22, 3, 30, 0, 0, time.UTC)
	result, err := Create(ctx, st, configPath, cfg.BackupDir, 3, now)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := VerifyArchiveChecksum(result.Path); err != nil {
		t.Fatalf("VerifyArchiveChecksum() error = %v", err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE sites SET name = 'After backup' WHERE id = ?`, site.ID); err != nil {
		t.Fatalf("mutate database: %v", err)
	}
	if err := st.CreateAdministratorSession(ctx, make([]byte, 32), make([]byte, 32), now, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateAdministratorSession() error = %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	manifest, err := Restore(ctx, result.Path, cfg.DatabasePath)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if manifest.SchemaVersion == 0 || !manifest.CreatedAt.Equal(now) {
		t.Fatalf("Restore() manifest = %#v", manifest)
	}
	restored, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		t.Fatalf("OpenExisting() error = %v", err)
	}
	defer restored.Close()
	got, err := restored.GetSite(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSite() error = %v", err)
	}
	if got.Name != "Before backup" {
		t.Fatalf("restored Site name = %q", got.Name)
	}
	var sessions int
	if err := restored.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM administrator_sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("restored sessions = %d, want 0", sessions)
	}
}

func TestArchiveChecksumDetectsTampering(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Default(filepath.Join(dir, "data"))
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	st, err := store.Initialize(ctx, cfg.DatabasePath, "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	result, err := Create(ctx, st, configPath, cfg.BackupDir, 3, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(result.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("tampered"); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err := VerifyArchiveChecksum(result.Path); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("VerifyArchiveChecksum() error = %v", err)
	}
}

func TestCreatePrunesOldBackups(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Default(filepath.Join(dir, "data"))
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	st, err := store.Initialize(ctx, cfg.DatabasePath, "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	base := time.Date(2026, time.July, 22, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		if _, err := Create(ctx, st, configPath, cfg.BackupDir, 3, base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("Create(%d) error = %v", i, err)
		}
	}
	archives, err := filepath.Glob(filepath.Join(cfg.BackupDir, "*.vtbackup"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 3 {
		t.Fatalf("retained archives = %d, want 3", len(archives))
	}
}
