package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoipupdate"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func runGeoIP(args []string) int {
	if len(args) == 0 || args[0] != "update" {
		fmt.Fprintln(os.Stderr, "usage: visitortrace geoip update [flags]")
		return 2
	}
	fs := flag.NewFlagSet("geoip update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	force := fs.Bool("force", false, "download even when the current monthly database is fresh")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip update: %v\n", err)
		return 1
	}
	if cfg.GeoIPUpdate == "disabled" {
		fmt.Fprintln(os.Stderr, "geoip update: updates are disabled in configuration")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip update: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "geoip update: migrate database: %v\n", err)
		return 1
	}
	runner := geoipupdate.New(cfg, st, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	result, err := runner.RunOnce(ctx, *force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip update: %v\n", err)
		return 1
	}
	if !result.Updated {
		fmt.Println("GeoIP database is current")
		return 0
	}
	fmt.Printf("GeoIP database updated\nsource: %s\nsha256: %s\nrestart VisitorTrace if it is currently running\n", result.Source, result.SHA256)
	return 0
}
