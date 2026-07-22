package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	backupservice "github.com/zzaiyan/VisitorTrace/internal/backup"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func runBackup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	outputDir := fs.String("output", "", "backup directory override")
	keep := fs.Int("keep", 3, "number of local snapshots to retain")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}
	if *outputDir == "" {
		*outputDir = cfg.BackupDir
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.SchemaReady(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}
	result, err := backupservice.CreateTracked(ctx, st, *configPath, *outputDir, *keep, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		return 1
	}
	fmt.Printf("backup created\npath: %s\nsha256: %s\n", result.Path, result.Checksum)
	return 0
}

func runRestore(args []string) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	archivePath := fs.String("from", "", "VisitorTrace backup archive")
	confirm := fs.Bool("confirm", false, "confirm that the service is stopped and restore the database")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *archivePath == "" || !*confirm {
		fmt.Fprintln(os.Stderr, "restore: --from and --confirm are required; stop VisitorTrace before restoring")
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\n", err)
		return 1
	}
	preRestoreDir := filepath.Join(cfg.BackupDir, "pre-restore")
	preRestore, err := backupservice.Create(ctx, st, *configPath, preRestoreDir, 3, time.Now())
	closeErr := st.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore: create pre-restore backup: %v\n", err)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "restore: close current database: %v\n", closeErr)
		return 1
	}
	manifest, err := backupservice.Restore(ctx, *archivePath, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\npre-restore backup: %s\n", err, preRestore.Path)
		return 1
	}
	fmt.Printf("restore complete\ncreated: %s\nschema: %d\npre-restore backup: %s\n", manifest.CreatedAt.Format(time.RFC3339), manifest.SchemaVersion, preRestore.Path)
	return 0
}
