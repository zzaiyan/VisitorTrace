# VisitorTrace

[![CI](https://github.com/zzaiyan/VisitorTrace/actions/workflows/ci.yml/badge.svg)](https://github.com/zzaiyan/VisitorTrace/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/zzaiyan/VisitorTrace?display_name=tag&sort=semver)](https://github.com/zzaiyan/VisitorTrace/releases)
[![License](https://img.shields.io/github/license/zzaiyan/VisitorTrace)](./LICENSE)

A tiny self-hosted visitor map and Pageview tracker for personal homepages, blogs, and other small websites.

[Chinese README](./README.zh-CN.md) · [User Guide](./docs/user-guide.md) · [Deployment Guide](./docs/deployment.md) · [Development Guide](./docs/development.md) · [Update History](./docs/update.md) · [Third-Party Notices](./THIRD_PARTY_NOTICES.md)

## Features

- Site-isolated Pageview ingestion
- Hostname-separated statistics for multi-domain Sites
- SQLite Pageview Records and durable aggregates
- Pluggable local GeoIP lookup and automatic updates for DB-IP, MaxMind, and IP2Location
- SVG visitor maps with provider-specific attribution
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

Published versions provide `visitortrace-<version>-linux-amd64` and `visitortrace-<version>-linux-arm64` executables, checksums, the license, and corresponding source archives on [GitHub Releases](https://github.com/zzaiyan/VisitorTrace/releases). Each executable also reports its embedded version, commit, build time, and database schema through `visitortrace version`.

## Basic Startup

```sh
./bin/visitortrace init \
  --data-dir "$HOME/.local/share/visitortrace" \
  --config "$HOME/.config/visitortrace/config.json" \
  --geoip /path/to/geoip.mmdb

./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

See the [User Guide](./docs/user-guide.md) for configuration, Site creation, website integration, and map parameters. Production setup with systemd and BT Panel's Nginx/SSL features is covered by the [Deployment Guide](./docs/deployment.md).

## License

VisitorTrace is free software licensed under the [GNU General Public License, version 3](./LICENSE). Third-party components and data retain their respective licenses; see [Third-Party Notices](./THIRD_PARTY_NOTICES.md).
