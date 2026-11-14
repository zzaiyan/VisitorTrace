# World basemap source

VisitorTrace vendors `world.zh.json` from `@surbowl/world-geo-json-zh` 2.1.5.

- Package: <https://www.npmjs.com/package/@surbowl/world-geo-json-zh/v/2.1.5>
- Archive: <https://registry.npmjs.org/@surbowl/world-geo-json-zh/-/world-geo-json-zh-2.1.5.tgz>
- Source SHA-256: `763e5af050a343852ca4140f144ac433fa44751ac4d883051a306a229a22c90c`
- Package integrity: `sha512-w9Cf4uE+1RkD9gaFzHrlxL4zlTXnFaGJ9fZYsee8jr0lp2++yIBc1PMRJEvrALKFxk2LARNDssoYQdso2aEFIA==`
- License: The Unlicense; the upstream license text is in [LICENSE](./LICENSE).

The upstream dataset derives from Natural Earth 1:50m v5.1.1 and documents
geometry adjustments referencing Ministry of Natural Resources standard maps
and DataV/Amap: Taiwan is included in China's geometry, with adjustments to the
China–India boundary and Heixiazi Island, and added Diaoyu/Chiwei islands,
South China Sea islands and ten-dash polygons. Its cited review numbers belong
to those reference maps, not to this community dataset or VisitorTrace's output.
The original GitHub repository is unavailable and npm marks the package as
deprecated; the exact source is vendored so builds need no remote map service.

Generate both embedded basemaps from the same source:

```sh
npm ci
node tools/generate-basemap.mjs tools/basemap/world.zh.json \
  internal/maprender/assets/world.path web/assets/world.geo.json
npm run build:analytics
node --test tools/basemap/basemap.test.mjs
```

The generator does not perform cartographic simplification and retains small islands and ten-dash polygons. The interactive
GeoJSON uses English country names from Node.js `Intl.DisplayNames` (CLDR),
matching the English place labels used by IP geolocation. The three regions
without ISO country codes use English metadata names from the underlying
Natural Earth dataset, stored in `region-names.en.json`. The upstream Chinese
labels remain in the vendored source. The generator uses `polygon-clipping`
(MIT, build time only) to wrap polygons into -170°..190° and union the pieces
within each country. This removes artificial outlines where the source was
cut at ±180°; endpoints within 0.0001° of that meridian are snapped together
to close source rounding gaps. Countries away from the display seam keep
their original coordinates. Antarctica is outside the viewport and omitted.
SVG and ECharts use the same joined geometry. `layers.json` identifies
the ten-dash polygons from the upstream `world.zh.jsonc` annotations (China
polygon indexes 13–22). SVG draws these same polygons in a separate stroke
layer, `internal/maprender/assets/world-boundaries.path`, so the configurable
country border cannot obscure narrow dashes. Neither output is a claim
that the resulting styled, projected visitor map has received map approval.
