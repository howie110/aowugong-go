# 嗷呜公 · 个人项目与服务器总控手册

这是嗷呜公个人项目、服务器和相关线上服务的总控手册。当前仓库是维护其他项目和服务器的根据地；根目录的 README.md 是唯一持续维护的设计与开发事实来源。

阅读关系：

- 先读本文件，了解产品、服务器、域名、数据和发布边界。
- 再读 AGENTS.md，遵守删除文件、生产操作、部署和外部通知等安全约束。
- 最后根据任务进入代码、脚本或配置。代码与旧文档和本手册冲突时，以用户最新明确确认的设计为准，并在同一次变更中更新本手册。

文档维护要区分已确认设计、代码实现和线上状态。2026-09-23 本次刷新依据本地工作区与已有操作记录；未重新连接服务器核实。下文服务器布局是维护基线，不能代替部署前的现场检查；已知未完成项见第 13 节。

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
| aowugong-moments | /opt/aowugong-moments/current，Docker 容器 | 个人朋友圈、OSS 图片与视频 | 独立 fork；本仓库维护跨项目部署事实 |
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
| https://pic.aowugong.top | `/moments/*` → 127.0.0.1:5002，其余 → 127.0.0.1:12345 | Moments 媒体与原有 Go 图片代理 |
| https://moments.aowugong.top | aowugong-moments：127.0.0.1:5002 | 个人朋友圈 |
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
| 5002 | 127.0.0.1 | Moments Docker 端口映射 | Caddy 转发站点和媒体请求 |
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

- 微信读书通过扫码绑定账号并从书架发现公众号；人工启用的公众号由手动抓取任务检查最近 20 篇文章，不再按 08:00、20:00 自动抓取。
- 只为数据库未知文章读取详情和微信公众号原文，不依赖外部 RSS 聚合。
- 登录凭据使用 AOWUGONG_ENCRYPTION_KEY 派生的 AES-256-GCM 密钥加密后存入 PostgreSQL；二维码中间态只保存在当前 Go 进程内。
- Go 按公众号生成回环地址 WeRead RSS，供同机 Miniflux 分源保存和阅读全文。
- 投资文章分析支持按文章、概念和成员筛选。
- 股票仓位支持截图导入和敏感信息遮罩。
- 私有工作导航保存在 storage/private/work/navigation.json，不放入数据库或公开页面。
- 真实交易开关 FINANCE_ENABLE_REAL_TRADE 默认是 false，任何打开真实交易的动作都必须单独确认。

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

### 3.6 博客合并设计（2026-09-30，用户已确认开工，尚未实施）

#### 已确认目标与当前差异

- 博客统一进入 `https://aowugong.top/blog`，复用现有 Go、React、PostgreSQL、登录和权限；退出 Astro 构建链和独立 Moments 服务，保持单一应用。
- 文章继续由笔记项目的博客 Markdown 提供，推送后自动更新网站；状态改由数据库保存，在个人工作台「内容服务 → 状态」通过手机发布。
- 设计要求是单向、直接、可理解：每类内容只有一个权威来源，不做双写、双向同步、多框架并存或切换到另一套内容源的兜底。
- 本地已核实：博客源码实际位于 `/Users/howie/project/aowugong-astro`；笔记位于 `/Users/howie/project/aowugong-note/【7】博客`。现有笔记工作流将该目录同步提交到 Astro 仓库，再由 Astro 工作流构建部署。
- 当前动态是 `blog-2000-01-05.md`，正文由分隔线和时间标记组织。第 1、5、10 节仍记录迁移前部署基线，不能据此节宣称新博客已上线或 Moments 已清除。

#### 页面与模块

| 入口 | 用途 | 数据来源与权限 |
|---|---|---|
| `/blog` | 博客文章列表 | 博客目录内的 Markdown，公开读取 |
| `/blog/posts/:slug` | 文章及关于、咖啡等原有文字页面 | 同一 Markdown 目录，公开读取 |
| `/blog/status` | 按发布时间倒序展示状态 | PostgreSQL，公开读取已发布状态 |
| `/work/content/status` | 手机发布、编辑、删除状态及管理草稿 | 复用工作台登录，仅获授权的管理者可操作 |
| `/blog/rss.xml` | 博客文章订阅 | 与文章页面共用解析结果 |

