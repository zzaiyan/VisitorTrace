# Third-Party Notices

[Chinese](./THIRD_PARTY_NOTICES.zh-CN.md)

VisitorTrace includes a generated world basemap derived from Natural Earth 1:110m Admin 0 Countries vector data. The Antarctica feature is omitted for the compact visitor-map presentation.

- Source repository: <https://github.com/nvkelso/natural-earth-vector>
- Source commit: `ca96624a56bd078437bca8184e78163e5039ad19`
- Source file: `geojson/ne_110m_admin_0_countries.geojson`
- Source SHA-256: `6866c877d39cba9c357620878839b336d569f8c662d3cfab4cb1dbe2d39c977f`
- Generated files: `internal/maprender/assets/world.path`, `web/assets/world.geo.json`

Natural Earth vector and raster map data is in the public domain. See <https://www.naturalearthdata.com/about/terms-of-use/>.

The bundled city-name data in `internal/geoip/citynames_gen.go` and `internal/geoip/citynames.go` incorporates selected place names and alternate names from [GeoNames](https://www.geonames.org/). GeoNames data is licensed under [Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/). VisitorTrace curates these names into English display labels and applies its own conservative alias and administrative-suffix rules.

VisitorTrace supports user-supplied and automatically downloaded databases from DB-IP City Lite, MaxMind GeoLite2 City, IP2Location LITE DB11, and ip2region. It can also query Tencent Location Service, Amap, IPinfo, and BigDataCloud using credentials supplied by the operator. No GeoIP database or provider credential is included in this repository. Each source remains subject to its own license, terms, account requirements, usage limits, and attribution requirements. See <https://db-ip.com/db/lite.php>, <https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/>, <https://lite.ip2location.com/ip2location-lite>, <https://github.com/lionsoul2014/ip2region>, <https://lbs.qq.com/>, <https://lbs.amap.com/>, <https://ipinfo.io/>, and <https://www.bigdatacloud.com/>.

The interactive Public Analytics charts use Apache ECharts 6.1.0 under the Apache License 2.0. Release artifacts include a browser bundle generated from the ECharts source. See <https://echarts.apache.org/>.
