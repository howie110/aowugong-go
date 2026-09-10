# 嗷呜公 · 个人项目与服务器总控手册

这是嗷呜公个人项目、服务器和相关线上服务的总控手册。当前仓库是维护其他项目和服务器的根据地；根目录的 README.md 是唯一持续维护的设计与开发事实来源。

阅读关系：

- 先读本文件，了解产品、服务器、域名、数据和发布边界。
- 再读 AGENTS.md，遵守删除文件、生产操作、部署和外部通知等安全约束。
- 最后根据任务进入代码、脚本或配置。代码与旧文档和本手册冲突时，以用户最新明确确认的设计为准，并在同一次变更中更新本手册。

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
| aowugong-go | /opt/aowugong-go/current，aowugong-go.service | 根域名主页、工作台、API、VPN 订阅、图片代理、定时任务 | 本仓库直接维护 |
| aowugong-blog | /opt/aowugong-blog/current，静态文件 | Astro 博客 | 独立项目；本仓库只记录域名契约 |
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
| https://aowugong.top | aowugong-go：127.0.0.1:12345 | 公开备案主页、登录工作台、API、VPN 订阅和图片代理 |
| https://www.aowugong.top | Caddy 重定向到根域名 | 兼容旧入口；旧博客 RSS 路径保留直出 |
| https://blog.aowugong.top | /srv/aowugong-blog/current | Astro 博客静态站点 |
| https://vault.aowugong.top | 127.0.0.1:8222 | Vaultwarden |
| https://miniflux.aowugong.top | 127.0.0.1:5000 | Miniflux |
| https://nextflux.aowugong.top | 127.0.0.1:5001 | Nextflux 静态前端 |
| https://pic.aowugong.top | aowugong-go：127.0.0.1:12345 | 图片上传后的自定义访问域名和 Go 图片代理 |
| https://movie.aowugong.top | /srv/aowugong-movie/current | Movie-Images 静态站点 |

根域名的特殊路径：

- / 是公开的“嗷呜公 · 工具分享”备案主页。
- /work 是登录后的工作台。
- /work/navigation 是私有工作导航。
- /api/ 是 Go API，包括 VPN 订阅接口。
- /blog/rss.xml、/feeds/rss-style.xsl 以及旧的 www RSS 地址继续服务兼容内容，不占用新的博客站点路由。

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
- aowugong-blog、aowugong-movie 和 nextflux 也采用 releases + current 的静态发布约定，但由各自项目负责构建。

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
| 总览 | 控制台、工作导航 | 控制台是公开入口；工作导航需要登录 |
| 投资研究 | 投资文章分析、投资文章抓取、股票仓位分析、股票仓位导入 | 登录后按角色使用 |
| 量化工具 | 回测、数据、交易 | 登录后使用；真实交易默认关闭 |
| 内容服务 | 微信读书、麻将战绩、订阅管理 | 登录后使用 |
| 资源分享 | VPN 分配、VPN 资源 | VPN 分配仅管理员；VPN 资源只看自己的资源 |
| 系统运维 | 监控管理、定时任务、通知、数据库、权限管理 | 管理员或对应权限使用 |

权限和数据原则：

- 普通用户只能读取自己的订阅、资源和个人数据。
- 管理员可维护用户和资源分配，也可查看公共规则原文，但不能把私有节点内容暴露到普通用户页面。
- VPN 用户是使用 VPN 资源页面和订阅能力的角色；管理员账号也可以被分配 DMIT、魔戒等资源，管理员身份不代表自动拥有某一套资源。
- 工作台入口不从公开根页面暴露；公开备案主页不依赖登录。

### 3.3 投资研究和内容服务

