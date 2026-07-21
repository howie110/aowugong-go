# aowugong-go

`aowugong-go` 是 Aowugong 工作台的 Go 模块化单体。Go 进程提供 HTTP API、React 静态资源、权限控制和统一任务注册表；业务数据继续使用现有 MySQL，服务器不需要 Go、Node 或 Python 编译环境。

## 技术栈

- 后端：Go、`net/http`、chi、`database/sql`、`go-sql-driver/mysql`、Goose、`log/slog`。
- 前端：React、TypeScript、Vite、Tailwind CSS、shadcn/ui、Lucide。
- 数据库：MySQL 8.4，版本迁移位于 `migrations/mysql`。
- 调度：`robfig/cron/v3`，固定时区 `Asia/Shanghai`。
- 通知：OpeniLink Hub，统一由 notification service 调用。
- 生产：Linux amd64 发布产物、systemd、MySQL 客户端；Go 服务本身不依赖 Docker。

当前服务器沿用独立运行的 MySQL 8.4 容器。MySQL 只绑定 `127.0.0.1:3306`，不向公网开放；FastAPI 回滚服务、Go 服务和 SSH 隧道都通过服务器回环入口访问同一数据库。

## 目录

```text
cmd/aowugong/             正式服务和统一 job CLI 入口
internal/app/             依赖组装、启动和优雅停止
internal/config/          环境变量与配置校验
internal/httpserver/      路由、中间件和 React 静态资源
internal/database/        MySQL 连接、Goose 迁移和逻辑备份
internal/scheduler/       任务注册、Cron、执行包装和 MySQL 锁
internal/client/          外部 HTTP/API 客户端
internal/{auth,rbac,...}/ 普通业务模块
internal/finance/         行情、文章、仓位、回测和任务
internal/testdatabase/    隔离 MySQL 集成测试夹具
web/                      React 前端
migrations/mysql/         版本化 MySQL SQL
configs/.env.example      唯一配置模板
init/systemd/             systemd 模板
scripts/                  本地运行、构建、部署、切换和回滚
storage/                  被 Git 忽略的运行数据和私有配置
```

依赖方向固定为 `handler -> service -> repository/client`。HTTP、页面手动任务和 CLI 任务共用同一 service 与 Registry；回测引擎只做纯计算。所有依赖在 `internal/app` 显式构造，不使用全局可变业务单例。

## 页面与接口

React 保留以下入口：

```text
/login  /  /weread  /mahjong  /subscriptions  /positions
/stock-analysis  /article-fetch  /article-analysis  /backtest
/data  /jobs  /trading  /notifications  /monitoring  /work  /permissions
```

API 前缀为 `/api/v1`，健康检查为：

```text
GET /api/v1/health
```

登录令牌有效期固定 72 小时。页面和 API 继续使用 admin/investor RBAC；真实交易默认关闭，只有 `FINANCE_ENABLE_REAL_TRADE=true` 时才允许启用。

私有工作导航读取 `AOWUGONG_WORK_NAVIGATION_PATH` 指向的 JSON。生产文件位于：

```text
/opt/aowugong-go/shared/storage/private/work/navigation.json
```

微信读书实时调用 Agent Gateway，不在本地落库；必须配置 `WEREAD_API_KEY`。

## MySQL

服务器应用账号和本地任务账号分离：

- `aowugong_app`：只对 `aowugong.*` 拥有应用与迁移权限，由 systemd 服务使用。
- `aowugong_worker`：只对 `aowugong.*` 拥有 `SELECT/INSERT/UPDATE/DELETE`，供本地 SSH 隧道任务使用。

Go 服务默认启动时执行 Goose 迁移。本地连接生产库时必须设置：

```dotenv
AOWUGONG_MYSQL_HOST=127.0.0.1
AOWUGONG_MYSQL_PORT=13306
AOWUGONG_MYSQL_SKIP_MIGRATIONS=true
AOWUGONG_SCHEDULER_ENABLED=false
FINANCE_ENABLE_REAL_TRADE=false
```

投资文章信号通过 `investment_signal_group` 和 `investment_signal_alias` 保存规范概念及原始名称的一对一映射。未知名称由 DeepSeek 明确选择复用现有组、新建稳定概念组或进入“待归类”；只有新名称会触发增量判断。`rebuild_investment_signal_groups` 可手动全局收敛完整词典，后端要求全部来源唯一覆盖且正式组不超过 40 个。信号榜直接读取持久化映射，并完整展示当前 60 天实际计入统计的原始名称。

CLI `aowugong job <name>` 无条件跳过迁移，只使用已经部署好的表结构。任务使用 MySQL `GET_LOCK` 跨进程互斥；文章同步和概念词典重建共用业务互斥键，因此本地补跑、服务器调度和手动重建不会交错覆盖映射。

## 本地运行

准备 Go 1.26.5、Node.js 22 和 Windows OpenSSH。真实 `.env` 放在仓库根目录且不会被 Git 跟踪。

先在一个 PowerShell 窗口保持 SSH 隧道：

```powershell
.\scripts\open-mysql-tunnel.ps1
```

再启动本地 Go 页面：

```powershell
.\scripts\run-local.ps1 -GoCommand C:\howiedata\tools\go1.26.5\bin\go.exe
```

本地页面固定访问 `http://127.0.0.1:2345`；Vite 开发服务器的 `/api` 也代理到该端口。

本地补跑线上任务：

```powershell
.\scripts\run-local.ps1 -JobName update_tushare_daily_data -GoCommand C:\howiedata\tools\go1.26.5\bin\go.exe
.\scripts\run-local.ps1 -JobName sync_investment_articles -GoCommand C:\howiedata\tools\go1.26.5\bin\go.exe
.\scripts\run-local.ps1 -JobName rebuild_investment_signal_groups -GoCommand C:\howiedata\tools\go1.26.5\bin\go.exe
```

