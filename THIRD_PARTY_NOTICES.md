# 第三方声明 / Third-Party Notices

## 中文

VisitorTrace 包含一个由 Natural Earth 1:110m Admin 0 Countries 矢量数据生成的世界底图。为了适应紧凑的访客地图展示，生成过程会排除南极洲要素。

- 源仓库：<https://github.com/nvkelso/natural-earth-vector>
- 源提交：`ca96624a56bd078437bca8184e78163e5039ad19`
- 源文件：`geojson/ne_110m_admin_0_countries.geojson`
- 源文件 SHA-256：`6866c877d39cba9c357620878839b336d569f8c662d3cfab4cb1dbe2d39c977f`
- 生成文件：`internal/maprender/assets/world.path`、`web/assets/world.geo.json`

Natural Earth 矢量和栅格地图数据属于公有领域，参见 <https://www.naturalearthdata.com/about/terms-of-use/>。

VisitorTrace 默认可下载 DB-IP IP to City Lite MMDB，但该数据库不包含在仓库中。DB-IP City Lite 每月更新，采用 Creative Commons Attribution 4.0 International License。使用该数据时必须保留指向 DB-IP 的归因链接。详见 <https://db-ip.com/db/download/ip-to-city-lite>。

公开分析页的交互式图表使用 Apache ECharts 6.1.0，采用 Apache License 2.0。发布包中包含由 ECharts 源码生成的浏览器 bundle。详见 <https://echarts.apache.org/>。

## English

VisitorTrace includes a generated world basemap derived from Natural Earth 1:110m Admin 0 Countries vector data. The Antarctica feature is omitted for the compact visitor-map presentation.

- Source repository: <https://github.com/nvkelso/natural-earth-vector>
- Source commit: `ca96624a56bd078437bca8184e78163e5039ad19`
- Source file: `geojson/ne_110m_admin_0_countries.geojson`
- Source SHA-256: `6866c877d39cba9c357620878839b336d569f8c662d3cfab4cb1dbe2d39c977f`
- Generated files: `internal/maprender/assets/world.path`, `web/assets/world.geo.json`

Natural Earth vector and raster map data is in the public domain. See <https://www.naturalearthdata.com/about/terms-of-use/>.

VisitorTrace can download the DB-IP IP to City Lite MMDB by default; the database is not included in this repository. DB-IP City Lite is updated monthly and licensed under the Creative Commons Attribution 4.0 International License. Keep the link attribution to DB-IP when using this data. See <https://db-ip.com/db/download/ip-to-city-lite>.

The interactive Public Analytics charts use Apache ECharts 6.1.0 under the Apache License 2.0. Release artifacts include a browser bundle generated from the ECharts source. See <https://echarts.apache.org/>.