- 前端使用现有 `web` React 工程，博客有独立公开布局；公开博客路由在登录检查前分流，工作台继续保持鉴权。保留文章、标签、目录、图片查看、代码复制和明暗主题这些现有阅读能力。
- 后端新增 `internal/blog`，Markdown 读取与解析、状态业务与数据库访问按职责分文件；HTTP handler 进入现有 `internal/httpserver`，前端 API 统一放在 `web/src/lib`。沿用 handler → service → repository/client，不引入通用 CMS 或插件系统。
- Markdown 由 Go 端一套解析器处理，读取既有 `title/date/tags/image` 元信息，正文输出经过安全处理的 HTML，目录从同一次解析提取。React 只展示，不再维护第二套 Markdown 解析。
- 迁移核对既有换行、链接、图片、表格和代码块表现；不执行 Markdown 中的脚本。保留文章 slug、RSS、Sitemap 以及每篇文章的标题、描述和 canonical 元信息，不能让所有文章都返回工作台的默认标题。

#### 文章自动更新

固定链路：笔记仓库 `main` 的博客变更 → 该仓库 GitHub Actions → SSH 上传博客文件 → 服务器 Go CLI 校验 → 原子切换文章目录 → 网站读取新内容。

- 本轮必须直接修改笔记项目 `/Users/howie/project/aowugong-note/.github/workflows/sync-blog.yml`，与 Go 接收和校验能力配套交付，不能只改 Go 仓库或仅记录后续事项。触发范围仍限 `【7】博客/**` 和工作流自身。这里的自动更新指推送到 `main` 后更新，本地仅保存未推送不会触发。
- 停止向 Astro 仓库提交文章，停止其独立部署工作流；不经过 Go 仓库再提交文章，不重新构建 React 或 Go，不重启业务服务，不增加轮询任务或公网同步接口。
- 工作流只打包博客目录中的公开 Markdown 及其实际需要的目录内附件；不传输私人笔记库、Git 元数据、凭据或博客目录外文件。已有 OSS 图片 URL 保持不变。拒绝符号链接、路径越界和博客目录外的附件引用，并报告具体文件。
- 服务器内容位于 `/opt/aowugong-go/shared/storage/blog`，应用通过明确的 `AOWUGONG_BLOG_CONTENT_DIR` 配置读取其中的 `current`。上传先进入独立暂存目录，再用同一 Go 二进制的博客校验命令解析全部文章；校验成功才原子替换 `current` 指向。
- 应用每次请求解析实际需要的文件，不增加独立缓存服务、文件监听或刷新接口。一个请求固定使用同一个内容目录版本；文章数量增长后才按实测需要优化。
- 同一笔记发布工作流串行执行；激活时拒绝旧版本覆盖已经上线的新版本。只保留当前和上一份有效内容目录，用于明确的手动回退，不无限累积快照。
- 文件删除在成功发布后反映为文章下线，不复制到其他来源继续展示。同步包为空、解析失败或上传失败时工作流报错且不切换目录；网站仍是上一次成功发布的版本，这属于未发布，不伪装成成功。
- SSH 凭据和固定服务器主机指纹在笔记仓库的 Actions Secrets 配置，只授予博客内容上传和校验所需权限。现有 Secrets 的名称、可用性和服务器权限需在实施时核实，不读取其他项目或全局通知配置；流程不发送外部通知。

#### 状态存储与发布

