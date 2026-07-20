# VisitorTrace · 访迹

A tiny self-hosted visitor map and pageview tracker.

VisitorTrace is a lightweight, self-hosted service for recording Pageviews, maintaining aggregate visitor statistics, and publishing embeddable visitor maps for personal websites.

The project is currently in early implementation. The first runnable milestone provides secure bootstrap, SQLite initialization, diagnostics, structured logging, and health endpoints. Tracking, map rendering, analytics, and the Admin Console are not implemented yet.

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
