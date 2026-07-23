# 访迹（VisitorTrace）部署指南

[英文版](./deployment.md)

本指南只使用 systemd 和专用服务账户运行 VisitorTrace；宝塔仅管理域名、SSL 证书和 Nginx 反向代理。应用监听本机回环地址，Nginx 是唯一的公网入口。

## 前置条件

- 使用 AMD64 或 ARM64 的 64 位 Linux 服务器；
- 已将 `stats.example.com` 等域名解析到服务器；
- 宝塔已安装 Nginx；
- 首次安装时具备 root 权限。

公网只需开放 80 和 443 端口，不要把 VisitorTrace 的监听端口暴露到互联网。

## 安装与初始化

设置版本和架构，然后下载对应二进制及校验文件：

```sh
VERSION=0.1.2
ARCH=amd64
curl -fLO "https://github.com/zzaiyan/VisitorTrace/releases/download/v${VERSION}/visitortrace-${VERSION}-linux-${ARCH}"
curl -fLO "https://github.com/zzaiyan/VisitorTrace/releases/download/v${VERSION}/checksums.txt"
grep " visitortrace-${VERSION}-linux-${ARCH}$" checksums.txt | sha256sum -c -
```

创建专用服务账户和受保护目录：

```sh
sudo useradd --system \
  --home-dir /var/lib/visitortrace \
  --create-home \
  --shell /usr/sbin/nologin \
  visitortrace
sudo install -Dm755 "visitortrace-${VERSION}-linux-${ARCH}" /usr/local/bin/visitortrace
sudo install -d -m700 -o visitortrace -g visitortrace /etc/visitortrace /var/lib/visitortrace
```

初始化数据库，并根据提示输入管理员密码：

```sh
sudo -u visitortrace /usr/local/bin/visitortrace init \
  --data-dir /var/lib/visitortrace \
  --config /etc/visitortrace/config.json
```

默认配置监听 `127.0.0.1:8790`，把 SQLite 和 GeoIP 数据保存在 `/var/lib/visitortrace`，并使用 DB-IP City Lite 作为自动更新后端。MaxMind GeoLite2 City 与 IP2Location LITE DB11 具备同等的自动更新支持，但需要账户凭据。可在初始化命令中加入 `--geoip-provider maxmind --maxmind-account-id ACCOUNT_ID --maxmind-license-key LICENSE_KEY`，或加入 `--geoip-provider ip2location --ip2location-token DOWNLOAD_TOKEN`。配置文件权限应保持为 `0600`；备份包含这些凭据，需要采用同等的访问控制。

安装完成后，可以在“管理员设置 > GeoIP 数据库”修改后端、凭证、下载源和更新策略。表单会原子更新同一份受保护配置并请求由 systemd 拉起服务，因此 systemd 单元需要保留后文所示的 `Restart=always`，并允许写入 `/etc/visitortrace`。

接入反向代理前，在 `/etc/visitortrace/config.json` 中把本机回环地址加入 `trusted_proxies`：

```json
"trusted_proxies": ["127.0.0.1/32", "::1/128"]
```

只应添加真实可信的代理地址；该设置决定服务是否接受代理提供的客户端 IP 和 HTTPS 协议头。

### 一键配置 systemd

如果已经将发布二进制放置到服务器，可以使用仓库中的一键脚本：

```sh
sudo ./scripts/install-systemd.sh --binary /usr/local/bin/visitortrace
```

脚本会创建或复用专用服务账户，创建受保护目录；在配置不存在时执行 `init`；初始化自更新稳定路径；写入 systemd 单元并启动服务。初始化时会提示输入管理员密码。脚本不会下载二进制或 GeoIP 文件，不会创建备份，也不会配置反向代理或宝塔。需要逐项检查命令时，再使用下面的手动步骤。

### Base URL 与子路径部署

可选配置项 `base_url` 用于生成接入代码和公开链接，同时启用子路径路由。例如：

```json
"base_url": "https://stats.example.com/visitortrace"
```

该值必须是没有凭据、查询参数和片段的完整 HTTP 或 HTTPS URL。根路径部署时可以留空。也可以在后台“管理员设置 > 公开 Base URL”中设置；保存后会写入受保护的配置文件并请求服务重启。要让新的路由前缀生效，systemd 必须使用 `Restart=always`。

