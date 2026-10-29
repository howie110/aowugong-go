# 嗷呜公 · 个人项目与服务器总控手册

这是嗷呜公个人项目、服务器和相关线上服务的总控手册。当前仓库是维护其他项目和服务器的根据地；根目录的 README.md 是唯一持续维护的设计与开发事实来源。

阅读关系：

- 先读本文件，了解产品、服务器、域名、数据和发布边界。
- 再读 AGENTS.md，遵守删除文件、生产操作、部署和外部通知等安全约束。
- 最后根据任务进入代码、脚本或配置。代码与旧文档和本手册冲突时，以用户最新明确确认的设计为准，并在同一次变更中更新本手册。

文档维护要区分已确认设计、代码实现和线上状态。2026-09-30 已现场核验博客合并上线、自动发布及 Moments 清理；其他业务沿用既有维护基线。部署前仍须现场核对，未完成项见第 13 节。

## 0. AI 开发前必读

- 这是个人私有项目，不以对外分发或第三方复现为目标。
- 本 README 只维护当前有效设计；不新增重复的设计 README、配置模板、草稿规则或版本说明。
- 代码、页面、服务器结构、域名分配或部署行为发生变化时，必须同步更新本文件对应章节。
- 涉及生产数据、服务重启、文件删除或外部系统修改时，先说明影响，再做最小范围操作。
- 只有用户在当前任务明确要求时才能部署；部署范围限于构建、上传、重启和健康检查。
- 默认不发送钉钉、邮件、微信等外部通知；环境变量存在不代表获得发送授权。
- 真实密钥、Token、节点地址、VPN 原始配置、数据库内容和生产日志不得写入 Git、README、发布包或前端代码。
- 根目录 .env 是本机私有配置，服务器配置在 /opt/aowugong-go/shared/.env；不要把实际凭证写进文档。
- configs/.env.example 已删除，今后不再维护配置模板。配置按当前启用的功能直接维护在对应环境的 .env 中。
- dist/ 等生成目录中的 README 只是发布产物，不是设计来源；源代码中只维护本文件。

## 1. 一页总览

### 1.1 项目和服务清单

| 项目或服务 | 线上位置/运行方式 | 作用 | 当前仓库关系 |
|---|---|---|---|
| aowugong-go | /opt/aowugong-go/current，aowugong-go.service | 根域名主页、博客、工作台、API、VPN 订阅、图片代理、定时任务 | 本仓库直接维护 |
| aowugong-movie | /opt/aowugong-movie/current，静态文件 | Movie-Images 页面 | 独立项目；本仓库只记录域名契约 |
| nextflux | /opt/nextflux/current/dist，nextflux.service | Nextflux 静态前端 | 独立项目；本仓库只记录域名契约 |
| Vaultwarden | /opt/vaultwarden，Docker 容器 | 密码库 | 服务器基础服务，不由本仓库业务代码提供 |
| Caddy | /opt/vaultwarden/caddy，Docker 容器 | 统一 TLS、域名入口和反向代理 | 服务器基础服务 |
| Miniflux | miniflux.service | RSS 阅读和公众号内容消费 | 独立服务；本仓库提供同机 WeRead RSS |
| Xray | xray.service，/usr/local/etc/xray/config.json | 服务器本机代理能力 | 独立基础服务 |
| PostgreSQL 15 | 服务器回环地址 127.0.0.1:5432 | aowugong-go 正式业务数据库 | 本仓库迁移、备份和恢复 |

服务器 SSH 入口是 8.138.123.59:22。账号和认证方式属于私密运维信息，不写入 README；需要操作时使用本机已授权的 SSH 凭证或用户当次提供的方式。

### 1.2 二级域名分配

所有公网域名先进入 Caddy，再转发到回环地址或读取静态发布目录。域名分配以服务器 /opt/vaultwarden/caddy/Caddyfile 为准：

| 域名 | 后端/目录 | 用途 |
|---|---|---|
| https://aowugong.top | aowugong-go：127.0.0.1:12345 | 公开主页、`/blog` 博客、登录工作台、API、VPN 订阅 |
| https://www.aowugong.top | Caddy 重定向到根域名 | 保留原路径跳转到根域名 |
| https://vault.aowugong.top | 127.0.0.1:8222 | Vaultwarden |
| https://miniflux.aowugong.top | 127.0.0.1:5000 | Miniflux |
| https://nextflux.aowugong.top | 127.0.0.1:5001 | Nextflux 静态前端 |
| https://pic.aowugong.top | aowugong-go：127.0.0.1:12345 | 原有 `pic/` 图床与 `blog/status/` 状态图片，使用独立身份 |
| https://movie.aowugong.top | /srv/aowugong-movie/current | Movie-Images 静态站点 |

根域名的特殊路径：

- / 是公开的“嗷呜公 · 工具分享”备案主页。
- /work 是登录后的工作台。
- /work/navigation 是私有工作导航。
- /api/ 是 Go API，包括 VPN 订阅接口。
- /blog 是公开博客，/blog/status 是数据库状态；/work/content/status 是「记录此刻」手机发布入口。
- /blog/rss.xml、/blog/sitemap.xml 由 Go 生成；/feeds/rss-style.xsl 保留旧订阅样式资源。

### 1.3 服务器端口和边界

| 端口 | 绑定范围 | 服务 | 边界 |
|---|---|---|---|
| 22 | 公网 | SSH | 仅用于运维登录 |
| 80、443 | 公网 | Caddy | 唯一公网 Web 入口，负责 TLS |
| 12345 | 127.0.0.1 | aowugong-go | 根域名、工作台、API、VPN 和图片代理 |
| 8222 | 127.0.0.1 | Vaultwarden | 只由 Caddy 访问 |
| 5000 | 127.0.0.1 | Miniflux | 只由 Caddy 和同机服务访问 |
| 5001 | 127.0.0.1 | Nextflux | 只由 Caddy 访问 |
| 5432 | 127.0.0.1 | PostgreSQL | 不对公网开放 |
| 6152、6153 | 本机回环 | Xray HTTP/SOCKS | 服务器本机代理端口，不作为 Web 服务入口 |

基本原则：公网只暴露 SSH 和 Caddy；业务服务、数据库和代理端口继续监听回环地址。新增服务或域名时，先确定端口、Caddy 路由、TLS 和备份责任，再改代码或部署。

## 2. 服务器维护边界

### 2.1 发布目录约定

aowugong-go 使用原子切换发布：

    /opt/aowugong-go/
    ├── current -> releases/version
    ├── previous -> releases/previous-version
    ├── releases/
    ├── shared/
    │   ├── .env
    │   ├── .env.canary
    │   └── storage/
    └── ...

- current 是当前运行版本，previous 用于应用产物回滚。
- 正式 systemd 工作目录是 /opt/aowugong-go/current，环境文件是 /opt/aowugong-go/shared/.env。
- 本地私有文件和生产私有文件都放在 storage/private 或对应环境的 shared/storage/private，不随发布包传输。
- aowugong-go.service 以 aowugong 用户运行，内存限制为 256 MB。
- canary 使用独立端口 2346 和 .env.canary，调度器必须关闭。
- aowugong-movie 和 nextflux 采用 releases + current 的静态发布约定，但由各自项目负责构建。

### 2.2 基础服务和操作顺序

服务器基础边界：

- Caddy 负责所有公网域名、证书和反向代理。
- PostgreSQL 负责 aowugong-go 正式业务数据；迁移由应用启动或发布流程执行。
- Vaultwarden 由 Docker 运行，相关 Caddy 和备份脚本位于 /opt/vaultwarden。
- Miniflux、Nextflux、Xray 和 aowugong-go 由各自 systemd 服务管理。
- 本仓库可以维护 aowugong-go、它的迁移、备份、发布和运维脚本；不能因为修改本仓库就顺手改动独立项目或基础服务。

生产操作固定按以下顺序思考：

1. 只读确认目标项目、域名、服务、当前版本和影响范围。
2. 涉及数据时先确认备份和恢复路径。
3. 修改最小范围的代码、配置或发布产物。
4. 构建、上传、重启后做对应域名和服务健康检查。
5. 只向用户报告结果，不自动通知第三方。

## 3. aowugong-go 产品设计

### 3.1 项目定位

aowugong-go 是 Go 模块化单体，统一提供：

- 公开根域名主页和工作台。
- React/Vite 前端静态资源。
- HTTP API、登录、RBAC 和管理员能力。
- 投资研究、内容服务、订阅、回测和资源分享。
- VPN 资源转换、用户订阅 Token 和公开订阅分发。
- 图片上传后的访问代理。
- 定时任务、统一 CLI、PostgreSQL 迁移和备份。

技术边界：

- 后端：Go、net/http、chi、database/sql、pgx、Goose、log/slog。
- 前端：React、TypeScript、Vite、shadcn/ui、Tailwind CSS。
- 数据库：PostgreSQL 15+；默认连接池为最大 8、空闲 4；时区为 Asia/Shanghai。
- 依赖方向固定为 handler -> service -> repository/client。
- 页面操作、定时任务和 CLI 补跑共用同一个 service 与任务注册表。
- 回测引擎不访问数据库和外部接口。
- 正式 Linux amd64 二进制不依赖 Go、Node、Python、MySQL 或 SQLite 运行。

### 3.2 页面和权限

公开页面与私有工作台分离：

| 区域 | 页面 | 访问规则 |
|---|---|---|
| 总览 | 控制台、工作导航 | 工作台控制台与工作导航均需登录；根路径备案主页独立公开 |
| 投资研究 | 投资文章分析、投资文章抓取、股票仓位分析、股票仓位导入 | 登录后按角色使用 |
| 量化工具 | 回测、数据、交易 | 登录后使用；真实交易默认关闭 |
| 内容服务 | 微信读书、麻将战绩、订阅管理 | 登录后使用 |
| 资源分享 | VPN 分配、VPN 资源 | VPN 分配仅管理员；VPN 资源只看自己的资源 |
| 系统运维 | 监控管理、定时任务、通知、数据库、权限管理 | 管理员或对应权限使用 |

权限和数据原则：

- 普通用户只能读取自己的订阅、资源和个人数据。
- 管理员可维护用户和资源分配，也可查看公共规则原文；VPN 资源页向已授权用户只读展示同一份规则，但不能把私有节点内容暴露到普通用户页面。
- VPN 用户是使用 VPN 资源页面和订阅能力的角色；管理员账号也可以被分配 DMIT、魔戒等资源，管理员身份不代表自动拥有某一套资源。
- 工作台入口不从公开根页面暴露；公开备案主页不依赖登录。

### 3.3 投资研究和内容服务

- 订阅管理列表将后端判定为“已结束”的记录放在底部，组内保持原有到期日排序；新增、编辑共用弹框，取消不保存，失败保留输入，成功关闭并刷新，保存期间禁止重复提交及关闭。字段、统计及删除规则不变。2026-10-16 随 `v20261016-content-ui` 部署，交互已通过本地测试，线上未执行真实订阅写入验收。
- 微信读书通过扫码绑定账号并从书架发现公众号；人工启用的公众号由手动抓取任务检查最近 20 篇文章，不再按 08:00、20:00 自动抓取。
- 只为数据库未知文章读取详情和微信公众号原文，不依赖外部 RSS 聚合。
- 登录凭据使用 AOWUGONG_ENCRYPTION_KEY 派生的 AES-256-GCM 密钥加密后存入 PostgreSQL；二维码中间态只保存在当前 Go 进程内。
- Go 按公众号生成回环地址 WeRead RSS，供同机 Miniflux 分源保存和阅读全文。
- 投资文章分析支持按文章、概念和成员筛选。
- 股票仓位支持截图导入和敏感信息遮罩。
- 私有工作导航保存在 storage/private/work/navigation.json，不放入数据库或公开页面。
- 真实交易开关 FINANCE_ENABLE_REAL_TRADE 默认是 false，任何打开真实交易的动作都必须单独确认。

#### 个人聊天归档（2026-10-17 已部署）