- 状态存 PostgreSQL 的 `blog_statuses`，保存主键、作者、纯文本正文、有序图片引用、草稿/已发布标记、发布时间、创建和修改时间。图片引用使用 JSONB 数组，不为固定的少量附图引入通用附件系统。
- 状态不是一篇 Markdown 文章。新状态只在手机友好的工作台页面编辑；正文按纯文本保留换行并安全展示，链接可点击，图片支持九宫格及大图查看。
- 第一版支持纯文字或文字配图、最多九张图片、上传进度和失败提示、草稿保存、发布、编辑、删除；视频、评论、点赞不纳入本轮。只有正文或图片至少一项非空才能发布。
- 图片通过现有 Go 应用的鉴权上传接口写入现有 OSS 的独立 `blog/status/` 前缀，复用既有图片访问域名和代理，不复用 Moments 服务。仅增加该前缀所需的写入权限，保留原有 `pic/` 读取边界。
- 图片上传全部成功后才提交状态；失败时保留编辑内容，明确失败项，由用户重试。数据库失败不得显示发布成功；防止连续点击导致重复发布。公开 API 不返回草稿，写接口复用现有 RBAC。
- 旧博客动态 Markdown 一次性导入状态表，保留正文、图片、原时间和顺序。导入前生成核对结果，对不能明确拆分或识别时间的条目报错，不猜时间或静默丢弃；迁移标识防止重复导入。导入验收后该文件不再进入文章同步，源笔记作为历史记录保留。
- 独立 Moments 按用户要求退役清理，不建立双向同步或常驻兼容适配层。Moments 数据不作为新状态来源；清理前核实其媒体是否被其他内容引用，被引用对象先迁移并更新引用。

#### 切换与清理边界

- 先在隔离环境完成新博客、笔记自动更新、历史动态导入及手机发布验收，再切换公网入口。旧 `blog.aowugong.top` 的文章路径永久跳转到对应 `/blog/posts/:slug`；旧动态路径明确跳转到 `/blog/status`，RSS 保持兼容，未知文章返回 404，不统一跳到首页。
- 应用升级及数据库迁移沿用现有发布和备份方式；文章内容目录独立于应用 release，后续笔记发布不影响应用版本。应用回滚不自动撤销已发布状态、数据库迁移或笔记内容。
- Moments 清理范围：专用运行及停止容器、镜像、服务器发布和数据目录、SQLite 备份及 systemd 备份任务、专用域名 DNS 和 Caddy 路由、媒体 `/moments/*` 路由、OSS `moments/` 对象、专用 CORS 来源、专用 RAM 用户/策略/密钥、项目内私有运行配置，以及本地与 GitHub 上该 Moments 项目的专用代码和部署资源。
- 用户明确要求删除 Moments 二级域名：清除 `moments.aowugong.top` 实际存在的 DNS 解析记录和 Caddy 站点配置，不保留该域名跳转；删除后旧朋友圈域名停止服务。`pic.aowugong.top` 继续供博客及其他图片使用，只移除其 Moments 专用路径转发。
- 删除前逐项只读核实真实目标与引用关系，并向用户说明实际影响；不按名称模糊匹配批量删除。用户已要求清除 Moments，但未知共享资源须先查清归属，不能扩大删除范围。
- 共用 OSS Bucket、`pic/` 图片、其他 RAM 身份、Caddy 主服务及其他域名必须保留。仅移除 Moments 对应的配置条目；如入口更新需要重启 Caddy，先说明对其他站点的短暂影响。
- 新博客上线后移除旧 Astro 发布链和服务器静态发布目录，迁移所需静态资产时保留必要许可声明。旧博客源码仓库的删除不由 Moments 清理请求自动授权。
- 用户提供的阿里云凭据文件只在云资源核查和操作时读取，不复制内容到代码、日志、聊天、Git 或部署包；核实有效身份与实际权限，不推断管理 Key 已启用，也不擅自启用失效密钥。

#### 验收与尚未核实事项

- 文章解析用现有内容样本覆盖元信息、换行、图片、代码、链接、目录和危险 HTML；缺失文章、损坏文件和越界路径必须明确报错。
- 自动更新验证新增、修改、删除、发布失败不切换及连续推送顺序；确认没有触发 Astro/Go 重建、服务重启、私人目录上传或外部通知。
- 状态验证未登录/无权限拒绝、草稿不可公开、手机选图发布和编辑删除、上传或数据库失败、重复点击，以及历史动态导入完整性。
- 切换验证博客文章、状态、旧链接跳转、RSS、Sitemap、图片及根站/工作台/VPN；清理后核对 Moments 专用资源消失、`moments.aowugong.top` 的 DNS 与入口配置已删除、共享资源继续正常。笔记项目须以一次真实推送验证新工作流自动更新网站。
- 当前尚未核实线上 Moments 数据和引用、Actions Secrets 可用性、SSH 最小权限及云凭据身份；以上属于实施前现场核查项。本节记录已确认设计，不是实现或部署完成记录。

### 3.7 博客合并实施计划（2026-09-30，待执行方式确认）

