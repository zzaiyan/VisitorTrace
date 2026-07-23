# VisitorTrace Update History

[Chinese](./update.zh-CN.md)

This file records user-facing changes for each published VisitorTrace release.

## Unreleased

- Replaced free-form IANA timezone fields with searchable browser-native dropdowns on Site creation and settings pages. Sites with Pageviews now explain why the statistics timezone is locked and how to unlock it safely.
- Aligned controls in multi-column forms when only one field includes helper text, including Site counting/retention and Administrator password settings.
- Added the missing `/admin/sites` management route, separated Site listing from creation and the operations dashboard, and returned deleted Sites to the list.
- Reorganized each Site page around integration, settings, Map Preset, recent records, and destructive operations. Site settings now use consistent identity, counting/retention, and collection/publication groups.
- Made the integrated and separated integration modes structurally consistent: Widget, Tracker, and Map SVG resources each expose an embed snippet, resource URL, and integrated copy control.
- Reorganized Administrator Settings into service configuration, GeoIP data operations, account security, and application update. Public Base URL and all GeoIP source/credential fields now save atomically with one password confirmation and one supervised restart.
- Styled the full GeoIP task summary, added content-revision URLs for embedded CSS/JavaScript to prevent stale post-upgrade assets, and made configuration-save failures point to the required systemd `ReadWritePaths` directory.
- Avoided redundant directory/file `chmod` calls when protected configuration modes are already correct, while retaining strict `0700`/`0600` enforcement for nonconforming paths.
- Added a local-file option to Administrator Settings self-update. Administrators can upload the signed `manifest.json` and matching platform binary from one Release while retaining Ed25519, size, SHA-256, candidate identity, schema, backup, and rollback checks.
- Documented the local signed workflow, its distinction from the same-schema manual systemd script, and the reverse-proxy upload-size setting.

## 0.1.2 - 2026-07-23

- Added a Site-page action that refreshes retained Pageview geography from the active GeoIP database and atomically rebuilds the corresponding country, region, and city PV/UV aggregates. Historical dates without detailed records remain untouched.
- Added graphical GeoIP management to Administrator Settings: provider selection, automatic/manual-only policy, official or custom sources, credential replacement/removal without secret echo, update status, immediate checks, and forced downloads.
- Added `visitortrace geoip query` and `scripts/query-mmdb.sh` to print formatted raw MMDB metadata and records for one IP, including the matched network and explicit no-record status.
- Added `scripts/update-systemd-binary.sh` for verified manual updates from an already-downloaded local binary, including a pre-update backup, atomic release switch, service restart, and automatic executable rollback.
- Added tracker hostname capture and hostname-scoped UV counting for Sites deployed on multiple domains. Hostnames are available in Pageview Records, filters, CSV exports, Public Analytics, Admin Analytics, and aggregate exports.
- Added DB-IP Chinese city-label normalization to remove district/subdistrict qualifiers where the City Lite record provides enough hierarchy information. The cleanup is now private to the DB-IP provider adapter and is never applied to MaxMind or IP2Location records.
- Added equal first-class GeoIP backends for DB-IP, MaxMind GeoLite2 City, and IP2Location LITE DB11, with unified location fields, provider-specific validation/attribution, built-in official download sources, credential handling, and automatic updates. The updater now extracts raw MMDB, gzip MMDB, tar.gz, and ZIP containers.
- This release advances the SQLite schema from 8 to 9. Existing installations are migrated automatically; historical records retain an empty hostname and historical aggregates cannot be reconstructed by hostname. Use the signed updater for an automatic 0.1.1 to 0.1.2 upgrade; the offline `update-systemd-binary.sh` intentionally rejects cross-schema updates.

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
