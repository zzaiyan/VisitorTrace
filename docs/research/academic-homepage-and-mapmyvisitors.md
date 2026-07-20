# VisitorTrace: Academic Homepage and MapMyVisitors Research

Research snapshot: 2026-07-21

Homepage repository: <https://github.com/zzaiyan/zzaiyan.github.io>  
Inspected commit: `7e4decae9ecef30321134e1d0093c6c311d03e5e` (2026-07-19)

This note separates repository evidence, observed third-party behavior, and the provider's product claims for the VisitorTrace replacement. MapMyVisitors does not present the widget endpoints discussed below as a stable public API, so its behavior may change without notice.

## Homepage Implementation

The academic homepage is a statically generated Jekyll site deployed through GitHub Pages. It pins the `github-pages` gem at version 232, uses Liquid layouts and includes, compiles Sass into compressed CSS, and serves ordinary static browser assets. The default layout includes the shared footer and then the shared script list.

The visitor map integration is split across four files:

- `_includes/footer.html` creates an empty `.page__footer-visitor-map` container. Its `data-visitor-map-src` attribute contains the complete MapMyVisitors `map.js` URL and widget configuration.
- `_includes/scripts.html` loads the local `assets/js/visitor-map.js` adapter with `defer`.
- `assets/js/visitor-map.js` waits until the container approaches the viewport, then creates the third-party `<script>` element. It uses an `IntersectionObserver` with a vertical `rootMargin` of 320 pixels and falls back to loading after the page load event on older browsers.
- `_sass/_footer.scss` reserves a 300 by 168 pixel minimum area on desktop and constrains the map to 360 pixels on narrow screens. This is the compatibility footprint for the replacement Public Map.

The adapter reuses an existing page-level jQuery instance by assigning it to `window.vmap_jq` when one is available. It marks the container unavailable when the initial third-party script fails, but otherwise delegates rendering, tracking, retries, and navigation to MapMyVisitors.

## Observed Widget Interface

The homepage currently loads a URL with this shape:

```text
GET https://mapmyvisitors.com/map.js
    ?d=<public-widget-id>
    &w=300
    &t=tt
    &cl=6f808f
    &co=f2f3f3
    &ct=9ba1a6
    &cmo=e34949
    &cmn=3fdd3f
```

Observed query semantics are:

| Parameter | Observed role |
| --- | --- |
| `d` | Public widget/Site identifier used to select the registered website and statistics. It is present in public HTML and is not a secret. |
| `w` | Widget width in pixels. The script derives map height as approximately `w / 2.04`; omitted or `a` values trigger width discovery and clamping. |
| `t` | Counter/date label mode. In the current script, `m` retains the date, `n` hides the Pageview label, and other values such as the homepage's `tt` hide the date while retaining the total. |
| `co` | Map-container background color. |
| `ct` | Counter/date text color. |
| `cl` | Land/background-map color included in the generated basemap asset key. |
| `cmo` | Older marker color. |
| `cmn` | New or recent marker color. |

This is an inferred widget contract, not a versioned or documented data API. The implementation accepts arbitrary query keys and forwards them to the next endpoint, so parameter support and validation are controlled entirely by the provider.

## Observed Request and Rendering Flow

```text
Homepage adapter
  -> map.js
     -> existing window.vmap_jq, or jQuery 1.12.4 from code.jquery.com
     -> widget_call_home.js JSONP
        -> callback returns JavaScript source as a string
        -> map.js evaluates that string
        -> jVectorMap renders the world map and markers
        -> /ajax/map JSONP obtains the initial/live map update
```

The inspected `map.js` response was 72,067 bytes before transport compression and reports an internal modification date of 2023-09-14. It embeds the jVectorMap implementation and world-map data. If the host page does not expose `window.vmap_jq`, it downloads jQuery 1.12.4 from a second domain.

The bootstrap request uses JSONP at `/widget_call_home.js`. The JSONP value is itself a JavaScript source string; the bootstrap script calls `eval()` on it. That generated program supplies the project identity, profile link, displayed total, marker values and coordinates, and map behavior. It then uses another JSONP request to `/ajax/map`. The widget therefore depends on executable responses and multiple provider-controlled resources rather than a stable JSON data representation.

In the observed homepage response, the widget rendered:

- a non-interactive world map at the embedded size;
- up to 20 aggregate location markers in the initial payload;
- marker radius based on the supplied location counts;
- hover tooltips such as a visit count and location name;
- an all-time `Total Pageviews` label;
- a click-through link to the provider-hosted statistics profile.

The embedded mode explicitly disables scroll zoom, drag panning, and zoom buttons. MapMyVisitors also advertises a 3D globe, real-time locations, configurable date-range trends, country/city tables, page-level traffic, referrers, browser/operating-system details, and multi-site management on its hosted pages. Those hosted features are product claims and are not all represented in the public widget payload.

## Counting Consequence of the Current Integration

The homepage does not load a separate MapMyVisitors tracking pixel or tracker. The widget request is therefore the observable tracking trigger. Because the local adapter does not request `map.js` until the footer is within 320 pixels of the viewport, visits that never approach the footer can avoid loading the provider and can be absent from its count.

This coupling explains a key replacement requirement: Pageview tracking must load independently near the start of the page, while the visible Public Map may remain lazy. The replacement tracker can then count a page load without forcing the visitor to download map assets, and map visibility no longer changes Pageview semantics.

## Data Access and Migration Limits

The inspected widget response exposed a changing all-time Pageview label and a limited list of aggregate markers. It did not expose a documented export endpoint, complete daily history, complete geographic history, individual observations, or a Unique Visitor series compatible with this project's Deduplication Window definition.

Consequently, the replacement should begin with an explicit tracking start date and empty local statistics. Evaluating or scraping the provider's executable response would be fragile, would preserve only a partial snapshot, and would still not reconstruct compatible historical Unique Visitors. A screenshot and the last displayed total may be kept outside the new service as an archival reference.

## Replacement Compatibility Target

The replacement offers two integration modes. The homepage can use either:

1. **Integrated Widget**: one `widget.js` snippet that records the current Pageview and inserts the self-hosted map. This is the closest replacement for the current MapMyVisitors installation.
2. **Separated Integration**: a small dependency-free tracker loaded on every page, plus an optional lazy `<img>` whose source is the self-hosted 300 by 168 Public Map SVG and whose link opens Public Analytics.

The intended installation shapes are:

```html
<!-- Integrated Widget -->
<script async src="https://stats.example.com/embed/widget.js?site_id=<SITE-ID>&w=300"></script>
```

```html
<!-- Separated Integration -->
<script async src="https://stats.example.com/embed/tracker.js?site_id=<SITE-ID>"></script>
<a href="https://stats.example.com/public/<SITE-ID>/analytics">
  <img loading="lazy" width="300" height="168"
       src="https://stats.example.com/api/v1/sites/<SITE-ID>/map.svg?w=300">
</a>
```

The Integrated Widget is one integration entry point, not one HTTP request. It internally uses the same collection request as the standalone tracker and the same cacheable SVG request as the standalone map. The map request never counts a Pageview, so image caching cannot suppress or duplicate tracking.

Both modes remove runtime jQuery, JSONP, `eval()`, and third-party tracking dependencies from the homepage. URL parameters continue to provide MapMyVisitors-like presentation overrides, while the saved Map Preset remains the default configuration.
