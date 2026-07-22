package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/buildinfo"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/selfupdate"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "print machine-readable version information")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(selfupdate.CandidateInfo{
			Version: buildinfo.Version, Commit: buildinfo.Commit, BuildTime: buildinfo.BuildTime,
			SchemaVersion: store.SupportedSchemaVersion(),
		})
		return 0
	}
	fmt.Printf("VisitorTrace %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)
	return 0
}

func runUpdate(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: visitortrace update <bootstrap|check|apply> [flags]")
		return 2
	}
	fs := flag.NewFlagSet("update "+args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update %s: %v\n", args[0], err)
		return 1
	}
	switch args[0] {
	case "bootstrap":
		manager := selfupdate.New(cfg, *configPath, nil)
		path, err := manager.Bootstrap()
		if err != nil {
			fmt.Fprintf(os.Stderr, "update bootstrap: %v\n", err)
			return 1
		}
		fmt.Printf("self-update layout ready\nstable executable: %s\nconfigure the process supervisor to run this path before using one-click updates\n", path)
		return 0
	case "check":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		manager := selfupdate.New(cfg, *configPath, nil)
		result, err := manager.Check(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "update check: %v\n", err)
			return 1
		}
		if result.Current {
			fmt.Printf("VisitorTrace %s is current\n", result.Manifest.Version)
		} else {
			fmt.Printf("update available: %s -> %s\npublished: %s\nschema: %d\n", buildinfo.Version, result.Manifest.Version, result.Manifest.PublishedAt.Format(time.RFC3339), result.Manifest.SchemaVersion)
		}
		return 0
	case "apply":
		return applyUpdate(cfg, *configPath)
	default:
		fmt.Fprintln(os.Stderr, "usage: visitortrace update <bootstrap|check|apply> [flags]")
		return 2
	}
}

func applyUpdate(cfg config.Config, configPath string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update apply: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.SchemaReady(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "update apply: %v\n", err)
		return 1
	}
	manager := selfupdate.New(cfg, configPath, st)
	result, err := manager.PrepareAndActivate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update apply: %v\n", err)
		return 1
	}
	if result.Current {
		fmt.Printf("VisitorTrace %s is current\n", result.Version)
		return 0
	}
	fmt.Printf("update prepared\nversion: %s\nbinary: %s\npre-update backup: %s\nstable executable: %s\nrestart the supervised service now\n", result.Version, result.BinaryPath, result.BackupPath, result.StablePath)
	return 0
}