- 组成：`skills/chat-archive-sync` Skill 包及 Python 同步脚本、现有 PostgreSQL、工作台“内容服务 → 聊天归档”（`/work/content/chat-archive`）。不加本地 SQLite、队列、独立上传服务；不做摘要、标签、长期记忆、聊天客户端或 Skill 云端管理。
- 采集：适配本地 Codex rollout JSONL（含 `.jsonl.zst` 压缩日志）、Claude Code session JSONL，只读明确配置的文件或 glob。只保存用户/AI 纯文字，不保存源文件、附件及引用、工具调用/输出、系统指令、推理通道或摘要；未知事件仅计数。Claude 桌面版、云端会话不支持。压缩日志只在内存中解压，不生成文件副本；缺少 `zstd` 时明确失败。会话索引的旧 `.jsonl` 路径不存在不等于记录丢失，应检查对应 `.jsonl.zst`。
- 同步：Skill 2.0 的 HTTPS 默认流程扫描全部授权源文件，与云端逐条比对消息身份/版本哈希，仅分批补齐缺失或变化的文字，再次比对并校验云端哈希；不依赖历史游标，因此能修复旧解析器漏采。每批最多500条且限制编码体积，失败重跑不重复追加；确认过的内容照常上传，来源不明的部分单独报告。原有 `/upload` 增量事务及游标协议仍兼容，补齐接口不修改它。压缩前后保持同一身份和解压偏移；单个源事件限128MiB，不上传源文件。读取中变化重读一次，坏压缩文件明确失败，末尾半行待下次读取；本地删除不删除云端，已不可获取的原文不能补救。
- 身份：有稳定 ID 时按来源＋账号范围＋原会话/消息 ID 合并；缺少消息 ID 时使用设备/逻辑文件/事件位置的弱身份，不猜测跨设备合并。消息观察保留批次、偏移、父 ID、来源时间与完整性，结构块/附件字段为空；同内容的多设备观察不等于多次编辑，`version_hash` 区分内容。不提供分支树。
- 查询：仅管理员且有页面权限可访问。消息列表默认每页 20 条，数据库按消息身份取最新观察，再排序、分页后读取正文；页面一条消息一行，展开显示原文和来源 ID，无导出按钮。独立 `/api/v1/chat-archive/statistics` 只聚合角色/时间，不传正文；总量包含所有日期，日统计默认北京时间最近 7 天，点击“更早 7 天”才查询历史。未知日期单独计数，统计与消息分页互不阻塞。统计和列表沿用同一消息 ID 快照；原有筛选/导出 API 保留管理员鉴权。
- 存储：继续使用迁移 `migrations/postgres/00013_chat_archive.sql` 的四张表，不修改已发布迁移。兼容保留 `chat_archive_raw_batches` 表名，但纯文字采集的 `raw` 固定为空，`sha256` 校验消息与顺序，解析器版本为 `dialogue-only-3`。Codex 按逐块来源标记只取 `user.text`，AI 只取 `phase=final_answer` 或 `channel=final`；跳过子 Agent 会话、中间进度和注入指令，缺失/不匹配标记时不猜测收录。标题仅来自保留的真人输入，旧格式可能缺失。现有管理员导出接口保留，但新批次没有源文件字节可导出。新增专用 Key 鉴权的 HTTPS 同步接口，管理员浏览接口仍单独鉴权。
- 代码入口：`skills/chat-archive-sync/scripts/{adapters,sync,http_store,configure_device}.py` 负责解析、采集 CLI、HTTPS 传输和设备配置，`store.py` 仅兼容旧 DSN；`internal/chatarchive/ingest*.go` 负责校验/事务写入/核验，`internal/httpserver/chatarchive_sync_handlers.go` 提供同步接口；`internal/chatarchive/audit.go` 与 `internal/httpserver/chatarchive_handlers.go` 提供 `/api/v1/chat-archive/messages` 只读核对接口；`web/src/lib/chat-archive.ts` 封装接口，`web/src/pages/chat-archive.tsx` 展示。`internal/chatarchive/statistics.go` 提供数据库聚合。迁移 14 新增 `source_at` 存储生成列及索引，来源时间无效时为 NULL，原始 source_time 不改动；旧客户端写入也会自动更新规范时间。

**HTTPS 同步接口**

仅此业务使用 `AOWUGONG_CHAT_ARCHIVE_SYNC_KEY`，32–4096 位非空白 ASCII，未配置即禁用；通过 `Authorization: Bearer <key>` 鉴权，不接受 URL 中的 Key，不能用此 Key 访问管理员业务 API。端点为 `GET /api/v1/chat-archive-sync/status`（兼容检查）、`POST /state`（source_key 查询进度）、`POST /upload`（原增量协议）、`GET /verify`（完整性核验），以及 v2 的 `POST /check`（来源/账号/会话内逐条查询 message_key＋version_hash，返回 present/different/missing）和 `POST /repair`（幂等追加缺失版本，返回 inserted/verified）。check/repair 每批最多500条；所有接口共用专用 Key、16MiB 请求限制、两路并发和速率限制，不返回聊天正文，不记录 Key/正文日志。

新版入口 `scripts/{archive_run,reconcile,native_history}.py` 由 sync.py 调用，仍通过 http_store.py 统一封装接口。旧 Codex user 缺来源标记时，从本机 `thread/turns/list` 的 full 分页结果匹配原生 userMessage 证据；支持 proxy→stdio 只读服务回退，不调用 turn/start/resume。缺证据、摘要/部分历史或不明阶段不猜测上传。Claude 排除元消息/子任务/工具中间文字，助手确认 end_turn；不明文字报告待核实。配置可提供 codex_command 数组指定本机程序。repair 使用来源锁与会话锁，旧版本不覆盖，不修改 sync_states/补传标记；旧 upload 的精确重试按 last_success_at 对应批次核实，不被 repair 新批次干扰。没有新增迁移。

客户端完成来源读取并确认未变化后才发送整份增量；服务端按来源锁和 expected_state 比较后原子提交。重复相同提交成功但不重复落库，不同内容或过期进度返回 409；客户端重新读取最多重试一次。保持 dialogue-only-3 哈希与原表格式。16 MiB 为筛选后文字/元数据大小，超限失败而非截断。HTTP 模式断网/失败由本次 CLI 明确报告，不承诺远端失败计数更新。

**使用与配置**

- Skill 源码随项目维护，本机 Codex 已安装 `~/.codex/skills/chat-archive-sync`。运行环境 Python 3.11+，HTTPS 模式只用标准库，不需要 psycopg 或 SSH；压缩日志仍需 zstd。旧 DSN 模式才需 requirements.txt。通用 Skill 不含凭据，私密交接包包含单独 API Key，不能公开分享。
- 设备私有 JSON 使用 `api_url`、`api_key`、固定 `device_id`、`account_scope`、`sources: [{source,path}]` 和可选 `codex_index`。本机 HTTPS 配置为 `.cache/chat-archive/http-config.json`（Git 忽略、600），旧 config.json 保留但不再作为默认。公司电脑用 `configure_device.py --config <私密配置> --codex-home <授权数据目录>` 初始化新 device_id 和本机路径，支持 UTF-8；Unix chmod600，Windows 设置当前用户专属 ACL。Windows 尚未实机验证。不同来源账号不可混用 scope。Key 不写进源码、Skill 或日志。
- 数据库继续只监听回环地址。新设备通过 HTTPS 调用站点，无需数据库账号/SSH 登录/隧道。历史 DSN 专用账号和文件未撤销；不用它们作为公司电脑交接内容。已有聊天记录保留，不因传输方式升级清库。
- 可选配置 `codex_index` 指定 Agent 自己的会话索引；本机为 `/Users/howie/.codex/state_5.sqlite`。只读核对配置范围的真人会话及缺失日志，不建立本地同步状态库。没有索引时仅报告扫描数量，不能宣称覆盖全部聊天；索引存在也只能证明来源文件覆盖，不保证来源曾完整保留全部内容。

```bash
# 使用设备虚拟环境中的 Python；将路径替换为明确提供的私有配置
python skills/chat-archive-sync/scripts/sync.py --config /private/path/archive.json --dry-run
python skills/chat-archive-sync/scripts/sync.py --config /private/path/archive.json
python skills/chat-archive-sync/scripts/sync.py --config /private/path/archive.json --verify
```

预览逐条比对但不写；正式同步永久保留纯文字（仍可能含敏感信息），用户调用同步即授权配置范围的本次上传。HTTPS 模式 --verify 比较本地与云端并校验完整性，不上传；旧 DSN 模式仍仅核验云端哈希。退出0表示配置内可确认的记录核对通过；退出2表示有待确认来源/缺失源文件或只读发现缺口，同步模式下已确认内容仍完成补传；退出1表示实际失败。不得将“已存哈希正确”或“已到文件末尾”说成源会话绝对完整。安装升级使用包内 install.py --config <这台设备原配置>，备份旧代码，保留设备ID/scope/授权路径并将配置复制到 Skill/config.json，不重新生成老设备身份。PostgreSQL 无法表示的字符转义并标记 escaped_source_text，不自动清理旧数据。

**验证与运维边界**

合成样本覆盖原有纯文字解析与增量边界、新接口鉴权/大小限制/参数校验/冲突/重复提交、隔离 PostgreSQL 事务回滚，以及真实 Python→HTTP→Go→PostgreSQL 端到端续传、501 条多批次写入、Go/Python 双向哈希核验。归档表继续纳入数据库备份；核验不等于源会话完整，也不能替代独立备份。线上测试只使用获授权记录，不能注入演示数据或自动清库；不启用定时同步或发送外部通知。

聊天归档首次发布：`v20261017-chat-archive-reconcile`（2026-10-17），上一版 `v20261017-chat-archive-performance`。新增 check/repair 接口已部署，未增加迁移；公网健康、专用Key核对/空补齐和未授权401验证通过，检查前后仍为100批次/14,893条观察，本次未上传真实聊天。Go全量测试/vet、54项Python测试通过；实际CLI经HTTP/隔离PostgreSQL完成502条历史漏采补齐、51页原生分类匹配、重跑零新增、保留游标、版本修改及后续追加。Windows实机和公司真实记录仍须在公司电脑执行，不能把测试当作已经全部归档。本机已更新Skill；完整私密包为 `.cache/chat-archive-sync-v2-private.zip`（含专用Key，不含SSH/数据库凭据），保留 `.cache/chat-archive/http-config.json`，均不提交Git。上一版迁移14与统计优化保留；数据库备份位于 root 专属 `/opt/aowugong-go/chat-api-stage.H5fRRf/performance-backup/database.dump`。400日志仅记录安全字段路径/错误类别。回滚应用不得删除列、回退数据或继续使用不理解新接口的客户端；服务端代码已随 2026-10-22 发布提交 main；本次补齐电脑端 `skills/chat-archive-sync` 源码及测试。

历史补传协议（2026-10-17）：`v20261017-chat-archive-backfill` 正式支持公司原脚本的 `native_user_backfill_v1`。此字段是可选、有序且不重复的 SHA-256 数组，最多 100,000 项，不能再当普通附加字段丢弃。新增标记必须精确等于原标记集合加本次 `[position,message_key,body]` 的 Python JSON UTF-8 哈希；仅 Codex user 消息、保持原 offset/prefix/conversation/version 的请求可作为历史补传，仍执行范围检查、source-key 锁、完整 expected_state CAS 和事务写入。消息与标记一起提交，失败一起回滚，完全相同提交可重试；普通增量或重扫客户端省略标记时，服务端保留 expected_state 中的标记，过期请求不能覆盖它们。已验证公司排查包原 backfill_native.py 和 sync.py 在隔离库完成 502 条跨批次补传、预览、重跑零新增、后续普通同步保留标记，并测试并发/回滚/哈希校验；全量 Go 测试和 vet 通过。部署未上传真实聊天。补传成功后不得直接回滚到不理解该状态字段的旧版本，应先评估状态兼容性。

### 3.4 接手开发的关键入口

| 入口 | 职责与修改位置 |
|---|---|
| cmd/aowugong/main.go、internal/app/run.go | HTTP 与 CLI 启动、依赖组装、数据库和调度器生命周期 |
| internal/config/config.go | 配置字段、默认值和环境变量加载；查询配置定义时从这里开始 |
| internal/httpserver/router.go 与同目录 handlers | API 路由、鉴权和请求处理；业务实现进入对应 internal 模块 |
| internal/finance/job/registry.go、internal/scheduler | 唯一任务定义、手动/定时边界、执行锁与任务结果 |
| internal/vpn/source.go、source_build.go、convert_*.go、routing.go、service.go | 私有目录读取、配置组装、节点格式转换、公共规则转换、分配与订阅鉴权 |
| internal/database、migrations/postgres | PostgreSQL 连接、迁移和备份；修改 schema 需考虑旧版本兼容 |
| web/src/main.tsx、web/src/pages、web/src/lib | 公开页与工作台入口、业务页面和前端 API 调用 |
| scripts、init/systemd | 本地启动、发布、回滚及服务器运行约定 |

