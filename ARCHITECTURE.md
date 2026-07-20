# VisitorTrace Architecture

This document is the current architecture snapshot for VisitorTrace (访迹). Domain language is defined in [CONTEXT.md](./CONTEXT.md).

## Project Identity

- The official English project name is **VisitorTrace**.
- The official Chinese project name is **访迹**.
- Bilingual product surfaces present the name as **VisitorTrace · 访迹**; English-only and Chinese-only contexts use the corresponding single-language name.
- The stable lowercase technical identifier is `visitortrace` for the repository, executable, operating-system service, configuration names, and default data paths.
- The English descriptor is “A tiny self-hosted visitor map and pageview tracker.”
- The Chinese descriptor is “轻量、自托管的访客地图与访问记录服务。”
- Public Map, Pageview, Site, and other terms in [CONTEXT.md](./CONTEXT.md) remain domain concepts rather than aliases for the project name.

## Goals and Constraints

- Run on a mainland China cloud server to minimize latency for domestic visitors.
- Serve a small number of personal Sites such as an academic homepage and blog.
- Keep CPU, memory, storage, and operational overhead low.
- Keep observations and statistics isolated per Site.

## Current Implementation Milestone

- Schema version 4 stores Sites, Pageview Records, visitor registrations, daily Aggregates, durable geographic coordinates, and opaque Administrator sessions.
- The service currently exposes Pageview collection, both public integration modes, SVG Public Maps, Public Analytics, Administrator login, Site settings, recent Pageview Records, Map Preset editing, and live administrative map previews.
- The current analytics pages use server-rendered tables and a small dependency-free trend visualization. The planned tree-shaken ECharts bundle remains a later enhancement for richer pan, zoom, and chart inspection.
- Retention cleanup, backup and restore commands, GeoIP auto-update, password reset, and release self-update remain later operational milestones.

## Runtime and Packaging

- The application is implemented as one Go service process.
- The same service exposes ingestion, Public Map and Public Analytics endpoints, and the Admin Console.
- HTML templates and browser assets are embedded in the executable.
- Retention cleanup, completed-window cleanup, and other lightweight scheduled maintenance run inside the service process.
- Database migration, Administrator password reset, and operational maintenance are exposed as subcommands of the same executable.
- Production deployment does not require a Node.js or Python runtime, a separate job worker, or separately deployed frontend files.

## Deployment

- The application runs as a conventional operating-system process and does not depend on a specific process manager, hosting panel, container runtime, or reverse-proxy product.
- An external process supervisor starts and restarts the service; the service supports graceful shutdown on standard termination signals and writes logs to standard output and error.
- A reverse proxy terminates HTTPS and forwards requests to the service on a loopback-only listener.
- The application accepts forwarded client IP and scheme headers only when the direct peer belongs to an explicitly configured trusted-proxy set.
- SQLite data, GeoIP data, configuration, and generated backup snapshots reside in a persistent directory outside the project release directory.
- Configuration is supplied through command-line flags, environment variables, or a protected configuration file without requiring a hosting-panel API.
- The first version uses neither Docker nor a CDN.
- The application provides a backup subcommand that creates a consistent SQLite snapshot before any external scheduler copies or uploads it.
- BaoTa Panel may implement the process-supervisor, reverse-proxy, TLS, scheduling, and log-collection roles in one deployment, but no application behavior is built specifically for BaoTa.

## First-Run Bootstrap

- A new deployment is initialized locally with `visitortrace init`; the application does not expose a web-based installation route or default Administrator credential.
- Initialization creates the persistent directories, SQLite database, protected configuration, and initial GeoIP database before `serve` may start.
- The Administrator password contains from 8 through 128 characters and has no character-class composition requirement.
- Interactive initialization reads and confirms the password from a TTY without echoing it.
- Automation supplies the password through standard input or a protected password file, never through a command-line argument.
- Persistent data directories use mode `0700`, and protected configuration and SQLite files use mode `0600` where supported by the operating system.
- `serve` refuses to start before successful initialization, and `init` refuses to overwrite an existing database.
- A lost password is replaced only with the server-side reset subcommand, which revokes all active Administrator sessions.

## Product Boundary

