package selfupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestManifestSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		FormatVersion: ManifestFormatVersion, Version: "1.2.3", PublishedAt: time.Date(2026, time.July, 22, 0, 0, 0, 0, time.UTC),
		SchemaVersion: 6, Assets: map[string]Asset{"linux-amd64": {URL: "visitortrace", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 42}},
	}
	signed, err := SignManifest(manifest, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyManifest(signed, publicKey); err != nil {
		t.Fatalf("VerifyManifest() error = %v", err)
	}
	signed.Version = "1.2.4"
	if err := VerifyManifest(signed, publicKey); err == nil {
		t.Fatal("VerifyManifest() accepted a modified release")
	}
}

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1},
		{"v2.0.0+build", "1.9.9", 1},
	}
	for _, test := range tests {
		got, err := compareSemanticVersions(test.left, test.right)
		if err != nil || got != test.want {
			t.Errorf("compareSemanticVersions(%q, %q) = %d, %v; want %d", test.left, test.right, got, err, test.want)
		}
	}
}
