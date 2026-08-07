# Aowugong Go

`aowugong-go` 是 Aowugong 工作台的 Go 模块化单体。Go 进程提供 HTTP API、React 静态资源、RBAC、定时任务和微信通知；生产业务数据保存在单个 SQLite 文件中。

## 技术栈

- 后端：Go、`net/http`、chi、`database/sql`、`modernc.org/sqlite`、Goose、`log/slog`
- 前端：React、TypeScript、Vite、shadcn/ui、Tailwind CSS
- 任务：`robfig/cron/v3`，固定时区 `Asia/Shanghai`
- 认证：bcrypt、JWT，登录有效期 72 小时
- 外部服务：DeepSeek、Tushare、WeChatRSS、OpeniLink Hub、微信读书、阿里云 OCR
- 生产：Linux amd64 发布产物、systemd，无需 Go、Node、Python、MySQL 或 Docker

## 目录

```text
cmd/aowugong/             HTTP 服务和统一任务 CLI
cmd/migrate/              MySQL 到 SQLite 一次性迁移工具
internal/app/             依赖组装、启动和优雅停止
internal/config/          环境变量和配置校验
internal/database/        SQLite 连接、Goose 迁移和安全快照
internal/databaseview/    管理员只读数据库查询
internal/httpserver/      API、认证中间件、开发代理和 React 静态资源
internal/scheduler/       任务注册、Cron、执行包装和 SQLite 锁
internal/client/          外部 HTTP/API 客户端
internal/finance/         数据、回测、仓位、文章分析和任务
internal/{auth,rbac,work,weread,mahjong,subscription,monitoring}/
internal/testdatabase/    隔离 SQLite 测试夹具
web/                      React 前端
migrations/sqlite/        版本化 SQLite SQL
configs/.env.example      唯一配置模板
init/systemd/             systemd 服务模板
scripts/                  构建、发布、部署、切换和回滚
storage/                  本地运行数据，生产使用 shared/storage
```

依赖方向固定为 `handler -> service -> repository/client`。页面手动任务、Cron 和 CLI 都调用同一个 service 与 `scheduler.Registry.Run`。

## SQLite

生产文件固定为：

```text
/opt/aowugong-go/shared/storage/data/aowugong.db
```

每条连接启用：

```text
journal_mode=WAL
foreign_keys=ON
busy_timeout=5000
synchronous=NORMAL
```

连接池默认最多 4 条连接。任务注册表使用 `job_execution_lock` 防止跨进程重复执行；写入仍由 SQLite 串行化。

管理员可访问 `/database`：

- 查看数据库大小、表、字段、行数和分页数据
- 搜索当前表
- 流式导出当前筛选的 CSV，单次最多 100,000 行
- 密码、令牌和密钥字段始终隐藏
- 不接受任意 SQL，不提供新增、修改或删除

## 本地开发

先构建前端：

```powershell
cd web
npm ci
npm test
npm run build
cd ..
```

本地 `2345` 使用当前前端，`/api` 直接代理线上 Go：

```powershell
.\scripts\run-local.ps1 -GoCommand C:\howiedata\tools\go1.26.5\bin\go.exe
```

访问 `http://127.0.0.1:2345`。本地不会创建 SQLite 副本；页面读取和业务操作使用线上同一份数据。生产环境禁止设置 `AOWUGONG_DEV_UPSTREAM_URL`。

补跑线上任务：

```powershell
.\scripts\run-remote-job.ps1 -JobName sync_investment_articles
```

脚本通过 SSH 在服务器加载正式环境后调用统一任务入口，不保存 SSH 密码。SQLite 不能通过网络文件系统直接挂载；需要直接核查或修复时，通过 SSH 在服务器执行，并在变更前创建快照。

## 定时任务

| 时间 | 任务 | 说明 |
|---|---|---|
| 09:00 | `test_crontab` | 每日任务链路测试 |
| 仅手动 | `sync_investment_articles` | 同步并分析投资文章；等待新文章源后恢复定时 |
| 22:00 | `check_service_monitors` | 服务连通性检查 |
| 09:30 | `check_subscription_expiry_notify` | 订阅到期提醒 |
| 10:00 | `openilink_reply_reminder` | OpeniLink 回复提醒 |
| 03:30 | `backup_sqlite` | SQLite 一致性快照 |

仅手动任务：

| 任务 | 说明 |
|---|---|
| `update_tushare_daily_data` | 按需恢复 Tushare 个股日线同步 |
| `rebuild_investment_signal_groups` | 全局重建投资信号概念词典 |

统一任务包装器负责进程内和跨进程防并发、超时、panic 恢复、耗时日志、结果入库和失败微信通知。失败通知保持“任务、时间、状态、信息”四段格式。

`backup_sqlite` 使用 `VACUUM INTO` 创建一致性快照，执行 `PRAGMA quick_check` 后原子发布，默认保留最近 7 份。

## 数据迁移

一次性迁移工具读取旧 MySQL，创建并重写 SQLite：

```powershell
go run ./cmd/migrate --confirm
```

需要配置 `AOWUGONG_MYSQL_HOST`、`AOWUGONG_MYSQL_PORT`、`AOWUGONG_MYSQL_DATABASE`、`AOWUGONG_MYSQL_USER` 和 `AOWUGONG_MYSQL_PASSWORD`。MySQL 仅是迁移来源，不是正式运行依赖。

