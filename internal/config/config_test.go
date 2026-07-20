package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visitortrace.json")
	want := Default(filepath.Join(dir, "data"))
	want.Listen = "127.0.0.1:9876"
	want.TrustedProxies = []string{"127.0.0.1/32"}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != want.Version || got.DataDir != want.DataDir || got.Listen != want.Listen {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, want)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat config: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visitortrace.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"data_dir":"/tmp/data","database_path":"/tmp/data.db","geoip_path":"/tmp/geoip.mmdb","listen":"127.0.0.1:8790","unexpected":true}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted an unknown field")
	}
}
