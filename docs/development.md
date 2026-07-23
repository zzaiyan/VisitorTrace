# VisitorTrace Development Guide

[Chinese](./development.zh-CN.md)

This guide is for contributors working on VisitorTrace or integrating its deployment. See the [User Guide](./user-guide.md) for Administrator commands and website integration.

## Design Boundary

VisitorTrace is a single-process Go service. Production requires only the executable, a SQLite database, and a local GeoIP MMDB. It does not require Node.js, Redis, a message broker, or a separate task service. The Admin Console and Public Analytics are server-rendered, and browser assets are embedded in the executable.

Package responsibilities:

- `cmd/visitortrace`: command-line entry points and service lifecycle;
- `internal/store`: SQLite models, migrations, transactions, and queries;
- `internal/server`: HTTP APIs, embed scripts, and administrative/public pages;
- `internal/maprender`: SVG map rendering without an external runtime dependency;
- `internal/backup`: consistent snapshots, archive verification, and restoration;
- `internal/maintenance`: in-process scheduling and bounded cleanup;
- `internal/geoip`: provider-aware local MMDB lookup and unified location mapping for DB-IP, MaxMind, and IP2Location.
- `internal/geoipupdate`: provider-aware download, archive extraction, verification, atomic activation, hot reload, and rollback.
- `internal/selfupdate`: signed manifests, candidate checks, release switching, readiness confirmation, and rollback.

## Development Checks

Go 1.25 or newer is required:

```sh
go test ./...
go test -race ./...
go vet ./...
go mod verify
```

`make check` runs the regular tests and static analysis. `tools/preview-demo.sh` provides a local manual-preview environment with generated data.

Node.js is needed only when changing the Public Analytics interaction bundle or basemap. `web/analytics-entry.js` imports selected ECharts modules, while `web/assets/world.geo.json` and the SVG basemap come from the same pinned Natural Earth source. Run:

```sh
make frontend
```

This installs the locked dependencies from `package-lock.json` and regenerates the committed `internal/server/assets/analytics.js`, `.br`, and `.gz` files. Go embeds those artifacts, so production does not require Node.js and does not spend request-time CPU compressing the bundle. If enhancement fails, server-rendered statistics, the date-linked SVG map, and the basic trend must remain usable.

## Database Evolution

Migrations are embedded in `internal/store/migrations.go` and applied in version order inside transactions. Startup performs forward migrations; downward migrations are not supported. Create a verified snapshot with `visitortrace backup` before a destructive upgrade.

The Pageview ingestion transaction stores the individual record, visitor-window registrations, and durable aggregates together. Expiring an individual record must not reverse its durable aggregate contributions.

Accepted ingestion derives a normalized `hostname` from the validated `Origin` header; the embedded tracker also reports `window.location.hostname`, and the server rejects a non-empty report that disagrees. The hostname is stored on each Pageview Record and added to the durable aggregate dimensions. Visitor registrations are internally scoped by `(Site, hostname, window, dimension, visitor)`, so the same visitor digest is counted independently on different hostnames even when they share one configured Site. Migration 9 adds the Pageview column and index; pre-migration detailed rows have an empty hostname and pre-migration aggregates cannot be reconstructed by hostname.

`site_deduplication_rules` stores counting-rule history as Site-local dates. A window change upserts a rule for the next local date. Ingestion selects the latest rule effective on the Pageview's local date and calculates windows from that rule's effective-date anchor. Existing `visitor_registrations.window_end` values remain unchanged; they can only delay cleanup of temporary registrations and cannot affect counting under the new rule.

Public and administrative aggregate queries share Site-local date boundaries. Public queries first enforce publication state and never read the path family; authenticated administrative queries can read path aggregates even when publication is disabled. Browser JSON is generated only from the already-authorized aggregate result and contains no individual records, original IPs, or Visitor Digests.

Hostname aggregates are non-sensitive and are available to both Public Analytics and Admin Analytics. Administrative Pageview filters and CSV exports include the stored hostname; aggregate exports accept the `hostname` dimension.

`serve` starts a lightweight maintenance loop that runs on startup and hourly. `visitortrace maintenance` exposes the same cleanup flow for diagnostics and external schedulers. Every deletion transaction has a bounded batch size; `operation_status` retains the latest maintenance outcome for the operational dashboard.