正式启动先连接 PostgreSQL，按配置执行迁移并同步权限基线，再装配 HTTP 与调度器。CLI 的 job 入口复用任务注册表，但不启动 HTTP/Cron，也不执行数据库迁移。开发代理模式提前分流，不组装本地业务后端，详见第 8 节。

### 3.5 代码风格与结构约定

- 新代码先归属业务模块，再按职责分文件；保持现有依赖方向，不因文件较长就新建包或抽象层。文件与函数长度只作为检查信号。
- API 先封装再调用：页面通过 web/src/lib 下对应业务函数访问后端，不直接拼接地址或调用 fetch/authorizedFetch。request.ts 复用 auth.ts 的认证入口，统一 JSON 结果和错误解析；上传 FormData、二维码 Blob、业务错误提示和兼容分支按接口保留，不自动增加重试。
- 仓位上传与报告接口分别由 positions.ts、stock-analysis.ts 管理；接口类型与封装放在一起，页面目录保留展示类型。后端第三方协议通过 client 封装；独立业务模块已有的客户端可留在本模块。
- VPN 页面入口 web/src/pages/vpn.tsx 只负责导出；分配页、资源页和表格/弹窗在相邻 vpn 目录，公共规则卡片由两页复用。Go 的 source.go 管理文件发现，source_build.go 组装输出，convert_*.go 负责格式转换。
- 投资文章分析保持在 internal/finance/articleanalysis 包内：service.go 管理依赖，sync.go/parse.go 管理抓取和正文解析，analysis.go/analysis_json.go 管理模型分析和响应解析，model_settings.go 管理模型设置，report.go/prompt.go 管理统计与提示词。业务流程、算法和事务不因拆文件而改变。
- 使用清晰英文标识符和一致业务术语，重要函数与复杂流程使用中文说明；简单辅助函数不强制套注释模板。修改实现时同步核对输入、输出、副作用和特殊处理说明。
- VPN 局部标识统一使用 subscriptionID；历史路由参数 deviceID、Token 派生中的 device: 字节前缀及旧数据契约为兼容性保留，不能机械替换。
- 结构调整优先验证行为。API 测试检查请求、参数、响应与失败分支，VPN 页面测试检查渲染和复制回调；尚存的其他源码匹配测试不代表完整交互验证。

### 3.6 博客与状态（2026-10-01 文章入库已上线）

- 2026-10-03 已部署（v20261003-status-editor）：「记录此刻」上方发布、下方按时间倒序读取全部动态（分批请求现有管理接口），支持就地编辑、图片增删排序、取消修改和确认删除。选图仅生成本地预览，发布/保存时才上传；成功上传的结果保留在编辑器内供失败重试复用。删除动态沿用现有数据库删除接口，OSS 文件回收尚未实现，不会删除历史导入图片。

#### 来源与架构

- 博客统一进入 `https://aowugong.top/blog`，复用现有 Go、React、PostgreSQL、登录和权限；退出 Astro 构建链和独立 Moments 服务，保持单一应用。
- 文章继续由笔记项目的博客 Markdown 提供，推送后自动更新网站；状态改由数据库保存，在个人工作台「内容服务 → 记录此刻」通过手机发布。
- 设计要求是单向、直接、可理解：每类内容只有一个权威来源，不做双写、双向同步、多框架并存或切换到另一套内容源的兜底。
- 文章唯一来源是笔记项目 `/Users/howie/project/aowugong-note/【7】博客`。Go 在导入时使用 Goldmark 解析 YAML 元信息和 Markdown，再用 bluemonday 清理 HTML 并存入 PostgreSQL；Go Web 查询数据库，React 负责阅读界面。旧 Astro 仅保留历史源码，不再同步内容、构建或部署。
- `blog-2000-01-05.md` 的 8 条动态已一次性导入 PostgreSQL，保留原时间和正文；已逐项核对线上动态的迁移标识、正文、图片和发布时间，并按用户要求删除笔记项目中的该文件；自动发布仍排除这一历史 slug。新状态仅由工作台写入数据库。

#### 页面与模块

| 入口 | 用途 | 数据来源与权限 |
|---|---|---|
| `/blog` | 博客文章列表 | PostgreSQL 文章表，公开读取 |
| `/blog/posts/:slug` | 文章及关于、咖啡等原有文字页面 | 同一 PostgreSQL 文章表，公开读取 |
| `/blog/status` | 按发布时间倒序展示状态 | PostgreSQL，公开读取已发布状态 |
| `/work/content/status` | 手机发布、编辑、删除状态及管理草稿 | 复用工作台登录，仅获授权的管理者可操作 |
| `/blog/rss.xml` | 博客文章订阅 | 与文章页面共用解析结果 |

- 前端使用现有 `web` React 工程，博客有独立公开布局；公开博客路由在登录检查前分流，工作台继续保持鉴权。保留文章、标签、目录、图片查看、代码复制和明暗主题这些现有阅读能力。
- 后端新增 `internal/blog`，Markdown 读取与解析、状态业务与数据库访问按职责分文件；HTTP handler 进入现有 `internal/httpserver`，前端 API 统一放在 `web/src/lib`。沿用 handler → service → repository/client，不引入通用 CMS 或插件系统。
- 博客首页迁入旧 Astro 雪人/小猫互动贴纸，宽屏时整组固定在左上角空白区，保留拖动、键盘微调、复位、位置记忆、音效和作者链接；沿用原素材，React 生命周期管理事件，不依赖 Astro。窄屏隐藏，与旧版桌面装饰定位一致。
- 文章正文采用书页风格排版，恢复 H1/H2/H3 标记；正文外链在新标签打开。博客始终预留纵向滚动条空间。动态先图后文，配图最高 200px，点击查看原图。
- 文章正文与文章标题继承博客的系统无衬线字体（Mac 中文通常为苹方），不再单独指定宋体；保留现有字号、行距、字距及代码块等宽字体。2026-10-16 随 `v20261016-content-ui` 部署。
- 文章列表使用纯文字布局，不显示缩略图；图片保留在文章正文中。2026-10-01 已通过本地验收。
- Markdown 由 Go 端一套解析器处理，读取既有 `title/date/tags/image` 元信息，正文输出经过安全处理的 HTML，目录从同一次解析提取。React 只展示，不再维护第二套 Markdown 解析。
- 迁移核对既有换行、链接、图片、表格和代码块表现；不执行 Markdown 中的脚本。保留文章 slug、RSS、Sitemap 以及每篇文章的标题、描述和 canonical 元信息，不能让所有文章都返回工作台的默认标题。

#### 文章自动更新

固定链路：笔记仓库 `main` 的博客变更 → 该仓库 GitHub Actions → SSH 上传博客文件 → 服务器 Go CLI 解析校验 → 事务写入 PostgreSQL → Go Web 查询数据库展示。

- 笔记项目 `/Users/howie/project/aowugong-note/.github/workflows/sync-blog.yml` 已直接发布到 Go 服务。触发范围仍限 `【7】博客/**` 和工作流自身。这里的自动更新指推送到 `main` 后更新，本地仅保存未推送不会触发。
- 停止向 Astro 仓库提交文章，停止其独立部署工作流；不经过 Go 仓库再提交文章，不重新构建 React 或 Go，不重启业务服务，不增加轮询任务或公网同步接口。
- 工作流只打包博客目录中的公开 Markdown 及其实际需要的目录内附件；不传输私人笔记库、Git 元数据、凭据或博客目录外文件。已有 OSS 图片 URL 保持不变。拒绝符号链接、路径越界和博客目录外的附件引用，并报告具体文件。
- Go CLI 在导入时使用 Goldmark 解析 Markdown、bluemonday 清理 HTML，将原文、标题、日期、标签、HTML、摘要和目录存入 `blog_articles`；博客目录内的图片附件存入 `blog_article_assets`，既有 OSS URL 保持不变。网站、API、RSS 和 Sitemap 只查数据库，不在请求时解析文件，也不回退到文件目录。
- 上传包暂存于 `/var/lib/blog-publisher/incoming`。全部内容验证后，在一个事务内替换文章和附件；`blog_article_publication` 保存发布序号并串行化并发导入，拒绝旧序号覆盖新数据。空包、解析或数据库写入失败均保留上次成功的数据；源文件删除在下次成功同步后体现为文章下线。
- `aowugong blog publish --archive <包> --sequence <序号>` 只读取发布专用 `BLOG_DATABASE_URL`，不装配应用、调度器或通知服务。成功后删除上传包，临时解压目录自动清理，不保留文件快照、目录软链接或文件锁。
- 笔记 Actions Secrets 保持 `BLOG_DEPLOY_HOST`、`BLOG_DEPLOY_USER`、`BLOG_DEPLOY_SSH_KEY`、`BLOG_DEPLOY_KNOWN_HOSTS`。`blog-publisher` 的 SSH key 使用 `restrict` 和 forced command，仅接受 `upload <序号>`、`publish <序号>`，不能运行任意 shell。
- 发布账号使用 PostgreSQL 本地 peer 认证，权限限上述三张文章表；连接地址存于 `/var/lib/blog-publisher/database.env`，不读取应用 `.env`，不具备状态或其他业务表权限。流程不发送外部通知。

#### 状态存储与发布

- 状态存 PostgreSQL 的 `blog_statuses`，保存主键、作者、纯文本正文、有序图片引用、草稿/已发布标记、发布时间、创建和修改时间。图片引用使用 JSONB 数组，不为固定的少量附图引入通用附件系统。
- 状态不是一篇 Markdown 文章。新状态只在手机友好的工作台页面编辑；正文按纯文本保留换行并安全展示，链接可点击，图片支持九宫格及大图查看。
- 「记录此刻」上方发布、下方列出全部动态，支持就地编辑和确认删除：纯文字或文字配图、最多九张图片、图片排序、上传进度、失败重试和发布反馈。选图只生成本地预览，发布或保存时才上传 OSS；复用既有管理接口。博客前台入口与页面标题统一称「动态」。只有正文或图片至少一项非空才能发布。
- 图片通过现有 Go 应用的鉴权上传接口写入现有 OSS 的独立 `blog/status/` 前缀，复用既有图片访问域名和代理，不复用 Moments 服务。仅增加该前缀所需的写入权限，保留原有 `pic/` 读取边界。
- 图片上传全部成功后才提交状态；失败时保留编辑内容，明确失败项，由用户重试。数据库失败不得显示发布成功；防止连续点击导致重复发布。公开 API 不返回草稿，写接口复用现有 RBAC。
- 旧博客动态 Markdown 一次性导入状态表，保留正文、图片、原时间和顺序。导入前生成核对结果，对不能明确拆分或识别时间的条目报错，不猜时间或静默丢弃；迁移标识防止重复导入。导入验收后该文件不再进入文章同步，源笔记已在与线上 8 条动态完整核对并保存本地备份后删除。
- 独立 Moments 按用户要求退役清理，不建立双向同步或常驻兼容适配层。Moments 数据不作为新状态来源；清理前核实其媒体是否被其他内容引用，被引用对象先迁移并更新引用。

#### 切换与清理边界

- 先在隔离环境完成新博客、笔记自动更新、历史动态导入及手机发布验收，再切换公网入口。博客仅使用主站 `/blog` 路径；未知文章返回 404，不统一跳到首页。
- 应用升级及数据库迁移沿用现有发布和备份方式；文章数据独立于应用 release，后续笔记发布不影响应用版本。应用回滚不自动撤销已发布状态、数据库迁移或笔记内容。
- Moments 清理范围：专用运行及停止容器、镜像、服务器发布和数据目录、SQLite 备份及 systemd 备份任务、专用域名 DNS 和 Caddy 路由、媒体 `/moments/*` 路由、OSS `moments/` 对象、专用 CORS 来源、专用 RAM 用户/策略/密钥、项目内私有运行配置，以及本地与 GitHub 上该 Moments 项目的专用代码和部署资源。
- 用户明确要求删除 Moments 二级域名：清除 `moments.aowugong.top` 实际存在的 DNS 解析记录和 Caddy 站点配置，不保留该域名跳转；删除后旧朋友圈域名停止服务。`pic.aowugong.top` 继续供博客及其他图片使用，只移除其 Moments 专用路径转发。
- 删除前逐项只读核实真实目标与引用关系，并向用户说明实际影响；不按名称模糊匹配批量删除。用户已要求清除 Moments，但未知共享资源须先查清归属，不能扩大删除范围。
- 共用 OSS Bucket、`pic/` 图片、其他 RAM 身份、Caddy 主服务及其他域名必须保留。仅移除 Moments 对应的配置条目；如入口更新需要重启 Caddy，先说明对其他站点的短暂影响。
- 新博客上线后移除旧 Astro 发布链和服务器静态发布目录，迁移所需静态资产时保留必要许可声明。旧博客源码仓库的删除不由 Moments 清理请求自动授权。
- 用户提供的阿里云凭据文件只在云资源核查和操作时读取，不复制内容到代码、日志、聊天、Git 或部署包；核实有效身份与实际权限，不推断管理 Key 已启用，也不擅自启用失效密钥。