> 执行技能：本会话逐项执行使用 `superpowers:executing-plans`；若用户选择子代理方式则使用 `superpowers:subagent-driven-development`。以下复选框只在验证结果支持时勾选。

**目标：** 在 Go 主站交付可自动更新的 Markdown 博客和手机状态发布，并退役 Moments。

**架构：** 文章只读文件，状态只读写 PostgreSQL，媒体使用现有 OSS；现有 React 工程提供公开页和管理页。笔记 Actions 直接发布文件到服务器，不增加中转仓库或常驻服务。

**技术栈：** 现有 Go/net/http/chi、database/sql/pgx、React/TypeScript/Vite、PostgreSQL、阿里云 OSS SDK；补充 Go Markdown 解析和 HTML 安全处理库，不增加另一套 Web 框架。

**设计依据：** 本 README 第 3.6 节。默认在本会话顺序实现；实际执行方式在计划审阅时确认。

#### 全局约束与重点检查

- 只维护本 README；保留现有未提交修改和 `LOG/`，不把它们夹带进实现提交。笔记、Astro 当前工作区干净；Moments 存在未提交源码，清理前单独核实和说明。
- 状态最多九张图；公开 API 不返回草稿；凭据不进入前端或 Git；禁止发送第三方通知。所有网络和生产操作使用本项目声明的配置及本任务提供的凭据。
- 关注五类失败：路径/符号链接越界；并发发布或旧版本覆盖；草稿与权限泄露；上传中断及重复提交；历史动态拆分或时区失真。分别由下列任务 1、5、2、3/4、6 验证。
- 实施开始时依据 `using-git-worktrees` 检查隔离工作区；跨仓库修改分别核对差异，不在生产代理开发模式下试写状态。

#### 任务 1：Markdown 内容模块与纯文件 CLI

文件：新增 `internal/blog/articles.go`、`markdown.go`、`articles_test.go`、`markdown_test.go`、`internal/app/blog.go`；修改 `cmd/aowugong/main.go`、`internal/config/config.go`、`internal/app/run.go`、`go.mod`、`go.sum`。

接口：`NewArticleStore(directory string) *ArticleStore`；`(*ArticleStore).List() ([]Article, error)`、`Get(slug string) (Article, error)`、`Validate() error`。`Article` 提供 slug/title/date/tags/html/description/toc；CLI `aowugong blog validate --dir <directory>` 不连接数据库、不启动调度器。

- [ ] 先写 `TestArticleStoreReadsFrontmatterAndBody`（标题、日期、tags、HTML 和目录一致）、`TestArticleStoreRejectsTraversalAndSymlinks`、`TestMarkdownRejectsExecutableHTML`、`TestArticleStoreReportsMalformedFile`；运行 `go test ./internal/blog` 确认新接口缺失导致失败。
- [ ] 实现同一次解析生成正文和目录，明确不把旧动态文件作为文章；新增内容目录配置，并给 CLI 添加独立参数入口，避免走通知任务注册表。补充图片与链接的目录边界校验。
- [ ] 运行 `go test ./internal/blog ./internal/app ./internal/config`，要求全部通过；用笔记博客的临时副本验证全部既有文章，输出仅包含文件名及校验结果。
- [ ] 检查 diff，仅提交任务 1 对应实现与测试。

#### 任务 2：状态表、业务服务和鉴权 API

文件：新增 `migrations/postgres/00010_blog_statuses.sql`、`internal/blog/status.go`、`repository.go`、`status_test.go`、`repository_test.go`、`internal/httpserver/blog_handlers.go`、`blog_handlers_test.go`；修改 `internal/rbac/model.go`、`internal/httpserver/router.go`、`internal/app/run.go`。

接口：`NewStatusService(repository *Repository) *StatusService`；`List(ctx context.Context, publishedOnly bool, limit, offset int) ([]Status, error)`、`Save(ctx context.Context, actorID int64, input StatusInput) (Status, error)`、`Delete(ctx context.Context, id int64) error`。`StatusInput` 包含 id/body/images/published/published_at；创建时客户端生成 UUID 作为 id，数据库主键保证重试不会创建第二条。