初始化一键自更新使用的稳定执行路径：

```sh
sudo -u visitortrace /usr/local/bin/visitortrace update bootstrap \
  --config /etc/visitortrace/config.json
```

进程管理器必须运行 `/var/lib/visitortrace/releases/current/visitortrace`，并在进程正常退出后也重新启动；验签通过的自更新切换稳定链接后会正常退出。

### 使用本地二进制手动更新

如果新版二进制和 `checksums.txt` 已经位于服务器，可以在不下载文件的情况下执行更新：

```sh
sudo ./scripts/update-systemd-binary.sh \
  --binary ./visitortrace-0.1.2-linux-amd64 \
  --checksum-file ./checksums.txt
```

脚本要求已有 `releases/current` 稳定路径。它会在提供校验文件时验证二进制，运行候选版本的 `doctor --upgrade-check`，创建带校验的升级前备份，原子切换稳定链接并重启 systemd 服务。若新版本启动失败，会恢复旧版本。手动二进制更新只适用于数据库 Schema 不变的版本；改变 Schema 时应使用签名更新器。脚本不会下载文件，也不会配置反向代理或宝塔；除升级前安全快照外，备份策略也不由它负责，代理和 SSL 配置保持不变。

## systemd 部署

创建 `/etc/systemd/system/visitortrace.service`：

```ini
[Unit]
Description=VisitorTrace visitor analytics
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=visitortrace
Group=visitortrace
WorkingDirectory=/var/lib/visitortrace
ExecStart=/var/lib/visitortrace/releases/current/visitortrace serve --config /etc/visitortrace/config.json
Restart=always
RestartSec=3s
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=/var/lib/visitortrace /etc/visitortrace

[Install]
WantedBy=multi-user.target
```

加载并启动服务：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now visitortrace
sudo systemctl status visitortrace
sudo journalctl -u visitortrace -f
```

这里必须使用 `Restart=always`：后台发起的自更新会以状态码 0 退出，但仍需要 systemd 启动新版本。执行 `systemctl stop visitortrace` 主动停止时不会被重新拉起。

### 每日备份

VisitorTrace 可以按需创建带校验的本地备份。需要每日执行时，创建 `/etc/systemd/system/visitortrace-backup.service`：

```ini
[Unit]
Description=Back up VisitorTrace

[Service]
Type=oneshot
User=visitortrace
Group=visitortrace
UMask=0077
ExecStart=/var/lib/visitortrace/releases/current/visitortrace backup --config /etc/visitortrace/config.json
```

创建 `/etc/systemd/system/visitortrace-backup.timer`：

```ini
[Unit]
Description=Daily VisitorTrace backup

[Timer]
OnCalendar=daily
Persistent=true
RandomizedDelaySec=30m

[Install]
WantedBy=timers.target
```

启用定时器：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now visitortrace-backup.timer
```

## 宝塔：HTTPS 与反向代理

不要在宝塔 Go 项目管理器中创建 VisitorTrace 项目。部分宝塔版本只允许选择 `www`、`root` 等预设账户，这些账户都不应直接运行 VisitorTrace。应用只由 systemd 托管，因此 `visitortrace` 无需出现在宝塔的用户下拉框中；也不要让 systemd 和宝塔同时启动同一服务。

### 网站与 SSL

1. 把 `stats.example.com` 解析到服务器，并在宝塔中创建该网站；
2. 网站不需要 PHP 或 Go 运行环境，也不要把 VisitorTrace 数据放进网站根目录；
3. 进入网站的“SSL”设置，申请 Let's Encrypt 证书或安装已有证书；
4. 开启 HTTPS，并按需设置 HTTP 自动跳转 HTTPS。

### 反向代理

根路径部署时，进入网站设置中的“反向代理”，添加一条与下表等价的规则：

| 设置 | 值 |
|---|---|
| 代理目录 | `/` |
| 目标 URL | `http://127.0.0.1:8790` |
| Host | 保留原始 Host |
| 缓存 | 关闭 |
| 内容替换 | 留空 |

