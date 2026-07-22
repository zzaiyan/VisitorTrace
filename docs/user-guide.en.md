# VisitorTrace User Guide

[简体中文](./user-guide.zh-CN.md)

VisitorTrace is a lightweight self-hosted visitor map and Pageview tracker for personal homepages, blogs, and other small websites. Production requires one Go executable, one SQLite database, and a local GeoIP MMDB.

## Quick Preview

The repository includes a disposable demo that creates a Site and seeds geographically distributed fake data:

```sh
./tools/preview-demo.sh
```

The default Admin Console is `http://127.0.0.1:8790/admin/login`, with password `VisitorTrace2026`. Press `Ctrl-C` to stop the service and remove the temporary database.

When the default port is occupied:

```sh
VISITORTRACE_LISTEN=127.0.0.1:8791 ./tools/preview-demo.sh
```

## Build

Go 1.25 or newer is required.

```sh
make check
make build
./bin/visitortrace version
```

## Initialize

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb
```

Initialization asks for an Administrator password containing at least eight characters. Keep the configuration, SQLite data, and Site HMAC keys in a protected persistent directory.

## Create a Site

```sh
./bin/visitortrace site create \
  --config "$HOME/.config/visitortrace/config.json" \
  --name "Personal homepage" \
  --origin "https://example.com"
```

Each Site has an independent Site ID, Allowed Origins, statistics timezone, visitor deduplication window, Pageview Record retention period, and Map Preset.

## Start the Service

```sh
./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

The default listener is `127.0.0.1:8790`. In production, terminate HTTPS at a reverse proxy. Only explicitly configured `trusted_proxies` may provide forwarded client IP and HTTPS scheme information.

## Administrative and Public Views

- Admin Console: `/admin/login`
- Public Analytics: `/public/<SITE-ID>/analytics`
- Public Map: `/api/v1/sites/<SITE-ID>/map.svg`
- Health checks: `/health/live`, `/health/ready`

The Admin Console manages Site settings, collection and publication state, Map Presets, and sensitive Pageview Record fields such as original IP, path, browser, operating system, and Visitor Digest. Public Analytics exposes aggregate data only.

## Website Integration

The integrated Widget records a Pageview and inserts the map:

```html
<script async src="https://stats.example.com/embed/widget.js?site_id=SITE_ID"></script>
```

The separated Tracker records a Pageview without rendering a map:

```html
<script async src="https://stats.example.com/embed/tracker.js?site_id=SITE_ID"></script>
```

The map can be loaded independently as an image:

```html
<img loading="lazy"
     src="https://stats.example.com/api/v1/sites/SITE_ID/map.svg"
     alt="Visitor map">
```

## Map Presets and URL Overrides

The Admin Console configures dimensions, title, PV/UV labels, font size, visible content, background, land, border, text, marker color, and marker metric. The automatic dimension buttons account for the current title, statistics band, and font size before calculating the other dimension required to preserve the world-map projection ratio.

The basemap omits Antarctica and places its left/right seam near the Bering Strait at `170°W` instead of using the `180°` meridian as the page boundary.

The Public Map accepts these parameters:

```text
w h title pv_label uv_label show fs bg land border text marker metric
```

Colors use six-digit hexadecimal values. Use this value for a transparent background:

```text
bg=transparent
```

URL parameters override one response without changing the saved Map Preset.

## GeoIP

Production requires a valid DB-IP City Lite MMDB. Without GeoIP, the service can still start and render existing aggregates and the basemap, but `/health/ready` remains unavailable and new Pageviews receive no geographic location.

## Backup and Restore

Create a consistent SQLite snapshot and configuration archive:

```sh
./bin/visitortrace backup \
  --config "$HOME/.config/visitortrace/config.json"
```

Backups are written to `backup_dir`, which defaults to `backups` inside the data directory. Every `.vtbackup` archive has a `.sha256` sidecar, while the database and configuration entries also carry individual SHA-256 digests in the archive manifest. The command runs a SQLite integrity check and retains the latest three archives by default. Use `--output` and `--keep` to override those defaults.

Stop VisitorTrace in aaPanel or another process supervisor before restoring:

```sh
./bin/visitortrace restore \
  --config "$HOME/.config/visitortrace/config.json" \
  --from /path/to/visitortrace-20260722T033000.000000000Z.vtbackup \
  --confirm
```

The restore command first creates a safety snapshot in `backup_dir/pre-restore`, then verifies the archive checksum, entry checksums, and SQLite integrity. It migrates the restored database to the current version and revokes all Administrator sessions. The archive includes the configuration captured at backup time, but a normal restore does not overwrite the active configuration file.

For scheduled backups, configure the operating system to run `visitortrace backup` daily. The service does not depend on a specific control panel or scheduler.

## Automatic Maintenance and Retention

The service runs maintenance once at startup and then every hour. Maintenance removes, per Site:

- Pageview Records whose actual age exceeds the configured retention period;
- visitor registrations for completed deduplication windows;
- expired Administrator sessions and sessions idle for at least 12 hours.

Deletion uses bounded transactional batches to avoid blocking ingestion for an extended period. Daily aggregates and map statistics remain after individual records expire. Reducing the retention period makes newly out-of-range records eligible at the next run; extending it cannot recover records already deleted.

Run the same maintenance flow manually with:

```sh
./bin/visitortrace maintenance \
  --config "$HOME/.config/visitortrace/config.json"
```

## Administrator Password

After signing in, open Administrator Settings and provide the current password to choose a new one. Passwords contain 8 to 128 characters. A successful change revokes every Administrator session and requires a new login.

If the password is lost, reset it on the server:

```sh
./bin/visitortrace password reset \
  --config "$HOME/.config/visitortrace/config.json"
```

The command reads and confirms the new password interactively. Automation may provide it through a `0600` file using `--password-file`. A command-line reset also revokes every session.

## Site Reset and Deletion

The bottom of each Site page contains two dangerous operations. Both require the complete Site ID and current Administrator password:

- Reset Site data removes Pageview Records, all aggregates, and map locations while preserving Site settings. It rotates the HMAC key, unlocks the statistics timezone, and leaves collection and public views disabled until they are reviewed and enabled manually.
- Permanently delete Site removes the Site, all associated data, and its settings. Its Site ID is never reassigned.

Both operations are irreversible. Create a backup first.

## Current Status

The current milestone implements Pageview ingestion, aggregate statistics, automatic record cleanup, SVG maps, Public Analytics, Administrator authentication and password management, Site reset/deletion, Map Presets, and checksum-verified backup and restore. Automatic GeoIP updates and one-click self-update remain follow-up work.