#### 验收原则

- 文章解析用现有内容样本覆盖元信息、换行、图片、代码、链接、目录和危险 HTML；缺失文章、损坏文件和越界路径必须明确报错。
- 自动更新验证新增、修改、删除、发布失败不切换及连续推送顺序；确认没有触发 Astro/Go 重建、服务重启、私人目录上传或外部通知。
- 状态验证未登录/无权限拒绝、草稿不可公开、手机选图发布和编辑删除、上传或数据库失败、重复点击，以及历史动态导入完整性。
- 切换验证博客文章、状态、旧链接跳转、RSS、Sitemap、图片及根站/工作台/VPN；清理后核对 Moments 专用资源消失、`moments.aowugong.top` 的 DNS 与入口配置已删除、共享资源继续正常。笔记项目须以一次真实推送验证新工作流自动更新网站。
- 现场结果见下节；不会把构建成功当成部署成功，也不会把仓库归档描述为删除。

### 3.7 博客运行与验证记录

- 旧博客二级域名已撤除 DNS 和 Caddy 入口，不保留跳转；HTTPS 直连原服务器也不再提供该站点。主站 `/blog` 正常。

- 本轮发布内容（2026-10-01，用户已授权部署）：笔记的 39 个博客文件已将 image 属性移到正文首行，博客模板移除 image；Go 网站 favicon 换为旧 Astro 的 `public/icons/aowugong.png`。笔记中的默认 spring_updates.jpg 图片引用已移除；前端已部署并通过健康及静态资源校验。`sync-blog.yml` 已恢复；经用户明确授权，仅给 `blog-publisher` 增加 `/opt/aowugong-go` 的 `--x` ACL，不增加目录读写权限。原 ACL 备份在 `shared/publisher-parent-acl.before-20261001`。修复后工作流 https://github.com/howie110/aowugong-note/actions/runs/36841423196 成功，38 篇文章逐篇读取正常且均无已删除默认图片引用。

