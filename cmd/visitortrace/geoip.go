package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"github.com/zzaiyan/VisitorTrace/internal/config"
	"github.com/zzaiyan/VisitorTrace/internal/geoipupdate"
	"github.com/zzaiyan/VisitorTrace/internal/store"
)

func runGeoIP(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: visitortrace geoip <update|query> [flags]")
		return 2
	}
	switch args[0] {
	case "update":
		return runGeoIPUpdate(args[1:])
	case "query":
		return runGeoIPQuery(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: visitortrace geoip <update|query> [flags]")
		return 2
	}
}

func runGeoIPUpdate(args []string) int {
	fs := flag.NewFlagSet("geoip update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	force := fs.Bool("force", false, "download even when the current database is fresh")
	datasetID := fs.String("dataset", "", "selected offline dataset ID to update (default: all selected offline datasets)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip update: %v\n", err)
		return 1
	}
	if cfg.GeoIPUpdate == "disabled" && !*force {
		fmt.Fprintln(os.Stderr, "geoip update: manual-only mode requires --force")
		return 1
	}
	var datasets []config.GeoIPDataset
	for _, selected := range cfg.SelectedGeoIPDatasets() {
		if *datasetID != "" && selected.ID != *datasetID {
			continue
		}
		if selected.Online {
			if *datasetID != "" {
				fmt.Fprintf(os.Stderr, "geoip update: %s is an online service; test it in Administrator Settings\n", selected.ID)
				return 1
			}
			continue
		}
		datasets = append(datasets, selected)
	}
	if len(datasets) == 0 {
		if *datasetID != "" {
			fmt.Fprintf(os.Stderr, "geoip update: no selected offline dataset named %q\n", *datasetID)
			return 1
		}
		fmt.Println("No offline GeoIP dataset is selected")
		return 0
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
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	failed := false
	updated := false
	for _, dataset := range datasets {
		current := cfg
		current.GeoIPProvider = dataset.Provider
		current.GeoIPPath = dataset.Path
		current.GeoIPUpdateURL = dataset.UpdateURL
		current.GeoIPChecksumURL = dataset.ChecksumURL
		if *force {
			current.GeoIPUpdate = "automatic"
		}
		runner := geoipupdate.New(current, st, logger)
		runner.OperationName = dataset.Operation
		result, err := runner.RunOnce(ctx, *force)
		if err != nil {
			fmt.Fprintf(os.Stderr, "geoip update %s: %v\n", dataset.ID, err)
			failed = true
			continue
		}
		if result.Updated {
			fmt.Printf("GeoIP dataset %s updated (sha256: %s)\n", dataset.ID, result.SHA256)
			updated = true
		} else {
			fmt.Printf("GeoIP dataset %s is current\n", dataset.ID)
		}
	}
	if updated {
		fmt.Println("Restart VisitorTrace if it is currently running")
	}
	if failed {
		return 1
	}
	return 0
}

type mmdbQueryMetadata struct {
	Description              map[string]string `json:"description,omitempty"`
	DatabaseType             string            `json:"database_type"`
	Languages                []string          `json:"languages,omitempty"`
	BinaryFormatMajorVersion uint              `json:"binary_format_major_version"`
	BinaryFormatMinorVersion uint              `json:"binary_format_minor_version"`
	BuildEpoch               uint              `json:"build_epoch"`
	BuildTime                string            `json:"build_time,omitempty"`
	IPVersion                uint              `json:"ip_version"`
	NodeCount                uint              `json:"node_count"`
	RecordSize               uint              `json:"record_size"`
}

type mmdbQueryDatabase struct {
	Path     string            `json:"path"`
	Metadata mmdbQueryMetadata `json:"metadata"`
}

type mmdbQueryOutput struct {
	IP             string            `json:"ip"`
	Database       mmdbQueryDatabase `json:"database"`
	Found          bool              `json:"found"`
	MatchedNetwork string            `json:"matched_network,omitempty"`
	Record         any               `json:"record"`
}

func runGeoIPQuery(args []string) int {
	fs := flag.NewFlagSet("geoip query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultConfigPath(), "protected config path")
	mmdbPath := fs.String("mmdb", "", "GeoIP MMDB path; defaults to the first selected local MMDB")
	datasetID := fs.String("dataset", "", "selected local MMDB dataset ID")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: visitortrace geoip query [--config PATH] [--dataset ID | --mmdb PATH] IP")
		return 2
	}
	if *datasetID != "" && *mmdbPath != "" {
		fmt.Fprintln(os.Stderr, "geoip query: --dataset and --mmdb cannot be used together")
		return 2
	}
	address, err := netip.ParseAddr(strings.TrimSpace(fs.Arg(0)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip query: invalid IP address %q: %v\n", fs.Arg(0), err)
		return 2
	}

	databasePath := strings.TrimSpace(*mmdbPath)
	if databasePath == "" {
		cfg, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "geoip query: %v\n", err)
			return 1
		}
		for _, dataset := range cfg.SelectedGeoIPDatasets() {
			if *datasetID != "" && dataset.ID != *datasetID {
				continue
			}
			if dataset.Online || dataset.Provider == "ip2region" {
				if *datasetID != "" {
					fmt.Fprintf(os.Stderr, "geoip query: dataset %q is not a local MMDB\n", *datasetID)
					return 1
				}
				continue
			}
			databasePath = dataset.Path
			break
		}
		if databasePath == "" {
			fmt.Fprintln(os.Stderr, "geoip query: no matching local MMDB is selected; use --mmdb for another file")
			return 1
		}
	}

	reader, err := maxminddb.Open(databasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip query: open MMDB: %v\n", err)
		return 1
	}
	defer reader.Close()

	metadata := mmdbQueryMetadata{
		Description:              reader.Metadata.Description,
		DatabaseType:             reader.Metadata.DatabaseType,
		Languages:                reader.Metadata.Languages,
		BinaryFormatMajorVersion: reader.Metadata.BinaryFormatMajorVersion,
		BinaryFormatMinorVersion: reader.Metadata.BinaryFormatMinorVersion,
		BuildEpoch:               reader.Metadata.BuildEpoch,
		IPVersion:                reader.Metadata.IPVersion,
		NodeCount:                reader.Metadata.NodeCount,
		RecordSize:               reader.Metadata.RecordSize,
	}
	if reader.Metadata.BuildEpoch > 0 {
		metadata.BuildTime = time.Unix(int64(reader.Metadata.BuildEpoch), 0).UTC().Format(time.RFC3339)
	}

	var record any
	network, found, err := reader.LookupNetwork(net.IP(address.AsSlice()), &record)
	if err != nil {
		fmt.Fprintf(os.Stderr, "geoip query: lookup %s: %v\n", address, err)
		return 1
	}
	output := mmdbQueryOutput{
		IP:       address.String(),
		Database: mmdbQueryDatabase{Path: databasePath, Metadata: metadata},
		Found:    found,
		Record:   record,
	}
	if found && network != nil {
		output.MatchedNetwork = network.String()
	}
	if err := writeMMDBQueryOutput(os.Stdout, output); err != nil {
		fmt.Fprintf(os.Stderr, "geoip query: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeMMDBQueryOutput(w io.Writer, output mmdbQueryOutput) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
