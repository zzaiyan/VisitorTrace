# VisitorTrace Update History

[Chinese](./update.zh-CN.md)

This file records user-facing changes for each published VisitorTrace release.

## Unreleased

- Added `scripts/update-systemd-binary.sh` for verified manual updates from an already-downloaded local binary, including a pre-update backup, atomic release switch, service restart, and automatic executable rollback.

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