- [ ] 先写 `TestPublicStatusesExcludeDrafts`、`TestStatusRequiresContentAndAtMostNineImages`、`TestStatusCreateIsIdempotent`、`TestStatusWriteRequiresPermission`；运行对应 Go 测试确认失败。
- [ ] 实现表结构、参数化 SQL、时间排序及权限 `blog.status.manage`；公开读取 `/api/v1/blog/statuses`，工作台读写 `/api/v1/blog/admin/statuses`，编辑使用 PUT、删除使用 DELETE 并明确区分不存在和无权限。只给管理员角色默认授予管理权限。
- [ ] 在临时 PostgreSQL 验证 JSONB、迁移、重复 id、时间排序和草稿过滤；运行 `go test ./internal/blog ./internal/httpserver ./internal/rbac`，确认鉴权和错误响应通过。
- [ ] 检查 diff，仅提交任务 2 对应实现与测试。

#### 任务 3：状态图片上传

文件：新增 `internal/blog/media.go`、`media_test.go`；扩展任务 2 的 handler 和测试、`internal/config/config.go` 与 `internal/app/run.go`；必要时调整 `internal/pictureproxy` 对新前缀的读取测试。

接口：`(*MediaStore).Upload(ctx context.Context, content io.Reader, contentType string) (Image, error)`；鉴权 `POST /api/v1/blog/admin/images` 返回对象 key、URL。Go 复用现有 OSS SDK，所有写入限定 `blog/status/`，使用专用最小权限配置。

- [ ] 先写 `TestImageUploadRejectsUnauthenticatedAndInvalidContent`、`TestImageUploadUsesBlogPrefix`、`TestImageUploadSurfacesStoreFailure`；覆盖伪造 MIME、超限图片、图片代理对原 `pic/` 的读取不变，运行测试确认失败。
- [ ] 实现每图最多 10 MiB、JPEG/PNG/WebP/GIF 的真实内容识别和上传限制；请求取消传入 OSS 调用，失败返回明确错误。正文接口只接受受控图片对象引用，拒绝任意媒体地址。
- [ ] 运行 `go test ./internal/blog ./internal/httpserver ./internal/pictureproxy ./internal/config`；使用隔离存储或模拟传输验证失败路径，不在此阶段创建生产对象。
- [ ] 检查 diff，仅提交任务 3 对应实现与测试。

#### 任务 4：公开博客与手机工作台

文件：新增 `web/src/lib/blog.ts`、`web/src/pages/blog.tsx`、`web/src/pages/blog-status.tsx`、`web/src/pages/blog/` 下按页面职责拆分的组件和样式；修改 `web/src/main.tsx`、`web/src/lib/finance.ts`、`web/src/components/layout/app-navigation.ts`、`web/src/pages/dashboard.tsx`；新增 `internal/httpserver/blog_pages.go` 及测试。

接口：文章 API `/api/v1/blog/posts` 和 `/api/v1/blog/posts/:slug` 返回任务 1 的内容；`blog.ts` 封装 `listArticles`、`getArticle`、`listStatuses`、`saveStatus`、`deleteStatus`、`uploadStatusImage`，组件不直接拼 API。页面路径沿用第 3.6 节。

- [ ] 先为公开路由、404、文章元信息、RSS/Sitemap 和草稿隔离编写 `blog_pages_test.go`；扩展现有前端 API 客户端测试以覆盖写入参数、错误状态和上传。运行对应测试确认失败。
- [ ] 实现公开布局、文章列表/详情/标签、状态时间线；Go 为文章响应提供对应标题/描述/canonical 并生成 RSS/Sitemap，不引入 Node 服务端运行。迁移必要样式和静态资产时保留许可。
- [ ] 在内容服务加入「状态」，实现草稿保存、最多九图、上传进度/失败重试及编辑删除；发布过程中禁用提交，保留失败表单，沿用同一创建 UUID。手机布局不出现横向溢出。
- [ ] 运行 `go test ./internal/httpserver` 以及 `cd web && npm test && npm run build`；在隔离本地环境实际检查桌面与手机尺寸，完成一次选图、草稿、发布、编辑、删除及上传中断流程。
- [ ] 检查 diff，仅提交任务 4 对应实现与测试。

#### 任务 5：笔记直接自动发布

