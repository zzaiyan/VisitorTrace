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

The initial HTTP surface is:

- `GET /health/live`
- `GET /health/ready`

The ready endpoint remains unavailable until SQLite, the schema, and a non-empty GeoIP file are present.
