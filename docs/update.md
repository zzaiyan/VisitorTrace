# VisitorTrace Update History

[Chinese](./update.zh-CN.md)

This file records user-facing changes for each published VisitorTrace release.

## Unreleased

- Simplified the Interactive Widget to marker hover/focus/touch details and click-through Analytics navigation. Pan, zoom, Reset, gesture state, and the Widget control registry were removed; those advanced interactions remain available on full Public and Admin Analytics maps.
- Simplified Site integration into four behavior-oriented, ready-to-use snippets and removed duplicate resource-address rows. Map Preset Widget previews now keep the deployed Widget interaction surface unobstructed. The Site detail page loads only eight recent records in a compact six-column table and links directly to the complete records view with the current Site filter applied.
- Refined ECharts map controls with a smaller locate-style Reset icon, made Image fallback previews render at their exact configured pixel dimensions without scaling, and removed redundant Site ID confirmation from Site-scoped destructive operations while retaining Administrator-password verification.
- Added dual Map Preset previews for the lightweight Interactive Widget and Image fallback, including Administrator-only rendering for private Sites. ECharts analysis maps expose an extensible control registry for future actions.
- Upgraded separated integration from a static Map SVG to an independent `map.js` loader. It performs no collection and mounts the same lazy, responsive Interactive Widget as integrated mode, so it can be paired with `tracker.js`, deferred independently, or used alone for read-only display.
- Replaced the Integrated JavaScript Widget's static image with a sandboxed, independently rendered iframe. City markers now expose mouse, keyboard, and touch details with GeoIP attribution, and map clicks open Public Analytics without moving Pageview collection out of the parent-page loader.
- Reduced every SVG map by defining the world basemap once and reusing it across the Bering Strait seam instead of serializing the same path twice. The iframe reports its effective Map Preset dimensions to the loader so responsive embedding preserves the projection ratio.

## 0.1.4 - 2026-07-27

- Top-aligned independent columns in Site integration layouts, preventing shorter code resources from being vertically distributed beside taller alternatives.
- Added the all-history interactive visitor map directly to each Site management page. It reuses the Public Analytics map interactions, remains available for private Sites, and retains the Administrator-authenticated SVG as a no-JavaScript fallback.
- Added a JavaScript-free Integrated Image Widget that records valid image loads and returns the SVG map from one endpoint. It validates Referer against Site Allowed Origins, supports fixed `path` and all map URL overrides, uses no-store client caching, and degrades to read-only rendering for unverifiable or bot traffic.
- Added Pageview collection-method metadata (`js` or `image`) to Administrator record tables, exact filtering, and CSV export. This advances the SQLite schema from 9 to 10; existing detailed records are marked as `js` automatically.

## 0.1.3 - 2026-07-24

- Reorganized Administrator Settings into service configuration, GeoIP data operations, account security, and application update. Configuration changes now save atomically with one password confirmation and one supervised restart, while protected systemd paths and permission handling are documented and enforced consistently.
- Added signed local-file self-update from the Admin Console, including manifest and platform binary verification, schema checks, backups, and rollback. Documented the workflow alongside the same-schema systemd update script and reverse-proxy upload limits.
- Rebuilt Site management around a dedicated `/admin/sites` list, Site settings, integration resources, Map Presets, records, analytics, and destructive operations. Integrated and separated API modes now expose symmetric snippets, resource URLs, and copy controls.
- Aligned multi-column form controls when only one field contains helper text, including Site counting/retention and Administrator password settings.
- Replaced free-form IANA timezone fields with searchable browser-native dropdowns on Site creation and settings pages. Sites with Pageviews now explain why the statistics timezone is locked and how to unlock it safely.

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
