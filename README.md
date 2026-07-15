# aowugong-go

`aowugong-go` 是替代原 FastAPI 服务的 Go 模块化单体。后端、内嵌定时任务、SQLite 数据库和 React 静态资源由一个 Go 进程统一运行；生产服务器只安装发布产物，不需要 Go、Node、Python、MySQL 客户端或 Docker。

开发与并行验收默认监听 `0.0.0.0:2346`，正式切换后监听 `0.0.0.0:2345`。健康检查为 `GET /api/v1/health`。

## 技术栈

- 后端：Go、`net/http`、chi、`database/sql`、modernc SQLite、`log/slog`。
- 认证：bcrypt、JWT，登录有效期固定 72 小时。
- 调度：`robfig/cron/v3`，时区固定 `Asia/Shanghai`。
- 外部客户端：Tushare HTTP、DeepSeek、RSS/WeChatRSS、OpeniLink Hub、微信读书、阿里云 OCR。
- 前端：React、TypeScript、Vite、shadcn/ui、Tailwind、Lucide。
- 发布：GitHub Actions 构建 Linux amd64 压缩包，systemd 运行，无 Docker。

运行时依赖方向固定为 `handler -> service -> repository/client`。HTTP、Cron、页面手动任务和 CLI 不实现业务规则；纯 Go 回测引擎不访问数据库、不请求外部接口、不发送通知。

## 目录

```text
cmd/aowugong/                正式服务和 job CLI 入口
cmd/migrate/                 MySQL 到 SQLite 盘点、迁移、核验工具
internal/app/                显式组装依赖、启动和优雅停止
internal/config/             环境变量和生产校验
internal/httpserver/         chi 路由、中间件和 React SPA
internal/database/           SQLite、版本迁移、备份和数据迁移
internal/scheduler/          唯一任务注册表、包装器和 Cron
internal/client/             外部 HTTP/API 客户端
internal/{auth,rbac,...}/    普通业务模块
internal/finance/            finance 业务、数据、策略、回测和任务
web/                         React + TypeScript 前端
migrations/                  版本化 SQLite SQL
configs/.env.example         唯一配置模板
init/systemd/                systemd 服务模板
scripts/                     构建、发布、迁移、切换和回滚脚本
.github/workflows/           tag 测试和 Linux 发布
storage/                     运行时数据；不提交 Git
```

## 页面矩阵

| 页面 | 权限/用途 | 主要后端模块 |
| --- | --- | --- |
| `/login` | 登录 | auth |
| `/` | 业务总览 | finance/service |
| `/weread` | 微信读书统计 | weread/client |
| `/mahjong` | 麻将战绩录入与报告 | mahjong |
| `/subscriptions` | 订阅 CRUD 与到期状态 | subscription |
| `/positions` | 仓位截图 OCR 导入 | finance/position |
| `/stock-analysis` | 仓位趋势与持仓分析 | finance/stockanalysis |
| `/article-fetch` | RSS 抓取、任务执行 | finance/articleanalysis、scheduler |
| `/article-analysis` | 60 天信号榜、3 天市场判断、文章抽屉 | finance/articleanalysis |
| `/backtest` | 股票、ETF、Web3 纯 Go 回测状态 | finance/backtest、strategy |
| `/data` | Tushare 行情和基础数据状态 | finance/data |
| `/jobs` | 任务定义和执行记录 | scheduler |
| `/trading` | 实盘开关和客户端状态 | finance/service |
| `/notifications` | OpeniLink 与通知日志状态 | notification |
| `/monitoring` | 外部服务探测 | monitoring/client |
| `/work` | 私有工作导航 | work |
| `/permissions` | admin/investor 角色分配 | rbac |

API 统一位于 `/api/v1`。现有前端使用的认证、权限、订阅、麻将、工作导航、微信读书、监控、仓位、股票分析、文章、finance 摘要和任务接口均由 Go 提供。任务手动执行使用：

```text
GET  /api/v1/finance/jobs/definitions
POST /api/v1/finance/jobs/{name}/run
```

真实交易默认关闭，只有 `FINANCE_ENABLE_REAL_TRADE=true` 时才显示为启用；当前对外可达接口不提供无保护的真实下单入口。

## SQLite

数据库固定为 `storage/data/aowugong.db`，生产环境使用绝对路径。连接初始化自动启用：

```text
journal_mode=WAL
foreign_keys=ON
busy_timeout=5000
synchronous=NORMAL
```

版本化 migration 创建 20 张有效业务表以及 Go 新增的 `job_execution`、`notification_log`。查询索引覆盖日期、代码、账户、状态和外键；行情等大表查询必须带日期或代码范围。

MySQL 迁移规格包含以下 20 张当前有效表：

```text
aowugong_fastapi_users        aowugong_roles
aowugong_permissions          aowugong_user_roles
aowugong_role_permissions     basic_operation
basic_position                finance_broker_account
finance_asset_snapshot        finance_position_holding_snapshot
investment_article_source     investment_article
investment_article_analysis   mahjong_game_record
service_monitor_result        subscription_record
tushare_daily                 tushare_etf_basic
tushare_stock_basic           tushare_trade_cal
```

