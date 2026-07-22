package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/selfupdate"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "generate":
		err = generate(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type assetFlags []string

func (values *assetFlags) String() string { return strings.Join(*values, ",") }

func (values *assetFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	privatePath := fs.String("private-key", "", "new protected private-key file")
	publicPath := fs.String("public-key", "", "new public-key file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *privatePath == "" || *publicPath == "" {
		return fmt.Errorf("keygen: --private-key and --public-key are required")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("keygen: %w", err)
	}
	if err := writeNewFile(*privatePath, []byte(base64.RawStdEncoding.EncodeToString(privateKey)+"\n"), 0o600); err != nil {
		return fmt.Errorf("keygen private key: %w", err)
	}
	if err := writeNewFile(*publicPath, []byte(base64.RawStdEncoding.EncodeToString(publicKey)+"\n"), 0o644); err != nil {
		_ = os.Remove(*privatePath)
		return fmt.Errorf("keygen public key: %w", err)
	}
	fmt.Printf("generated Ed25519 release key pair\nprivate: %s\npublic: %s\nUPDATE_PUBLIC_KEY=%s\n", *privatePath, *publicPath, base64.RawStdEncoding.EncodeToString(publicKey))
	return nil
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	version := fs.String("version", "", "release semantic version without a v prefix")
	publishedAt := fs.String("published-at", "", "release publication time in RFC3339 format")
	schemaVersion := fs.Int("schema-version", store.SupportedSchemaVersion(), "release database schema version")
	outputPath := fs.String("output", "", "unsigned manifest output")
	var assetValues assetFlags
	fs.Var(&assetValues, "asset", "release asset as platform=path; repeat for every platform")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *publishedAt == "" || *outputPath == "" || len(assetValues) == 0 {
		return fmt.Errorf("generate: --version, --published-at, --output, and at least one --asset are required")
	}
	publicationTime, err := time.Parse(time.RFC3339, *publishedAt)
	if err != nil {
		return fmt.Errorf("generate: parse published-at: %w", err)
	}
	manifest := selfupdate.Manifest{
		FormatVersion: selfupdate.ManifestFormatVersion,
		Version:       *version,
		PublishedAt:   publicationTime.UTC(),
		SchemaVersion: *schemaVersion,
		Assets:        make(map[string]selfupdate.Asset, len(assetValues)),
	}
	for _, value := range assetValues {
		platform, path, ok := strings.Cut(value, "=")
		platform = strings.TrimSpace(platform)
		path = strings.TrimSpace(path)
		if !ok || platform == "" || path == "" {
			return fmt.Errorf("generate: asset %q must use platform=path", value)
		}
		if _, exists := manifest.Assets[platform]; exists {
			return fmt.Errorf("generate: duplicate asset platform %q", platform)
		}
		checksum, size, err := checksumFile(path)
		if err != nil {
			return fmt.Errorf("generate: asset %s: %w", platform, err)
		}
		manifest.Assets[platform] = selfupdate.Asset{
			URL: filepath.Base(path), SHA256: checksum, Size: size,
		}
	}
	if err := selfupdate.ValidateUnsignedManifest(manifest); err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	output, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("generate: encode manifest: %w", err)
	}
	if err := writeNewFile(*outputPath, append(output, '\n'), 0o644); err != nil {
		return fmt.Errorf("generate: write manifest: %w", err)
	}
	return nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	privatePath := fs.String("private-key", "", "protected Ed25519 private-key file")
	manifestPath := fs.String("manifest", "", "unsigned manifest JSON")
	outputPath := fs.String("output", "", "signed manifest output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *privatePath == "" || *manifestPath == "" || *outputPath == "" {
		return fmt.Errorf("sign: --private-key, --manifest, and --output are required")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(*privatePath)
		if err != nil {
			return fmt.Errorf("sign: stat private key: %w", err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("sign: private-key permissions %o are too broad; want 600", info.Mode().Perm())
		}
	}
	keyData, err := os.ReadFile(*privatePath)
	if err != nil {
		return fmt.Errorf("sign: read private key: %w", err)
	}
	privateKey, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("sign: private key is invalid")
	}
	manifestData, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("sign: read manifest: %w", err)
	}
	var manifest selfupdate.Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("sign: decode manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("sign: manifest has trailing content")
	}
	signed, err := selfupdate.SignManifest(manifest, ed25519.PrivateKey(privateKey))
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	output, err := json.MarshalIndent(signed, "", "  ")
	if err != nil {
		return fmt.Errorf("sign: encode signed manifest: %w", err)
	}
	output = append(output, '\n')
	if err := writeNewFile(*outputPath, output, 0o644); err != nil {
		return fmt.Errorf("sign: write signed manifest: %w", err)
	}
	return nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	publicKeyValue := fs.String("public-key", "", "Base64 RawStdEncoding Ed25519 public key")
	manifestPath := fs.String("manifest", "", "signed manifest JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *publicKeyValue == "" || *manifestPath == "" {
		return fmt.Errorf("verify: --public-key and --manifest are required")
	}
	publicKey, err := selfupdate.DecodePublicKey(*publicKeyValue)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	data, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("verify: read manifest: %w", err)
	}
	manifest, err := selfupdate.DecodeManifest(data)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if err := selfupdate.VerifyManifest(manifest, publicKey); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	return nil
}

func checksumFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("not a regular file")
	}
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	if written != info.Size() {
		return "", 0, fmt.Errorf("read %s of %s bytes", strconv.FormatInt(written, 10), strconv.FormatInt(info.Size(), 10))
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./tools/release-manifest <keygen|generate|sign|verify> [flags]")
}
