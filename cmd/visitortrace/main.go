package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/zzaiyan/VisitorTrace/internal/buildinfo"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoip"
	"github.com/zzaiyan/VisitorTrace/internal/password"
	"github.com/zzaiyan/VisitorTrace/internal/server"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "init":
		code = runInit(os.Args[2:])
	case "serve":
		code = runServe(os.Args[2:])
	case "doctor":
		code = runDoctor(os.Args[2:])
	case "site":
		code = runSite(os.Args[2:])
	case "version":
		fmt.Printf("VisitorTrace %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)
	default:
		usage()
		code = 2
	}
	if code != 0 {
		os.Exit(code)
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", config.DefaultDataDir(), "persistent data directory")
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	passwordFile := fs.String("password-file", "", "protected file containing the administrator password")
	geoIPPath := fs.String("geoip", "", "existing GeoIP MMDB path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := config.Default(*dataDir)
	if *geoIPPath != "" {
		cfg.GeoIPPath = *geoIPPath
	}
	if _, err := os.Stat(cfg.DatabasePath); err == nil {
		fmt.Fprintf(os.Stderr, "init refused: database already exists at %s\n", cfg.DatabasePath)
		return 1
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "init: check database: %v\n", err)
		return 1
	}
	if _, err := os.Stat(*configPath); err == nil {
		fmt.Fprintf(os.Stderr, "init refused: config already exists at %s\n", *configPath)
		return 1
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "init: check config: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "init: create data directory: %v\n", err)
		return 1
	}
	if err := os.Chmod(cfg.DataDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "init: protect data directory: %v\n", err)
		return 1
	}
	value, err := password.Read(*passwordFile, os.Stdin, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	hash, err := password.Hash(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: hash password: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := config.Save(*configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "init: save config: %v\n", err)
		return 1
	}
	st, err := store.Initialize(ctx, cfg.DatabasePath, hash)
	if err != nil {
		_ = os.Remove(*configPath)
		fmt.Fprintf(os.Stderr, "init: initialize database: %v\n", err)
		return 1
	}
	defer st.Close()
	fmt.Printf("initialized VisitorTrace\nconfig: %s\ndata: %s\n", *configPath, cfg.DataDir)
	if _, err := os.Stat(cfg.GeoIPPath); err != nil {
		fmt.Printf("warning: GeoIP database is not installed; readiness will remain unavailable\n")
	}
	return 0
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "serve: migrate database: %v\n", err)
		return 1
	}
	if err := st.SchemaReady(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	geoResolver, geoErr := geoip.Open(cfg.GeoIPPath)
	if geoErr != nil {
		logger.Warn("GeoIP database is unavailable", "path", cfg.GeoIPPath, "error", geoErr)
	} else {
		defer geoResolver.Close()
	}
	app := server.New(cfg, st, logger)
	app.SetGeoIP(geoResolver)
	httpServer := app.HTTPServer()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server starting", "listen", cfg.Listen, "version", buildinfo.Version, "commit", buildinfo.Commit)
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-stopCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			return 1
		}
		logger.Info("server stopped")
		return 0
	case err := <-serverErrors:
		logger.Error("server stopped unexpectedly", "error", err)
		return 1
	}
}

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("config: failed (%v)\n", err)
		return 1
	}
	fmt.Printf("config: ok (%s)\n", filepath.Clean(*configPath))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Printf("database: failed (%v)\n", err)
		return 1
	}
	defer st.Close()
	version, err := st.SQLiteVersion(ctx)
	if err != nil {
		fmt.Printf("sqlite: failed (%v)\n", err)
		return 1
	}
	if !store.SQLiteVersionAtLeast(version, store.MinimumSQLiteVersion) {
		fmt.Printf("sqlite: failed (%s; need %s or newer)\n", version, store.MinimumSQLiteVersion)
		return 1
	}
	fmt.Printf("sqlite: ok (%s)\n", version)
	if err := st.SchemaReady(ctx); err != nil {
		fmt.Printf("schema: failed (%v)\n", err)
		return 1
	}
	fmt.Println("schema: ok")
	geoResolver, err := geoip.Open(cfg.GeoIPPath)
	if err != nil {
		fmt.Printf("geoip: failed (%v)\n", err)
		return 1
	}
	_ = geoResolver.Close()
	fmt.Printf("geoip: ok (%s)\n", cfg.GeoIPPath)
	return 0
}

func runSite(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: visitortrace site <create|list> [flags]")
		return 2
	}
	switch args[0] {
	case "create":
		return runSiteCreate(args[1:])
	case "list":
		return runSiteList(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: visitortrace site <create|list> [flags]")
		return 2
	}
}

func runSiteCreate(args []string) int {
	fs := flag.NewFlagSet("site create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	name := fs.String("name", "", "Site display name")
	timezone := fs.String("timezone", "Asia/Shanghai", "IANA Site timezone")
	dedupWindow := fs.Int("dedup-window", 1, "Unique Visitor deduplication window in days")
	retention := fs.Int("retention", 30, "Pageview Record retention in days")
	var origins stringList
	fs.Var(&origins, "origin", "Allowed Origin; repeat for multiple origins")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "site create: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "site create: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "site create: migrate database: %v\n", err)
		return 1
	}
	created, err := st.CreateSite(ctx, store.CreateSiteParams{
		Name:            *name,
		Timezone:        *timezone,
		AllowedOrigins:  origins,
		DedupWindowDays: *dedupWindow,
		RetentionDays:   *retention,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "site create: %v\n", err)
		return 1
	}
	fmt.Printf("created Site\nid: %s\nname: %s\ntimezone: %s\n", created.ID, created.Name, created.Timezone)
	for _, origin := range created.AllowedOrigins {
		fmt.Printf("origin: %s\n", origin)
	}
	return 0
}

func runSiteList(args []string) int {
	fs := flag.NewFlagSet("site list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "site list: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := store.OpenExisting(ctx, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "site list: %v\n", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "site list: migrate database: %v\n", err)
		return 1
	}
	sites, err := st.ListSites(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "site list: %v\n", err)
		return 1
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tTIMEZONE\tORIGINS")
	for _, item := range sites {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Timezone, strings.Join(item.AllowedOrigins, ","))
	}
	_ = w.Flush()
	return 0
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: visitortrace <init|serve|doctor|site|version> [flags]")
}