- The Public Map exposes only aggregate geographic distribution, Pageview totals, and Unique Visitor totals.
- Public Analytics is unauthenticated and exposes Pageview and Unique Visitor totals, daily trends, geographic aggregate maps and tables, and aggregate browser and operating-system shares.
- Public Analytics does not expose a real-time visitor feed, Pageview Records, original IP addresses, anonymous visitor digests, exact visit timestamps, page paths, or settings.
- The password-authenticated Admin Console exposes all Pageview Records, Aggregates, and Site settings.
- The first version excludes session paths, referrer analysis, and a general-purpose analytics platform.

## Localization

- The first version supports Simplified Chinese and English user-interface text.
- The Admin Console defaults to Simplified Chinese and persists the Administrator's language preference.
- Each Site selects an automatic, Simplified Chinese, or English default for Public Analytics; automatic mode follows the request's `Accept-Language` preference.
- Public Analytics accepts `lang=zh-CN` and `lang=en` as explicit shareable overrides.
- Geographic data retains stable country and subdivision codes together with Chinese and English display names so language changes do not split Aggregate identities.
- Public Map SVG labels remain controlled by the Map Preset and documented label overrides; SVG rendering does not add a language parameter.
- Additional interface languages are outside the first-version scope.

## Existing Service Transition

- The current academic-homepage integration obtains a generated JavaScript program from MapMyVisitors through an undocumented JSONP endpoint rather than a stable export API.
- The observed response exposes an all-time Pageview label and a limited set of aggregate map markers, but not daily history, complete geographic history, or Unique Visitors compatible with this system's definition.
- A replacement Site starts with empty Pageview Records and Aggregates and exposes its explicit tracking start date.
- The service does not scrape, evaluate, or import MapMyVisitors responses and does not create synthetic legacy counters.
- The previous map and displayed total may be retained outside the service as a screenshot or other static archive.

## Deferred Privacy Features

- The first version does not inspect Global Privacy Control or Do Not Track signals.
- The tracker does not provide built-in opt-in, opt-out, consent-gating, prior-record deletion, or privacy-notice generation features.
- Any consent flow, disclosure, or privacy-policy obligation is handled outside this service by each Site operator.

## Site Lifecycle

- A Site uses an IANA timezone selected at creation and defaulting to `Asia/Shanghai`.
- The timezone remains editable until the first accepted Pageview and is locked while the Site retains observations or statistics.
- Each Site independently controls whether it accepts new Pageviews and whether its Public Map and Public Analytics are published.
- Disabling Pageview acceptance rejects new tracking reports without hiding or deleting existing public or administrative data.
- Disabling publication makes Public Map and Public Analytics unavailable without stopping ingestion unless Pageview acceptance is also disabled.
- Pageview Record expiry and other maintenance continue while either capability is disabled.
- Permanent deletion is available only after both capabilities are disabled and the Administrator re-enters both the current password and exact Site ID.
- Permanent deletion removes the Site's Pageview Records, Aggregates, settings, Map Preset, visitor registrations, and HMAC key.
- A deleted Site can be recovered only by restoring an earlier complete backup, and its Site ID is never assigned to another Site.
- A separate Site-data reset requires Pageview acceptance to be disabled and the Administrator to re-enter both the current password and exact Site ID.
- Reset removes Pageview Records, Aggregates, visitor registrations, and the existing HMAC key while preserving the Site ID, Allowed Origins, publication setting, and Map Preset.
- Reset generates a new HMAC key and unlocks the timezone for editing; historical data is not rebucketed into a new timezone.

## Counting Semantics

- Every accepted tracking report creates one Pageview.
- Unique Visitors are estimated independently per Site.
- Repeated Pageviews are deduplicated within a Site-configured Deduplication Window.
- Deduplication Windows are configurable as an integer from 1 through 30 calendar days, begin at local midnight in the Site's timezone, and default to one day.
- Unique Visitor totals spanning multiple Deduplication Windows are calculated by summing the completed and active window counts without cross-window deduplication.
- A Deduplication Window change takes effect at the next midnight in the Site's timezone.
- A visitor contributes to the daily Unique Visitor trend only on the date of their first accepted Pageview in the active Deduplication Window.
- Completed Aggregates are not recalculated after a Deduplication Window change.
- The Admin Console marks the effective date of each counting-rule change on affected trends.

## Tracking Trust Boundary

