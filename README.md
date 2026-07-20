# VisitorTrace · 访迹

A tiny self-hosted visitor map and pageview tracker.

VisitorTrace is a lightweight, self-hosted service for recording Pageviews, maintaining aggregate visitor statistics, and publishing embeddable visitor maps for personal websites.

The current runnable milestone provides secure bootstrap, SQLite initialization, Pageview collection, local GeoIP lookup, SVG Public Maps, Public Analytics, an authenticated Admin Console, and per-Site Map Preset management. Retention cleanup, backups, self-update, and password reset remain follow-up operational work.

## Documentation

- [Architecture](./ARCHITECTURE.md)
- [Domain language](./CONTEXT.md)
- [Architecture decisions](./docs/adr/)
- [Academic homepage and MapMyVisitors research](./docs/research/academic-homepage-and-mapmyvisitors.md)

## Planned Runtime

- One Go service and executable
- SQLite as the sole durable datastore
- Local IP geolocation
- Script-free SVG Public Maps
- Integrated Widget and Separated Integration modes
- Public Analytics and a password-protected Admin Console

## Development

VisitorTrace requires Go 1.25 or newer. The current development toolchain is Go 1.26.5.

```sh
make check
make build
./bin/visitortrace version
```

Initialize a development instance interactively:

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb
```

Start the loopback-only HTTP server:

```sh
./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

Create a Site for local integration testing:

```sh
./bin/visitortrace site create \
  --config "$HOME/.config/visitortrace/config.json" \
  --name "Academic homepage" \
  --origin "https://example.com"
```

The current separated integration tracker is available at:

```text
GET /embed/tracker.js?site_id=<SITE-ID>
```

It posts Pageviews to:

```text
POST /api/v1/sites/<SITE-ID>/pageviews
Content-Type: text/plain
```

The current Public Map and integrated widget routes are:

```text
GET /api/v1/sites/<SITE-ID>/map.svg?w=300&h=168
GET /embed/widget.js?site_id=<SITE-ID>&w=300&h=168
```

The browser-facing views are:

```text
GET /public/<SITE-ID>/analytics?range=30d
GET /admin/login
GET /admin
GET /admin/sites/<SITE-ID>
```

The Admin Console accepts the single Administrator password created by `visitortrace init`. It manages Sites, publication and collection settings, recent Pageview Records, Map Presets, and live SVG previews. The Public Analytics view exposes only aggregate trends, geography, browser, and operating-system statistics.

For a local preview, create a disposable instance and Site, start `visitortrace serve`, then open `/admin/login` at `http://127.0.0.1:8790`. A valid DB-IP City Lite MMDB is required for `/health/ready` and real geographic markers; without it the service still renders the basemap and aggregate counters.

To regenerate the checked-in Natural Earth basemap after obtaining the pinned source file:

```sh
node tools/generate-basemap.mjs \
  /path/to/ne_110m_admin_0_countries.geojson \
  internal/maprender/assets/world.path
```

The initial HTTP surface is:

- `GET /health/live`
- `GET /health/ready`

The ready endpoint remains unavailable until SQLite, the schema, and a valid GeoIP MMDB are loaded.
