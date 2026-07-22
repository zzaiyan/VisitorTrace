# VisitorTrace Update History

[Chinese](./update.zh-CN.md)

This file records user-facing changes for each published VisitorTrace release.

## Unreleased

- Added `visitortrace geoip query` and `scripts/query-mmdb.sh` to print formatted raw MMDB metadata and records for one IP, including the matched network and explicit no-record status.
- Added `scripts/update-systemd-binary.sh` for verified manual updates from an already-downloaded local binary, including a pre-update backup, atomic release switch, service restart, and automatic executable rollback.
- Added tracker hostname capture and hostname-scoped UV counting for Sites deployed on multiple domains. Hostnames are available in Pageview Records, filters, CSV exports, Public Analytics, Admin Analytics, and aggregate exports.
- Added DB-IP Chinese city-label normalization to remove district/subdistrict qualifiers where the City Lite record provides enough hierarchy information. The cleanup is now private to the DB-IP provider adapter and is never applied to MaxMind or IP2Location records.
- Added equal first-class GeoIP backends for DB-IP, MaxMind GeoLite2 City, and IP2Location LITE DB11, with unified location fields, provider-specific validation/attribution, built-in official download sources, credential handling, and automatic updates. The updater now extracts raw MMDB, gzip MMDB, tar.gz, and ZIP containers.
- This upcoming change advances the SQLite schema to 9. Existing installations are migrated automatically; historical records retain an empty hostname and historical aggregates cannot be reconstructed by hostname.

## 0.1.1 - 2026-07-23

- Fixed deformation in the interactive Public Analytics and Admin Analytics maps by preserving the world-map aspect ratio and using fixed Bering Strait bounds.
- Repaired GeoJSON rings that crossed the map seam, removing the long strip artifacts previously visible across the United States and Russia.
- Added a map control snippet to the separated integration area. It includes a lazy-loading `<img>` example and its own copy button alongside the separated Tracker code.
- Added `scripts/install-systemd.sh` for one-step service-account creation, protected directory setup, initialization, self-update bootstrap, systemd unit creation, and service startup.
- Updated the systemd hardening example so the Admin Console can persist Base URL changes under `ProtectSystem=strict`.
- Refreshed the English and Chinese deployment and user documentation.

This release does not change the SQLite schema or require a data migration.

## 0.1.0 - 2026-07-23

- First public release of VisitorTrace / 访迹.
- Added Pageview ingestion, visitor-window deduplication, durable aggregates, detailed record retention, local GeoIP lookup, SVG maps, Public Analytics, Admin Console, Site management, backups, maintenance, and signed self-update support.
