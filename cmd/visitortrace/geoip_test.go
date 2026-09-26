package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func TestDoctorAcceptsOnlineOnlyGeoIP(t *testing.T) {
	cfg := config.Default(t.TempDir())
	cfg.GeoIPPreset = "basic"
	cfg.GeoIPBasicBackend = "ipinfo"
	cfg.OnlineServices = map[string]config.OnlineServiceConfig{"ipinfo": {Key: "token"}}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	st, err := store.Initialize(context.Background(), cfg.DatabasePath, "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if code := runDoctor([]string{"--config", configPath}); code != 0 {
		t.Fatalf("doctor rejected configured online-only GeoIP: %d", code)
	}
}

func TestWriteMMDBQueryOutput(t *testing.T) {
	var output bytes.Buffer
	err := writeMMDBQueryOutput(&output, mmdbQueryOutput{
		IP: "203.0.113.7",
		Database: mmdbQueryDatabase{
			Path: "/tmp/example.mmdb",
			Metadata: mmdbQueryMetadata{
				DatabaseType: "DB-IP City Lite",
				BuildEpoch:   1,
				BuildTime:    "1970-01-01T00:00:01Z",
			},
		},
		Found:          true,
		MatchedNetwork: "203.0.113.0/24",
		Record: map[string]any{
			"city": map[string]any{
				"names": map[string]any{"en": "Example City"},
			},
			"location": map[string]any{"latitude": 1.25},
		},
	})
	if err != nil {
		t.Fatalf("write output: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	if decoded["ip"] != "203.0.113.7" {
		t.Fatalf("unexpected IP: %#v", decoded["ip"])
	}
	if decoded["found"] != true {
		t.Fatalf("unexpected found value: %#v", decoded["found"])
	}
	if decoded["matched_network"] != "203.0.113.0/24" {
		t.Fatalf("unexpected network: %#v", decoded["matched_network"])
	}
	database, ok := decoded["database"].(map[string]any)
	if !ok || database["path"] != "/tmp/example.mmdb" {
		t.Fatalf("unexpected database: %#v", decoded["database"])
	}
	record, ok := decoded["record"].(map[string]any)
	if !ok || record["city"] == nil {
		t.Fatalf("unexpected raw record: %#v", decoded["record"])
	}
}
