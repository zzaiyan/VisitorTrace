package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/maintenance"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func runMaintenance(args []string) int {
	fs := flag.NewFlagSet("maintenance", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "maintenance: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "maintenance: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "maintenance: migrate database: %v\n", err)
		return 1
	}
	runner := maintenance.New(st, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	result, err := runner.RunOnce(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "maintenance: %v\n", err)
		return 1
	}
	fmt.Printf("maintenance complete\npageview records: %d\nvisitor registrations: %d\nadministrator sessions: %d\n", result.PageviewRecords, result.VisitorRegistrations, result.AdministratorSessions)
	return 0
}