- The Administrator creates Sites in the Admin Console.
- Each Site receives a public Site ID and a configured list of Allowed Origins.
- Browser tracking ingestion is an unauthenticated public write operation because static Sites cannot keep a client secret.
- Embed code must not contain a value represented as a secret or authentication credential.
- The ingestion endpoint enforces Allowed Origin and CORS checks, per-Site and per-IP rate limits, payload validation, and basic bot filtering.
- Allowed Origin checks reduce accidental and browser-based misuse but can be spoofed by a determined non-browser sender.
- The system accepts that a determined attacker can pollute statistics; tamper-proof analytics would require a server-side proxy or dynamic signatures at every Site and is outside the current scope.

## Public Integration Modes

- The service exposes two public integration modes over one shared collection and rendering core: Integrated Widget and Separated Integration.
- Integrated Widget is the MapMyVisitors-like path. A Site embeds one `widget.js` entry point, which reports the current Pageview and inserts the Public Map using the same Map Preset and URL override rules as the standalone map endpoint.
- Integrated Widget is one integration experience, not one HTTP transaction: the browser performs the normal Pageview collection request and the map request independently.
- The Integrated Widget reports through the normal Browser Visitor ID, path normalization, Origin validation, rate limiting, bot filtering, and aggregate transaction path. It does not count the map image request.
- A collection failure does not prevent the map from rendering, and a map failure does not turn a successfully collected Pageview into a failed Pageview.
- Separated Integration exposes the tracker script and Public Map endpoint independently. A Site may install only the tracker, install both with the map loaded lazily, or omit the map entirely.
- The tracker script in both modes uses the same runtime duplicate guard for a Site and normalized path, so accidentally including both integration snippets does not intentionally emit two reports in one browser runtime.
- The Public Map endpoint remains cacheable and never creates a Pageview as a side effect. This keeps browser and proxy cache revalidation from changing counting semantics.
- The Admin Console generates installation examples for both modes and identifies which snippet is responsible for Pageview collection.

The canonical public route shapes are:

- `GET /embed/widget.js?site_id=<Site-ID>&...`: Integrated Widget script. The documented map presentation overrides are accepted on this route and apply only to the map it inserts.
- `GET /embed/tracker.js?site_id=<Site-ID>`: Separated Integration tracker script.
- `POST /api/v1/sites/<Site-ID>/pageviews`: low-level Pageview collection endpoint used by the tracker scripts and advanced SPA integrations.
- `GET /api/v1/sites/<Site-ID>/map.svg?...`: standalone cacheable Public Map SVG endpoint used by Separated Integration and by the Integrated Widget internally.

The route names are part of the first-version public contract; implementation code may share handlers and internal services behind them.

## Ingestion Interface

- The ingestion endpoint accepts only `POST` requests with a strictly parsed JSON object carried as CORS-safelisted `text/plain` content and returns `204 No Content` after a successful commit.
- Request bodies are limited to 2 KiB, normalized paths to 512 bytes, and Browser Visitor IDs to the documented 128-bit representation.
- An `Origin` header must exactly match one of the Site's Allowed Origins; missing, opaque, malformed, or unlisted origins are rejected.
- An in-memory token bucket limits each Site and client IP combination to a burst of 30 requests and a sustained rate of 120 requests per minute.
- A second in-memory token bucket limits each Site to a burst of 500 requests and a sustained rate of 3,000 requests per minute.
- Empty User-Agent values and User-Agents clearly identifying common crawlers, spiders, bots, link previews, or headless automation are rejected by the basic bot filter.
- Rejected and rate-limited reports do not create a Pageview Record or update Aggregates.
- Rate-limit defaults are adjustable through service configuration rather than ordinary Site settings, and limiter state is not persisted across process restarts.

## Tracking Transport

- The shared tracker implementation is dependency-free and reports a Pageview on page load even when the page has no Public Map or the map never enters the viewport.
- The tracker script targets approximately 2 KB or less after gzip compression.
- Tracking uses `navigator.sendBeacon()` when available and falls back to a keepalive `fetch()` request.
- Separated Integration sites can load Public Map assets and aggregate data lazily when the map approaches the viewport without affecting counting.
- Integrated Widget sites may use the generated widget snippet for immediate display; the widget's map request remains independent from its Pageview collection request.
- Traditional multi-page Sites report automatically on each complete page load.
- SPA and PJAX Sites call an explicit `track()` API after a client-side navigation has completed; the tracker does not monkey-patch browser History APIs.
- The tracker ignores query parameters and URL fragments and applies a short in-memory debounce to duplicate calls for the same normalized path within one page runtime.
- Client-side debounce prevents integration mistakes only; it does not change the server-side definition of a Pageview.

## Page Path Normalization