The Administrator password is stored as an Argon2id hash. Both the Admin Console change flow and `visitortrace password reset` update the credential and revoke every session in one transaction. Session records retain the most recent password-verification time for short-lived reauthentication gates on high-risk operations such as updates.

A Site-data reset first disables ingestion and publication in the same transaction, then removes records, visitor registrations, aggregates, and map locations and rotates the Site HMAC key. Permanent deletion also disables external behavior before foreign-key cascading cleanup. The HTTP layer additionally requires both the Administrator password and exact Site ID.

The GeoIP resolver opens one configured provider at a time. `provider_dbip.go`, `provider_maxmind.go`, and `provider_ip2location.go` each own database validation, schema mapping, attribution, and the official update profile. The shared resolver only opens MMDB files, selects an adapter, and returns the common `geoip.Location` fields. DB-IP and MaxMind use the nested country/subdivision/city/location shape; IP2Location primarily uses its flat country/region/city/latitude/longitude shape and also accepts its MaxMind-compatible MMDB variant.

The updater runs at startup and every 24 hours. Provider profiles choose calendar-month freshness for DB-IP/IP2Location and a 72-hour freshness interval for MaxMind. Official MaxMind requests use Basic Authentication; IP2Location receives its Download Token as a query parameter. Authentication is host-bound and is never attached to custom mirrors. Raw MMDB, gzip MMDB, tar.gz, and ZIP containers are identified by content; an archive must contain exactly one MMDB. `{YYYY-MM}` expands using the UTC month. Downloaded input is limited to 1 GiB and the expanded MMDB to 2 GiB. A configured SHA-256 sidecar verifies the downloaded container first; full MMDB verification always follows. The candidate is created on the target filesystem and activated by rename, preserving the prior file as `.previous`. The service swaps resolvers behind a mutex so an old reader is not closed during an active lookup.

DB-IP City Lite's `city.names` can contain a city together with a district or subdistrict qualifier, while its Lite schema does not expose a feature code for selecting an administrative level. `normalizeDBIPCity` is private to `provider_dbip.go`: it removes lower-level qualifiers from Chinese labels and uses the broad subdivision for Beijing, Shanghai, Tianjin, and Chongqing. No shared resolver logic performs this cleanup, so MaxMind and IP2Location city names are preserved after schema mapping.

Pageview Record lists use a compound `(occurred_at, id)` cursor with server-controlled ordering. Each cursor carries a fingerprint of normalized filters and cannot be reused across a changed filter set. Responses contain no more than 200 rows. Record and aggregate exports iterate SQLite rows directly into `encoding/csv` without temporary export files; sensitive exports exist only on authenticated Administrator routes and send `Cache-Control: no-store`.

`internal/operations` collects read-only runtime information: build metadata, process uptime, combined SQLite/WAL/SHM size, filesystem capacity, GeoIP and local-backup state, and task outcomes from `operation_status`. Linux uses `statfs`; other platforms report unavailable data rather than inventing values. Manual Admin operations reuse the same backup, maintenance, and GeoIP implementations as the CLI and require an Administrator session plus CSRF validation. The GeoIP settings route additionally re-verifies the Administrator password, preserves omitted secrets, and atomically saves the protected configuration before requesting a supervised restart. Manual GeoIP checks temporarily enable the runner without changing a saved **Manual only** policy. GeoIP and maintenance runners use process-wide exclusion to prevent duplicate runs.

The effective Public Map Options `CacheKey` is namespaced by Site ID. The in-memory LRU has a five-minute TTL, at most 256 variants per Site, and a 32 MiB global SVG-body budget; an in-process flight coalesces misses for one key. Map Preset updates, Site resets, and deletions advance the Site cache generation so a stale in-flight render cannot repopulate invalidated data.

## Release Signing and Self-Update

`.github/workflows/ci.yml` checks Go tests, the Race Detector, Vet, module integrity, locked frontend dependencies, dependency vulnerabilities, and reproducibility of committed browser assets. Normal CI has read-only repository access.

Before the first production release, generate the project release key once in an offline or otherwise protected environment:

```sh
go run ./tools/release-manifest keygen \
  --private-key .release-secrets/update.ed25519 \
  --public-key .release-secrets/update.pub
```

