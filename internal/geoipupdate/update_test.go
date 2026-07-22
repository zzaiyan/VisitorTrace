package geoipupdate

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestRunOnceDownloadsVerifiesAndActivates(t *testing.T) {
	payload := []byte("valid city database")
	compressed := gzipBytes(t, payload)
	digest := fmt.Sprintf("%x", sha256.Sum256(compressed))
	server := geoIPServer(compressed, digest)
	defer server.Close()

	runner, st, cfg := testRunner(t, server.URL)
	defer st.Close()
	if err := os.WriteFile(cfg.GeoIPPath, []byte("previous database"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Validate = func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, payload) {
			return fmt.Errorf("unexpected candidate")
		}
		return nil
	}
	activated := false
	runner.Activate = func(path string) error {
		data, err := os.ReadFile(path)
		activated = err == nil && bytes.Equal(data, payload)
		return err
	}
	result, err := runner.RunOnce(context.Background(), true)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !result.Updated || result.SHA256 != digest || !activated {
		t.Fatalf("RunOnce() = %#v, activated = %v", result, activated)
	}
	current, err := os.ReadFile(cfg.GeoIPPath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("current GeoIP = %q, %v", current, err)
	}
	previous, err := os.ReadFile(cfg.GeoIPPath + ".previous")
	if err != nil || string(previous) != "previous database" {
		t.Fatalf("previous GeoIP = %q, %v", previous, err)
	}
	statuses, err := st.OperationStatuses(context.Background())
	if err != nil || len(statuses) != 1 || statuses[0].Succeeded == nil || !*statuses[0].Succeeded {
		t.Fatalf("operation statuses = %#v, %v", statuses, err)
	}
}

func TestRunOnceRejectsChecksumMismatch(t *testing.T) {
	server := geoIPServer(gzipBytes(t, []byte("candidate")), strings.Repeat("0", 64))
	defer server.Close()
	runner, st, cfg := testRunner(t, server.URL)
	defer st.Close()
	if err := os.WriteFile(cfg.GeoIPPath, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Validate = func(string) error { return nil }
	if _, err := runner.RunOnce(context.Background(), true); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("RunOnce() error = %v", err)
	}
	current, _ := os.ReadFile(cfg.GeoIPPath)
	if string(current) != "current" {
		t.Fatalf("current GeoIP changed to %q", current)
	}
}

func TestRunOnceRollsBackActivationFailure(t *testing.T) {
	payload := []byte("candidate")
	compressed := gzipBytes(t, payload)
	digest := fmt.Sprintf("%x", sha256.Sum256(compressed))
	server := geoIPServer(compressed, digest)
	defer server.Close()
	runner, st, cfg := testRunner(t, server.URL)
	defer st.Close()
	if err := os.WriteFile(cfg.GeoIPPath, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.Validate = func(string) error { return nil }
	runner.Activate = func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Equal(data, payload) {
			return fmt.Errorf("activation failed")
		}
		return nil
	}
	if _, err := runner.RunOnce(context.Background(), true); err == nil || !strings.Contains(err.Error(), "activation failed") {
		t.Fatalf("RunOnce() error = %v", err)
	}
	current, _ := os.ReadFile(cfg.GeoIPPath)
	if string(current) != "current" {
		t.Fatalf("rollback restored %q", current)
	}
}

func TestRunOnceSkipsFreshDatabase(t *testing.T) {
	runner, st, cfg := testRunner(t, "http://127.0.0.1:1")
	defer st.Close()
	if err := os.WriteFile(cfg.GeoIPPath, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cfg.GeoIPPath, now, now); err != nil {
		t.Fatal(err)
	}
	runner.Now = func() time.Time { return now }
	runner.Probe = func(string) error { return nil }
	result, err := runner.RunOnce(context.Background(), false)
	if err != nil || result.Updated {
		t.Fatalf("RunOnce() = %#v, %v", result, err)
	}
}

func testRunner(t *testing.T, baseURL string) (*Runner, *store.Store, config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default(dir)
	cfg.GeoIPUpdateURL = baseURL + "/dbip-city-lite-{YYYY-MM}.mmdb.gz"
	cfg.GeoIPChecksumURL = baseURL + "/dbip-city-lite-{YYYY-MM}.mmdb.gz.sha256"
	st, err := store.Initialize(context.Background(), cfg.DatabasePath, "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	runner := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.Now = func() time.Time { return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC) }
	return runner, st, cfg
}

func geoIPServer(database []byte, checksum string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			_, _ = io.WriteString(w, checksum+"  database.mmdb.gz\n")
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(database)
	}))
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestValidateRemoteURL(t *testing.T) {
	for _, value := range []string{"https://example.com/file", "http://127.0.0.1/file"} {
		parsed, _ := url.Parse(value)
		if err := validateRemoteURL(parsed); err != nil {
			t.Errorf("validateRemoteURL(%q) = %v", value, err)
		}
	}
	parsed, _ := url.Parse("http://example.com/file")
	if err := validateRemoteURL(parsed); err == nil {
		t.Fatal("validateRemoteURL() accepted remote HTTP")
	}
}
