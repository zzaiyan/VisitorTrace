# VisitorTrace

[![CI](https://github.com/zzaiyan/VisitorTrace/actions/workflows/ci.yml/badge.svg)](https://github.com/zzaiyan/VisitorTrace/actions/workflows/ci.yml)

A tiny self-hosted visitor map and Pageview tracker for personal homepages, blogs, and other small websites.

[Chinese README](./README.zh-CN.md) · [User Guide](./docs/user-guide.md) · [Development Guide](./docs/development.md) · [Third-Party Notices](./THIRD_PARTY_NOTICES.md)

## Features

- Site-isolated Pageview ingestion
- SQLite Pageview Records and durable aggregates
- Local GeoIP lookup and SVG visitor maps
- Date-linked Public Analytics with zoomable trends and an interactive map
- Simplified Chinese and English interfaces with a per-Site public default
- A password-protected Admin Console
- Filterable, cursor-paginated, CSV-exportable Pageview Records
- Backup, cleanup, GeoIP, and resource health overview
- One-click self-update with Ed25519 verification, automatic backup, and rollback
- Live Map Preset previews
- Integrated Widget and separated Tracker modes

## Quick Preview

```sh
./tools/preview-demo.sh
```

The launcher creates a temporary database, seeds geographically distributed fake data, and starts the local service. The default Admin password is `VisitorTrace2026`; pressing `Ctrl-C` removes the temporary data.

## Build

Go 1.25 or newer is required.

```sh
make check
make build
./bin/visitortrace version
```

Published versions provide `linux-amd64` and `linux-arm64` executables, checksums, the license, and corresponding source archives on [GitHub Releases](https://github.com/zzaiyan/VisitorTrace/releases).

## Basic Startup

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb

./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

See the [User Guide](./docs/user-guide.md) for complete configuration, Site creation, website integration, map parameters, and deployment guidance.

## License

VisitorTrace is free software licensed under the [GNU General Public License, version 3 only](./LICENSE). Third-party components and data retain their respective licenses; see [Third-Party Notices](./THIRD_PARTY_NOTICES.md).
