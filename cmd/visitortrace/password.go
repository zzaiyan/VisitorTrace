package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/password"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func runPassword(args []string) int {
	if len(args) == 0 || args[0] != "reset" {
		fmt.Fprintln(os.Stderr, "usage: visitortrace password reset [flags]")
		return 2
	}
	fs := flag.NewFlagSet("password reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	passwordFile := fs.String("password-file", "", "protected file containing the new administrator password")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	value, err := password.Read(*passwordFile, os.Stdin, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "password reset: %v\n", err)
		return 1
	}
	hash, err := password.Hash(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "password reset: hash password: %v\n", err)
		return 1
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "password reset: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "password reset: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "password reset: migrate database: %v\n", err)
		return 1
	}
	if err := st.UpdateAdministratorPassword(ctx, hash); err != nil {
		fmt.Fprintf(os.Stderr, "password reset: %v\n", err)
		return 1
	}
	fmt.Println("administrator password reset; all sessions revoked")
	return 0
}