- 正式应用版本为 `/opt/aowugong-go/releases/v20261001-workbench-avatar`，保留已经上线的通知 API。迁移 `00010_blog_statuses.sql` 和 `00012_blog_articles.sql` 已应用，生产文章 38 篇、历史状态 8 条。状态需要 `blog.status.manage` 权限，默认管理员具备；普通用户不可发布。
- 笔记工作流提交 `f72dd4b3` 已合入 main；[第 32 次自动导入](https://github.com/howie110/aowugong-note/actions/runs/36828365573) 成功，数据库发布序号为 32，Go PID 未变化。旧 Markdown 快照、文件锁、目录软链接和 `AOWUGONG_BLOG_CONTENT_DIR` 已移除。
- 原 Astro 工作流已删除、部署 Secrets 已撤除，服务器静态目录和旧发布账号已删除，Caddy 不再挂载旧博客目录。旧源码仓库保留并标记退役。
- Go 全量测试、go vet、博客/HTTP/图片代理 race 测试、临时 PostgreSQL 集成测试、88 项前端测试和构建通过。390px 手机布局验证了发布、编辑、撤回草稿和上传失败重试；内部导航有未保存内容提醒。
- 生产验证了图片上传与公网读取、原 `pic/` 图片读取、草稿隔离、重复创建幂等、编辑与删除；测试草稿和对象已清除。公开文章、状态、RSS、Sitemap、旧文章跳转与主站、工作台、VPN 页面健康检查通过。
- 2026-10-01 文章入库验证：全量 Go 测试、go vet、博客与 HTTP race 通过；真实 PostgreSQL 导入 38 篇文章，并验证并发旧序号拒绝与失败批次回滚。删除服务器全部 Markdown 快照后，文章 API 仍返回 38 篇；44 个公开地址检查通过。此次生产备份为 `shared/backups/blog-database/before-articles.dump`，已校验。
- 迁移前 PostgreSQL 完整备份在 `/opt/aowugong-go/shared/backups/blog-migration/before-blog.dump`，已通过 pg_restore 目录校验；它包含私有业务数据，不可进入 Git。应用回滚仅恢复二进制，不能撤销状态数据或内容发布。

文章导入 CLI（网站直接查询数据库，不解析文件）：

```bash
aowugong blog validate --dir <文章目录>
# 使用专用 BLOG_DATABASE_URL
aowugong blog publish --archive <tar.gz> --sequence <工作流序号>
aowugong blog import-status --file <旧动态.md>                         # 只核对
aowugong blog import-status --file <旧动态.md> --author-id <id> --apply # 一次性导入
```

导入工具只在显式 apply 时连接数据库，使用原文上海时间和确定性 UUID，重复执行不新增。文件校验不连接数据库；文章发布仅连接专用数据库账号，不装配调度器或通知。发布失败回滚事务并明确报错。

## 4. VPN 资源与订阅设计

### 4.1 资源分配

“资源分享 > VPN 分配”是管理员入口，负责：

- 给用户分配资源。
- 重新发布、轮换或撤销资源。
- 维护用户与资源的绑定状态。
- 查看唯一公共规则原文。

“资源分享 > VPN 资源”是用户入口，负责：

- 只展示当前登录用户已获配的资源。
- 为同一账号提供手机和电脑共用的订阅。
- 提供二维码和可复制的订阅链接。
- 只读展示所有资源共用的公共规则原文，没有编辑或保存入口。

DMIT、魔戒等只是不同的上游资源来源。进入系统后都走相同的资源分配、转换、订阅和刷新流程；资源来源不会改变用户使用方式。管理员账号和普通账号一样，只有被分配后才拥有对应资源。

### 4.2 原始资源和统一输出

原始资源边界：

- 本地原始文件位于 storage/private/vpn，生产原始文件位于 /opt/aowugong-go/shared/storage/private/vpn。
- 目录被 Git 忽略，文件可能包含服务器地址、Token、UUID 和私有订阅链接。
- 资源文件按文件名中的资源编码归组；资源来源可以不同，但统一转换为客户端可消费的订阅。
- 设计目标是各资源订阅采用同一套分流策略。当前 Clash、Shadowrocket、Surge 输出合并公共规则；v2rayN/v2rayNG 的标准节点订阅存在下述限制，不能视为已经实现规则随订阅同步。

四种客户端输出：

| 客户端 | 输出策略 |
|---|---|
| Clash / FlClash | 生成标准 Clash 配置，并合并公共规则 |
| Shadowrocket | 生成或替换 [Rule] 段，并合并公共规则 |
| Surge | 生成或替换 [Rule] 段，并合并公共规则 |
| v2rayN / v2rayNG | 返回标准节点订阅；不另发独立规则资源 |

v2rayN/v2rayNG 的标准节点订阅无法像 Clash、Surge 那样携带完整分流配置，因此不再提供独立的 v2rayN 规则资源。历史 routing 地址只为兼容旧链接保留，不代表要维护多套公共规则。

### 4.3 唯一公共规则

公共规则是所有订阅的共同输入，不是用户单独领取的一套资源：

- 唯一文件：storage/private/vpn/common-routing.json。
- 格式：Xray routing 对象。
- 所有用户、所有资源只使用当前最新的一份。
- 不使用数据库，不提供页面编辑，不保留草稿、版本、发布、回滚或多份并存。
- 管理员分配页和用户资源页按原始匹配顺序紧凑展示公共规则，相邻同用途规则分组，每组只显示一行备注，组内不再逐条注释；资源页将公共规则放在订阅资源之后，默认全部展开；管理员仍可复制 JSON 原文。展示直接解析接口返回的唯一规则文件，不维护第二份规则。2026-10-04 已部署 v20261004-vpn-rules-grouped。
- 每次生成订阅都会重新读取该文件；更新私有规则文件后，支持合并规则的客户端刷新订阅即可得到新规则。v2rayN/v2rayNG 标准节点订阅不携带这些规则。
- 规则目标统一为 proxy、direct、block，分别表示代理、直连和拒绝。
- 公共策略为：保留明确指定的代理/直连/拦截规则，局域网、订阅服务及节点入口直连，国内域名集合和国内 IP 直连，其余默认走代理。节点入口随资源变更核对；显式例外优先于国内集合。
- `geosite:cn` 是本项目的国内域名集合标识，转换为 blackmatrix7/ios_rule_script 的 ChinaMax 外部域名集：Clash 使用 domain rule-provider（86400 秒更新），Surge/Shadowrocket 使用 DOMAIN-SET，由客户端下载缓存；不在服务器按请求下载。集合来源是 [ChinaMax](https://github.com/blackmatrix7/ios_rule_script/tree/master/rule/Surge/ChinaMax)，不是自动探测封锁情况，也不承诺不同客户端的 GeoIP 数据完全相同。首次导入需确认规则集下载成功，必要时先使用可用代理更新。
- `geoip:cn` 转为 `GEOIP,CN,DIRECT`；局域网使用明确 IPv4/IPv6 CIDR，避免依赖不同客户端对 PRIVATE 的解释。最后一条 `network: "tcp,udp"` 转为 Clash 的 MATCH 或 Surge/Shadowrocket 的 FINAL，禁止放在中间遮蔽后续规则。页面只读展示集合引用及默认代理，不展开集合全量域名。v2rayN 节点订阅仍不携带这些规则。
- 文件缺失或为空时保持无公共规则的兼容行为；文件存在但 JSON 无效或格式不支持时，阻止生成不完整的订阅。
- 例如 geosite:cn 和私有 IP 可以统一指向 direct，具体内容以当前文件为准。

    {
      "domainStrategy": "IPIfNonMatch",
      "rules": [
        {"type": "field", "domain": ["geosite:cn"], "outboundTag": "direct"},
        {"type": "field", "ip": ["geoip:private"], "outboundTag": "direct"}
      ]
    }

### 4.4 订阅安全和访问要求

- VPN_PUBLIC_URL 当前为 https://aowugong.top，必须在设备尚未连接代理时也能访问。
- 公开接口不提供资源列表，只接受每个用户独立的高强度 Token。
- Token 由 AOWUGONG_ENCRYPTION_KEY、订阅主键和版本通过 HMAC 派生；数据库不保存 Token 或节点正文。
- 普通用户不能读取其他用户的资源、节点内容或订阅 Token；已授权用户可以在 VPN 资源页只读查看公共规则原文。
- 节点正文、规则正文和订阅 Token 不写入日志、数据库或 Git。已授权用户的页面按功能需要接收自己的订阅链接、二维码和公共规则原文；不将私有资源嵌入前端静态发布文件。
- 发布 VPN 代码或规则前，先用未连接代理的设备确认订阅 URL 可以访问，再在四类客户端分别测试刷新和导入。

## 5. 图片上传和阿里云凭证

### 5.1 上传链路

- 图片存储在阿里云 OSS。
- Go 服务在同地域使用 OSS 内网 Endpoint 读取，减少线上读取成本和跨网问题。
- PicGo 使用外网 Endpoint 上传，上传后通过自定义域名访问。
- 当前 PicGo 约定：Bucket 为 aowugong-pic-gz-f2bf98d3，地域为 oss-cn-guangzhou，存储路径为 pic/，自定义域名为 https://pic.aowugong.top。
- PicGo 是本机应用配置，不由 Go 服务直接读取；本机上传配置和服务器读取配置是同一 OSS 资源的不同使用端。

### 5.2 凭证原则

- 正式服务只使用阿里云 RAM 子账号 AccessKey。用户当次提供的管理凭据仅用于已授权资源操作，不进入服务配置；不擅自启用或禁用其全局管理 Key。
- RAM 权限按最小范围配置，只允许当前图片上传、读取或代理所需的 Bucket 和路径。
- 本机项目根目录 .env 可以保存私有工具和服务配置，文件权限应为 600 并始终被 Git 忽略。
- PicGo 的密钥输入框、项目 .env、系统钥匙串和服务器 .env 都不能复制到 README、日志、截图或发布包。
- 如果 RAM 权限发生变化，先确认需要的动作，再使用有权限的阿里云账号调整，不重新启用主账号 AccessKey。

### 5.3 博客状态图片与 Moments 退役

- 状态图片经 Go 的鉴权接口上传，支持 JPEG、PNG、WebP、GIF，真实内容检测，每张最多 10 MiB、40MP，每条最多 9 张；不支持视频或浏览器直传。
- 服务器 `.env` 使用 `BLOG_OSS_ENDPOINT`、`BLOG_OSS_BUCKET`、`BLOG_OSS_ACCESS_KEY_ID`、`BLOG_OSS_ACCESS_KEY_SECRET`。RAM 用户 `aowugong-blog` 的 `AowugongBlogStatusObjects` 策略只允许共享 Bucket 内 `blog/status/*` 的 PutObject/GetObject。`pic/` 继续使用原有只读身份。
- OSS 使用同地域内网 Endpoint，公开 URL 为 `https://pic.aowugong.top/blog/status/...`。图片未被状态引用时不会自动公开列举；删除状态只删除数据库引用，不扩大运行账号的 OSS 删除权限。
- 动态图片展示优化（2026-10-16 已部署 `v20261016-content-ui`）：公开动态、工作台「记录此刻」列表共用图库，编辑已有图片也使用缩略图；点击才请求清晰大图，并保留“查看原图”。原图及数据库引用不变，历史 `pic/` 与新 `blog/status/` JPEG/PNG 均可使用固定 `variant=thumb-v1`（长边 640、WebP 质量 80）和 `variant=large-v1`（长边 2048、WebP 质量 85）；等比缩小、不放大、自动纠正方向。Go 代理通过 OSS SDK 请求处理结果，使用结果自身的类型、长度及 ETag，沿用长期缓存、并发和大小限制，不接受任意 OSS 处理参数。GIF/WebP 保持原样以避免破坏动画；外部图片、带查询参数的历史地址和本地预览不改写。暂不引入 CDN，也不覆盖或批量重传原图，图片仍经服务器公网出口。线上照片抽测原图 6,330,852 字节、缩略图 49,658 字节、清晰大图 420,952 字节，处理请求成功；处理费用仍需按实际 OSS 账单观察。
- 2026-09-30 已清除 Moments 的运行/停止容器、四个专用镜像、SQLite 数据和备份、systemd 备份任务、服务器目录、本地源码及私有配置、专用 DNS/Caddy 路由及证书、OSS `moments/`、专用 CORS 来源、RAM 用户/密钥/策略。清理前确认 Memo 为 0、媒体仅为空目录标记；共享 Bucket 和 `pic/` 保留。
- GitHub `howie110/aowugong-moments` 已归档；删除 API 返回 403，**远端仓库尚未删除**，等待用户在仓库设置页删除。不要把归档误记为清理完成。

## 6. 数据、私有文件和备份

### 6.1 数据边界

| 数据 | 正式位置 | 是否进 Git | 说明 |
|---|---|---|---|
| PostgreSQL 业务数据 | 服务器 PostgreSQL 15 | 否 | 账户、文章、投资、订阅和系统业务数据 |
| VPN 原始配置 | shared/storage/private/vpn | 否 | 节点、Token、UUID 和私有订阅内容 |
| 工作导航 | shared/storage/private/work/navigation.json | 否 | 私有导航配置 |
| 服务器环境变量 | shared/.env、shared/.env.canary | 否 | 生产和 canary 配置、凭证 |
| 本地环境变量 | 项目根目录 .env | 否 | 本地运行和本机工具配置 |
| 博客文章、状态 | PostgreSQL | 是 | 文章从笔记 Markdown 导入，状态在工作台维护 |
| 发布产物 | /opt/*/releases | 否 | 可重建的版本包和静态文件 |
| 设计事实来源 | 根目录 README.md | 是 | 唯一当前设计文档 |

PostgreSQL 启动时自动执行 migrations/postgres。连接默认使用 127.0.0.1:5432、连接池最大 8、空闲 4，连接时区为 Asia/Shanghai。migrations/sqlite 和 cmd/migrate 只用于旧 SQLite 一次性迁移和隔离测试，正式服务不使用 SQLite。

### 6.2 PostgreSQL 备份

每日 03:30 使用 pg_dump --format=custom 创建一致性备份，再用 pg_restore --list 校验后原子发布；默认保留最近 7 份。

恢复示例：

    createdb -U postgres aowugong_restore
    pg_restore --no-owner --no-privileges -d aowugong_restore storage/backup/aowugong-时间.dump

恢复前必须确认目标数据库和影响范围，不覆盖线上正式库；应用产物回滚不会自动回滚数据库 schema 或业务数据。

### 6.3 Vaultwarden 备份

- 每日 03:45 创建 PostgreSQL、附件、Send 数据和签名密钥备份，默认保留 14 份。
- 开启 VAULTWARDEN_BACKUP_EMAIL_ENABLED 后，Go 任务每周日 05:00 读取最新备份，用 age 公钥加密后发送异地邮箱。
- 邮件恢复包包含加密备份、使用说明.md 和全新服务器重建脚本；临时文件发送后立即删除。
- 服务器只保存公钥，解密私钥只保存在本地 storage/private/backup/vaultwarden-age-key.txt，不得上传服务器。
- 灾难恢复默认面向空白新服务器；目标已有 Vaultwarden 时，必须先停止并人工确认。

本地解密后，在新服务器安装 Docker Engine 和 PostgreSQL 15，再使用恢复脚本。恢复操作会影响密码库，执行前必须另行确认。

### 6.4 GitHub 代码备份

- 任务默认关闭；启用后通过 GitHub API 发现认证账号拥有的全部公有和私有仓库。
- 额外固定备份 KES-IT/KES-SCM 和 KES-IT/KES-BIS，不枚举其他组织项目。
- 每个项目保存为裸 Git 仓库，更新前的分支和标签保留在内部历史引用中。
- 仓库失去权限或不再被发现时只记录状态，不删除最后副本。
- 默认目录为 AOWUGONG_BACKUP_DIR/github。
- GITHUB_BACKUP_TOKEN 只通过 Git 子进程环境使用，不写入远端地址、清单或日志。

## 7. 配置和凭证

### 7.1 配置文件位置

| 环境 | 配置位置 | 用途 |
|---|---|---|
| 本机 | /Users/howie/project/aowugong-go/.env | 本地开发、远程 API、PicGo/运维辅助配置 |
| 生产 | /opt/aowugong-go/shared/.env | 正式 Go 服务 |
| Canary | /opt/aowugong-go/shared/.env.canary | 并行验收；调度器关闭 |
| 私有业务文件 | /opt/aowugong-go/shared/storage/private | VPN、工作导航等生产私有文件 |

实际配置直接维护在对应环境的 .env 中，不再提供 .env.example。至少应根据启用功能配置：

- 数据库：AOWUGONG_DATABASE_URL。
- 登录和加密：AOWUGONG_JWT_SECRET、AOWUGONG_ENCRYPTION_KEY。
- VPN：VPN_SOURCE_DIR、VPN_PUBLIC_URL。
- 图片、通知、文章、GitHub 备份等功能：只配置实际启用功能所需的密钥和开关。
- 真实交易：FINANCE_ENABLE_REAL_TRADE=false，除非用户另行明确确认。

本地和生产可以使用同一套经授权的服务凭证，但文件仍然按环境独立保存；同步凭证时只同步必要值，不覆盖已有的业务开关、数据库地址和路径。

### 7.2 凭证操作规则

- 不从 Codex、其他项目或父进程的全局变量读取通知配置。
- 不将凭证写进源码、前端、Git remote URL、发布包或命令输出。
- 读取或修改生产 .env 前先确认目标服务和字段，避免整文件覆盖。
- 服务器凭证拿到本机后只写入本机 .env 或系统钥匙串，并检查 chmod 600。
- 用户已禁用阿里云主账号 AccessKey；后续只围绕 RAM 子账号排查权限，不能为了排错重新启用主账号。
- 失效的 AccessKey、订阅 Token 或节点资源应轮换并清理旧副本，不在聊天中回显。

## 8. 本地开发

本地默认只运行当前代码的页面和 Go 代理，通过线上 API 查看线上业务数据：

- 不直连生产 PostgreSQL。
- 不启动定时任务。
- 不执行真实交易。
- 默认上游为 https://aowugong.top。
- 本地监听 127.0.0.1:2345。

启动：

    cd web
    npm ci
    npm run build
    cd ..
    pwsh -File ./scripts/run-local.ps1

开发工具链要求 Go 1.26.5（以 go.mod 为准）、Node/npm；上述 .ps1 脚本另需 PowerShell（pwsh）。旧版 Go 可通过 GOTOOLCHAIN=auto 选择项目要求的工具链。本地 Go 服务读取 web/dist，修改页面后需重新构建静态资源。

访问 http://127.0.0.1:2345，停止时按 Ctrl+C。脚本会加载项目根目录 .env，并强制设置：

- AOWUGONG_DEV_UPSTREAM_URL=https://aowugong.top
- AOWUGONG_HTTP_ADDRESS=127.0.0.1:2345
- AOWUGONG_SCHEDULER_ENABLED=false
- FINANCE_ENABLE_REAL_TRADE=false

此模式适合验证本地页面与线上 API 的联动，不会运行刚修改的 Go 业务 handler/service。页面写操作仍会请求线上 API，不能当作隔离测试环境；后端变更应通过相应测试或独立环境验证。

如果要补跑或修改线上数据，应通过 SSH 在服务器加载正式环境，并调用统一 CLI，而不是让本地开发进程直接连接生产数据库：

    ./scripts/run-remote-job.ps1 sync_investment_articles

## 9. 定时任务和统一 CLI

调度时区固定为 Asia/Shanghai。

| 时间 | 任务 | 作用 |
|---|---|---|
| 09:00 | test_crontab | 每日任务链路测试 |
| 22:00 | check_service_monitors | 服务连通性检查 |
| 每月 1 日 09:30 | check_subscription_expiry_notify | 汇总有效订阅并按到期日发送月报 |
| 03:30 | backup_postgres | PostgreSQL 一致性备份 |
| 03:45 | Vaultwarden backup 脚本 | 创建密码库备份 |
| 周日 04:00 | backup_github_code | GitHub 代码备份 |
| 周日 05:00 | email_vaultwarden_backup | 加密并异地发送最新 Vaultwarden 备份 |

以下任务保留为手动或 CLI 执行：

- update_tushare_daily_data
- sync_investment_articles
- rebuild_investment_signal_groups

微信读书不再自动保活、定时探测或定时抓取；需要更新文章时先在页面重新扫码，再手动执行抓取。任务失败通知统一包含任务、时间、状态和信息；是否发送及发送到哪里取决于正式环境中明确启用的通知配置，Codex 不代发外部通知。

手动任务的唯一依据是 internal/finance/job/registry.go 中的 ManualOnly 标记；sync_investment_articles 和 rebuild_investment_signal_groups 共用 investment_signal_groups 并发锁。Vaultwarden 03:45 备份由独立 systemd timer 执行，不属于 Go 进程内 Cron。CLI 补跑也可能触发任务自身的失败通知，执行前需核实通知影响及当次授权。

常用 CLI：

    /opt/aowugong-go/current/aowugong job backup_postgres
    /opt/aowugong-go/current/aowugong job backup_github_code
    /opt/aowugong-go/current/aowugong job email_vaultwarden_backup

### 9.1 跨项目微信通知接口

接口复用现有企业微信群机器人，固定发送到项目配置的群，不是向任意个人微信号发私信。使用现有主站域名和 HTTP 服务，不另开端口；同机项目和本地开发脚本都能调用。2026-10-01 已部署 `v20261001-notification-api`，开通 `receipt-split` 项目 Token；健康、鉴权和参数校验已验证，未发送真实消息验收。

- 服务端配置 `AOWUGONG_NOTIFICATION_TOKENS`，值为“项目标识 → 专用 Token”的 JSON 对象；未配置或 `{}` 时不注册接口。启用时必须同时配置有效的 `WECOM_BOT_WEBHOOK_URL`。
- 每个项目一个不同的随机 Token，建议使用密码学随机生成的 32 字节、编码为 64 位十六进制。配置接受 32–256 位字母、数字、下划线或连字符；项目标识接受 1–128 位字母数字及 `._:-`，以字母或数字开头。
- 服务端配置保存在本项目私有环境文件；调用方仅在自己的后端或本地私密配置保存专用 Token，绝不能放在浏览器、小程序前端、URL、Git 或日志中。此 Token 只用于通知接口，不是网站登录 JWT、阿里云密钥或企业微信 Webhook。
- `source` 由服务端根据 Token 自动确定，请求体不能自行指定。更换 Token 时保留项目标识，去重记录继续有效；删除该项目配置并重启服务即撤销权限。所有配置变更均在服务重启后生效。
- 迁移 `00011_internal_notifications.sql` 已在线上应用，为 `notification_log` 增加来源、通知编号和唯一索引；SQLite 对应迁移仅用于测试。原有任务通知不需要此 Token。该次发布的数据库和配置备份位于服务器 `shared/storage/backup/v20261001-notification-api/`。
- 实现入口：`internal/httpserver/notification_auth.go`、`notification_handlers.go` 和 `internal/notification/dispatch.go`。开发上游代理模式不创建本地通知服务；携带专用 Token 请求线上地址（包括经开发代理转发）仍会真实发送，不能作为隔离测试。

调用协议：

```http
POST https://aowugong.top/api/v1/notifications/wechat
Authorization: Bearer <该项目的专用Token>
Content-Type: application/json
```

```json
{
  "request_id": "receipt-123-recognition-failed-v1",
  "title": "小票识别失败",
  "content": "账单 123 识别失败，请检查后台。"
}
```

三个字段必填。`request_id` 为业务通知唯一编号，格式与项目标识相同；标题为单行，标题与正文禁止 NUL 字符。统一格式为 `【项目 / 标题】` 加正文，完整消息最多 2048 UTF-8 字节，超长拒绝而不截断；JSON 请求最多 16 KiB。正文不要包含凭证或不必要的个人信息。调用方统一封装接口，使用 HTTPS，超时建议至少 35 秒。

响应示例：`{"id":123,"status":"success","duplicate":false}`。`success` 仅表示企业微信明确返回 `errcode=0`，不表示用户已读。占位阶段（含排队和数据库等待）最多 3 秒，发送最多 20 秒，结果写回最多 5 秒。

| HTTP 状态 | 含义与调用方处理 |
|---|---|
| 200 | 发送已确认；同项目、同编号、同内容的重试返回历史记录，不重复发送 |
| 202 | `status=unknown`：正在发送或结果不确定，不是排队承诺，不自动重发；用原编号和内容查询，持续未知时人工核实 |
| 400 / 413 / 415 | 参数、大小或媒体类型错误，修正请求 |
| 401 | 缺少、无效或已撤销的项目 Token；不能用网站登录 Token 代替 |
| 404 / 405 | 接口未启用、路径或方法不允许 |
| 409 | 同一项目和编号对应不同内容，不允许覆盖原通知 |
| 429 | 通知接口全局固定窗口每 60 秒最多 10 次新发送；按 `Retry-After` 等待后使用原编号重试 |
| 503 | 数据库暂时不可用；可能尚未发送，也可能已发送但写回失败，只能用原编号重试，不能换编号盲目重发 |

去重通过数据库唯一索引实现，跨并发和重启有效；先持久化 `unknown` 再发送，网络错误、进程中断和写回失败都优先避免重复。没有后台队列或自动补发，可能存在占位后未发送的记录，需要人工核实。去重以记录存续为限，不应随意清理。限流是进程内全局计数，重启重置，不包含原有任务通知；单个项目也会消耗共享额度。

开发测试使用模拟发送器和隔离数据库，不读取真实通知凭证。部署只启用接口不代表授权发送测试消息；实际发消息验收仍需用户当次明确授权。

### 9.2 微信通知分类与降噪

通知只发需要处理的问题和恢复，不发周期性“一切正常”。标题统一为“分类 / 对象”，正文保留时间、对象、状态/影响和处理建议；企业微信仍限制 2048 UTF-8 字节。

| 分类 | 触发 | 说明 |
|---|---|---|
| 任务 | 定时或手动任务失败 | 连续相同错误由 scheduler 去重 |
| 备份 | PostgreSQL、GitHub、Vaultwarden 备份失败 | GitHub 周报在成功时仍发送 |
| 监控 | 服务异常和恢复 | 异常正文列出具体目标和原因；恢复合并成一条；相同异常集合不重复 |
| 订阅 | 每月有效订阅月报 | 低频、可操作 |
| 项目 | 跨项目通知 API | `receipt-split` 等 |

任务失败标题由任务定义中的 `Category` 和 `FailureTitle` 决定；未声明时回退为“任务 / 任务名”，scheduler 不再按任务名维护映射。

监控任务 `check_service_monitors` 在每轮探测前读取上一轮状态，只在 `up→down`、首次 `down`、`down→up` 时产生事件。异常由 scheduler 发一条带明细的失败通知；恢复由任务发一条合并恢复通知。同一组异常不会按调度频率重复发送。

Vaultwarden 03:45 systemd 备份：

- 失败通过现有跨项目微信 API 发送 `【server-alerts / Vaultwarden 备份失败】`，恢复发送 `【server-alerts / Vaultwarden 备份恢复】`。
- `AOWUGONG_NOTIFICATION_TOKENS` 需要包含 `server-alerts` 项目 Token；修改后重启 aowugong-go 生效。
- 告警脚本为 `/usr/local/sbin/vaultwarden-backup-notify`，配置为 `/etc/aowugong-alert.env`（0600），由 `scripts/install-vaultwarden-alert.sh` 安装。
- 手动检查 payload：`/usr/local/sbin/vaultwarden-backup-notify dry-run`。
- 状态文件位于 `/var/lib/aowugong-alert`；失败只发一次，恢复成功后清除。

通知页同时提供发送历史：`GET /api/v1/finance/notifications/logs?q=&status=&category=&page=&page_size=`，数据来自 `notification_log`，支持关键字、状态（`success`/`failed`/`unknown`）、分类（任务、备份、监控、订阅、项目、其他）和分页；点击记录可查看完整正文。权限与通知页一致（`page:finance:notifications`）。所有发送尝试都会入库，包括发送失败的记录。发送历史分类中，`server-alerts` 来源归入“备份”，其他带来源的跨项目通知归入“项目”，其余按标题前缀判断。

外部服务调用复用现有 `POST /api/v1/notifications/wechat`，固定使用 `AOWUGONG_NOTIFICATION_TOKENS` 中的 `external` 项目 Token：

- 请求体只接受 `request_id`、`title`、`content`，不支持文件或附件。
- 限制与 9.1 相同，具体数值以 9.1 和接口配置为准。
- 所有外部调用共用 `external` 来源，`request_id` 建议带服务名前缀，避免跨服务冲突。
- 配置 `external` Token 并重启后，通知页的“外部调用 / MCP”卡片会显示请求地址、固定 Key、curl 示例和可复制的 MCP 工具描述；Key 等同于发送权限，只在管理员页面展示，不要发到公开渠道。

本版本不引入 Gotify、邮件通知或其他推送服务。

## 10. 构建、部署和回滚

### 10.1 发布前验证

文档或代码变更完成后，按影响范围验证。完整验证命令：

    GOTOOLCHAIN=auto go test -race ./...
    GOTOOLCHAIN=auto go vet ./...
    cd web
    npm ci
    npm test
    npm run build
    cd ..
    node --test scripts/tests/*.test.mjs
    for script in scripts/*.sh; do bash -n "$script"; done

还应运行 git diff --check，确认没有空白错误、凭证和私有资源文件进入变更。

### 10.2 正式发布

本地构建 Linux amd64 发布包：

    GOTOOLCHAIN=auto node scripts/build-release.mjs v1.0.0

本地与 CI 共用此入口；原 PowerShell 命令仍作为薄包装保留。需要 Node.js、Go、Git、tar 和 Bash（Windows 可用 Git Bash）。发布要求干净且已提交的工作区；前后端验证、Go Race/Vet、脚本语法检查任一步失败即停止，不生成新的发布包。运行支持文件从同一 Git 提交归档，包内 `REVISION` 记录提交，`VERSION` 记录发布名称。

发布包包含：

- Linux amd64 Go 二进制。
- web/dist 静态资源。
- PostgreSQL 版本化迁移。
- systemd 模板、部署脚本、回滚脚本、已提交的 skills 工具源码和本 README。

发布包不包含 .env、VPN 原始文件、工作导航、节点内容、数据库备份或任何生产私有配置。Git tag 会触发 GitHub Actions，执行前后端测试、Race、Vet 和构建。

部署只在用户明确要求时执行：

    sudo DEPLOY_MODE=main /opt/aowugong-go/current/scripts/deploy-release.sh v1.0.0

部署动作限于构建、上传、原子切换 current、重启 aowugong-go.service 和健康检查；不自动发送钉钉、邮件、微信等通知。正式发布前确认 migration 向后兼容，并保留数据库备份。

首次准备服务器时，显式运行 `ENV_FILE=/明确的私有配置路径 bash scripts/initialize-server.sh`，它负责必要的 Swap、运行用户、共享目录和初始配置；已有配置不覆盖，共享数据不递归改权限。`bootstrap-release.sh` 是下载安装入口，会先初始化再发布。日常升级直接调用 `deploy-release.sh`，未初始化时只报错，不自动准备机器。

日常部署不操作 Swap、用户或 Vaultwarden，也不递归修改 shared 权限；仍更新应用运行配置与 systemd unit、切换版本并检查健康。失败恢复原配置、原 unit、版本链接及服务状态；新建的 Canary 配置在失败时移除。已有同名版本目录拒绝复用。脚本设置 AOWUGONG_DATABASE_SKIP_MIGRATIONS=false，因此重启会执行尚未应用的 PostgreSQL 迁移；应用回滚不回滚 schema。Vaultwarden 辅助脚本的更新属于该独立服务的显式维护，Go 日常发布不再顺带同步。`SYSTEMD_UNIT_DIR` 默认 `/etc/systemd/system`，测试时才指定隔离目录。

本地发布包可通过 RELEASE_ARCHIVE 指定，必须同时提供对应 .sha256 文件。使用与发布包匹配的部署脚本；旧服务器脚本可能仍要求已删除的 configs/.env.example，不能为绕过校验恢复废弃模板。每次使用新版本目录，并分别检查服务状态、内网/公网健康接口和本次变更涉及的功能。

### 10.3 Canary 和回滚

- Canary 使用 DEPLOY_MODE=canary，监听 2346，并关闭调度器。
- 正式发布把旧版本保存为 previous。
- 回滚只切换应用发布产物，不自动回滚 PostgreSQL schema、业务数据、VPN 私有文件或服务器 .env。

    sudo /opt/aowugong-go/current/scripts/rollback.sh

回滚后检查根域名、登录、工作台、API、VPN 订阅和图片代理；如果问题来自数据库迁移或数据内容，必须走单独的数据恢复方案。

### 10.4 独立项目发布边界

- aowugong-movie、nextflux 有自己的源码、构建和发布流程；博客已并入本仓库，文章内容由笔记工作流独立发布。
- 当前仓库只维护它们在服务器上的目录约定、域名分配和联动契约。
- 修改电影或 Nextflux 时，先进入对应项目确认 Git remote、README、部署脚本和当前版本；不要在本仓库伪造或复制一份实现。
- Caddy 路由、TLS、Docker 或 systemd 属于服务器基础设施变更，改动前必须先说明受影响的域名和服务。

## 11. 测试与验收

后端验收：

- go test ./...
- go test -race ./...
- go vet ./...

前端验收：

- cd web && npm ci
- npm test
- npm run build

发布验收：

- git diff --check。
- 发布包中没有 .env、节点文件、VPN Token、数据库内容或真实凭证。
- 正式域名 aowugong.top、blog、vault、miniflux、nextflux、pic、movie 按影响范围健康检查。
- VPN 在未连接代理的网络环境中可以打开订阅 URL；Clash、Shadowrocket、Surge、v2rayN/v2rayNG 分别验证导入或刷新。
- 公共规则更新后，至少用一套 Clash/Surge/Shadowrocket 配置验证分流；v2rayN/v2rayNG 验证节点订阅本身。
- 图片上传验证 PicGo 上传、pic.aowugong.top 访问和 Go 代理读取。
- 备份任务至少检查文件生成、完整性校验和保留策略。

测试使用临时 SQLite 夹具验证仓储契约；正式二进制不包含 SQLite 驱动。PostgreSQL 基线迁移和旧数据迁移应在临时 PostgreSQL 数据库中单独验证。

## 12. 一次性数据迁移

迁移只执行一次，目标是从旧 SQLite 重建 PostgreSQL 业务表。执行前必须安排维护窗口、确认备份并停止正式 Go 服务：

    set -a
    . /opt/aowugong-go/shared/.env
    set +a
    go build -o /tmp/aowugong-migrate ./cmd/migrate
    AOWUGONG_SQLITE_SOURCE_PATH=/安全路径/aowugong.db \
      /tmp/aowugong-migrate --confirm

迁移工具会执行：

1. SQLite quick_check。
2. PostgreSQL migrations。
3. 单事务复制。
4. 序列校准。
5. 逐表核对行数和主键范围。

核对成功后，正式服务只读取 PostgreSQL；旧 SQLite 先保留一份离线归档，确认无误后才能删除。删除旧数据库属于破坏性操作，不能与普通部署顺手执行。

## 13. 当前限制和重要决策

- 当前只维护根目录 README.md；其他源代码目录不再新增重复 README。
- configs/.env.example 已删除；私有项目直接维护各环境实际 .env。
- VPN 资源统一支持 Clash/FlClash、Shadowrocket、Surge、v2rayN/v2rayNG 四类客户端，但公共规则只有一份。
- v2rayN/v2rayNG 只使用标准节点订阅，不维护独立的 v2rayN 规则资源。
- DMIT、魔戒等资源使用同一套分配和转换流程；管理员也可以被分配资源。
- 公共规则由 storage/private/vpn/common-routing.json 单文件维护，AI 修改后更新到生产私有目录，支持合并规则的客户端刷新订阅生效。
- PicGo 与博客图片使用各自的 RAM 身份；已退役的 Moments 运行资源不再保留，GitHub 归档仓库的待删除限制见第 5.3 节。
- 生产应用只通过 Caddy 对外提供域名入口，内部服务和数据库不公开。
- 服务器项目、域名和端口变化时，先更新第 1 节总览，再更新受影响的项目章节和部署边界。
- 任何未来 AI 开发都必须先读本文件，并把新的明确设计沉淀到这里。

已知交接状态（历史核实，不代表实时线上状态）：2026-09-12 已部署 v20260912-113811-vpn-routing，服务运行及公网健康检查通过；当时 common-routing.json 缺失或为空，因此规则只读展示功能上线不等于规则已配置。后续仍需补齐并验证公共规则、核实魔戒资源的实际展示与客户端订阅可用性。v2rayN 标准节点订阅无法满足规则一并同步的目标，不能仅凭页面展示规则宣称该缺口已解决。

## 14. 后续优化方向

已确认治理批次（2026-10-22，C1–C3）：发布构建共用 Node 入口，PowerShell/CI 不维护第二份打包清单；服务器初始化与日常部署分开；文章抓取页的绑定文案、公众号保留、三个操作按钮改为渲染/回调测试。保留现有技术栈、SQLite 测试能力与轨迹旧密钥兼容。不一次性改写其他页面的源码匹配测试。发布脚本测试使用临时目录和系统命令替身，覆盖包内容、构建失败退出、首次部署、升级、失败恢复及回滚；不能替代真实 Linux/systemd 验收。

后续已确认 C4–C6：移除未调用的 GPSLoggerSetup 与旧手动配置生成函数，轨迹配置以当前设备行复制 URL 为入口，后端旧密钥兼容、轮换和撤销接口保留。工作台路径由 finance.ts 的单一映射维护，保留原 URL、前后斜杠归一化与未知路径回退控制台。文章客户端未配置且来源状态写入失败时，Sync 返回明确错误；微信读书认证/验证异常分支合并凭据保存和健康状态写入失败，保留 errors.Is 对原始错误的识别，不增加自动重试或改变成功文章落库流程。

已确认 C7–C9：删除没有业务调用的 Item 组件及其文件存在断言；麻将热力图测试迁入 web/tests 并接入 npm test，保留原有日期窗口、同日汇总和颜色断言；按用户确认删除旧 FastAPI crontab 校验、清理与恢复函数。当前部署与回滚不依赖这三个函数，本次未访问或修改服务器 crontab，亦未核查仓库外历史调用。

已确认 C10–C16：VPN 由公开根地址和数据库状态直接管理，删除空 Distributor/远端载荷层，保留原 Token 派生及 JSON 字段；配置校验失败同时报告状态落库失败。文章分析统一使用 AnalysisModels 配置，测试与生产使用同一初始化入口。剩余工作台 JSON 客户端复用 requestJSON，保留 ApiError 身份、状态码、业务提示、Blob/FormData 和原有认证处理。删除未使用正文回退及无效参数，测试专用构造器/包装函数移入测试文件，保留裁剪图片落盘行为。

业务时区统一由 internal/timezone 提供，标准库内嵌 tzdata 消除运行环境差异：Shanghai 使用 IANA 历史规则，供日历日期和调度；Beijing 保留原统计/文本格式的固定 UTC+8 契约。Cron 缺省时区及麻将导入不再依赖主机 time.Local。发布 CI 使用 PostgreSQL 15 临时服务和 analytics_test/chat_archive_test 专用库，显式提供全部测试 DSN 和 Python 环境；集成测试缺失或跳过即阻止发布。保留回测/策略、旧聊天归档 DSN 模式，不扩大业务删除范围。

治理衡量采用同一范围下的源码物理行数（含空行/注释），分别列出生产、测试、脚本/CI 及总量，排除依赖、构建产物、锁文件与文档。总量减少是直观指标，同时核对依赖、兼容入口和行为测试；不靠压缩排版、删除必要测试或挪到统计范围外制造下降。

行数基线（C10–C16 前 → 后）：生产 48,367 → 48,174，测试 16,376 → 16,436，脚本/CI 1,934 → 1,975，总计 66,677 → 66,585，净减 92 行。相对上次提交（C1–C16 累计），生产减少 338 行、脚本/CI 减少 35 行、测试增加 458 行，总计增加 85 行；累计总量尚未下降，不能将测试补齐隐藏为“全仓减量”。统计范围为 Git 已跟踪及未忽略新增文件的 `.go/.ts/.tsx/.js/.cjs/.mjs/.py/.sh/.ps1/.yml/.yaml/.css/.sql`；测试文件/目录优先归类为测试，其余 scripts/.github 归类脚本/CI。

C10–C16 本地验证：全量 Go race 测试通过 420 个测试项（含子测试），其中 14 个 PostgreSQL 集成测试无跳过；Go vet、126 项前端测试、前端构建、14 项发布脚本测试、Shell/YAML 语法与 diff 检查通过。未配置独立小票临时服务、可用私有 VPN 资源的两项环境测试仍跳过；未访问生产资源，也未在 GitHub 执行新流水线。测试数据库验证后已停止。此处为优化完成时的验证记录；实际发布状态见后续部署记录。

优化部署记录（2026-10-22）：C1–C16 代码提交 `ffad8d9aa6a39ae349b3bc627182c29be641e09c` 已推送 main，生产版本为 `/opt/aowugong-go/releases/v20261022-simplify-c1-c16`，上一版本 `v20261022-website-operations` 保留为 previous。统一发布入口完成前端 126 项测试/构建、Go 全量普通/race 测试和 vet，显式使用隔离 PostgreSQL；14 项发布脚本测试通过。16 个迁移文件与发布前生产完全一致，无新增 schema 变更。内外网健康、主页/网站运维/我的轨迹页面及入口资源一致性、四个敏感 API 匿名 401 检查通过，JWT/加密/数据库/小票统计凭据摘要前后一致。运行配置和服务文件备份在 `/opt/aowugong-go/deploy-simplify.SBtIbL/before`，仅 root 可访问。未发送外部通知，未修改其他服务；本地测试数据库已停止。此后如仅提交本段部署记录，线上 REVISION 仍对应上述代码提交。

全项目扫描批次（2026-10-29，N1–N10，本地完成、未提交未部署）：Vaultwarden 恢复包补齐告警脚本；前端权限策略统一到 `lib/permissions.ts`，Dashboard 收口为 6 个数据页并删除 18 个死端点；微信读书新公众号默认关闭、人工开关接回 PATCH，删除无调用的凭据体检/保活；Miniflux 文章抓取客户端、配置和旧路径移除，Miniflux 继续作为独立 RSS 服务；时间文本统一使用上海时区，前端新增 `lib/beijing-time.ts`；回测摘要改为“未接入”，一次性迁移二进制不再进发布包、按需构建；清理无引用前端 API。复查后补充 R1–R3/N9：微信读书设置与人工开关改为单条 SQL 原子更新，避免部分写入；北京时间测试直接加载生产 `lib/beijing-time.ts`，覆盖解析、格式化、日期加减和跨 realm Date；删除 `canViewChatArchive` 与导航 `permission` 字段，页面权限统一由 `canAccessPage` 判断；通知 Token 与企业微信 Webhook 的组合校验收口到 `Config.Validate`；新增 `web/tests/admin-pages.test.cjs`，覆盖数据库页的首个非空表选择、敏感值/空值渲染、搜索防抖、分页与筛选导出，以及权限页的角色分配、失败恢复和无角色提示。本轮再复查补充 B1–B5：清理 Miniflux 应用配置脚本中已废弃的 Token/分类键和测试夹具；记录此刻页与文章日期改用北京时间 helper；删除微信读书旧设置/开关入口，只保留原子更新并区分 RowsAffected 错误；前端测试改为目录级发现；删除无调用的抓取摘要和公众号刷新接口；随后补充 C1–C3：删除来源包装层和死 helper，修正 Miniflux 配置注释。仍保留两项：开发 JWT 兜底、position/client 超时默认值保留，原因是安全兜底和显式传参；deploy 测试对 macOS 临时目录父路径的放行属于测试折中。行数（README 口径，扫描前 68,078 → 后 67,577，净减 501；生产 50,224 → 49,660，测试 15,738 → 15,814，脚本/CI 2,116 → 2,103）。本地验证：Go 全量测试、vet、前端构建、136 项前端测试、14 项发布脚本测试、Shell 语法与 diff 检查通过；未访问生产、未发送通知。

- 日志与可观测性：系统复杂后，评估引入 Wide Event / Canonical Log Line，为请求、任务、消息、批量处理和人工覆盖统一记录上下文，让正常流程清晰、例外可追踪和可审计。当前只记录为优化方向，暂不实施具体架构。
- 资料来源：[Logging Sucks](https://loggingsucks.com/)、[logging-best-practices skill](https://github.com/boristane/agent-skills/tree/main/skills/logging-best-practices)。

## 访问统计（2026-10-22）

已确认范围：本 Go 项目的全部页面（公开主页、博客、登录、工作台），不含独立子站与 API/资源请求。入口为「网站运维 → 访问统计」，管理员专用。复用 Go/chi、PostgreSQL、React/shadcn 和既有认证权限，不增加服务实例。

实施计划与口径：
- [x] `internal/analytics`、迁移 16：匿名访客按服务端 HMAC(IP、UA、月度周期)识别，不保存原始 IP/UA，不关联账号；30 分钟无浏览开始新访问。服务端时间为准，事件 UUID 幂等，访问更新与事件插入同一事务。
- [x] `internal/httpserver/analytics_handlers.go`：同源采集、大小限制、全局限流、可信代理校验；管理员统计接口使用参数化查询与查询超时。
- [x] `web/src/lib/analytics-tracker.ts`：启动一次，采集首次加载与 history/popstate 路由变化，忽略 hash、非白名单查询参数及相同地址的重复路由通知；刷新仍计浏览。排除当前浏览器可选，默认包含管理员。
- [x] `web/src/pages/analytics`：五指标及前期对比、趋势、网页/来源/环境/位置排行、地图、星期小时热力图，共用时间和精确匹配维度筛选；北京时间展示。
- [x] 接入导航、权限和启动配置；验证匿名识别、敏感参数剔除、并发/幂等、过滤、边界、管理员权限，运行 Go/前端检查。

统计口径：范围采用 [from,to)，访客为范围内去重匿名标识（跨月可能重复），访问次数为去重 visit_id；跳出率与时长按范围及筛选后每次访问的浏览记录计算，单次仅一个浏览记为跳出、时长 0；多页首末浏览时间差之和除以访问次数，不代表活跃阅读时长。入口/退出页是整次访问的首末页，进行中的退出页可能变化。前期对比使用相邻等长区间，同样的筛选。URL 只保留 utm_source/utm_medium/utm_campaign，来源仅存外部域名。渠道为简单规则分类，无法识别时为 referral/unknown。


运行配置与保留策略：
- `AOWUGONG_ANALYTICS_GEOIP_PATH` 可选，指向 GeoLite2 City 兼容 MMDB；未配置时位置为空、看板明确提示，不影响其他统计。地理库需运维自行取得并定期替换，启动时读入内存，更新后重启生效；不向外部服务查询访客 IP。
- `AOWUGONG_ANALYTICS_TRUSTED_PROXIES` 为逗号分隔 CIDR，默认只信任回环地址。生产 Caddy 若经 Docker 网桥连接，部署前核对实际代理地址并仅添加必要 CIDR；按 X-Forwarded-For 右至左剥离可信代理，不信任任意客户端头。
- 采集只接受与 `AOWUGONG_PUBLIC_URL` 对应的 Origin/Host，默认本地开发代理不能向生产统计写入。匿名标识使用现有有效 JWT 密钥加独立用途标签做 HMAC；轮换该密钥会改变访客身份，统计不记录账号和原始 IP/UA。浏览器不生成持久访客 Cookie。
- 访问明细保留 365 天；成功采集后至多每分钟清理 100 次最后活动已超过 365 天的访问及关联事件，只涉及两张 analytics 表。无流量时清理暂停，历史数据不导入，不生成外部通知。
- 管理员接口 `GET /api/v1/analytics/report/`，公开同源采集 `POST /api/v1/analytics/collect`；读取在可重复读快照内执行，范围最多 366 天，SQL 超时 8 秒、请求超时 15 秒。入口/退出排行按访问次数，其他排行及地图按浏览量；普通维度最多 100 项，国家最多 300 项。
- 世界地图 `web/src/pages/analytics/world.json` 从 Natural Earth `ne_110m_admin_0_countries.geojson` 转换为等距圆柱投影 SVG 路径，公共领域；来源：https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_110m_admin_0_countries.geojson 。Umami 仅参考字段、统计与交互设计，未直接复制其实现代码。
- 全量采集包含统计页本身；默认记录本人访问。排除开关仅作用当前浏览器之后的浏览，不删除历史记录。采集失败不自动重试，因此短暂故障、拦截器及禁用 JS 会导致少计。

本地验证记录：Go 全量测试与 vet 通过；前端 113 项测试与 `npm run build` 通过。临时 PostgreSQL 独立 schema 下的 race 测试验证并发幂等、30 分钟新访问、指标/入口计数、筛选参数化、时间右边界、敏感参数剔除和 365 天清理。浏览器使用临时管理员实际验证全站采集、管理员看板、无匹配筛选与清除筛选。测试截图位于 `.cache/analytics-preview.png`，全部为本地测试数据。已发布并完成生产迁移；地理库尚未配置，生产 Caddy 为 host 网络，仅信任回环代理。

## 小票分账统计（2026-10-22）

入口「网站运维 → 小票分账」，管理员专用。receipt-split 负责 SQLite 只读聚合，本服务通过回环 HTTP 和独立 Key 读取汇总，不读取另一项目数据库，不返回身份/账单内容，不修改小程序。

实施计划（沿用 writing-plans / executing-plans / 测试驱动流程）：
- [x] receipt-split 日期聚合与每日补零：历史登录用户、周期识别用户/任务及状态、账单人数/数量；排除 local-developer，可选排除运营者。历史登录用户不受时间筛选影响，不声称支持日活/新增用户。
- [x] `/internal/statistics` GET：默认关闭，回环直连、拒绝转发头、专用 Bearer Key、参数校验和超时。北京时间 [from,to)，最多 366 天，查询只读快照。
- [x] `internal/receiptsplit` 受限客户端（回环、拒绝重定向、响应限制）和 30 秒/32 组缓存，管理员 `/api/v1/receipt-split/statistics`。
- [x] React 独立页面：日期、运营者开关、指标、趋势和每日明细，配置缺失/失败/无数据状态；导航与权限接入。
- [x] 双项目测试、前端构建与链路验收、同步文档。已发布并完成线上接口检查，不触发通知。


运行配置：`AOWUGONG_RECEIPT_SPLIT_URL` 设置为 `http://127.0.0.1:18080`，`AOWUGONG_RECEIPT_SPLIT_STATS_KEY` 配置与小票后端 `RECEIPT_STATS_KEY` 一致的独立 Key；只读取本项目显式配置，两者均空表示未配置，半配置或非回环 URL 导致启动校验失败。不读取小票项目私有配置文件，浏览器不接触 Key。仅允许 HTTP 回环 IP 根地址，拒绝重定向，HTTP 超时 5 秒、响应上限 1 MiB；响应版本、日期、日序列和计数结构校验失败时显示不可用，不伪造零值。缓存最多 32 组、每组 30 秒；错误不入缓存，不返回过期旧数据。

工作台路径 `/work/website/receipt-split`，权限 `page:website:receipt_split`，同时强制管理员角色。数据接口 `/api/v1/receipt-split/statistics` 接受 from/to 日期和 exclude_operators 布尔值，默认排除运营者；未设置名单时只排除固定开发账号并明确提示。上游接口不经 Caddy，本服务不修改 receipt-split 数据。统计含历史登录用户、识别用户/任务、账单用户/数量、已完成任务成功率、每日趋势与明细；不声称覆盖完整日活、新增或匿名分享访问。

本次发布 Go 工作台并配置回环统计连接；小票后端已部署，小程序无改动。运行时只读取 Go 项目自己的连接配置，密钥由部署流程从已声明的小票服务配置同步。

后续部署状态（2026-10-22）：用户另行授权后，receipt-split 后端已发布源码 `0485893`，版本目录 `/opt/receipt-split/releases/20261022-048589373e54`；回环统计接口和公网健康检查通过，独立 Key 已保存在其私有配置，运营者名单未配置。aowugong-go 页面和连接配置已补齐，线上管理员统计接口检查通过。

本地验证记录：两个 Go 项目全量测试与 vet 通过，receipt-split store/server race 测试通过；工作台前端 114 项测试及构建通过，小程序既有 66 项测试通过。使用独立临时 SQLite/PostgreSQL、demo OCR、关闭通知的本地服务完成实际接口联调及浏览器验收：日期筛选、每日补零、运营者排除前后计数一致；管理员鉴权、缓存过期后上游故障、不完整日期响应均有回归测试。验收截图 `.cache/receipt-split-preview.png` 使用合成数据；测试服务验收后关闭；本次发布前重新执行必要检查。

## GPSLogger 我的轨迹（2026-10-22）

已确认设计：Android GPSLogger 经 HTTPS POST 上传到现有 Go/chi 后端，保存现有 PostgreSQL；工作台「个人工具 → 我的轨迹」仅管理员使用，设备和轨迹按当前账号隔离。不新增 Java 服务，不做地图、地址解析或 AI 分析，不自动清理历史记录。已提交 main 并部署生产，代码版本 `b0858672a2b122c08440087b9cf0f8814fe57c9a`。

实施计划（brainstorming / writing-plans / executing-plans）：
- [x] 数据层：迁移 15；设备 UUID、所属账号、密钥 SHA256、撤销时间；轨迹包含设备、定位/接收时间、WGS84 坐标、精度、速度(m/s)、方向、海拔、来源及白名单原始字段。验证坐标/时间/可选数值，设备+时间+坐标幂等；允许乱序和断网补传。
- [x] HTTP：`POST /api/v1/trajectory/collect` 只接受 form 请求体和独立 Bearer 上传密钥；拒绝 URL 参数，限制 8 KiB、并发和速率，不记录正文/密钥。管理 API 复用登录、管理员和页面权限，按当前用户隔离设备创建/轮换/撤销和日期分页查询；原始密钥只在创建/轮换响应中出现。
- [x] 工作台：`/work/personal/trajectory`；今日点数、最后定位/接收时间，设备和日期筛选、50 条分页明细、设备管理和 GPSLogger 配置复制；日期北京时间，设备失联只展示最后接收时间，不伪称实时在线。
- [x] 验证：解析异常/时间边界、重复并发、撤销与轮换、跨账号隔离、普通用户禁止、分页、前端日期和请求格式；本地 PostgreSQL 和浏览器链路验收，Go 全量测试/vet、前端测试/构建。

实现决策：上传格式 `application/x-www-form-urlencoded`，GPSLogger body `timestamp=%TIMESTAMP&lat=%LAT&lon=%LON&acc=%ACC&alt=%ALT&spd=%SPD&dir=%DIR&prov=%PROV`；定位时间为 Unix 秒，数据库存 UTC，查询按北京时间 [from,to)，最长 366 天。定位时间允许 2000 年起至服务器时间后 10 分钟；重复点保留首次内容和接收时间，另更新设备最后接收时间。密钥轮换立刻使旧密钥失效（旧请求队列需要重新发送），撤销保留设备和历史记录。设备上限每账号 20 台；首次凭证关闭后不能重新读取，可轮换。

验收关注：断网补传不当成今天定位；空可选值与 0 区分；旧密钥并发写入不能越过撤销；分页在新点写入时使用固定查询快照；查询错误不显示为零条。轨迹表不进入通用数据库浏览器，避免绕过账号隔离。手机实际后台保活、重启和耗电需要安装后实机验收。

本地验收：独立 PostgreSQL schema 的 race 测试通过，覆盖并发重复上传、分页期间新点插入、跨账号隔离、密钥轮换和撤销。浏览器验证管理员导航、空状态、创建设备、一次性密钥隐藏及有数据列表；模拟 GPSLogger 经真实 HTTP 接口上传 5 个点，重复点未新增、昨日补传未计入今日。Go 全量测试和 vet、前端 117 项测试与构建通过。截图 `.cache/trajectory-preview.png` 为合成数据。尚未在真实手机运行；后台保活、耗电和手机网络恢复补传仍需实机验收。

手机配置入口：登录 https://aowugong.top/work/personal/trajectory，每个有效设备仅显示「复制配置 URL」，粘贴到 GPSLogger「从 URL」。已有设备无需轮换密钥，复制配置不修改数据库凭证或影响已配置手机。配置使用兼容凭证（设备 UUID + 128 位 HMAC，绑定原密钥摘要，以现有加密配置密钥和独立用途标识签名）；原随机密钥仍可上传。轮换原密钥、撤销设备或停用账号会使相应凭证失效；服务器加密密钥变更使兼容凭证失效，但不影响原随机密钥。数据库仍只存原密钥摘要，不保存明文或新增表。POST /devices/{id}/profile 复用管理员权限与所属账号检查。临时下载链接 15 分钟有效、仅内存保存、不缓存；复制可重新生成链接。配置含 HTTPS POST、60 秒记录、本地 CSV 备份和离线补传。手机需手动授权后台定位及取消电池优化。

生产发布：2026-10-22，版本 `v20261022-trajectory-copyprofile`，设备操作仅「复制配置 URL」，已有设备原密钥保持有效。发布前在隔离 PostgreSQL 验证原密钥和兼容凭证上传、跨账号隔离、轮换撤销失效及分页，race 测试通过；轨迹/httpserver/app vet、前端 113 项测试及构建通过。备份 `/opt/aowugong-go/shared/backups/trajectory-copyprofile-20261022T060313Z`，数据库仍为迁移 15；部署前后设备密钥摘要一致，服务健康、权限拒绝和公网前端一致性检查通过。上一版本 `/opt/aowugong-go/releases/v20261022-trajectory-import` 保留供回滚。现有手机已确认手动配置上传成功；兼容配置文件的真实安卓导入仍待手机验收。

网站运维生产发布（2026-10-22）：已推送 main，代码 `4fbdbb45390b44781921eaffb73c96cb027974e9`，线上版本 `/opt/aowugong-go/releases/v20261022-website-operations`。数据库备份 `/opt/aowugong-go/shared/backups/website-operations-20261022T063722Z`，迁移 16 已完成；原轨迹密钥摘要保持不变。小票统计密钥仅在服务器内同步至 Go 私有环境配置，Caddy host 网络，可信代理为 `127.0.0.1/32,::1/128`。Go 全量测试/vet、访问统计与轨迹隔离 PostgreSQL race 测试、HTTP/小票客户端 race 测试及前端 117 项测试/构建通过。生产健康检查、管理员统计读取、匿名访问拒绝、网站运维页面与发布文件一致性和采集入口检查通过；采集验收使用已知机器人 UA，不注入测试浏览事件。访问统计从本次上线起采集，未导入 Umami 历史；小票读取已有历史汇总。地理位置库和运营者排除名单尚未配置，界面保留提示。上一版本 `v20261022-trajectory-copyprofile` 保留供回滚。

发布完整性核对（2026-10-22）：网站运维、轨迹及聊天归档服务端均已在 main 和生产。补提交电脑端 `skills/chat-archive-sync`，仅发布源码及测试，不包含私有配置、不执行同步、不上传真实聊天。Python 测试 54 项，43 项通过、11 项旧 PostgreSQL 存储测试因未设置独立 DSN 跳过。CI 与 PowerShell 打包入口通过 Git 归档纳入已提交 skills，避免遗漏且排除本地凭证/缓存；PowerShell 入口在本机未实跑。`.DS_Store`、`LOG/`、技能 `config.json` 是非发布文件，保留本地并忽略。
