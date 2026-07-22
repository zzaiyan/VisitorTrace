# 访迹（VisitorTrace）部署指南

[英文版](./deployment.md)

本指南将一个 VisitorTrace 进程部署在 HTTPS 反向代理之后。应用仅监听本机回环地址，反向代理是唯一的公网入口。

## 前置条件

- 使用 AMD64 或 ARM64 的 64 位 Linux 服务器；
- 已将 `stats.example.com` 等域名解析到服务器；
- 安装 Nginx 或其他反向代理，并配置有效的 HTTPS 证书；
- 首次安装时具备 root 权限。

公网只需开放 80 和 443 端口，不要把 VisitorTrace 的监听端口暴露到互联网。

## 安装与初始化

设置版本和架构，然后下载对应二进制及校验文件：

```sh
VERSION=0.1.0
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

默认配置监听 `127.0.0.1:8790`，把 SQLite 和 GeoIP 数据保存在 `/var/lib/visitortrace`，并在启动时下载当月 DB-IP City Lite。配置文件权限应保持为 `0600`。

接入反向代理前，在 `/etc/visitortrace/config.json` 中把本机回环地址加入 `trusted_proxies`：

```json
"trusted_proxies": ["127.0.0.1/32", "::1/128"]
```

只应添加真实可信的代理地址；该设置决定服务是否接受代理提供的客户端 IP 和 HTTPS 协议头。

初始化一键自更新使用的稳定执行路径：

```sh
sudo -u visitortrace /usr/local/bin/visitortrace update bootstrap \
  --config /etc/visitortrace/config.json
```

进程管理器必须运行 `/var/lib/visitortrace/releases/current/visitortrace`，并在进程正常退出后也重新启动；验签通过的自更新切换稳定链接后会正常退出。

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
ReadWritePaths=/var/lib/visitortrace

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

## Nginx 反向代理

由 Nginx 终止 HTTPS，并把完整域名代理到 VisitorTrace：

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

不要为采集、后台、健康检查或分析路由开启代理缓存。浏览器静态资源和 SVG 响应已经自行发送缓存头。

重新加载 Nginx 后检查：

```sh
curl -fsS http://127.0.0.1:8790/health/live
curl -fsS https://stats.example.com/health/live
curl -fsS https://stats.example.com/health/ready
```

首次 GeoIP 下载完成前，ready 检查可能暂时不可用；若持续失败，应查看服务日志。

## 宝塔部署

宝塔只是可选的进程管理和反向代理界面。VisitorTrace 不调用宝塔 API，仍使用与 systemd 场景相同的二进制、配置、数据、健康检查和更新约定。

### 进程管理

先按前述通用步骤安装并初始化 VisitorTrace。在 Go 项目管理器中创建项目，并填写与下表等价的内容：

| 设置 | 值 |
|---|---|
| 项目目录 | `/var/lib/visitortrace` |
| 可执行文件 | `/var/lib/visitortrace/releases/current/visitortrace` |
| 参数或启动命令 | `serve --config /etc/visitortrace/config.json` |
| 监听端口 | `8790` |
| 运行用户 | `visitortrace` |
| 重启策略 | 进程每次退出后都重新启动 |

不同面板版本对“可执行文件”和“参数”的字段命名可能不同，最终操作系统命令必须等价于：

```sh
/var/lib/visitortrace/releases/current/visitortrace serve --config /etc/visitortrace/config.json
```

如果管理器不能运行已有二进制，或不能在状态码 0 时重新启动，请改用前面的 systemd 单元，只让宝塔管理 Nginx、TLS 和日志。不要同时使用两个管理器守护同一进程。

若面板强制使用自己的进程账户，应把 `/var/lib/visitortrace` 的所有权授予该账户，并允许其读取 `/etc/visitortrace/config.json`；不要把配置文件改成所有用户可读。

### 网站与反向代理

1. 为 `stats.example.com` 创建网站并签发 SSL 证书；
2. 进入网站设置中的“反向代理”，添加代理目录 `/`；
3. 目标 URL 设置为 `http://127.0.0.1:8790`；
4. 保留原始 Host、关闭代理缓存，内容替换留空；
5. 检查生成的 Nginx 配置是否传递 `X-Forwarded-For` 和 `X-Forwarded-Proto`。

当前宝塔导航和反向代理字段可参考[宝塔官方反向代理文档](https://docs.bt.cn/user-guide/site/php/site-config/reverse-proxy)。

在“计划任务”中创建每日 Shell 任务：

```sh
sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace backup \
  --config /etc/visitortrace/config.json
```

systemd timer 和宝塔计划任务二选一，不要重复执行。

## 部署后操作

1. 访问 `https://stats.example.com/admin/login` 登录后台；
2. 创建 Site，并填写网站准确的 Origin；
3. 设置时区、保留期、访客合并周期和地图预设；
4. 从 Site 页面安装一体式 Widget 或分离式 Tracker；
5. 手动创建一次备份，并确认运行状态页能够显示该备份。

持续限制 `/etc/visitortrace`、`/var/lib/visitortrace` 和备份目录的访问权限。发布签名私钥应单独备份，绝不能放在应用服务器上。