- 微信读书通过扫码绑定账号并从书架发现公众号；人工启用的公众号由 08:00、20:00 任务检查最近 20 篇文章。
- 只为数据库未知文章读取详情和微信公众号原文，不依赖外部 RSS 聚合。
- 登录凭据使用 AOWUGONG_ENCRYPTION_KEY 派生的 AES-256-GCM 密钥加密后存入 PostgreSQL；二维码中间态只保存在当前 Go 进程内。
- Go 按公众号生成回环地址 WeRead RSS，供同机 Miniflux 分源保存和阅读全文。
- 投资文章分析支持按文章、概念和成员筛选。
- 股票仓位支持截图导入和敏感信息遮罩。
- 私有工作导航保存在 storage/private/work/navigation.json，不放入数据库或公开页面。
- 真实交易开关 FINANCE_ENABLE_REAL_TRADE 默认是 false，任何打开真实交易的动作都必须单独确认。

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

DMIT、魔戒等只是不同的上游资源来源。进入系统后都走相同的资源分配、转换、订阅和刷新流程；资源来源不会改变用户使用方式。管理员账号和普通账号一样，只有被分配后才拥有对应资源。

### 4.2 原始资源和统一输出

原始资源边界：

- 本地原始文件位于 storage/private/vpn，生产原始文件位于 /opt/aowugong-go/shared/storage/private/vpn。
- 目录被 Git 忽略，文件可能包含服务器地址、Token、UUID 和私有订阅链接。
- 资源文件按文件名中的资源编码归组；资源来源可以不同，但统一转换为客户端可消费的订阅。
- 每套输出都包含“资源节点配置 + 当前公共分流规则”，不是只返回孤立节点。

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
- 管理员页面只读展示规则原文；规则由 AI 按用户要求修改并部署。
- 每次生成订阅都会重新读取该文件；部署新文件后，用户刷新订阅即可得到新规则。
- 规则目标统一为 proxy、direct、block，分别表示代理、直连和拒绝。
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
- 普通用户不能读取其他用户的资源或公共规则原文。
- 节点正文、规则正文和订阅 Token 不写入日志、数据库、浏览器或公开 Git。
- 发布 VPN 代码或规则前，先用未连接代理的设备确认订阅 URL 可以访问，再在四类客户端分别测试刷新和导入。

## 5. 图片上传和阿里云凭证

### 5.1 上传链路

- 图片存储在阿里云 OSS。
- Go 服务在同地域使用 OSS 内网 Endpoint 读取，减少线上读取成本和跨网问题。
- PicGo 使用外网 Endpoint 上传，上传后通过自定义域名访问。
- 当前 PicGo 约定：Bucket 为 aowugong-pic-gz-f2bf98d3，地域为 oss-cn-guangzhou，存储路径为 pic/，自定义域名为 https://pic.aowugong.top。
- PicGo 是本机应用配置，不由 Go 服务直接读取；本机上传配置和服务器读取配置是同一 OSS 资源的不同使用端。

### 5.2 凭证原则

- 只使用阿里云 RAM 子账号 AccessKey；主账号 AccessKey 已禁用。
- RAM 权限按最小范围配置，只允许当前图片上传、读取或代理所需的 Bucket 和路径。
- 本机项目根目录 .env 可以保存私有工具和服务配置，文件权限应为 600 并始终被 Git 忽略。
- PicGo 的密钥输入框、项目 .env、系统钥匙串和服务器 .env 都不能复制到 README、日志、截图或发布包。
- 如果 RAM 权限发生变化，先确认需要的动作，再使用有权限的阿里云账号调整，不重新启用主账号 AccessKey。

## 6. 数据、私有文件和备份

### 6.1 数据边界

| 数据 | 正式位置 | 是否进 Git | 说明 |
|---|---|---|---|
| PostgreSQL 业务数据 | 服务器 PostgreSQL 15 | 否 | 账户、文章、投资、订阅和系统业务数据 |
| VPN 原始配置 | shared/storage/private/vpn | 否 | 节点、Token、UUID 和私有订阅内容 |
| 工作导航 | shared/storage/private/work/navigation.json | 否 | 私有导航配置 |
| 服务器环境变量 | shared/.env、shared/.env.canary | 否 | 生产和 canary 配置、凭证 |
| 本地环境变量 | 项目根目录 .env | 否 | 本地运行和本机工具配置 |
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

    pwsh -File ./scripts/run-local.ps1

访问 http://127.0.0.1:2345，停止时按 Ctrl+C。脚本会加载项目根目录 .env，并强制设置：

