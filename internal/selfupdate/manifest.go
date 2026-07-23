package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	ManifestFormatVersion = 1
	MaxManifestBytes      = int64(1 << 20)
	MaxReleaseAssetBytes  = int64(200 << 20)
)

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	FormatVersion int              `json:"format_version"`
	Version       string           `json:"version"`
	PublishedAt   time.Time        `json:"published_at"`
	SchemaVersion int              `json:"schema_version"`
	Assets        map[string]Asset `json:"assets"`
	Signature     string           `json:"signature"`
}

type signedPayload struct {
	FormatVersion int              `json:"format_version"`
	Version       string           `json:"version"`
	PublishedAt   time.Time        `json:"published_at"`
	SchemaVersion int              `json:"schema_version"`
	Assets        map[string]Asset `json:"assets"`
}

func DecodeManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Manifest{}, fmt.Errorf("decode release manifest: trailing content")
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	if err := ValidateUnsignedManifest(manifest); err != nil {
		return err
	}
	if manifest.Signature == "" {
		return fmt.Errorf("release manifest signature is required")
	}
	return nil
}

func ValidateUnsignedManifest(manifest Manifest) error {
	if manifest.FormatVersion != ManifestFormatVersion {
		return fmt.Errorf("unsupported release manifest format %d", manifest.FormatVersion)
	}
	if _, err := parseSemanticVersion(manifest.Version); err != nil {
		return fmt.Errorf("invalid release version: %w", err)
	}
	if manifest.PublishedAt.IsZero() {
		return fmt.Errorf("release publication time is required")
	}
	if manifest.SchemaVersion < 1 {
		return fmt.Errorf("release schema version must be positive")
	}
	if len(manifest.Assets) == 0 {
		return fmt.Errorf("release manifest has no assets")
	}
	for platform, asset := range manifest.Assets {
		if strings.TrimSpace(platform) == "" || strings.TrimSpace(asset.URL) == "" {
			return fmt.Errorf("release asset platform and URL are required")
		}
		if asset.Size < 1 || asset.Size > MaxReleaseAssetBytes {
			return fmt.Errorf("release asset %s has an invalid size", platform)
		}
		if len(asset.SHA256) != 64 {
			return fmt.Errorf("release asset %s has an invalid SHA-256", platform)
		}
		if _, err := hex.DecodeString(asset.SHA256); err != nil {
			return fmt.Errorf("release asset %s has an invalid SHA-256", platform)
		}
	}
	return nil
}

func SigningPayload(manifest Manifest) ([]byte, error) {
	return json.Marshal(signedPayload{
		FormatVersion: manifest.FormatVersion, Version: manifest.Version, PublishedAt: manifest.PublishedAt.UTC(),
		SchemaVersion: manifest.SchemaVersion, Assets: manifest.Assets,
	})
}

func VerifyManifest(manifest Manifest, publicKey []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("self-update public key is unavailable")
	}
	signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("release manifest signature is invalid")
	}
	payload, err := SigningPayload(manifest)
	if err != nil {
		return fmt.Errorf("encode release manifest signature payload: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return fmt.Errorf("release manifest signature verification failed")
	}
	return nil
}

func DecodePublicKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("self-update public key is unavailable")
	}
	key, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("self-update public key is invalid")
	}
	return key, nil
}

func SignManifest(manifest Manifest, privateKey ed25519.PrivateKey) (Manifest, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return Manifest{}, fmt.Errorf("Ed25519 private key is invalid")
	}
	if err := ValidateUnsignedManifest(manifest); err != nil {
		return Manifest{}, err
	}
	payload, err := SigningPayload(manifest)
	if err != nil {
		return Manifest{}, err
	}
	manifest.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	return manifest, nil
}