以下历史表当前页面、API 和任务均不可达，因此不会迁移：`guestbook`、`revenue`、`user`、`visits`、`vpn_data`、`work_webmaps`。发现其他未知源表时迁移工具会直接失败，不能静默跳过。

迁移程序在 MySQL `REPEATABLE READ` 只读事务中盘点字段、类型、索引和精确行数；任何未映射源字段或缺失关键核验字段都会阻止迁移。程序只向最终库同目录的唯一临时 SQLite 流式写入，逐表比较行数、关键字段范围、首尾抽样并执行 `PRAGMA foreign_key_check`；BIGINT 和 DECIMAL 使用精确数值规范化。全部通过后才执行 WAL checkpoint、文件与目录同步并原子发布，失败不会留下半迁移正式库。

```powershell
go run ./cmd/migrate `
  -mode inventory `
  -env-file ../aowugong-fastapi/.env `
  -report storage/exports/mysql-inventory.json

go run ./cmd/migrate `
  -mode migrate `
  -env-file ../aowugong-fastapi/.env `
  -sqlite storage/data/aowugong.db `
  -report storage/exports/mysql-sqlite.json `
  -batch-size 2000

go run ./cmd/migrate `
  -mode verify `
  -env-file ../aowugong-fastapi/.env `
  -sqlite storage/data/aowugong.db `
  -report storage/exports/mysql-sqlite-verify.json
```

也可使用 `-mysql-url`、`-mysql-dsn`，或环境变量 `AOWUGONG_MYSQL_URL`、`AOWUGONG_MYSQL_DSN`。报告不输出数据库密码。

## 定时任务

全部任务由同一个 Registry 定义，自动、页面手动和 `aowugong job <name>` 共用同一包装器。包装器负责同任务互斥、超时、panic 恢复、开始/结束日志、耗时、SQLite 执行记录和失败微信通知。

| 时间（Asia/Shanghai） | 任务 | 作用 |
| --- | --- | --- |
| 09:00 | `test_crontab` | 任务链路自检 |
| 20:00 | `update_tushare_daily_data` | 补齐开市日股票日线 |
| 08:00、20:00 | `sync_investment_articles` | 抓取最多 2000 篇，按 50 篇最多分析 10 批 |
| 08:30 | `check_service_monitors` | 检查服务连通性 |
| 09:30 | `check_subscription_expiry_notify` | 提醒正好 10 天后到期的订阅 |
| 10:00 | `openilink_reply_reminder` | OpeniLink 回复保活提醒 |
| 03:30 | `backup_sqlite` | SQLite 安全快照，默认保留 7 份 |

任务失败通知固定为“任务、时间、状态、信息”四段。业务包只调用 notification service；OpeniLink 文本、图片和文件发送均由统一客户端实现并写入通知日志。

手动执行示例：

```powershell
go run ./cmd/aowugong job test_crontab
```

## 本地开发

需要 Go 1.26.5 和 Node.js 22。首次运行：

```powershell
$env:AOWUGONG_ENV = "development"
$env:AOWUGONG_HTTP_ADDRESS = "0.0.0.0:2346"

Push-Location web
npm ci
npm test
npm run build
Pop-Location

go run ./cmd/aowugong
```

访问 `http://127.0.0.1:2346`。前端热更新使用 `npm run dev`，Vite 会把 `/api` 代理到 `127.0.0.1:2346`。

完整校验命令：

```powershell
go test ./...
go test -race ./...
go vet ./...
npm --prefix web test
npm --prefix web run build
```

## 配置

生产环境从 systemd 的 `/opt/aowugong-go/shared/.env` 读取。模板位于 `configs/.env.example`，必须自行填写真实值，禁止提交 `.env`、Token 或密码。

| 配置 | 说明 |
| --- | --- |
| `AOWUGONG_ENV` | `development` 或 `production` |
| `AOWUGONG_HTTP_ADDRESS` | 开发 `0.0.0.0:2346`，正式 `0.0.0.0:2345` |
| `AOWUGONG_DATABASE_PATH` | SQLite 绝对或相对路径 |
| `AOWUGONG_STATIC_DIR` | `web/dist` 路径 |
| `AOWUGONG_MIGRATIONS_DIR` | migration 目录；发布环境使用绝对路径 |
| `AOWUGONG_JWT_SECRET` | 生产必填，JWT 签名密钥 |
| `AOWUGONG_ENCRYPTION_KEY` | 生产必填，应用加密密钥 |
| `AOWUGONG_SCHEDULER_ENABLED` | 并行验收 `false`，正式切换 `true` |
| `AOWUGONG_BACKUP_DIR`、`AOWUGONG_BACKUP_RETENTION` | SQLite 备份目录和保留份数 |
| `AOWUGONG_WORK_NAVIGATION_PATH` | 私有工作导航 JSON |
| `AOWUGONG_POSITION_UPLOAD_DIR`、`AOWUGONG_POSITION_TEMP_DIR` | 仓位文件目录 |
| `WEREAD_*` | 微信读书 Gateway、API Key 和 skill 版本 |
| `INVESTMENT_ARTICLE_AGGREGATE_RSS_URL` | 聚合 RSS 地址 |
| `WECHAT_RSS_MONITOR_URL` | WeChatRSS 状态地址，默认端口 5000 |
| `DEEPSEEK_*` | DeepSeek 地址、Token、模型 |
| `TUSHARE_*` | Tushare 地址和 Token |
| `OPENILINK_*` | OpeniLink Hub 地址、Token、默认接收人，默认端口 9800 |
| `POSITION_*`、`ALIYUN_OCR_*` | 仓位 OCR 配置 |
| `SERVICE_MONITOR_TARGETS` | 额外监控目标 |
| `FINANCE_ENABLE_REAL_TRADE` | 真实交易总开关，默认 `false` |
| `QMT_ACCOUNT`、`BINANCE_API_KEY`、`OKX_API_KEY` | 现有交易端配置状态 |

