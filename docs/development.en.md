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
- `internal/geoipupdate`: monthly download, verification, atomic activation, hot reload, and rollback.

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

The Administrator password is stored as an Argon2id hash. Both the Admin Console change flow and `visitortrace password reset` update the credential and revoke every session in one transaction. Session records retain the most recent password-verification time for short-lived reauthentication gates on high-risk operations such as updates.

A Site-data reset first disables ingestion and publication in the same transaction, then removes records, visitor registrations, aggregates, and map locations and rotates the Site HMAC key. Permanent deletion also disables external behavior before foreign-key cascading cleanup. The HTTP layer additionally requires both the Administrator password and exact Site ID.

The GeoIP updater runs at startup and every 24 hours. `{YYYY-MM}` expands using the UTC month. Compressed input is limited to 1 GiB and the expanded MMDB to 2 GiB. A configured SHA-256 sidecar verifies the downloaded container first; full MMDB verification always follows. The candidate is created on the target filesystem and activated by rename, preserving the prior file as `.previous`. The service swaps resolvers behind a mutex so an old reader is not closed during an active lookup.

Pageview Record lists use a compound `(occurred_at, id)` cursor with server-controlled ordering. Each cursor carries a fingerprint of normalized filters and cannot be reused across a changed filter set. Responses contain no more than 200 rows. Record and aggregate exports iterate SQLite rows directly into `encoding/csv` without temporary export files; sensitive exports exist only on authenticated Administrator routes and send `Cache-Control: no-store`.

`internal/operations` collects read-only runtime information: build metadata, process uptime, combined SQLite/WAL/SHM size, filesystem capacity, GeoIP and local-backup state, and task outcomes from `operation_status`. Linux uses `statfs`; other platforms report unavailable data rather than inventing values. Manual Admin operations reuse the same backup, maintenance, and GeoIP implementations as the CLI and require an Administrator session plus CSRF validation. GeoIP and maintenance runners use process-wide exclusion to prevent duplicate runs.

## Backup Format

A `.vtbackup` file is a ZIP container with:

- `visitortrace.sqlite3`: a consistent snapshot created with SQLite `VACUUM INTO`;
- `config.json`: the protected configuration captured at backup time;
- `manifest.json`: format, application, and schema versions plus file sizes and SHA-256 digests.

The adjacent `.sha256` file verifies the complete container. Before activation, restore verifies both checksum layers, runs SQLite integrity checking, applies forward migrations, and revokes every Administrator session.

## Published Documentation Boundary

This guide and both user guides are published with the repository. Root-level `ARCHITECTURE.md` and `CONTEXT.md`, together with `docs/adr`, `docs/research`, and `docs/internal`, are local design material excluded by `.gitignore` and must not be committed to GitHub.
