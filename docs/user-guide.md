# VisitorTrace User Guide

[Chinese](./user-guide.zh-CN.md)

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
./bin/visitortrace doctor --config "$HOME/.config/visitortrace/config.json"
```

## Install a Release

GitHub Releases provide versioned Linux executables that do not require a Go toolchain. Select `visitortrace-<version>-linux-amd64` or `visitortrace-<version>-linux-arm64` for the server architecture, download `checksums.txt` beside it, and verify it. For example, for version `0.1.3` on AMD64:

```sh
grep ' visitortrace-0.1.3-linux-amd64$' checksums.txt | sha256sum -c -
install -Dm700 visitortrace-0.1.3-linux-amd64 "$HOME/.local/bin/visitortrace"
"$HOME/.local/bin/visitortrace" version
```

Replace `0.1.3` with the downloaded release version and use the `linux-arm64` filename on an ARM64 server. Each Release also provides the GPL text and the corresponding source archive from the same tag. The release manifest carries an Ed25519 signature for the built-in updater; a manual installation should still check `checksums.txt` first. When using a release, substitute `$HOME/.local/bin/visitortrace` for `./bin/visitortrace` in the examples below.

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

The timezone field is a searchable IANA timezone dropdown. Type a city or timezone fragment to narrow the browser-supported list. The statistics timezone is locked after the first Pageview because changing it would shift historical aggregation dates; reset the Site data before choosing a different timezone.

Changing the visitor deduplication window schedules a new rule for the next midnight in the Site timezone and anchors the new sequence at that date. Saving does not alter the current window immediately, and completed aggregates are never recalculated. Administrative aggregate trends mark each effective date.

## Start the Service

```sh
./bin/visitortrace serve \
  --config "$HOME/.config/visitortrace/config.json"
```

For a production installation with systemd and BT Panel's Nginx/SSL features, continue with the [Deployment Guide](./deployment.md).

The default listener is `127.0.0.1:8790`. In production, terminate HTTPS at a reverse proxy. Only explicitly configured `trusted_proxies` may provide forwarded client IP and HTTPS scheme information.

### Base URL and subpath routing

Set **Administrator Settings > Service configuration > Public access** to the public address of the service, for example `https://stats.example.com/visitortrace`. This value is used by Site integration snippets and public links, and its path becomes the application route prefix. Leave it empty when VisitorTrace is deployed at the domain root.

When a Base URL is configured, every application route includes its path: `/visitortrace/admin/login`, `/visitortrace/health/live`, `/visitortrace/embed/tracker.js`, and so on. The reverse proxy must preserve that path. This setting also prevents integration code from falling back to a local address such as `127.0.0.1`.

## Administrative and Public Views

- Admin Console: `/admin/login`
- Site management: `/admin/sites`
- Public Analytics: `/public/<SITE-ID>/analytics`
- Public Map: `/api/v1/sites/<SITE-ID>/map.svg`
- Health checks: `/health/live`, `/health/ready`

When a Base URL has a path, prepend that path to each route above.

The Admin Console manages Site settings, collection and publication state, Map Presets, and sensitive Pageview Record fields such as original IP, path, browser, operating system, and Visitor Digest. Public Analytics exposes aggregate data only.

Site management has a dedicated list at `/admin/sites`; creating a Site is a separate action from that list. Each Site page is organized into integration, settings, Map Preset, recent records, and destructive operations. Site settings group identity and Allowed Origins, counting and retention rules, and collection/publication controls separately.

The selected Public Analytics date range applies consistently to PV/UV summaries, trends, the geographic map, and dimension tables. Presets cover today, 7/30/90 days, all history, and a custom range. With JavaScript, the trend can be zoomed and the map can be panned and zoomed. Without JavaScript, the page retains a server-rendered SVG map and basic trend for the same range.

Aggregate Analytics on the Site management page uses the same ranges and interactive components and additionally exposes path aggregates. It requires an Administrator session and remains available when the Site's public view is disabled. Path aggregates never appear in Public Analytics.

The Admin Console defaults to Simplified Chinese and stores the Chinese or English choice in the browser. Every Site can set Public Analytics to automatic, Simplified Chinese, or English. Automatic mode follows the visitor's `Accept-Language`; the public language switch and `lang=zh-CN` or `lang=en` URL parameter override that default. SVG titles and PV/UV labels remain explicit Map Preset content and are not translated with the interface.