## 构建与发布

推送 `v*` tag 后，`.github/workflows/release.yml` 会执行前端测试/构建、`go test ./...`、race、vet，并交叉编译 Linux amd64 的 `aowugong` 和 `aowugong-migrate`。GitHub Release 包含二进制、`web/dist`、migrations、配置模板、systemd 模板和服务器脚本，同时发布 SHA-256 校验文件。

Windows 本地可生成相同结构的发布包：

```powershell
./scripts/build-release.ps1 -Version v1.0.0
```

服务器首次部署只下载发布产物。先在服务器准备一份不在仓库中的生产 env，然后执行：

```bash
curl -fsSL \
  https://raw.githubusercontent.com/howie110/aowugong-go/v1.0.0/scripts/bootstrap-release.sh \
  -o /tmp/aowugong-go-bootstrap.sh

sudo ENV_FILE=/root/aowugong-go.env \
  APP_PORT=2346 \
  SCHEDULER_ENABLED=false \
  bash /tmp/aowugong-go-bootstrap.sh v1.0.0
```

部署脚本会检查 `swapon --show`；仅在没有任何 Swap、`/swapfile` 不存在且根文件系统至少剩余 2.5GB 时创建 2GB swapfile，并设置 `vm.swappiness=10`。Swap 初始化中途失败会撤销已创建的文件、fstab 项和启用状态。它不会修改 SSH、安全组、防火墙、Caddy、Nginx、Docker、WeChatRSS 5000 或 OpeniLink 9800。

发布布局：

```text
/opt/aowugong-go/
├── releases/<version>/       不可变发布产物
├── current -> releases/...   当前版本
├── previous -> releases/...  上一版本
└── shared/
    ├── .env
    └── storage/              数据、备份、导出、上传和私有文件
```

## 并行验收与切换

1. 使用上面的部署命令让 Go 在 2346、调度关闭状态运行。
2. 在服务器直接创建并核验 SQLite：

```bash
sudo LEGACY_ENV_FILE=/www/wwwroot/docker-file/aowugong-fastapi/.env \
  bash /opt/aowugong-go/current/scripts/migrate-mysql.sh
```

3. 检查健康接口、登录、全部关键页面、角色权限和任务手动执行。迁移报告位于 `/opt/aowugong-go/shared/storage/exports/`。
4. 正式切换执行：

```bash
sudo FINAL_MIGRATION=true \
  LEGACY_PROJECT=/www/wwwroot/docker-file/aowugong-fastapi \
  bash /opt/aowugong-go/current/scripts/cutover.sh
```

`cutover.sh` 会先确认 Go 2346 健康，只备份并移除旧 FastAPI 自己的 crontab 块，记录旧 unit 状态后停止并禁用 `aowugong-fastapi`，执行最后一次一致性全量迁移，随后让 Go 接管 2345 并启用内嵌调度。数据库切换前后均拒绝遗留 WAL/SHM；任一步失败会原子恢复旧 SQLite、合并恢复旧任务块并按原状态恢复 FastAPI，不覆盖同期新增的其他 crontab。旧代码、service 配置和 MySQL 数据均保留；确认没有其他项目使用 MySQL 后只停止 MySQL，不删除数据。

切换后检查：

```bash
curl -fsS http://127.0.0.1:2345/api/v1/health
systemctl status aowugong-go --no-pager
journalctl -u aowugong-go -n 100 --no-pager
```

## 回滚

Go 发布版本回滚，不回退 SQLite 数据：

```bash
sudo bash /opt/aowugong-go/current/scripts/rollback.sh release
```

恢复 FastAPI 时可同时指定切换过程生成的 crontab 备份：

```bash
sudo CRONTAB_BACKUP=/opt/aowugong-go/shared/storage/backup/fastapi-crontab-YYYYMMDD-HHMMSS \
  LEGACY_RUN_USER=root \
  bash /opt/aowugong-go/current/scripts/rollback.sh fastapi
```

SQLite 每天 03:30 使用 `VACUUM INTO` 创建一致快照并执行 7 份保留策略。恢复前必须停止 `aowugong-go`，保留当前数据库副本，再把已验证快照放回 `AOWUGONG_DATABASE_PATH`；不要直接复制仍在运行的 WAL 数据库文件。
