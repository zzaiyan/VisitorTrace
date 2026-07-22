package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/buildinfo"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

const (
	formatVersion = 1
	databaseName  = "visitortrace.sqlite3"
	configName    = "config.json"
	manifestName  = "manifest.json"
	extension     = ".vtbackup"
)

type FileMetadata struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	FormatVersion int                     `json:"format_version"`
	CreatedAt     time.Time               `json:"created_at"`
	AppVersion    string                  `json:"app_version"`
	SchemaVersion int                     `json:"schema_version"`
	Files         map[string]FileMetadata `json:"files"`
}

type Result struct {
	Path     string
	Checksum string
	Manifest Manifest
}

func CreateTracked(ctx context.Context, st *store.Store, configPath, outputDir string, keep int, now time.Time) (Result, error) {
	started := now.UTC()
	result, runErr := Create(ctx, st, configPath, outputDir, keep, now)
	if err := st.StartOperation(ctx, "backup", started); err != nil {
		if runErr != nil {
			return result, fmt.Errorf("%v; record backup status: %w", runErr, err)
		}
		return result, fmt.Errorf("record backup status: %w", err)
	}
	summary := "error="
	if runErr == nil {
		summary = "archive=" + filepath.Base(result.Path) + " sha256=" + result.Checksum
	} else {
		summary += runErr.Error()
	}
	if err := st.FinishOperation(ctx, "backup", time.Now().UTC(), runErr == nil, summary); err != nil && runErr == nil {
		runErr = err
	}
	return result, runErr
}

