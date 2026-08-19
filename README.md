# Aowugong Go

`aowugong-go` 是 Aowugong 工作台的 Go 模块化单体。Go 进程提供 HTTP API、React 静态资源、RBAC、定时任务和微信通知，业务数据统一保存在 PostgreSQL。

## 技术栈

- 后端：Go、`net/http`、chi、`database/sql`、pgx、Goose、`log/slog`
- 前端：React、TypeScript、Vite、shadcn/ui、Tailwind CSS
- 数据库：PostgreSQL 15+，连接池默认 `8/4`，时区 `Asia/Shanghai`
- 调度：`robfig/cron/v3`，任务统一经过并发锁、超时、panic 恢复、日志、结果入库和失败微信通知
- 外部服务：微信读书、微信公众号原文、Miniflux、DeepSeek、Tushare、OpeniLink Hub、阿里云 OCR
- 生产：Linux amd64 发布产物、systemd；Go 服务无需 Go、Node、Python 或 MySQL，公网 TLS 复用 Vaultwarden 的 Caddy 容器与 IP 证书

## 目录

```text
cmd/aowugong/             正式服务和统一任务 CLI
cmd/migrate/              SQLite 到 PostgreSQL 一次性迁移工具
internal/app/             依赖组装、启动和优雅停止
internal/config/          环境变量和配置校验
internal/database/        PostgreSQL 连接、迁移和 pg_dump 备份
internal/databaseview/    管理员只读数据库页面
internal/httpserver/      路由、中间件和 React 静态资源
internal/vpn/             私有 VPN 资源转换、用户 Token 和 Go 直连分发
internal/scheduler/       任务注册、Cron、执行包装和数据库锁
internal/finance/         行情、仓位、文章分析、回测和任务
internal/testdatabase/    隔离 SQLite 测试夹具，不进入生产运行路径
web/                      React 前端
migrations/postgres/      正式 PostgreSQL 版本化迁移
migrations/sqlite/        仅供旧数据迁移和隔离测试
configs/.env.example      唯一配置模板
init/systemd/             正式和 canary systemd 模板
scripts/                  本地运行、构建、发布、远程任务和回滚
storage/                  本地运行目录；生产使用 shared/storage
```

业务依赖固定为 `handler -> service -> repository/client`。定时任务、页面手动执行和 CLI 补跑共用同一 service 与 `scheduler.Registry.Run`；回测引擎不访问数据库和外部接口。

## PostgreSQL

正式服务使用：

```env
AOWUGONG_DATABASE_URL=postgres://aowugong:密码@127.0.0.1:5432/aowugong?sslmode=disable
AOWUGONG_DATABASE_MAX_OPEN_CONNS=8
AOWUGONG_DATABASE_MAX_IDLE_CONNS=4
AOWUGONG_DATABASE_CONN_MAX_LIFETIME_MINUTES=30
AOWUGONG_DATABASE_SKIP_MIGRATIONS=false
```

启动时自动执行 `migrations/postgres`。连接自动设置上海时区，并把仓储中的标准 `?` 参数占位符统一转换成 PostgreSQL `$1...`。

每日 `03:30` 使用 `pg_dump --format=custom` 创建一致性备份，并用 `pg_restore --list` 校验后原子发布；默认保留最近 7 份。恢复示例：

```bash
createdb -U postgres aowugong_restore
pg_restore --no-owner --no-privileges -d aowugong_restore storage/backup/aowugong-时间.dump
```

## 本地运行

本地默认只启动前端和 Go 代理，业务请求通过 HTTPS 转发到线上 `2345`，因此看到的是线上 PostgreSQL 数据，不创建本地业务数据库，也不会启动定时任务：

```powershell
./scripts/run-local.ps1
```

访问 `http://127.0.0.1:2345`。停止本地进程：

```powershell
./scripts/stop-local.ps1
```

投资文章不再依赖外部 RSS 聚合接口。在“投资研究 > 投资文章抓取”中扫码绑定一个微信读书账号，服务会从书架发现公众号；人工启用的公众号由 08:00、20:00 任务各检查最近 20 篇，只为数据库未知文章读取详情和微信公众号原文。登录凭据使用 `AOWUGONG_ENCRYPTION_KEY` 派生的 AES-256-GCM 密钥加密后存入 PostgreSQL，二维码中间态只保存在当前 Go 进程内。Go 同时按公众号从已入库文章生成 `http://127.0.0.1:2345/feeds/weread/{account_id}.xml`，仅允许服务器回环地址访问，供同机 Miniflux 分源保存和阅读公众号全文。