The top of the Admin dashboard reports application version and uptime, SQLite version/schema/size, available disk space, the GeoIP file, and the latest local backup. A task table retains the latest backup, maintenance cleanup, and GeoIP update outcomes. Low disk, a backup older than 48 hours, GeoIP data older than 35 days, stalled cleanup, or failed operations produce warnings. The page can trigger an immediate backup, cleanup, or GeoIP check.

**Administrator Settings > Service configuration** groups the Public Base URL, GeoIP provider, update policy, official/custom source, optional checksum URL, and provider credentials into one form. Saved secrets are never rendered back to the browser: an empty credential field retains its value, while the explicit removal checkbox clears it. One Administrator-password confirmation saves every changed field atomically and requests one supervised restart. **GeoIP data** is a separate operational section for database status, the latest task summary, an immediate check, or a forced download even when the update policy is **Manual only**.

### Pageview Records and Exports

The Admin Console's Pageview Records view covers every Site. It shows 100 rows by default, with 50 and 200 row options. Filter-bound cursors move toward older or newer records without the drift of offset page numbers while ingestion continues.

Exact filters can be combined for Site, hostname, UTC start/end time, normalized path, original IP, Visitor Digest, country code, region code, city, browser, and operating system. On-screen timestamps use each record's Site timezone; hovering reveals UTC.

Export current filters streams every matching record to CSV and is not limited by the current page size. The file contains UTC and Site-local timestamps plus every detailed field, including coordinates, original IP, and Visitor Digest. Text beginning with `=`, `+`, `-`, or `@` receives a leading apostrophe so spreadsheet software does not interpret external data as a formula.

Aggregate export requires one Site and separately exports overall, hostname, path, country, region, city, browser, or operating-system families, optionally bounded by Site-local dates.

When one configured Site is used on multiple domains, each hostname appears as an independent aggregate row. Pageview Records also retain the hostname that the tracker reported and the server confirmed from the allowed Origin; the same visitor is therefore counted independently on different hostnames.

The **Refresh geography** action on a Site page queries the active GeoIP database again for every retained Pageview Record. It updates each record and rebuilds country, region, and city PV/UV for every Site-local date represented by those records. Dates with no remaining records are left untouched, as are overall, hostname, path, browser, and operating-system aggregates. A valid IP with no database match has stale geography cleared and is counted under the unknown country; a malformed stored IP is skipped and keeps its existing geography. The action is available only while a GeoIP database is loaded and invalidates cached maps after a successful transaction.

## Website Integration

The integrated Widget records a Pageview and inserts the map:

```html
<script async src="https://stats.example.com/embed/widget.js?site_id=SITE_ID"></script>
```

For a subpath deployment, use the Base URL shown in the Admin Console:

```html
<script async src="https://stats.example.com/visitortrace/embed/widget.js?site_id=SITE_ID"></script>
```

The separated Tracker records a Pageview without rendering a map:

```html
<script async src="https://stats.example.com/embed/tracker.js?site_id=SITE_ID"></script>
```

The tracker reports the current page hostname. The server derives the authoritative hostname from the validated request Origin, so different domains sharing one Site remain separate in hostname statistics and Unique Visitor counting.

The separated integration area also provides a copyable map control snippet for lazy loading. The map can be loaded independently as an image:

```html
<img loading="lazy"
     src="https://stats.example.com/api/v1/sites/SITE_ID/map.svg"
     alt="Visitor map">
```

The Site page provides one-click copy controls for every integration snippet and resource URL. The integrated mode pairs the Widget embed code with its script URL. The separated mode does the same independently for the Tracker and lazy-loaded Map SVG.

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

Equivalent parameters normalize to one SVG cache entry. Public maps return an `ETag` and remain cached for five minutes, so a new Pageview can take up to about five minutes to appear at an existing map URL. The service keeps at most 256 variants per Site and 32 MiB globally and coalesces concurrent first renders of the same variant.

## GeoIP

VisitorTrace uses one active local MMDB provider at a time and maps all supported schemas to the same internal fields: country code/name, region code/name, city, latitude, and longitude. Select the provider with `geoip_provider`:

| Provider | Database format | Default update behavior |
| --- | --- | --- |
| `dbip` | DB-IP City Lite (`.mmdb.gz`) | Monthly, no credentials |
| `maxmind` | MaxMind GeoLite2 City (`.tar.gz`) | Automatic, Account ID and License Key required |
| `ip2location` | IP2Location LITE DB11 MMDB (`.zip`) | Monthly, Download Token required |

