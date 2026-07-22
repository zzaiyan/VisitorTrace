# VisitorTrace Development Guide

[简体中文](./development.zh-CN.md)

This guide is for contributors working on VisitorTrace or integrating its deployment. See the [User Guide](./user-guide.en.md) for Administrator commands and website integration.

## Design Boundary

VisitorTrace is a single-process Go service. Production requires only the executable, a SQLite database, and a local GeoIP MMDB. It does not require Node.js, Redis, a message broker, or a separate task service. The Admin Console and Public Analytics are server-rendered, and browser assets are embedded in the executable.

Package responsibilities:

- `cmd/visitortrace`: command-line entry points and service lifecycle;
- `internal/store`: SQLite models, migrations, transactions, and queries;
- `internal/server`: HTTP APIs, embed scripts, and administrative/public pages;
- `internal/maprender`: SVG map rendering without an external runtime dependency;
- `internal/backup`: consistent snapshots, archive verification, and restoration;
- `internal/maintenance`: in-process scheduling and bounded cleanup;
- `internal/geoip`: local MMDB lookup.

## Development Checks

Go 1.25 or newer is required:

```sh
go test ./...
go test -race ./...
go vet ./...
go mod verify
```

`make check` runs the regular tests and static analysis. `tools/preview-demo.sh` provides a local manual-preview environment with generated data.

## Database Evolution

Migrations are embedded in `internal/store/migrations.go` and applied in version order inside transactions. Startup performs forward migrations; downward migrations are not supported. Create a verified snapshot with `visitortrace backup` before a destructive upgrade.

The Pageview ingestion transaction stores the individual record, visitor-window registrations, and durable aggregates together. Expiring an individual record must not reverse its durable aggregate contributions.

`serve` starts a lightweight maintenance loop that runs on startup and hourly. `visitortrace maintenance` exposes the same cleanup flow for diagnostics and external schedulers. Every deletion transaction has a bounded batch size; `operation_status` retains the latest maintenance outcome for the operational dashboard.

## Backup Format

A `.vtbackup` file is a ZIP container with:

- `visitortrace.sqlite3`: a consistent snapshot created with SQLite `VACUUM INTO`;
- `config.json`: the protected configuration captured at backup time;
- `manifest.json`: format, application, and schema versions plus file sizes and SHA-256 digests.

The adjacent `.sha256` file verifies the complete container. Before activation, restore verifies both checksum layers, runs SQLite integrity checking, applies forward migrations, and revokes every Administrator session.

## Published Documentation Boundary

This guide and both user guides are published with the repository. Root-level `ARCHITECTURE.md` and `CONTEXT.md`, together with `docs/adr`, `docs/research`, and `docs/internal`, are local design material excluded by `.gitignore` and must not be committed to GitHub.