Git ignores `.release-secrets`. Keep the private key in protected storage outside the repository and never publish it or place it on the application server. Losing it prevents trusted updates for existing clients; replacing the public key does not make installed versions trust the replacement automatically.

Create a GitHub Environment named `release`, preferably with required reviewers, then add:

- Environment secret `UPDATE_PRIVATE_KEY`: the single-line Base64 private-key file content;
- Environment variable `UPDATE_PUBLIC_KEY`: the single-line Base64 public-key file content.

The Release workflow embeds only the public key in production executables. The private key is exposed only to the Environment-protected signing step; the build-and-sign job has `contents: read`, while a separate publishing job alone has `contents: write`.

A production binary with self-update enabled can still be built locally:

```sh
make build \
  VERSION=0.2.0 \
  UPDATE_PUBLIC_KEY="BASE64_RAW_ED25519_PUBLIC_KEY"
```

Releases use semantic-version tags. Once CI on `main` is green, create and push a tag:

```sh
git tag -a v0.1.1 -m "VisitorTrace v0.1.1"
git push origin v0.1.1
```

`.github/workflows/release.yml` reruns the tests, builds `visitortrace-<version>-linux-amd64` and `visitortrace-<version>-linux-arm64` with `CGO_ENABLED=0`, and embeds the version, commit, build time, database schema, and public key. It derives SHA-256 digests, sizes, and the update manifest from the actual files, then verifies the result with the final public key. Each Release also carries the unmodified GPL text and a source archive generated from the same tagged commit; all files are covered by `checksums.txt`. Verified assets are uploaded to a draft Release and published only after every step succeeds. A rerun can refresh that draft but cannot overwrite an already published release. SemVer tags containing `-` become prereleases and do not replace the stable `releases/latest` target.

To generate a manifest locally:

```sh
go run ./tools/release-manifest generate \
  --version 0.1.1 \
  --published-at 2026-07-23T00:00:00Z \
  --asset linux-amd64=dist/visitortrace-0.1.1-linux-amd64 \
  --asset linux-arm64=dist/visitortrace-0.1.1-linux-arm64 \
  --output manifest.unsigned.json

go run ./tools/release-manifest sign \
  --private-key .release-secrets/update.ed25519 \
  --manifest manifest.unsigned.json \
  --output manifest.json

go run ./tools/release-manifest verify \
  --public-key "BASE64_RAW_ED25519_PUBLIC_KEY" \
  --manifest manifest.json
```

The signature payload is the fixed Go structure serialized without `signature`; `encoding/json` sorts Asset map keys. `manifest.json` uses relative asset URLs, so a domestic mirror can copy the complete Release file set unchanged. Never modify a published manifest or executable in place.

The updater places a candidate at `data_dir/releases/v<version>/visitortrace` and switches the relative `releases/current` symbolic link. `data_dir/.update-pending.json` records old/new targets, the pre-update snapshot, and startup attempts. `serve` registers an attempt before opening SQLite; the third failure restores the snapshot without forward migration. Pending state completes only after HTTP SQLite, schema, and GeoIP readiness. An Admin-triggered update re-verifies the password in the current request and asks the process to exit gracefully after sending the response.

## Backup Format

A `.vtbackup` file is a ZIP container with:

- `visitortrace.sqlite3`: a consistent snapshot created with SQLite `VACUUM INTO`;
- `config.json`: the protected configuration captured at backup time;
- `manifest.json`: format, application, and schema versions plus file sizes and SHA-256 digests.

The adjacent `.sha256` file verifies the complete container. Before activation, restore verifies both checksum layers, runs SQLite integrity checking, applies forward migrations, and revokes every Administrator session.

## Licensing Contributions

VisitorTrace is licensed under GPL-3.0. Contributions must be available under terms compatible with that license, and copied code or assets must retain their original notices. Record every bundled third-party component or dataset in the [Third-Party Notices](../THIRD_PARTY_NOTICES.md). Do not assume that a dependency's availability on a public repository makes it license-compatible.

## Published Documentation Boundary

The English and Chinese README, user, deployment, and development guides are published with the repository. Root-level `ARCHITECTURE.md` and `CONTEXT.md`, together with `docs/adr`, `docs/research`, and `docs/internal`, are local design material excluded by `.gitignore` and must not be committed to GitHub.