The default provider is `dbip`. All providers use the same automatic updater and atomic activation path. Initialize MaxMind with credentials from the MaxMind account portal:

```sh
visitortrace init \
  --data-dir /var/lib/visitortrace \
  --config /etc/visitortrace/config.json \
  --geoip-provider maxmind \
  --maxmind-account-id ACCOUNT_ID \
  --maxmind-license-key LICENSE_KEY
```

MaxMind official downloads use HTTP Basic Authentication. Initialize IP2Location with its account Download Token:

```sh
visitortrace init \
  --data-dir /var/lib/visitortrace \
  --config /etc/visitortrace/config.json \
  --geoip-provider ip2location \
  --ip2location-token DOWNLOAD_TOKEN
```

The equivalent protected configuration fields are `maxmind_account_id`, `maxmind_license_key`, and `ip2location_download_token`. The three built-in sources are:

```text
DB-IP:       https://download.db-ip.com/free/dbip-city-lite-{YYYY-MM}.mmdb.gz
MaxMind:     https://download.maxmind.com/geoip/databases/GeoLite2-City/download?suffix=tar.gz
IP2Location: https://www.ip2location.com/download?file=DB11LITEMMDB
```

IP2Location publishes the exact download code in the account Download page. `DB11LITEMMDB` is the built-in LITE DB11 MMDB code; if the account page shows a different code, override `geoip_update_url` with the URL shown there. The updater follows the HTTPS redirects used by MaxMind and IP2Location.

VisitorTrace checks at startup and every 24 hours. DB-IP and IP2Location use a calendar-month freshness policy; MaxMind is checked after 72 hours. It recognizes raw MMDB, gzip-compressed MMDB, tar.gz, and ZIP by file signature. VisitorTrace bounds downloaded and expanded sizes, verifies the complete MMDB search tree and data section, validates the configured provider's database shape, and only then atomically replaces and hot-loads the database. The prior version remains at `<geoip_path>.previous`; a failed activation rolls back automatically.

Check and update manually with:

```sh
./bin/visitortrace geoip update \
  --config "$HOME/.config/visitortrace/config.json"
```

Use `--force` to download again despite the provider's freshness policy. A command-line update runs in a separate process, so restart a running systemd service afterward. The built-in automatic update hot-loads the new database directly.

Inspect the raw MMDB record for one IP when diagnosing a city-level result:

```sh
# Use the configured geoip_path.
./scripts/query-mmdb.sh --binary ./bin/visitortrace \
  --config "$HOME/.config/visitortrace/config.json" \
  1.2.3.4

# Or bypass the configuration and select an MMDB explicitly.
./scripts/query-mmdb.sh --binary ./bin/visitortrace \
  --mmdb /path/to/geoip.mmdb \
  1.2.3.4
```

The command prints formatted JSON containing the database metadata, the matched CIDR, a `found` flag, and the unmodified MMDB `record` tree. It does not apply VisitorTrace's city-level label normalization. A missing address returns `found: false` and a `null` record. On a deployed system, the same operation can be run directly with the installed executable, for example `sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace geoip query --config /etc/visitortrace/config.json 1.2.3.4`.

The updater can consume an HTTPS mirror that exposes any supported container and an optional SHA-256 sidecar. Provider credentials are attached only to that provider's exact official hostname and are never sent to a custom mirror. Configure a private or domestic mirror explicitly:

```json
{
  "geoip_update": "automatic",
  "geoip_update_url": "https://mirror.example.com/dbip-city-lite-{YYYY-MM}.mmdb.gz",
  "geoip_checksum_url": "https://mirror.example.com/dbip-city-lite-{YYYY-MM}.mmdb.gz.sha256"
}
```

`geoip_checksum_url` is optional. When present, VisitorTrace verifies the downloaded container's SHA-256 before extraction. Remote sources must use HTTPS, except loopback test endpoints. Set `"geoip_update": "disabled"` to disable downloads. Existing `"monthly"` values are migrated to `"automatic"` when read.

The account credentials are secrets. Keep the configuration at mode `0600`, restrict backup access because backups include the configuration, and do not put credentials in either update URL.

Existing installations can switch providers without rerunning `init`: use **Administrator Settings > Service configuration**, enter new credentials when required, and save the combined configuration. The service restarts with the new provider. Credentials saved for other providers remain available for a later switch unless explicitly removed.