func Create(ctx context.Context, st *store.Store, configPath, outputDir string, keep int, now time.Time) (Result, error) {
	if keep < 1 {
		return Result{}, fmt.Errorf("backup retention must be at least one")
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create backup directory: %w", err)
	}
	if err := os.Chmod(outputDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("protect backup directory: %w", err)
	}
	workDir, err := os.MkdirTemp(outputDir, ".backup-*")
	if err != nil {
		return Result{}, fmt.Errorf("create backup workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	databasePath := filepath.Join(workDir, databaseName)
	if err := st.OnlineBackup(ctx, databasePath); err != nil {
		return Result{}, err
	}
	if err := store.IntegrityCheckFile(ctx, databasePath); err != nil {
		return Result{}, err
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return Result{}, fmt.Errorf("read configuration for backup: %w", err)
	}
	configSnapshot := filepath.Join(workDir, configName)
	if err := os.WriteFile(configSnapshot, configData, 0o600); err != nil {
		return Result{}, fmt.Errorf("write configuration snapshot: %w", err)
	}
	schemaVersion, err := st.SchemaVersion(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("read schema version for backup: %w", err)
	}
	manifest := Manifest{
		FormatVersion: formatVersion,
		CreatedAt:     now.UTC(),
		AppVersion:    buildinfo.Version,
		SchemaVersion: schemaVersion,
		Files:         make(map[string]FileMetadata, 2),
	}
	for _, name := range []string{databaseName, configName} {
		metadata, err := fileMetadata(filepath.Join(workDir, name))
		if err != nil {
			return Result{}, err
		}
		manifest.Files[name] = metadata
	}

	stamp := now.UTC().Format("20060102T150405.000000000Z")
	finalPath := filepath.Join(outputDir, "visitortrace-"+stamp+extension)
	temporaryPath := finalPath + ".tmp"
	if err := writeArchive(temporaryPath, workDir, manifest); err != nil {
		return Result{}, err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		_ = os.Remove(temporaryPath)
		return Result{}, fmt.Errorf("activate backup archive: %w", err)
	}
	checksum, _, err := checksumFile(finalPath)
	if err != nil {
		_ = os.Remove(finalPath)
		return Result{}, err
	}
	sidecar := finalPath + ".sha256"
	if err := writeAtomic(sidecar, []byte(checksum+"  "+filepath.Base(finalPath)+"\n"), 0o600); err != nil {
		_ = os.Remove(finalPath)
		return Result{}, err
	}
	if err := prune(outputDir, keep); err != nil {
		return Result{}, err
	}
	return Result{Path: finalPath, Checksum: checksum, Manifest: manifest}, nil
}

func Restore(ctx context.Context, archivePath, databasePath string) (Manifest, error) {
	return restore(ctx, archivePath, databasePath, true)
}

func RestoreForRollback(ctx context.Context, archivePath, databasePath string) (Manifest, error) {
	return restore(ctx, archivePath, databasePath, false)
}

func restore(ctx context.Context, archivePath, databasePath string, migrate bool) (Manifest, error) {
	if err := VerifyArchiveChecksum(archivePath); err != nil {
		return Manifest{}, err
	}
	targetDir := filepath.Dir(databasePath)
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("create database directory: %w", err)
	}
	workDir, err := os.MkdirTemp(targetDir, ".restore-*")
	if err != nil {
		return Manifest{}, fmt.Errorf("create restore workspace: %w", err)
	}
	defer os.RemoveAll(workDir)
	manifest, err := extractAndVerify(archivePath, workDir)
	if err != nil {
		return Manifest{}, err
	}
	extractedDatabase := filepath.Join(workDir, databaseName)
	if err := store.IntegrityCheckFile(ctx, extractedDatabase); err != nil {
		return Manifest{}, err
	}
	restored, err := store.OpenExisting(ctx, extractedDatabase)
	if err != nil {
		return Manifest{}, fmt.Errorf("open restored database: %w", err)
	}
	if migrate {
		if err := restored.Migrate(ctx); err != nil {
			_ = restored.Close()
			return Manifest{}, fmt.Errorf("migrate restored database: %w", err)
		}
	}
	if err := restored.RevokeAdministratorSessions(ctx); err != nil {
		_ = restored.Close()
		return Manifest{}, err
	}
	if err := restored.Close(); err != nil {
		return Manifest{}, fmt.Errorf("close restored database: %w", err)
	}
	if err := os.Chmod(extractedDatabase, 0o600); err != nil {
		return Manifest{}, fmt.Errorf("protect restored database: %w", err)
	}

	rollbackPath := databasePath + ".restore-rollback"
	_ = os.Remove(rollbackPath)
	_ = os.Remove(rollbackPath + "-wal")
	_ = os.Remove(rollbackPath + "-shm")
	if _, err := os.Stat(databasePath); err == nil {
		if err := os.Rename(databasePath, rollbackPath); err != nil {
			return Manifest{}, fmt.Errorf("stage current database: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("check current database: %w", err)
	}
	removeSQLiteSidecars(databasePath)
	if err := os.Rename(extractedDatabase, databasePath); err != nil {
		_ = os.Rename(rollbackPath, databasePath)
		return Manifest{}, fmt.Errorf("activate restored database: %w", err)
	}
	_ = os.Remove(rollbackPath)
	removeSQLiteSidecars(rollbackPath)
	return manifest, nil
}

func VerifyArchiveChecksum(archivePath string) error {
	data, err := os.ReadFile(archivePath + ".sha256")
	if err != nil {
		return fmt.Errorf("read backup checksum: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 || len(fields[0]) != sha256.Size*2 {
		return fmt.Errorf("backup checksum file is invalid")
	}
	actual, _, err := checksumFile(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(fields[0], actual) {
		return fmt.Errorf("backup archive checksum mismatch")
	}
	return nil
}

func extractAndVerify(archivePath, destination string) (Manifest, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return Manifest{}, fmt.Errorf("open backup archive: %w", err)
	}
	defer reader.Close()
	allowed := map[string]bool{databaseName: true, configName: true, manifestName: true}
	seen := make(map[string]bool, len(allowed))
	for _, item := range reader.File {
		if !allowed[item.Name] || seen[item.Name] || filepath.Base(item.Name) != item.Name {
			return Manifest{}, fmt.Errorf("backup archive contains unexpected entry %q", item.Name)
		}
		seen[item.Name] = true
		if item.UncompressedSize64 > 8<<30 {
			return Manifest{}, fmt.Errorf("backup entry %q is too large", item.Name)
		}
		if err := extractFile(item, filepath.Join(destination, item.Name)); err != nil {
			return Manifest{}, err
		}
	}
	for name := range allowed {
		if !seen[name] {
			return Manifest{}, fmt.Errorf("backup archive is missing %s", name)
		}
	}
	manifestData, err := os.ReadFile(filepath.Join(destination, manifestName))
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestData)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode backup manifest: %w", err)
	}
	if manifest.FormatVersion != formatVersion {
		return Manifest{}, fmt.Errorf("unsupported backup format %d", manifest.FormatVersion)
	}
	for _, name := range []string{databaseName, configName} {
		want, ok := manifest.Files[name]
		if !ok {
			return Manifest{}, fmt.Errorf("backup manifest is missing %s metadata", name)
		}
		actual, size, err := checksumFile(filepath.Join(destination, name))
		if err != nil {
			return Manifest{}, err
		}
		if !strings.EqualFold(want.SHA256, actual) || want.Size != size {
			return Manifest{}, fmt.Errorf("backup entry %s failed verification", name)
		}
	}
	return manifest, nil
}

func writeArchive(path, sourceDir string, manifest Manifest) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create backup archive: %w", err)
	}
	writer := zip.NewWriter(file)
	closeWithError := func(cause error) error {
		_ = writer.Close()
		_ = file.Close()
		_ = os.Remove(path)
		return cause
	}
	for _, name := range []string{databaseName, configName} {
		if err := addFile(writer, name, filepath.Join(sourceDir, name)); err != nil {
			return closeWithError(err)
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return closeWithError(fmt.Errorf("encode backup manifest: %w", err))
	}
	data = append(data, '\n')
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: manifestName, Method: zip.Deflate})
	if err != nil {
		return closeWithError(fmt.Errorf("create backup manifest entry: %w", err))
	}
	if _, err := entry.Write(data); err != nil {
		return closeWithError(fmt.Errorf("write backup manifest entry: %w", err))
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("close backup archive: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sync backup archive: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close backup archive file: %w", err)
	}
	return nil
}

func addFile(writer *zip.Writer, name, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open backup source %s: %w", name, err)
	}
	defer source.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o600)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create backup entry %s: %w", name, err)
	}
	if _, err := io.Copy(entry, source); err != nil {
		return fmt.Errorf("write backup entry %s: %w", name, err)
	}
	return nil
}

func extractFile(item *zip.File, path string) error {
	source, err := item.Open()
	if err != nil {
		return fmt.Errorf("open backup entry %s: %w", item.Name, err)
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restored entry %s: %w", item.Name, err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return fmt.Errorf("extract backup entry %s: %w", item.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close restored entry %s: %w", item.Name, closeErr)
	}
	return nil
}

func fileMetadata(path string) (FileMetadata, error) {
	checksum, size, err := checksumFile(path)
	if err != nil {
		return FileMetadata{}, err
	}
	return FileMetadata{SHA256: checksum, Size: size}, nil
}

func checksumFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open file for checksum: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, fmt.Errorf("checksum file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".write-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func prune(directory string, keep int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("list backup directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "visitortrace-") && strings.HasSuffix(entry.Name(), extension) {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	if len(paths) <= keep {
		return nil
	}
	for _, path := range paths[keep:] {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove expired backup %s: %w", filepath.Base(path), err)
		}
		_ = os.Remove(path + ".sha256")
	}
	return nil
}

func removeSQLiteSidecars(path string) {
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}