- The tracker derives a page path from `location.pathname`; query parameters and URL fragments never enter the reported or stored path.
- An empty path becomes `/`, dot segments are resolved, and percent encoding is converted to one canonical form.
- Paths containing control characters or exceeding 512 bytes after normalization are rejected.
- Path case, repeated slashes, trailing slashes, and explicit index filenames are preserved because Sites may assign them distinct routing semantics.
- Explicit SPA or PJAX `track()` calls pass through the same normalization and validation.
- The first version does not provide path aliases, regular-expression rewrites, or historical path merges.

## Visitor Identity Estimation

- The tracker maintains a cryptographically random 128-bit Browser Visitor ID in Site-scoped browser local storage and reports it with each Pageview.
- The service derives a Visitor Digest with HMAC-SHA-256 using the Browser Visitor ID and a secret key unique to the Site.
- The raw Browser Visitor ID is validated during ingestion but is never stored.
- When browser local storage is unavailable, the service derives the Visitor Digest from the original client IP and normalized User-Agent instead.
- A complete User-Agent may be processed for fallback digest construction and browser or operating-system parsing but is discarded after ingestion.
- Clearing browser data, changing browser or device, or changing the Site identity can cause the same person to be estimated as a new visitor.
- Site HMAC keys are durable data stored in SQLite and must be preserved by backup and restore.
- A sender can forge Browser Visitor IDs; this remains part of the accepted unauthenticated-ingestion trust boundary.

## Geolocation

- IP geolocation is resolved locally during ingestion; Pageview acceptance does not depend on an external geolocation API.
- The first version uses the DB-IP City Lite database in MMDB format for IPv4 and IPv6 lookups.
- The database supplies country, first-level region, city, and approximate coordinates.
- A scheduled monthly update downloads and verifies a new database before replacing the active file atomically.
- The Public Map embed markup and Public Analytics display the attribution required by the DB-IP City Lite license.
- A lookup with no usable result is stored as unknown and is not retried through an external service.
- `ip2region` is not part of the first version; it may later be evaluated as a China-specific override if measured accuracy warrants the added complexity.

## Public Map Rendering

- The embeddable Public Map is a script-free SVG generated by the service from durable Aggregates.
- Its default footprint is 300 by 168 CSS pixels to replace the current academic-homepage widget without changing the footer layout.
- The SVG presents a world basemap, city markers scaled by aggregate activity, cumulative Pageviews, and cumulative Unique Visitors.
- The footer preview does not provide zooming or rich interaction; clicking it opens the Site's Public Analytics.
- Sites embed the SVG as a lazily loaded image, independently of Pageview tracking.
- SVG responses use an `ETag` and a five-minute public cache lifetime.
- Each Site has one Map Preset that supplies default dimensions, visible label content, font sizes, and related presentation choices.
- Supported URL query parameters may override Map Preset values for one embed without changing the saved defaults.
- Semantically equivalent requests are normalized to one effective rendering configuration and share one cached SVG variant.
- Public SVG endpoints accept only documented query parameters and return `400 Bad Request` for unknown, malformed, or out-of-range values.
- Rendered dimensions range from 160 through 1200 pixels wide and 90 through 800 pixels high.
- Font sizes range from 8 through 32 pixels, custom labels contain at most 40 characters, and colors use normalized hexadecimal values.
- The first-version override fields are `w`, `h`, `title`, `pv_label`, `uv_label`, `show`, `fs`, `bg`, `land`, `border`, `text`, `marker`, and `metric`; `bg=transparent` is the explicit transparent-background value.
- `show` selects a combination of title, Pageview, and Unique Visitor labels; `metric` selects Pageviews or Unique Visitors as the city-marker scale.
- Font families, map projections, arbitrary CSS, marker-radius controls, and time-range controls are not configurable in the first version.
- Rendered SVG variants are held in an in-memory LRU cache for five minutes, with at most 256 variants per Site and 32 MiB across the service.
- Concurrent cache misses for one normalized configuration are coalesced into one render operation.
- The Admin Console renders a live preview through the same override and rendering path before the Map Preset is saved.
- The SVG image itself does not draw provider attribution.
- Generated embed markup contains a DB-IP attribution link that appears when the map is hovered or keyboard-focused; the same behavior is visible in the Admin Console preview.
- Public Analytics displays a persistent DB-IP attribution link.
- Provider attribution cannot be disabled through Public Map override parameters.

## Analytics Frontend Rendering