Without GeoIP, the service can still start and render existing aggregates and the basemap, but `/health/ready` remains unavailable and new Pageviews receive no geographic location. The map hover details, Admin previews, and Public Analytics show the attribution for the active provider. DB-IP Chinese city-label normalization applies only to DB-IP records; MaxMind and IP2Location city names are mapped as supplied by those databases.

## Backup and Restore

Create a consistent SQLite snapshot and configuration archive:

```sh
./bin/visitortrace backup \
  --config "$HOME/.config/visitortrace/config.json"
```

Backups are written to `backup_dir`, which defaults to `backups` inside the data directory. Every `.vtbackup` archive has a `.sha256` sidecar, while the database and configuration entries also carry individual SHA-256 digests in the archive manifest. The command runs a SQLite integrity check and retains the latest three archives by default. Use `--output` and `--keep` to override those defaults.

Stop VisitorTrace through systemd before restoring:

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

## One-Click Self-Update

Self-update uses side-by-side version directories and a stable symbolic link; it never overwrites the running executable. Bootstrap the layout once:

```sh
./bin/visitortrace update bootstrap \
  --config "$HOME/.config/visitortrace/config.json"
```

The command prints a stable executable path similar to:

```text
$HOME/.local/share/visitortrace/releases/current/visitortrace
```

Configure the systemd service to run that stable path and restart the process after it exits. BT Panel is only used for Nginx and TLS; VisitorTrace does not depend on a BT Panel API.

You can then check or apply releases on the server:

```sh
visitortrace update check --config "$HOME/.config/visitortrace/config.json"
visitortrace update apply --config "$HOME/.config/visitortrace/config.json"
```

When a release binary has already been downloaded manually, use the repository script to install it without network access:

```sh
sudo ./scripts/update-systemd-binary.sh \
  --binary ./visitortrace-0.1.3-linux-amd64 \
  --checksum-file ./checksums.txt
```

The script verifies the local checksum when supplied, runs the candidate's `doctor --upgrade-check`, creates a verified pre-update backup, switches the stable release link atomically, and restarts the systemd service. If the new process does not stay active, it restores the previous release. It preserves an intentionally inactive service as inactive. To keep automatic rollback from mixing executable and database versions, local updates must keep the same database schema; use the signed updater for a schema-changing release. The defaults match the deployment guide; use `--user`, `--data-dir`, `--config`, or `--service-name` for a custom installation.

Administrator Settings offers two signed update methods. **Online update** fetches the configured manifest and platform asset. **Local files** accepts the `manifest.json` and the binary for the displayed platform from the same Release, which is useful when the server cannot reach the release host. Both methods re-verify the current Administrator password. Unlike `update-systemd-binary.sh`, the local-file Admin method uses the complete signed updater and can apply a release that changes the database schema.

After either method prepares the candidate, the current process exits gracefully and the supervisor starts the new version through the stable path. A reverse proxy must allow the local upload size; the deployment guide uses `client_max_body_size 210m` to cover the signed 200 MiB asset limit plus multipart overhead.

The updater verifies the Ed25519 manifest signature, platform asset size and SHA-256, candidate version/schema identity, and the candidate's `doctor --upgrade-check`. Only then does it create a pre-update database snapshot, persist pending state, and atomically switch `current`. Readiness confirms the new release and retains the two latest prior versions. Three failed readiness startups restore the pre-update database and switch back to the prior release.

Production release builds must embed the project's release public key. Development builds without a key disable both update methods. The manifest defaults to GitHub Releases and can point to a domestic mirror through protected configuration:

```json
{
  "update_manifest_url": "https://mirror.example.com/visitortrace/manifest.json"
}
```

A mirror cannot replace the trust root. The manifest must verify against the public key embedded in the executable regardless of its download location.

## License

VisitorTrace is distributed under the [GNU General Public License, version 3](../LICENSE). Third-party components and data remain under their respective terms as listed in the [Third-Party Notices](../THIRD_PARTY_NOTICES.md). The license text, rather than this guide, defines the applicable rights and obligations.

## Current Status

The current milestone implements the agreed first-version scope, including Pageview ingestion and aggregates, counting-rule history, automatic cleanup, automatic GeoIP updates, bounded SVG-map caching, bilingual interactive analytics, administrative data and health views, password and Site lifecycles, backup/restore, and signature-verified one-click self-update.
