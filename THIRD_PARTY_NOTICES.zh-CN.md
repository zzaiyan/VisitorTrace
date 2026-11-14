# 第三方声明

[英文版](./THIRD_PARTY_NOTICES.md)

VisitorTrace 世界底图使用社区数据集 `@surbowl/world-geo-json-zh` 2.1.5，采用 The Unlicense。原始数据和许可文本保存在 `tools/basemap/`，见[来源与重建说明](tools/basemap/README.md)。该数据基于 Natural Earth 1:50m v5.1.1，作者说明参照自然资源部标准地图和 DataV/高德调整了中国相关边界及岛屿；台湾包含在中国几何中，并包含钓鱼岛、赤尾屿、南海诸岛和十段线多边形。

`internal/maprender/assets/world.path`、`internal/maprender/assets/world-boundaries.path` 与 `web/assets/world.geo.json` 从同一锁定数据生成，不再二次简化几何；现有显示范围不包含南极洲。上游引用的审图号属于其参考地图，不代表社区数据集或 VisitorTrace 最终绘制地图已获审核。底层 Natural Earth 数据属于公有领域，参见 <https://www.naturalearthdata.com/about/terms-of-use/>。

`internal/geoip/place_names.tsv.gz` 中生成的地名数据采用了 [GeoNames](https://www.geonames.org/) 的地名及别名，并包含由 [world_countries](https://github.com/stefangabos/world_countries) 派生的行政区代码。GeoNames 数据采用 [知识共享署名 4.0 许可](https://creativecommons.org/licenses/by/4.0/)；world_countries 行政区数据采用 [知识共享署名-相同方式共享 4.0 许可](https://creativecommons.org/licenses/by-sa/4.0/)。VisitorTrace 将其压缩并转换为英文展示名称、行政区映射和别名规则。生成输入捕获于 2026-10-08：GeoNames SHA-256 分别为 `bd51fc33c0847de2079e2cb7b32a5f1bd32bd21ae064757fbc4ac35986f11922`、`9e7f4ec73433b17aa6344a00031461ac475f891200aab9f2559f8e2ccaa4a449`、`1da92a6323a5fec3176f3f743bf4cf4040fd56a876da55e46fbca23c863aa60a`；world_countries 提交为 `7edbb1e30ddc9b616ee07f76a3cad3af2416b618`。

VisitorTrace 支持用户自行提供或自动下载 DB-IP City Lite、MaxMind GeoLite2 City、IP2Location LITE DB11 和 ip2region 数据库，也可使用运营者提供的凭证查询腾讯位置服务、高德、IPinfo 和 BigDataCloud。仓库不包含任何 GeoIP 数据库或供应商凭据。每个数据源仍分别适用其自身的许可证、使用条款、账户要求、调用限制和归因要求。供应商信息见：<https://db-ip.com/db/lite.php>、<https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/>、<https://lite.ip2location.com/ip2location-lite>、<https://github.com/lionsoul2014/ip2region>、<https://lbs.qq.com/>、<https://lbs.amap.com/>、<https://ipinfo.io/> 和 <https://www.bigdatacloud.com/>。

公开分析页的交互式图表使用 Apache ECharts 6.1.0，采用 Apache License 2.0。发布包中包含由 ECharts 源码生成的浏览器 bundle。详见 <https://echarts.apache.org/>。

底图生成使用 MIT 许可的 [polygon-clipping](https://github.com/mfogel/polygon-clipping) 拼合同一国家在日期变更线处被切开的多边形。该依赖只在构建时运行，不包含在浏览器脚本中。
