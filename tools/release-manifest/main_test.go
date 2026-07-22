package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/zzaiyan/VisitorTrace/internal/selfupdate"
)

func TestGenerateAndSignManifest(t *testing.T) {
	directory := t.TempDir()
	assetPath := filepath.Join(directory, "visitortrace-linux-amd64")
	unsignedPath := filepath.Join(directory, "manifest.unsigned.json")
	signedPath := filepath.Join(directory, "manifest.json")
	privatePath := filepath.Join(directory, "update.ed25519")
	if err := os.WriteFile(assetPath, []byte("candidate executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privatePath, []byte(base64.RawStdEncoding.EncodeToString(privateKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generate([]string{
		"--version", "0.2.0",
		"--published-at", "2026-07-22T04:00:00+08:00",
		"--schema-version", "8",
		"--asset", "linux-amd64=" + assetPath,
		"--output", unsignedPath,
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := sign([]string{"--private-key", privatePath, "--manifest", unsignedPath, "--output", signedPath}); err != nil {
		t.Fatalf("sign: %v", err)
	}
	encodedPublicKey := base64.RawStdEncoding.EncodeToString(publicKey)
	if err := verify([]string{"--public-key", encodedPublicKey, "--manifest", signedPath}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	data, err := os.ReadFile(signedPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := selfupdate.DecodeManifest(data)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if err := selfupdate.VerifyManifest(manifest, publicKey); err != nil {
		t.Fatalf("verify manifest: %v", err)
	}
	asset := manifest.Assets["linux-amd64"]
	if asset.URL != filepath.Base(assetPath) || asset.Size != int64(len("candidate executable")) {
		t.Fatalf("asset = %#v", asset)
	}
	if manifest.PublishedAt.Format("2006-01-02T15:04:05Z07:00") != "2026-07-21T20:00:00Z" {
		t.Fatalf("published_at = %s", manifest.PublishedAt)
	}
}

func TestGenerateRejectsDuplicatePlatform(t *testing.T) {
	directory := t.TempDir()
	assetPath := filepath.Join(directory, "visitortrace")
	if err := os.WriteFile(assetPath, []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := generate([]string{
		"--version", "0.2.0", "--published-at", "2026-07-22T00:00:00Z",
		"--asset", "linux-amd64=" + assetPath,
		"--asset", "linux-amd64=" + assetPath,
		"--output", filepath.Join(directory, "manifest.json"),
	})
	if err == nil {
		t.Fatal("expected duplicate platform error")
	}
}