- Public Analytics and the Admin Console use server-rendered HTML with small, dependency-free browser scripts for navigation, forms, filters, and settings.
- Interactive analytics charts use a self-hosted, tree-shaken Apache ECharts build with the SVG renderer.
- The chart bundle includes only the line, bar, pie, geographic-map, and scatter series plus the tooltip and data-zoom components required by the documented views.
- Chart code is loaded lazily only on pages that render interactive analytics and targets approximately 250 KiB or less after gzip compression.
- Browser assets are embedded in the Go executable; production requests never load ECharts or other application dependencies from a CDN.
- Node.js is permitted only in the release build toolchain and is not required to install, run, or update the production service.
- Interactive world views use Natural Earth 1:110m public-domain geography simplified and converted into application assets during the release build.
- Public Analytics supports map panning and zooming, aggregate marker tooltips, and interactive trend inspection; the Admin Console reuses the same chart components for administrative aggregate views.
- Every chart view has a server-rendered tabular representation that remains usable when JavaScript is unavailable.
- Public Map embeds and their Admin Console live previews remain service-generated SVG images and do not load the analytics chart bundle.

## Data Lifecycle

- Every Pageview updates durable Aggregates and creates a temporary Pageview Record.
- Aggregates remain after their source Pageview Records expire.
- Each Site has a Retention Period that defaults to 30 days and can be configured from 1 through 90 days.
- Retention is based on each record's actual age rather than calendar-month boundaries.
- Expired Pageview Records are deleted automatically; shortening the Retention Period makes newly out-of-range records immediately eligible for deletion.
- Extending the Retention Period cannot restore records that have already been deleted.

## Storage and Aggregation

- SQLite is the sole durable datastore; all Sites share one database file and are logically isolated by Site identity.
- SQLite runs in WAL mode using version 3.51.3 or later, or an earlier release carrying the official WAL-reset fix.
- Application writes are serialized while read connections may operate concurrently.
- One ingestion transaction creates the Pageview Record, registers the Site-scoped visitor digest for the active Deduplication Window, and updates the relevant Aggregates.
- A Unique Visitor counter is incremented only when the visitor digest is first registered for that Site and window.
- Failure of any ingestion write rolls back the complete transaction.
- Expired Pageview Records and completed-window visitor-digest registrations are removed in bounded batches.
- The first version does not require PostgreSQL, Redis, a message broker, or a separate asynchronous aggregation worker.

## Aggregate Model and Querying

- Durable Aggregates use the Site's local calendar date as their time grain.
- The system maintains independent daily Aggregate families for the Site overall, normalized page path, country and first-level region and city, browser type, and operating-system type.
- Every Aggregate family records Pageviews and Unique Visitors.
- Unique Visitors are deduplicated independently within each dimension value and Deduplication Window; dimension rows are not additive representations of the overall Unique Visitor count.
- The first version does not materialize cross-dimensional combinations such as city by browser or path by operating system.
- Public Analytics defaults to the latest 30 local calendar days and offers today, 7-day, 30-day, 90-day, all-history, and custom-date ranges.
- Public Analytics exposes overall, geographic, browser, and operating-system Aggregates but not path Aggregates.
- The Admin Console may query every Aggregate family, including normalized paths.
- The Public Map uses all-history Aggregates and does not accept a time-range override in the first version.

## Pageview Record Boundary

A Pageview Record contains:

- Site identity and timestamp;
- normalized page path without query parameters or fragments;
- country, first-level region, city, and coarse map coordinates;
- a Site-scoped anonymous visitor digest;
- the original IP address;
- parsed operating-system and browser types.

A Pageview Record does not contain:

- the complete User-Agent string;
- referrer details;
- URL query parameters or fragments.

## Administrative Data Access

- Pageview Records are read-only in the Admin Console and are ordered newest first by default.
- Record lists use cursor pagination with 100 rows by default and no more than 200 rows per response.
- Filters cover Site, timestamp range, normalized path, exact original IP, Visitor Digest, country, first-level region, city, browser type, and operating-system type.
- The current filtered Pageview Record result can be streamed directly as CSV without creating a temporary server-side export file.
- Record exports contain every Pageview Record field and represent timestamps in both UTC and the Site's local timezone.
- Aggregate families have separate CSV exports.
- The first version does not edit or delete individual Pageview Records because their durable Aggregate contributions cannot always be reversed consistently.
- Site-data reset is the supported destructive correction mechanism.

