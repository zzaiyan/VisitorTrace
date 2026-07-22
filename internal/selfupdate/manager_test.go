package selfupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestPrepareAndCompleteUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release layout uses symbolic links")
	}
	manager, st, cfg, configPath, closeServer := updateFixture(t)
	defer closeServer()
	defer st.Close()
	stable, err := manager.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if stable != manager.StableBinaryPath() {
		t.Fatalf("stable path = %q", stable)
	}
	manager.ConfigPath = configPath
	result, err := manager.PrepareAndActivate(context.Background())
	if err != nil {
		t.Fatalf("PrepareAndActivate() error = %v", err)
	}
	if result.Current || result.Version != "1.1.0" || !HasPending(cfg.DataDir) {
		t.Fatalf("PrepareAndActivate() = %#v", result)
	}
	target, err := os.Readlink(filepath.Join(manager.ReleasesRoot(), "current"))
	if err != nil || target != "v1.1.0" {
		t.Fatalf("current target = %q, %v", target, err)
	}
	if rolledBack, err := RegisterStartup(context.Background(), cfg, "1.1.0"); err != nil || rolledBack {
		t.Fatalf("RegisterStartup() = %v, %v", rolledBack, err)
	}
	if err := CompletePending(context.Background(), cfg, st, time.Now()); err != nil {
		t.Fatalf("CompletePending() error = %v", err)
	}
	if HasPending(cfg.DataDir) {
		t.Fatal("pending state remained after readiness")
	}
	statuses, err := st.OperationStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Operation != "self_update" || statuses[0].Succeeded == nil || !*statuses[0].Succeeded {
		t.Fatalf("update status = %#v", statuses)
	}
}

func TestThirdFailedStartupRollsBackReleaseAndDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release layout uses symbolic links")
	}
	manager, st, cfg, configPath, closeServer := updateFixture(t)
	defer closeServer()
	if _, err := manager.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	site, err := st.CreateSite(context.Background(), store.CreateSiteParams{Name: "Before update", AllowedOrigins: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	manager.ConfigPath = configPath
	if _, err := manager.PrepareAndActivate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(context.Background(), `UPDATE sites SET name = 'After update' WHERE id = ?`, site.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		rolledBack, err := RegisterStartup(context.Background(), cfg, "1.1.0")
		if err != nil {
			t.Fatalf("RegisterStartup(%d) error = %v", attempt, err)
		}
		if rolledBack != (attempt == 3) {
			t.Fatalf("RegisterStartup(%d) rolledBack = %v", attempt, rolledBack)
		}
	}
	target, err := os.Readlink(filepath.Join(manager.ReleasesRoot(), "current"))
	if err != nil || target != "v1.0.0" {
		t.Fatalf("rolled-back target = %q, %v", target, err)
	}
	restored, err := store.OpenExisting(context.Background(), cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredSite, err := restored.GetSite(context.Background(), site.ID)
	if err != nil || restoredSite.Name != "Before update" {
		t.Fatalf("restored Site = %#v, %v", restoredSite, err)
	}
}

func TestInterruptedActivationKeepsPreviousRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release layout uses symbolic links")
	}
	manager, st, cfg, _, closeServer := updateFixture(t)
	defer closeServer()
	if _, err := manager.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := st.StartOperation(context.Background(), "self_update", time.Now()); err != nil {
		t.Fatal(err)
	}
	pending := PendingUpdate{
		FormatVersion: pendingFormatVersion, Version: "1.1.0", PreviousTarget: "v1.0.0", NewTarget: "v1.1.0",
		BackupPath: filepath.Join(cfg.DataDir, "backups", "pre-update", "unused.vtbackup"), SchemaBefore: 6, SchemaAfter: 6, CreatedAt: time.Now(),
	}
	if err := writePending(cfg.DataDir, pending); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := RegisterStartup(context.Background(), cfg, "1.0.0")
	if err != nil || rolledBack || HasPending(cfg.DataDir) {
		t.Fatalf("RegisterStartup() = %v, %v; pending=%v", rolledBack, err, HasPending(cfg.DataDir))
	}
	target, err := os.Readlink(filepath.Join(manager.ReleasesRoot(), "current"))
	if err != nil || target != "v1.0.0" {
		t.Fatalf("current target = %q, %v", target, err)
	}
}

func updateFixture(t *testing.T) (*Manager, *store.Store, config.Config, string, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default(dir)
	cfg.GeoIPUpdate = "disabled"
	configPath := filepath.Join(dir, "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	st, err := store.Initialize(context.Background(), cfg.DatabasePath, "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	candidate := []byte("#!/bin/sh\ncase \"$1\" in\nversion) printf '%s\\n' '{\"version\":\"1.1.0\",\"commit\":\"test\",\"build_time\":\"2026-07-22T00:00:00Z\",\"schema_version\":6}' ;;\ndoctor) exit 0 ;;\n*) exit 1 ;;\nesac\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(candidate))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		FormatVersion: ManifestFormatVersion, Version: "1.1.0", PublishedAt: time.Date(2026, time.July, 22, 0, 0, 0, 0, time.UTC),
		SchemaVersion: 6, Assets: map[string]Asset{"test-platform": {URL: "/visitortrace", SHA256: digest, Size: int64(len(candidate))}},
	}
	manifest, err = SignManifest(manifest, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_, _ = w.Write(manifestData)
		case "/visitortrace":
			_, _ = w.Write(candidate)
		default:
			http.NotFound(w, r)
		}
	}))
	cfg.UpdateManifestURL = server.URL + "/manifest.json"
	if err := config.Save(configPath, cfg); err != nil {
		server.Close()
		t.Fatal(err)
	}
	currentExecutable := filepath.Join(dir, "current-visitortrace")
	if err := os.WriteFile(currentExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		server.Close()
		t.Fatal(err)
	}
	manager := New(cfg, configPath, st)
	manager.PublicKey = publicKey
	manager.CurrentVersion = "1.0.0"
	manager.Platform = "test-platform"
	manager.ExecutablePath = currentExecutable
	manager.Now = func() time.Time { return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC) }
	return manager, st, cfg, configPath, server.Close
}
