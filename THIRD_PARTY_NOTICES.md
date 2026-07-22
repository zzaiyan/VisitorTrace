# Third-Party Notices

[Chinese](./THIRD_PARTY_NOTICES.zh-CN.md)

VisitorTrace includes a generated world basemap derived from Natural Earth 1:110m Admin 0 Countries vector data. The Antarctica feature is omitted for the compact visitor-map presentation.

- Source repository: <https://github.com/nvkelso/natural-earth-vector>
- Source commit: `ca96624a56bd078437bca8184e78163e5039ad19`
- Source file: `geojson/ne_110m_admin_0_countries.geojson`
- Source SHA-256: `6866c877d39cba9c357620878839b336d569f8c662d3cfab4cb1dbe2d39c977f`
- Generated files: `internal/maprender/assets/world.path`, `web/assets/world.geo.json`

Natural Earth vector and raster map data is in the public domain. See <https://www.naturalearthdata.com/about/terms-of-use/>.

VisitorTrace supports user-supplied MMDB files from DB-IP City Lite, MaxMind GeoLite2 City, and IP2Location LITE DB11. No GeoIP database is included in this repository. DB-IP City Lite is the default provider and can be downloaded monthly; MaxMind and IP2Location are supported as manually installed providers by default. Each database remains subject to its own license, terms, and attribution requirements. See the provider documentation: <https://db-ip.com/db/lite.php>, <https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/>, and <https://lite.ip2location.com/ip2location-lite>.

The interactive Public Analytics charts use Apache ECharts 6.1.0 under the Apache License 2.0. Release artifacts include a browser bundle generated from the ECharts source. See <https://echarts.apache.org/>.
