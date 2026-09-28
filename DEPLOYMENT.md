# Deployment Plan

V1 的部署目标是：GitHub Actions 预先编译 Docker 镜像，任意一台普通 Linux 服务器只负责拉取镜像 + Docker Compose 运行。

## 目标服务器要求

- Linux x86_64
- Docker Engine
- Docker Compose v2
- 一个域名，例如 `chat.example.com`
- HTTPS 证书，可以由 Caddy/Traefik/Nginx 自动签发或外部反代提供
- Postal SMTP 账号

## 计划中的部署形态

```text
server
  reverse proxy / TLS
  support-api       prebuilt GHCR image
  support-worker    same prebuilt GHCR image, different entrypoint
  mysql             MySQL 8
  redis             Redis
  uploads volume    local image storage
```

默认镜像：

```text
ghcr.io/jimo008/chat-v0:latest
```

仓库为私有时，服务器需要先登录 GHCR：

```bash
echo 'YOUR_GITHUB_TOKEN' | docker login ghcr.io -u jimo008 --password-stdin
```

Token 至少需要能读取该私有包。如果镜像包设为公开，则不需要登录。

## 计划中的命令

后续会提供一个 `supportctl` 交互式命令。

```bash
./supportctl install
./supportctl init-agent
./supportctl site create
./supportctl site list
./supportctl site code
./supportctl status
./supportctl backup
```

## Site 创建

V1 不做 Web 管理后台。创建 Site 使用交互式命令：

```bash
./supportctl site create
```

输入网站名称后，系统生成：

```html
<script src="https://chat.example.com/widget.js" data-site="1001"></script>
```

## 配置项

实际代码阶段会提供 `.env.example`，至少包含：

```env
APP_BASE_URL=https://chat.example.com
MYSQL_DSN=...
REDIS_ADDR=redis:6379
POSTAL_SMTP_HOST=...
POSTAL_SMTP_PORT=587
POSTAL_SMTP_USER=...
POSTAL_SMTP_PASSWORD=...
UPLOAD_STORAGE_PATH=/data/support-chat/uploads
EMAIL_UNREAD_DELAY_SECONDS=300
AGENT_SYNC_INTERVAL_SECONDS=3
EMERGENCY_DEVICE_ALIVE_SECONDS=60
EMERGENCY_EXPIRE_SECONDS=180
TZ=Asia/Shanghai
```

## 首次部署顺序

```bash
git clone https://github.com/jimo008/chat-v0.git
cd chat-v0
chmod +x supportctl
./supportctl install
# edit .env
./supportctl up
./supportctl status
./supportctl init-agent
./supportctl site create
```

`./supportctl up` 会执行 `docker compose pull support-api support-worker`，再启动服务；不会在服务器上编译 Go。只有开发调试时才使用：

```bash
./supportctl build-local
```

部署代码示例：

```html
<script src="https://chat.example.com/widget.js" data-site="st_xxxxxxxx"></script>
```

## 部署原则

- 不把密钥提交到 GitHub。
- 数据库、上传文件、日志使用 Docker volume 或宿主机目录持久化。
- `RETENTION_CLEANUP_ENABLED=false` 为默认值；确认备份和保留策略后再改为 `true`。
- 重要状态以 MySQL 为事实来源，Redis 只做实时状态、缓存、队列和短期数据。
- Postal 失败不能阻塞聊天 API，由 Worker 异步重试。