- AOWUGONG_DEV_UPSTREAM_URL=https://aowugong.top
- AOWUGONG_HTTP_ADDRESS=127.0.0.1:2345
- AOWUGONG_SCHEDULER_ENABLED=false
- FINANCE_ENABLE_REAL_TRADE=false

如果要补跑或修改线上数据，应通过 SSH 在服务器加载正式环境，并调用统一 CLI，而不是让本地开发进程直接连接生产数据库：

    ./scripts/run-remote-job.ps1 sync_investment_articles

## 9. 定时任务和统一 CLI

调度时区固定为 Asia/Shanghai。

| 时间 | 任务 | 作用 |
|---|---|---|
| 08:00、20:00 | sync_investment_articles | 从微信读书书架公众号增量抓取并分析投资文章 |
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

常用 CLI：

    /opt/aowugong-go/current/aowugong job backup_postgres
    /opt/aowugong-go/current/aowugong job backup_github_code
    /opt/aowugong-go/current/aowugong job email_vaultwarden_backup

## 10. 构建、部署和回滚

### 10.1 发布前验证

文档或代码变更完成后，按影响范围验证。完整验证命令：

    GOTOOLCHAIN=auto go test ./...
    GOTOOLCHAIN=auto go test -race ./...
    GOTOOLCHAIN=auto go vet ./...
    cd web
    npm ci
    npm test
    npm run build
    cd ..
    for script in scripts/*.sh; do bash -n "$script"; done

还应运行 git diff --check，确认没有空白错误、凭证和私有资源文件进入变更。

### 10.2 正式发布

本地构建 Linux amd64 发布包：

    ./scripts/build-release.ps1 -Version v1.0.0

发布包包含：

- Linux amd64 Go 二进制。
- web/dist 静态资源。
- PostgreSQL 版本化迁移。
- systemd 模板、部署脚本、回滚脚本和本 README。

发布包不包含 .env、VPN 原始文件、工作导航、节点内容、数据库备份或任何生产私有配置。Git tag 会触发 GitHub Actions，执行前后端测试、Race、Vet 和构建。

部署只在用户明确要求时执行：

    sudo DEPLOY_MODE=main /opt/aowugong-go/current/scripts/deploy-release.sh v1.0.0

部署动作限于构建、上传、原子切换 current、重启 aowugong-go.service 和健康检查；不自动发送钉钉、邮件、微信等通知。正式发布前确认 migration 向后兼容，并保留数据库备份。

### 10.3 Canary 和回滚

- Canary 使用 DEPLOY_MODE=canary，监听 2346，并关闭调度器。
- 正式发布把旧版本保存为 previous。
- 回滚只切换应用发布产物，不自动回滚 PostgreSQL schema、业务数据、VPN 私有文件或服务器 .env。

    sudo /opt/aowugong-go/current/scripts/rollback.sh

回滚后检查根域名、登录、工作台、API、VPN 订阅和图片代理；如果问题来自数据库迁移或数据内容，必须走单独的数据恢复方案。

### 10.4 独立项目发布边界

- aowugong-blog、aowugong-movie、nextflux 有自己的源码、构建和发布流程。
- 当前仓库只维护它们在服务器上的目录约定、域名分配和联动契约。
- 修改博客、电影或 Nextflux 时，先进入对应项目确认 Git remote、README、部署脚本和当前版本；不要在本仓库伪造或复制一份实现。
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
    AOWUGONG_SQLITE_SOURCE_PATH=/安全路径/aowugong.db \
      /opt/aowugong-go/current/aowugong-migrate --confirm

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
- 公共规则由 storage/private/vpn/common-routing.json 单文件维护，AI 修改后部署，用户刷新订阅生效。
- PicGo 使用阿里云 OSS 的 RAM 子账号；主账号 AccessKey 已禁用。
- 生产应用只通过 Caddy 对外提供域名入口，内部服务和数据库不公开。
- 服务器项目、域名和端口变化时，先更新第 1 节总览，再更新受影响的项目章节和部署边界。
- 任何未来 AI 开发都必须先读本文件，并把新的明确设计沉淀到这里。