迁移规则：

- 迁移当前全部业务表、权限、文章、仓位、任务和通知数据
- 保留空 `tushare_daily` 表，但不迁移历史个股日线
- 迁移后逐表核对行数、关键字段首尾完整样本和日期范围
- 提交前执行 `PRAGMA integrity_check` 和 `PRAGMA foreign_key_check`
- 任一核对失败即回滚 SQLite 数据事务

canary 和正式停写迁移都会把 JSON 核验报告保存到 `/opt/aowugong-go/shared/storage/backup`，文件权限为 `0600`。

MySQL 旧数据和 FastAPI 服务配置保留用于回滚。完成页面和数据验收、确认没有其他项目使用 MySQL 后，可以停止 MySQL，但不删除旧数据。

## 配置

从 `configs/.env.example` 创建真实 `.env`，不得提交密钥。

| 配置 | 说明 |
|---|---|
| `AOWUGONG_ENV` | `development` 或 `production` |
| `AOWUGONG_HTTP_ADDRESS` | HTTP 监听地址，生产正式端口为 `0.0.0.0:2345` |
| `AOWUGONG_SQLITE_PATH` | SQLite 文件路径 |
| `AOWUGONG_SQLITE_*` | 连接池、锁等待和迁移开关 |
| `AOWUGONG_DEV_UPSTREAM_URL` | 本地页面复用线上 API，生产必须为空 |
| `AOWUGONG_MIGRATIONS_DIR` | SQLite 迁移目录 |
| `AOWUGONG_JWT_SECRET` | 生产 JWT 密钥 |
| `AOWUGONG_ENCRYPTION_KEY` | 生产加密密钥 |
| `AOWUGONG_BACKUP_*` | SQLite 快照目录和保留数 |
| `AOWUGONG_SCHEDULER_ENABLED` | 是否启动内嵌 Cron |
| `FINANCE_ENABLE_REAL_TRADE` | 真实交易总开关，默认 `false` |
| `AOWUGONG_MYSQL_*` | 仅一次性迁移旧数据时使用 |

其余 DeepSeek、Tushare、OpeniLink、微信读书、WeChatRSS 和 OCR 配置见 `.env.example`。

## 验证

```powershell
$env:GOCACHE="$PWD\.cache\go-build"
go test -buildvcs=false ./...
go test -buildvcs=false -race ./...
go vet -buildvcs=false ./...

cd web
npm test
npm run build
```

测试使用临时 SQLite 文件，不访问生产数据。

## 构建发布

本地构建 Linux amd64 发布包：

```powershell
.\scripts\build-release.ps1 -Version v1.0.0
```

发布包包含：

```text
aowugong
aowugong-migrate
web/dist
migrations/sqlite
configs
init
scripts
README.md
VERSION
```

版本 tag 的 GitHub Actions 执行前端测试/构建、Go test/race/vet、Linux amd64 编译并发布压缩包和 SHA-256 文件。

## 部署切换

首次 SQLite 并行部署：

```bash
DEPLOY_MODE=canary MIGRATE_FROM_MYSQL=true APP_PORT=2346 SCHEDULER_ENABLED=false \
  ./scripts/bootstrap-release.sh v1.0.0
```

部署器只下载发布产物。`canary` 使用独立的 `aowugong-go-canary` 服务和环境文件，不修改当前 `aowugong-go` 2345；首次切换必须显式设置 `MIGRATE_FROM_MYSQL=true`，迁移和核对成功后才启动 2346。

页面和数据验收通过后正式切换：

```bash
sudo /opt/aowugong-go/canary/scripts/cutover.sh
```

切换脚本检查 2346，停止旧 Go、FastAPI、旧 crontab 和 canary 写入，再从 MySQL 完整迁移一次，补齐并行验收期间的新数据；随后提升 canary、启用内嵌调度并检查 2345。失败会恢复原环境、链接、服务和 crontab。

完成首次 SQLite 切换后的普通版本发布：

```bash
DEPLOY_MODE=main APP_PORT=2345 SCHEDULER_ENABLED=true \
  ./scripts/bootstrap-release.sh v1.0.1
```

发布和切换不会修改 WeChatRSS 5000、OpeniLink Hub 9800、SSH、防火墙或安全组。

## 回滚

回滚到上一 SQLite 发布版本：

```bash
sudo /opt/aowugong-go/current/scripts/rollback.sh release
```

首次切换后恢复原 MySQL Go：

```bash
sudo CUTOVER_ENV_BACKUP=/opt/aowugong-go/shared/storage/backup/sqlite-cutover-时间戳.env \
  /opt/aowugong-go/current/scripts/rollback.sh mysql-go
```

恢复 FastAPI：

```bash
sudo CRONTAB_BACKUP=/opt/aowugong-go/shared/storage/backup/sqlite-cutover-时间戳.fastapi-crontab \
  /opt/aowugong-go/current/scripts/rollback.sh fastapi
```

恢复 MySQL Go 或 FastAPI 前必须确保旧 MySQL 已启动。回滚只适合切换后短时间应急；SQLite 已产生新写入后，回到旧库前必须先核对并补回差异数据。
