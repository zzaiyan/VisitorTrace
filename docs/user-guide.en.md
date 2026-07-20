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

## Current Status

The current milestone implements Pageview ingestion, aggregate statistics, SVG maps, Public Analytics, Administrator authentication, and Map Presets. Automatic record cleanup, backup and restore, automatic GeoIP updates, password reset, and one-click self-update remain follow-up work.
