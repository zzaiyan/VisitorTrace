# Third-Party Notices

[Chinese](./THIRD_PARTY_NOTICES.zh-CN.md)

VisitorTrace includes a generated world basemap derived from Natural Earth 1:110m Admin 0 Countries vector data. The Antarctica feature is omitted for the compact visitor-map presentation.

- Source repository: <https://github.com/nvkelso/natural-earth-vector>
- Source commit: `ca96624a56bd078437bca8184e78163e5039ad19`
- Source file: `geojson/ne_110m_admin_0_countries.geojson`
- Source SHA-256: `6866c877d39cba9c357620878839b336d569f8c662d3cfab4cb1dbe2d39c977f`
- Generated files: `internal/maprender/assets/world.path`, `web/assets/world.geo.json`

Natural Earth vector and raster map data is in the public domain. See <https://www.naturalearthdata.com/about/terms-of-use/>.

The generated place-name data in `internal/geoip/place_names.tsv.gz` incorporates place names and alternate names from [GeoNames](https://www.geonames.org/) and subdivision codes derived from [world_countries](https://github.com/stefangabos/world_countries). GeoNames data is licensed under [Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/); its subdivision data is distributed under [Creative Commons Attribution-ShareAlike 4.0](https://creativecommons.org/licenses/by-sa/4.0/). VisitorTrace compresses and transforms the names into English display labels, subdivision mappings, and alias rules. Generation inputs were captured on 2026-10-08: GeoNames SHA-256 `bd51fc33c0847de2079e2cb7b32a5f1bd32bd21ae064757fbc4ac35986f11922`, `9e7f4ec73433b17aa6344a00031461ac475f891200aab9f2559f8e2ccaa4a449`, and `1da92a6323a5fec3176f3f743bf4cf4040fd56a876da55e46fbca23c863aa60a`; world_countries commit `7edbb1e30ddc9b616ee07f76a3cad3af2416b618`.

VisitorTrace supports user-supplied and automatically downloaded databases from DB-IP City Lite, MaxMind GeoLite2 City, IP2Location LITE DB11, and ip2region. It can also query Tencent Location Service, Amap, IPinfo, and BigDataCloud using credentials supplied by the operator. No GeoIP database or provider credential is included in this repository. Each source remains subject to its own license, terms, account requirements, usage limits, and attribution requirements. See <https://db-ip.com/db/lite.php>, <https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/>, <https://lite.ip2location.com/ip2location-lite>, <https://github.com/lionsoul2014/ip2region>, <https://lbs.qq.com/>, <https://lbs.amap.com/>, <https://ipinfo.io/>, and <https://www.bigdatacloud.com/>.

The interactive Public Analytics charts use Apache ECharts 6.1.0 under the Apache License 2.0. Release artifacts include a browser bundle generated from the ECharts source. See <https://echarts.apache.org/>.
