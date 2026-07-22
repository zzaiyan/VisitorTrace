package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

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