“资源分享 > VPN 分配”仅管理员可见，用于给登录用户分配、重新发布、轮换或撤销 VPN 资源。“资源分享 > VPN 资源”只展示当前登录用户获配的资源，用户选择客户端格式后扫码即可配置。服务从 `storage/private/vpn` 读取被 Git 忽略的 Clash、Xray、Shadowrocket 和 Surge 文件，按文件名中的资源编码归组，并统一生成 Clash/FlClash、Shadowrocket、Surge、v2rayN/v2rayNG 四种订阅。分配时会自动加入 `VPN 用户` 角色；用户 Token 由 `AOWUGONG_ENCRYPTION_KEY`、订阅主键和版本通过 HMAC 派生，数据库不保存 Token 或节点正文。

需要补跑或修改线上数据时，通过 SSH 在服务器加载正式环境并调用统一 CLI：

```powershell
./scripts/run-remote-job.ps1 sync_investment_articles
```

## 定时任务

调度时区固定为 `Asia/Shanghai`。

| 时间 | 任务 | 说明 |
|---|---|---|
| 08:00、20:00 | `sync_investment_articles` | 从微信读书书架公众号增量抓取并分析投资文章 |
| 09:00 | `test_crontab` | 每日任务链路测试 |
| 22:00 | `check_service_monitors` | 服务连通性检查 |
| 09:30 | `check_subscription_expiry_notify` | 订阅到期提醒 |
| 10:00 | `openilink_reply_reminder` | OpeniLink 待回复提醒 |
| 03:30 | `backup_postgres` | PostgreSQL 一致性备份 |
| 周日 04:00 | `backup_github_code` | 启用后备份账号自有仓库及两个固定组织仓库 |
| 周日 05:00 | `email_vaultwarden_backup` | 加密最新 Vaultwarden 备份并发送到异地邮箱 |

以下任务保留为手动或 CLI 执行：`update_tushare_daily_data`、`rebuild_investment_signal_groups`、`check_weread_credential`。微信读书凭据状态以每日 `08:00/20:00` 的真实文章抓取为准，避免额外探测增加风控风险。

```bash
/opt/aowugong-go/current/aowugong job backup_postgres
/opt/aowugong-go/current/aowugong job backup_github_code
/opt/aowugong-go/current/aowugong job email_vaultwarden_backup
```

任务失败通知固定包含“任务、时间、状态、信息”，并统一通过 OpeniLink Hub 发送。

GitHub 代码备份默认关闭，通过 GitHub API 自动发现认证账号拥有的全部公有和私有仓库，再额外包含 `GITHUB_BACKUP_REQUIRED_REPOSITORIES` 中的组织仓库，当前固定为 `KES-IT/KES-SCM` 和 `KES-IT/KES-BIS`，不会枚举其他组织项目。每个项目保存为裸 Git 仓库，更新前的分支和标签保留在内部历史引用中；仓库失去权限或不再被发现时只记录状态，不删除最后副本。备份默认写入 `AOWUGONG_BACKUP_DIR/github`。

Vaultwarden 每日 `03:45` 创建 PostgreSQL、附件、Send 和签名密钥备份，默认保留 14 份；启用 `VAULTWARDEN_BACKUP_EMAIL_ENABLED` 后，Go 任务每周日 `05:00` 读取最新备份，使用 `age` 公钥加密，再把加密文件、`使用说明.md` 和全新服务器重建脚本打入单个恢复 ZIP 后发送邮件，临时文件发送后立即删除。服务器只保存公钥，解密私钥只保存在本地 `storage/private/backup/vaultwarden-age-key.txt`。

从邮件恢复 Vaultwarden：

```powershell
# 1. 解压邮件中的 recovery.zip，再在本地解密其中的 .tar.gz.age。
storage/private/backup/age.exe --decrypt -i storage/private/backup/vaultwarden-age-key.txt `
  -o vaultwarden-backup.tar.gz vaultwarden-时间.tar.gz.age