## Data Access Boundary

- Original IP addresses are stored as plaintext rather than field-level encrypted values.
- The Admin Console can access original IP addresses directly.
- Public Analytics must not expose sensitive Pageview Record fields.
- One global Administrator manages every Site.
- The system has no registration, invitations, additional accounts, roles, or per-Site permissions.
- Administrator authentication uses one password credential.
- Password recovery is performed with a server-side command rather than email.
- The password is stored as a salted Argon2id hash using at least 19 MiB of memory, two iterations, and one degree of parallelism.
- A successful login creates a 256-bit random opaque session token; only its SHA-256 digest is stored in SQLite.
- The session cookie uses `Secure`, `HttpOnly`, `SameSite=Strict`, host-only scope, and a root path.
- Sessions expire after 12 hours of inactivity and after an absolute lifetime of 7 days.
- State-changing Admin Console requests require a CSRF token.
- Failed login attempts are rate-limited by source IP without an external CAPTCHA service.
- Changing or resetting the Administrator password revokes every active session.
- The Admin Console requires HTTPS outside the local machine. Loopback hosts may use HTTP solely for local development and preview.

## Backup and Restore

- An external scheduler runs `visitortrace backup` every day at 03:30 Asia/Shanghai time.
- The backup subcommand uses SQLite's online backup mechanism to create a consistent snapshot without stopping ingestion.
- Every snapshot passes a database integrity check and receives a SHA-256 checksum before it is eligible for retention or upload.
- The server keeps the latest three snapshots locally.
- Private remote storage retains 14 daily snapshots, 8 weekly snapshots, and 6 monthly snapshots.
- Backups contain SQLite data and required application configuration, including Site HMAC keys and the Administrator password hash.
- The application binary and downloadable GeoIP database are excluded from recurring backup snapshots.
- Backup files are not encrypted by the application; local and remote backup locations must remain access-controlled.
- Restore verifies the checksum and database integrity before activation and revokes every restored Administrator session before service startup.

## Operational Contract

- `GET /health/live` reports whether the process can serve requests without checking external or durable dependencies.
- `GET /health/ready` reports readiness only when SQLite is usable, migrations are complete, and the GeoIP database is loaded.
- Health endpoints expose only a short status and HTTP status code; they do not disclose filesystem paths, versions, or data volumes.
- The Admin Console shows application version and uptime, SQLite size, disk availability, and the latest cleanup, GeoIP update, and backup outcomes.
- Application logs are structured and written to standard output and error for collection by any process supervisor.
- Stale backups, stale GeoIP data, failed cleanup, and low disk space are reflected in Admin Console status and error logs.
- The application does not embed a provider-specific restart, email, messaging, monitoring, or alert-delivery integration.

## Release Upgrade and Rollback

- Releases provide precompiled Linux `amd64` and `arm64` executables and published checksums.
- New and previous executables are installed side by side rather than overwriting the running file in place.
- `visitortrace doctor` validates configuration, GeoIP availability, disk space, SQLite version, and schema compatibility before activation.
- Every upgrade creates a consistent pre-upgrade database snapshot and then gracefully stops request processing.
- Forward database migrations are embedded in the new executable and run transactionally; the project does not implement downward migrations.
- Readiness is checked after activation, and the two most recent prior executables and the pre-upgrade snapshot remain available.
- A release with no schema change can roll back by switching executables; a release with a completed schema migration rolls back by restoring its pre-upgrade snapshot.
- The Admin Console must provide a user-initiated one-click self-update workflow in addition to the equivalent command-line workflow.
- Release manifests are signed with Ed25519 using a public key embedded in the application, and downloaded binaries must also match the manifest's SHA-256 digest.
- The release base URL is configurable only through server configuration so a domestic mirror may be used without weakening signature verification.
- Self-update requires an Administrator session whose password was verified within the previous ten minutes.
- An update is downloaded to a versioned directory and must pass `doctor` before the pre-upgrade snapshot is created and a durable pending-update state is written.
- Activation atomically switches a stable `current` symbolic link, drains active requests, and exits cleanly so any conforming external process supervisor restarts the configured command.
- A new release clears pending-update state only after reaching ready status; after three failed startup attempts, startup switches back to the prior executable and restores the pre-upgrade snapshot when a schema migration occurred.
- Self-update is initiated explicitly through the Admin Console or CLI and never installs a release silently in the background.

## Open Design Questions

None.