不同宝塔版本的字段名称可能不同。检查生成的 Nginx 配置，确认最终生效的 location 包含以下请求头：

```nginx
location / {
    proxy_pass http://127.0.0.1:8790;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

`X-Forwarded-For` 用于传递真实访客 IP，`X-Forwarded-Proto` 用于让后台在 HTTPS 反向代理后正确设置安全 Cookie。VisitorTrace 只接受 `trusted_proxies` 中回环地址提供的这些请求头。不要为采集、后台、健康检查或分析路由开启代理缓存；静态资源和 SVG 响应已经自行发送缓存头。

如果使用 `/visitortrace` 这样的子路径，需要在后台设置相同的 Base URL，并让 Nginx 保留此前缀。`proxy_pass` 末尾不要加斜杠：

```nginx
location = /visitortrace {
    return 308 /visitortrace/;
}

location /visitortrace/ {
    proxy_pass http://127.0.0.1:8790;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

配置后，后台地址为 `https://stats.example.com/visitortrace/admin/login`，健康检查为 `https://stats.example.com/visitortrace/health/live`，各 Site 页面生成的接入代码也会自动带上此前缀。不要把 `/visitortrace/` 代理到以 `/visitortrace/` 结尾的目标地址；VisitorTrace 自己负责此前缀的路由匹配。

当前宝塔导航和反向代理字段可参考[宝塔官方反向代理文档](https://docs.bt.cn/user-guide/site/php/site-config/reverse-proxy)。

## 验证与排障

分别检查应用本机端点和公网 HTTPS 端点：

```sh
curl -fsS http://127.0.0.1:8790/health/live
curl -fsS https://stats.example.com/health/live
curl -sS https://stats.example.com/health/ready
```

如果 `base_url` 包含 `/visitortrace`，公网检查应改为 `/visitortrace/health/live` 和 `/visitortrace/health/ready`；本机检查同样要带此前缀，因为应用自身负责此前缀路由。

两个 live 检查可以区分 systemd 与宝塔代理问题：若都返回 `{"status":"ok"}`，说明进程管理、Nginx、DNS 和 SSL 已经正常。完全就绪时返回：

```json
{"checks":{"geoip":true,"schema":true,"sqlite":true},"status":"ready"}
```

首次 GeoIP 下载可能因网络访问、后端凭据无效或供应商调整下载 code 而失败或暂时不可用，此时 ready 检查会返回 HTTP 503。使用不带 `-f` 的 `curl` 保留诊断 JSON，然后检查并重试 GeoIP：

```sh
sudo journalctl -u visitortrace -n 100 --no-pager
sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace doctor \
  --config /etc/visitortrace/config.json
sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace geoip update \
  --config /etc/visitortrace/config.json \
  --force
sudo systemctl restart visitortrace
```

命令行 GeoIP 更新运行在服务进程之外，因此手动更新成功后必须重启服务。若自动更新被关闭或不可用，应通过可信网络或镜像取得与当前后端匹配的有效 MMDB，以 `visitortrace` 所有者和 `0600` 权限放到配置的 `geoip_path`，然后重启服务。关闭自动更新并不能取消本地有效 MMDB 的要求。

根路径部署时，访问 `https://stats.example.com/` 会跳转到 `/admin`；后台入口是 `https://stats.example.com/admin/login`，公开 Site 使用 `/public/<SITE-ID>/analytics`。子路径部署时，在这些路径前加上已经设置的前缀即可。若希望裸域名直接跳转到子路径后台，可在代理规则旁添加精确匹配：

```nginx
location = / {
    return 302 /visitortrace/admin/login;
}
```

## 部署后操作

1. 访问 `https://stats.example.com/admin/login` 登录后台；
2. 创建 Site，并填写网站准确的 Origin；
3. 设置时区、保留期、访客合并周期和地图预设；
4. 从 Site 页面安装一体式 Widget 或分离式 Tracker；
5. 手动创建一次备份，并确认运行状态页能够显示该备份。

持续限制 `/etc/visitortrace`、`/var/lib/visitortrace` 和备份目录的访问权限。发布签名私钥应单独备份，绝不能放在应用服务器上。
