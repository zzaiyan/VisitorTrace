# 第三方声明

[英文版](./THIRD_PARTY_NOTICES.md)

VisitorTrace 包含一个由 Natural Earth 1:110m Admin 0 Countries 矢量数据生成的世界底图。为了适应紧凑的访客地图展示，生成过程会排除南极洲要素。

- 源仓库：<https://github.com/nvkelso/natural-earth-vector>
- 源提交：`ca96624a56bd078437bca8184e78163e5039ad19`
- 源文件：`geojson/ne_110m_admin_0_countries.geojson`
- 源文件 SHA-256：`6866c877d39cba9c357620878839b336d569f8c662d3cfab4cb1dbe2d39c977f`
- 生成文件：`internal/maprender/assets/world.path`、`web/assets/world.geo.json`

Natural Earth 矢量和栅格地图数据属于公有领域，参见 <https://www.naturalearthdata.com/about/terms-of-use/>。

VisitorTrace 支持用户自行提供或自动下载 DB-IP City Lite、MaxMind GeoLite2 City 和 IP2Location LITE DB11 的 MMDB 文件，仓库不包含任何 GeoIP 数据库或供应商凭据。每种数据库仍分别适用其自身的许可证、使用条款、账户要求、下载限制和归因要求。供应商文档见：<https://db-ip.com/db/lite.php>、<https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/> 和 <https://lite.ip2location.com/ip2location-lite>。

公开分析页的交互式图表使用 Apache ECharts 6.1.0，采用 Apache License 2.0。发布包中包含由 ECharts 源码生成的浏览器 bundle。详见 <https://echarts.apache.org/>。