文件：修改笔记项目 `.github/workflows/sync-blog.yml`；新增 Go 项目 `internal/blog/publish.go`、`publish_test.go` 和 `scripts/publish-blog-content.sh`；扩展 `internal/app/blog.go`、`cmd/aowugong/main.go`。正式切换时删除 Astro `.github/workflows/deploy-blog.yml`。

接口：`aowugong blog publish --dir <staged> --root <content-root> --sequence <run-number>` 校验并激活目录；与任务 1 共用解析，发布锁和序号存在内容根目录，序号来自同一笔记 workflow 的 `github.run_number`。先上传成功再调用该命令，不调用应用部署脚本。

- [ ] 先写 `TestPublishKeepsCurrentOnInvalidContent`、`TestPublishRejectsOlderSequence`、`TestPublishSerializesActivation`、`TestPublishRemovesDeletedArticle`；在临时目录运行，确认未实现时失败。
- [ ] 实现同文件系统原子切换、当前与上一份保留、明确失败退出码；笔记 Actions 直接打包允许的博客文件，经 SSH 上传和激活，固定主机指纹、串行 concurrency，移除 checkout/提交 Astro 和 `BLOG_SYNC_TOKEN` 依赖。
- [ ] 本地验证 `bash -n scripts/publish-blog-content.sh`、工作流 YAML 解析和 `go test ./internal/blog`；使用虚拟 SSH 命令检查上传目标及不包含私人文件、通知、重建、重启。生产 Secrets 只在任务 7 配置并现场验证。
- [ ] 分别核对 Go 与笔记仓库 diff，各自提交任务 5 变更，不提前推送会触发正式工作流的分支。

#### 任务 6：历史动态一次性导入

文件：新增 `internal/blog/import.go`、`import_test.go`；扩展任务 1 的博客 CLI。接口：`ParseLegacyStatuses(source []byte) ([]StatusInput, error)`，按原文时间使用 Asia/Shanghai；`aowugong blog import-status --file <file>` 默认只输出核对摘要，显式 `--apply --author-id <id>` 才连接数据库写入。

- [ ] 先写 `TestLegacyImportPreservesTimeOrderAndImages`、`TestLegacyImportRejectsAmbiguousBlock`、`TestLegacyImportIsIdempotent`，覆盖正文内分隔线、重复时间、旧图片 URL 和不可识别时间，运行确认失败。
- [ ] 实现明确的旧格式解析、原文位置错误报告和稳定迁移 id；旧格式转换仅留在一次性导入工具，日常状态服务不解析 Markdown。旧 `pic/` 图片迁移允许保留可信引用，不能被公开写接口用来绕过新上传限制。
- [ ] 用原动态文件在临时 PostgreSQL 导入并逐条核对数量、正文、时间和媒体；第二次导入不新增。确认新正文展示与旧内容一致后，将该 MD 排除出笔记发布包。
- [ ] 检查 diff，仅提交任务 6 对应工具与测试。

#### 任务 7：总体验证、上线与 Moments 清理

文件：按核查结果最小修改现有发布配置、服务器 Caddy、笔记 Actions Secrets；更新本 README 第 1、3、5、10、13 节为实际核实状态，不另存含凭据的运维清单。

- [ ] 完成 `go test ./...`、`go test -race ./...`、`go vet ./...`、前端测试/构建、Shell 语法和 `git diff --check`；按 requesting-code-review 技能做全量变更审阅并修复发现的问题。
- [ ] 只读核实生产版本、备份、权限、Moments 专用资源及共享引用，先向用户说明实际重启、数据库写入和删除影响；确认部署授权后按最小范围完成新应用、文章目录、历史状态导入和媒体配置。
- [ ] 先验证新 `/blog`、工作台手机发布及共享站点，再切换旧博客跳转与 RSS；配置笔记仓库 Secrets，推送新工作流，并用一次真实笔记推送验证文章自动更新且 Go 进程未重启，随后移除旧 Astro 部署链。
- [ ] 按第 3.6 节清单逐项清理 Moments，并核对 `moments.aowugong.top` DNS/Caddy 均已删除；处理本地未提交 Moments 文件及远端仓库时先明确告知删除对象。共用 OSS、图片代理和其他域名必须健康。
- [ ] 更新本 README 的实现与部署事实，报告版本、访问入口、自动发布验证和清理结果。任何未完成项如实列出，不发送外部通知。