```

```bash
# 2. 在全新服务器安装 Docker Engine 和 PostgreSQL 15 后，上传明文备份和恢复包内的 scripts。
PUBLIC_IP=新服务器公网IP bash scripts/install-vaultwarden.sh
BACKUP_ARCHIVE=/root/vaultwarden-backup.tar.gz CONFIRM_DISASTER_RESTORE=YES \
  bash scripts/restore-vaultwarden-disaster.sh
```

解密只在本地执行，私钥不得上传服务器。灾难恢复默认面向空白新服务器；目标已有 Vaultwarden 时必须先停止并人工确认。

## 配置

复制并填写 `configs/.env.example`。生产必须配置：

- `AOWUGONG_DATABASE_URL`
- `AOWUGONG_JWT_SECRET`
- `AOWUGONG_ENCRYPTION_KEY`
- 实际启用客户端所需的 Token 或密钥

启用代码备份时还需配置 `GITHUB_BACKUP_ENABLED=true` 和具备账号全部仓库及两个固定组织仓库只读权限的 `GITHUB_BACKUP_TOKEN`。Token 只通过 Git 子进程环境使用，不写入仓库远端地址、清单或日志。

VPN 订阅直接由已开放的 Go 服务提供，确保设备在尚未连接代理时也能访问。生产 `.env` 配置服务器公开根地址：

```env
VPN_SOURCE_DIR=storage/private/vpn
VPN_PUBLIC_URL=https://8.138.123.59:2345
```

原始 VPN 文件需单独放入生产 `shared/storage/private/vpn`，不得加入 Git、发布包或日志。公开接口不提供资源列表，客户端 URL 使用每名登录用户独立的高强度 Token；管理员在“VPN 分配”管理订阅，`VPN 用户` 在“VPN 资源”中只读取自己的订阅并扫码。生产使用 Let’s Encrypt 公网 IP 短期证书，由 Caddy 在 `2345` 终止 TLS 并转发到仅监听 `127.0.0.1:12345` 的 Go 服务；同端口收到 HTTP 请求时自动跳转到 HTTPS。systemd 每日检查续期，证书变化后自动重启 Caddy。

`FINANCE_ENABLE_REAL_TRADE` 默认 `false`。`AOWUGONG_SQLITE_SOURCE_PATH` 只供一次迁移工具读取，正式服务不使用。

## 验证

```powershell
go test ./...
go test -race ./...
go vet ./...
cd web
npm ci
npm test
npm run build
```

测试使用临时 SQLite 夹具验证仓储契约；正式 `aowugong` 二进制不包含 SQLite 驱动。PostgreSQL 基线迁移和数据迁移还应在临时 PostgreSQL 数据库中执行一次。

## 构建与发布

本地构建 Linux amd64 发布包：

```powershell
./scripts/build-release.ps1 -Version v1.0.0
```

Git tag 会触发 GitHub Actions，执行前后端测试、Race、Vet 和构建，再发布包含二进制、`web/dist`、PostgreSQL migrations、配置模板和脚本的压缩包。

服务器安装正式版本：

```bash
sudo DEPLOY_MODE=main /opt/aowugong-go/current/scripts/deploy-release.sh v1.0.0
```

并行验收使用 `DEPLOY_MODE=canary` 和 `2346`，调度器会强制关闭。正式发布原子切换 `current`，并把上一版保存为 `previous`。

回滚发布产物：

```bash
sudo /opt/aowugong-go/current/scripts/rollback.sh
```

回滚只切换应用产物，不自动回滚 PostgreSQL schema 或业务数据。发布前应先确认 migration 向后兼容，并保留当日 `pg_dump` 备份。

## 一次性数据迁移

在维护窗口停止正式 Go 服务后，使用只读 SQLite 来源重建 PostgreSQL 业务表：

```bash
set -a
. /opt/aowugong-go/shared/.env
set +a
AOWUGONG_SQLITE_SOURCE_PATH=/安全路径/aowugong.db \
  /opt/aowugong-go/current/aowugong-migrate --confirm
```

迁移工具会执行 SQLite `quick_check`、PostgreSQL migrations、单事务复制、序列校准，并逐表核对行数及主键范围。核对成功后，正式服务只读取 PostgreSQL；旧 SQLite 仅保留一份离线归档，确认无误后可删除。