`run-local.ps1` 会拒绝公网 MySQL 地址，并强制关闭迁移、自动调度和真实交易。任务业务计算在本地电脑完成，结果直接写入服务器 MySQL；SSH 隧道关闭后连接立即失效。

前端开发：

```powershell
cd web
npm ci
npm run dev
```

## 定时任务

| 上海时间 | 任务名 | 作用 |
|---|---|---|
| 09:00 | `test_crontab` | 检查任务执行链路 |
| 20:00 | `update_tushare_daily_data` | 补齐 Tushare 日线 |
| 08:00、20:00 | `sync_investment_articles` | 同步并分析投资文章 |
| 08:30 | `check_service_monitors` | 检查服务状态 |
| 09:30 | `check_subscription_expiry_notify` | 提醒即将到期订阅 |
| 10:00 | `openilink_reply_reminder` | OpeniLink 回复提醒 |
| 03:30 | `backup_mysql` | MySQL 一致性压缩逻辑备份 |
| 手动 | `rebuild_investment_signal_groups` | 全局重建投资信号概念词典，不由 Cron 自动触发 |

Registry 统一负责同任务互斥、超时、panic 恢复、开始结束日志、耗时、`job_execution` 记录和失败微信通知。失败通知固定包含“任务、时间、状态、信息”四段。

`backup_mysql` 使用 `mysqldump --single-transaction --quick --skip-lock-tables` 流式生成 gzip，密码只通过子进程环境传递，不进入参数或日志。默认目录为 `storage/backup`，保留最近 7 份。生产启用调度前必须安装兼容 MySQL 8 的 `mysqldump`。

## 配置

完整模板见 `configs/.env.example`。主要配置组如下：

| 配置 | 用途 |
|---|---|
| `AOWUGONG_ENV`、`AOWUGONG_HTTP_ADDRESS` | 环境与监听地址 |
| `AOWUGONG_MYSQL_*` | MySQL 地址、身份、连接池、备份命令和迁移开关 |
| `AOWUGONG_JWT_SECRET`、`AOWUGONG_ENCRYPTION_KEY` | 生产必需密钥 |
| `AOWUGONG_STATIC_DIR`、`AOWUGONG_MIGRATIONS_DIR` | 发布产物目录 |
| `AOWUGONG_*_DIR`、`AOWUGONG_WORK_NAVIGATION_PATH` | 共享存储与私有导航 |
| `AOWUGONG_SCHEDULER_ENABLED` | 内嵌调度开关 |
| `WEREAD_*`、`DEEPSEEK_*`、`TUSHARE_*` | 外部数据和模型客户端 |
| `OPENILINK_*`、`WECHAT_RSS_MONITOR_URL` | 通知与服务监控 |
| `POSITION_*`、`ALIYUN_OCR_*` | 仓位截图 OCR |
| `FINANCE_ENABLE_REAL_TRADE`、交易端键 | 真实交易保护和状态 |

生产 `.env` 位于 `/opt/aowugong-go/shared/.env`，权限为 `0600`。不得提交 Token、密码、真实 `.env`、工作导航或上传文件。

## 测试与构建

```powershell
go test ./...
go test -race ./...
go vet ./...

cd web
npm test
npm run build
```

MySQL 集成测试只读取 `AOWUGONG_TEST_MYSQL_*`，为每个测试创建独立 schema；缺少完整测试账号时会跳过，绝不会回退生产身份。

构建 Linux amd64 包：

```powershell
$env:Path = "C:\howiedata\tools\go1.26.5\bin;$env:Path"
.\scripts\build-release.ps1 -Version v1.0.0
```

产物位于 `dist/aowugong-go-v1.0.0-linux-amd64.tar.gz`，并带 SHA-256 文件。版本 tag 的 GitHub Actions 会运行前端测试/构建、Go 测试、race、vet 和 MySQL 集成测试，再发布相同结构的 Linux 包。

## 部署

服务器布局：

```text
/opt/aowugong-go/
  current -> releases/<version>
  previous -> releases/<previous-version>
  shared/.env
  shared/storage/
```

服务器只安装发布产物，不执行 `go build`、`npm install` 或 `npm build`。首次部署先在 2346、调度关闭状态运行：

```bash
APP_PORT=2346 SCHEDULER_ENABLED=false \
  bash /path/to/deploy-release.sh v1.0.0
```

本地上传包可设置 `RELEASE_ARCHIVE=/tmp/aowugong-go-v1.0.0-linux-amd64.tar.gz`；正式 tag 发布默认从 GitHub Release 下载并校验。

并行验收通过后切换：

```bash
bash /opt/aowugong-go/current/scripts/cutover.sh
```

切换脚本确认 2346 健康，备份并移除旧 FastAPI crontab，记录旧 service 状态，停止 FastAPI，再让 Go 监听 2345 并启用调度。MySQL 不复制、不删除，切换前后使用同一份数据。

回滚上一 Go 产物：

```bash
bash /opt/aowugong-go/current/scripts/rollback.sh release
```

恢复 FastAPI：

```bash
CRONTAB_BACKUP=/opt/aowugong-go/shared/storage/backup/<backup> \
  bash /opt/aowugong-go/current/scripts/rollback.sh fastapi
```

systemd 单元使用 `MemoryLimit=256M` 限制 Go 服务，目标空闲 RSS 低于 100MB。服务器使用 2GB Swap、`vm.swappiness=10`；部署脚本只在没有 Swap 时创建，不修改 SSH、防火墙、阿里云安全组、WeChatRSS 5000 或 OpeniLink Hub 9800。