计划自检：七项覆盖设计中的内容、发布、权限、媒体、历史迁移、域名和退役要求；每项有明确接口与验收。现有源码入口已核对，云资源及 Actions Secret 的可用性须在执行阶段现场确认。

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
- 管理员分配页和用户资源页只读展示规则原文；规则由 AI 按用户要求修改并部署。
- 每次生成订阅都会重新读取该文件；更新私有规则文件后，支持合并规则的客户端刷新订阅即可得到新规则。v2rayN/v2rayNG 标准节点订阅不携带这些规则。
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

- 正式服务只使用阿里云 RAM 子账号 AccessKey。2026-09-29 用户为 Moments 配置提供了临时云管理 Key，配置后由用户禁用；其当前禁用状态不能沿用历史记录推断。
- RAM 权限按最小范围配置，只允许当前图片上传、读取或代理所需的 Bucket 和路径。
- 本机项目根目录 .env 可以保存私有工具和服务配置，文件权限应为 600 并始终被 Git 忽略。
- PicGo 的密钥输入框、项目 .env、系统钥匙串和服务器 .env 都不能复制到 README、日志、截图或发布包。
- 如果 RAM 权限发生变化，先确认需要的动作，再使用有权限的阿里云账号调整，不重新启用主账号 AccessKey。

### 5.3 Moments 媒体存储接入

- 2026-09-29 已上线 `https://moments.aowugong.top`，域名 A 记录为服务器地址，Caddy 自动签发和续期 HTTPS。站点标题为“嗷呜公的朋友圈”，页面底部 GitHub 图标已移除；初始关闭注册、评论、邮件通知，默认管理员密码在公网开放前已替换。
- fork 为 `howie110/aowugong-moments`，本机源码在 `/Users/howie/project/aowugong-moments`、分支 `codex/moments-oss-deploy`，本次修改尚未提交或推送。正式镜像 `aowugong-moments:20260929-footer1` 从该源码构建，容器同名，仅映射 `127.0.0.1:5002:3000`，限制 256 MB 内存、1 CPU、128 个进程，以 UID 10001 运行。
- 服务器发布目录为 `/opt/aowugong-moments/releases/20260929-footer1`，`current` 指向该版本；持久化数据在 `shared/data/db.sqlite`，运行配置在 `shared/.env`。此前构建的镜像和已停止容器保留用于回退；回退应用版本前仍需考虑数据库兼容性。
- 用户已确认图片和视频使用现有私有 OSS Bucket `aowugong-pic-gz-f2bf98d3`，独立存入 `moments/YYYY/MM/DD/随机标识`，PicGo 继续使用原有 `pic/`。RAM 用户 `aowugong-moments` 的策略 `AowugongMomentsObjects` 仅允许 `moments/*` 的 `oss:PutObject`、`oss:GetObject`，没有删除或 Bucket 管理权限。
- 图片和视频经浏览器直传 OSS：S3 Endpoint 为 `https://s3.oss-cn-guangzhou.aliyuncs.com`，Region 为 `cn-guangzhou`，Bucket CORS 仅允许 `https://moments.aowugong.top` 的 PUT/GET/HEAD，后台媒体 Domain 为 `https://pic.aowugong.top`，缩略图后缀留空。
- Caddy 仅将 `pic.aowugong.top/moments/*` 转发至 Moments；该服务使用专用 S3 凭证经同地域内网端点签名读取，支持 GET、HEAD、Range、ETag，最多同时处理 8 个媒体请求，不转发访客 Cookie/Token 和任意查询参数。原有 `pic/` 代理和 RAM 权限未改动，主站 Go 服务未重启。
- 本机运行凭证在 `storage/private/moments/oss.json` 和 `runtime.json`，管理员登录信息在同目录 `login.txt`；均为 600 权限且被 Git 忽略。运行不依赖本次临时管理 Key；配置结束后由用户禁用该管理 Key，不能误禁用 Moments 专用 RAM 凭证。
- SQLite 每日由 `aowugong-moments-backup.timer` 在服务器本地时间 04:10 调度一致性备份，校验完整性并保留最近 14 份，位置为 `shared/backups/`。备份含 S3 配置，必须保持私密；不包含 OSS 媒体，尚未配置异机备份。
- 已验证：真实 PNG/MP4 上传、媒体字节一致性、HEAD、Range 206/416、ETag 304、浏览器图片显示及视频直接/内嵌播放、专用凭证访问 `pic/` 被拒绝、首页和登录 API。原有网站 HTTP 检查与变更前一致；`receipt-split.aowugong.top/` 在变更前后均为 404，不属于本次修改。
- Caddy 配置修改前的副本为 `/opt/vaultwarden/caddy/Caddyfile.before-moments-20260929`。现有 Caddy 管理接口关闭，配置校验后执行了一次入口容器重启；后续 Moments 更新只替换其独立容器。
- 本次生成的 OSS 测试媒体和发布暂存包、服务器临时解密配置与私钥已清理；正式凭证仅保留在私有运行配置和数据库中。没有发送第三方通知。

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

    GOTOOLCHAIN=auto pwsh -File ./scripts/build-release.ps1 -Version v1.0.0

发布包包含：

- Linux amd64 Go 二进制。
- web/dist 静态资源。
- PostgreSQL 版本化迁移。
- systemd 模板、部署脚本、回滚脚本和本 README。

发布包不包含 .env、VPN 原始文件、工作导航、节点内容、数据库备份或任何生产私有配置。Git tag 会触发 GitHub Actions，执行前后端测试、Race、Vet 和构建。

部署只在用户明确要求时执行：

    sudo DEPLOY_MODE=main /opt/aowugong-go/current/scripts/deploy-release.sh v1.0.0

部署动作限于构建、上传、原子切换 current、重启 aowugong-go.service 和健康检查；不自动发送钉钉、邮件、微信等通知。正式发布前确认 migration 向后兼容，并保留数据库备份。

现有 deploy-release.sh 还包含初始化操作：缺少 Swap 时创建 Swap、准备用户和目录、调整 shared 权限、更新运行环境和 systemd unit，并可能同步 Vaultwarden 备份脚本。执行前应核对这些副作用是否符合本次范围，不能把整个脚本视为仅切换二进制。脚本设置 AOWUGONG_DATABASE_SKIP_MIGRATIONS=false，因此服务重启会检查并执行尚未应用的 PostgreSQL 迁移；不能承诺普通部署绝不修改 schema。

本地发布包可通过 RELEASE_ARCHIVE 指定，必须同时提供对应 .sha256 文件。使用与发布包匹配的部署脚本；旧服务器脚本可能仍要求已删除的 configs/.env.example，不能为绕过校验恢复废弃模板。每次使用新版本目录，并分别检查服务状态、内网/公网健康接口和本次变更涉及的功能。

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
- 公共规则由 storage/private/vpn/common-routing.json 单文件维护，AI 修改后更新到生产私有目录，支持合并规则的客户端刷新订阅生效。
- PicGo 使用阿里云 OSS 的 RAM 子账号；2026-09-29 临时管理 Key 的使用和待禁用状态见第 5 节。
- 生产应用只通过 Caddy 对外提供域名入口，内部服务和数据库不公开。
- 服务器项目、域名和端口变化时，先更新第 1 节总览，再更新受影响的项目章节和部署边界。
- 任何未来 AI 开发都必须先读本文件，并把新的明确设计沉淀到这里。

已知交接状态（历史核实，不代表实时线上状态）：2026-09-12 已部署 v20260912-113811-vpn-routing，服务运行及公网健康检查通过；当时 common-routing.json 缺失或为空，因此规则只读展示功能上线不等于规则已配置。后续仍需补齐并验证公共规则、核实魔戒资源的实际展示与客户端订阅可用性。v2rayN 标准节点订阅无法满足规则一并同步的目标，不能仅凭页面展示规则宣称该缺口已解决。

## 14. 后续优化方向

- 日志与可观测性：系统复杂后，评估引入 Wide Event / Canonical Log Line，为请求、任务、消息、批量处理和人工覆盖统一记录上下文，让正常流程清晰、例外可追踪和可审计。当前只记录为优化方向，暂不实施具体架构。
- 资料来源：[Logging Sucks](https://loggingsucks.com/)、[logging-best-practices skill](https://github.com/boristane/agent-skills/tree/main/skills/logging-best-practices)。
