# CUPS Web 开发者指南

本文档面向开发者，介绍项目架构、API、开发流程与扩展方式。用户文档请参阅 [README.md](README.md)。

> 📚 **深度文档**：原理说明、故障案例与历史决策已移至 [docs/](docs/README.md)，本文件只保留可快速扫读的规则与契约。

## 📦 项目概述

- **项目定位**：基于 CUPS 的 Web 打印管理工具，前后端分离
- **技术栈**：Go 1.26（后端）+ Vue 3（前端）+ SQLite（存储）+ IPP（打印协议）
- **部署形态**：单二进制（前端 `go:embed`）连接外部 CUPS；**单容器（AIO）Docker 镜像**（`cupsd` + `cups-web` 同容器，内置 LibreOffice + Java 21 + OFD 转换器 + Ghostscript + 打印驱动生态）
- **仅 Web 镜像**：同一 `Dockerfile` 的 `web-runtime` 目标，保留文档转换依赖，通过 `CUPS_HOST` 或 `CUPS_SOCKET_PATH` 连接外部 CUPS；标签带 `-web` 后缀，使用 `docker-compose.web.yml`，不运行 cupsd 或安装驱动/扫描工具。

> ⚠️ **历史形态提示**：仓库曾经是「cups 镜像 + cups-web 镜像」双容器（`cups/` 目录），已在合并提交里删除。现在只有根目录的一份 `Dockerfile` / `entrypoint.sh`，构建脚本在 `scripts/build/`、驱动脚本在 `scripts/driver/`。

## 🛠️ 技术栈

### 后端

| 组件 | 说明 |
| --- | --- |
| Go 1.26 | 见 `go.mod` |
| `gorilla/mux` | HTTP 路由 |
| `gorilla/securecookie` | 会话管理 |
| `modernc.org/sqlite` | 纯 Go SQLite，无 CGO |
| `OpenPrinting/goipp` | IPP 协议 |
| `rsc.io/pdf` + `phpdave11/gofpdf` | PDF 解析 / 生成 |
| `golang.org/x/image/draw` | 大图下采样（CatmullRom） |
| `golang.org/x/crypto/bcrypt` | 密码哈希 |

### 前端

| 组件 | 说明 |
| --- | --- |
| Vue 3.5 + Vue Router | hash 模式 |
| Vite 7 | 构建 |
| `@nuxt/ui` v4 + Tailwind CSS v4 | UI / 样式 |
| `pdfjs-dist` | 预览（PDF 生成由后端 `/api/convert` 负责） |
| Bun（本地）/ npm（CI + Docker） | 包管理。npm 用于覆盖 `linux/arm/v7`（Bun 不支持 32-bit ARM） |

### 外部依赖

CUPS（IPP 通信）、LibreOffice（Office → PDF）、Java 21 + `ofd-converter.jar`（OFD → PDF）、Ghostscript（PDF 标准化）、`dpkg`/`apt-get`（运行时驱动安装）。

> 各依赖的坑位说明（LibreOffice 可写 HOME、gs 字体破坏性改造、runtime 无 `dpkg-dev`）见 [docs/architecture.md](docs/architecture.md)。

## 📁 项目结构

```text
cups-web/
├── cmd/server/                    # 后端主程序
│   ├── main.go                    # 入口与路由注册
│   ├── app.go                     # 全局变量
│   ├── bootstrap.go               # 默认 admin 初始化
│   ├── auth_handlers.go           # 登录 / 登出 / session / csrf
│   ├── login_limiter.go           # 登录失败限流
│   ├── admin_handlers.go          # 管理员：用户 / 设置 / 清理
│   ├── user_handlers.go           # /api/me
│   ├── print_handlers.go          # /api/print（主打印入口）
│   ├── print_records_handlers.go  # 打印记录查询 / 下载 / 重打
│   ├── printer_info_handler.go    # 打印机属性查询
│   ├── convert_handler.go         # /api/convert
│   ├── convert_utils.go           # LibreOffice / OFD 转换工具
│   ├── compose_handler.go         # /api/compose（多页拼版）
│   ├── estimate_handler.go        # /api/estimate（预估页数）
│   ├── preview_handler.go         # /api/preview-page（预览兜底：PDF 单页 → PNG）
│   ├── driver_handlers.go         # /api/admin/drivers/* + 后台任务
│   ├── driver_registry.go         # 驱动注册表
│   ├── file_utils.go              # 文件保存 / 类型识别 / 页数
│   ├── pdf_utils.go               # 图片 / 文本 → PDF
│   ├── pdf_compose.go             # 多页拼版
│   ├── pdf_reorder.go             # 页序重排 + 测试
│   ├── watermark.go               # 水印
│   ├── pdf_normalize.go           # PDF 标准化管线
│   ├── fonts.go                   # 中文字体加载
│   ├── maintenance.go             # 后台维护任务
│   └── version.go                 # 构建期版本号
├── internal/
│   ├── auth/session.go            # securecookie 会话 + CSRF
│   ├── middleware/                 # csrf / security 中间件
│   ├── ipp/                       # IPP 客户端 + URI 校验
│   ├── server/static.go           # 静态资源嵌入（SPA fallback）
│   └── store/                     # 数据层（users / prints / settings）
├── frontend/                      # Vue 3 前端（go:embed dist）
├── ofd-converter/                 # Java OFD → PDF
├── scripts/
│   ├── build/install-cups.sh      # 源码编译 CUPS
│   └── driver/                    # 驱动管理命令 + 安装脚本 + capture-debs.sh（apt 钩子）
├── docker-fonts/                  # 构建期字体与 gs/fontconfig 配置
├── entrypoint.sh                  # AIO 容器启动脚本
├── Dockerfile                     # 共享转换依赖 + AIO / 仅 Web 两个目标
├── docker-compose.yml             # 单服务 AIO
├── docker-compose.web.yml         # 仅 Web，连接外部 CUPS
└── Makefile                       # 构建脚本
```

## 🔌 HTTP API

所有接口以 `/api` 为前缀。除登录/登出/csrf/session 外均需 `RequireSession` + `ValidateCSRF`；管理员接口再叠加 `RequireAdmin`。

> **CSRF 约定**：登录成功后下发 `csrf_token` Cookie（非 HttpOnly）；前端非 GET 请求带 `X-CSRF-Token` 头。

> **API Key 通道（issue #113）**：`protected` 与 `admin` 子路由都在 `RequireSession` 之前挂了 `middleware.APIKeyAuth`。请求头 `Authorization: Bearer cw_...` 或 `X-API-Key: cw_...` 命中数据库 `api_keys.token_hash`（SHA-256 hex）后，中间件把归属 `auth.Session` 注入 context，`RequireSession`/`RequireAdmin` 无差别放行，`ValidateCSRF` 通过 `auth.IsAPIKeyAuth` 豁免 double-submit 校验。guest 保留账号硬编码禁止签发/使用 key；`POST /api/api-keys` 与 `DELETE /api/api-keys/{id}` 在 handler 里显式拒绝 API Key 通道，防止密钥自繁殖。

### 公开接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/login` | 登录 |
| POST | `/api/logout` | 登出 |
| GET | `/api/csrf` | 刷新 csrf token |
| GET | `/api/session` | 查询会话 |
| GET | `/api/version` | 构建版本号 |

### 已登录用户接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me` | 当前用户信息 |
| GET | `/api/printers` | 列出打印机 |
| GET | `/api/printer-info?uri=<uri>` | 打印机属性 |
| GET | `/api/printer-options?uri=<uri>&force=1` | 驱动纸张 / 介质 / 质量选项，force 可选 |
| POST | `/api/estimate` | 估算页数 |
| POST | `/api/convert` | 文档 → PDF（单文件 `file` / 多图 `files`） |
| POST | `/api/preview-page` | PDF 单页渲染为 PNG（`file` + `page`，响应头 `X-Page-Count`），浏览器 pdf.js 加载失败时的预览兜底 |
| POST | `/api/print` | 提交打印 |
| POST | `/api/compose` | 多页拼版 |
| GET/POST | `/api/scheduled-prints` | 定时任务列表 / 创建 |
| PUT/DELETE | `/api/scheduled-prints/{id}` | 更新 / 删除定时任务 |
| POST | `/api/scheduled-prints/{id}/run` | 异步提前执行，返回 `202 {ok, status:"running"}` |
| GET | `/api/scheduled-prints/{id}/files/{index}` | 照片队列缩略图（本人或管理员） |
| GET | `/api/print-records` | 打印记录 |
| GET | `/api/print-records/{id}/file` | 下载原始文件 |
| POST | `/api/print-records/{id}/reprint` | 重打参数预填 |
| GET | `/api/scan/devices` | 列出扫描仪(`scanimage -L`) |
| GET | `/api/scan/options?device=<name>` | 列出设备参数(`scanimage -A`,MVP 只关心 mode/resolution/source) |
| POST | `/api/scan/jobs` | 提交扫描任务(异步)→ `202 {jobId, recordId, filename}` |
| GET | `/api/scan/jobs/{id}` | 轮询扫描任务状态 + 增量日志 |
| DELETE | `/api/scan/jobs/{id}` | 取消运行中的扫描任务 |
| GET | `/api/scan/records` | 扫描记录(admin 可用 `?username=` 过滤) |
| GET | `/api/scan/records/{id}/file` | 下载扫描文件 |
| DELETE | `/api/scan/records/{id}` | 删除扫描记录 + 文件 |
| GET | `/api/api-keys` | 列出自己名下的 API 密钥（不含明文）(issue #113) |
| POST | `/api/api-keys` | 生成新密钥，明文仅此响应返回一次；仅浏览器 session 可调 |
| DELETE | `/api/api-keys/{id}` | 删除自己名下的密钥；仅浏览器 session 可调 |
| GET | `/api/print-presets` | 列出自己的打印预置（issue #128）；guest 403 |
| POST | `/api/print-presets` | 新建 `{name, settings}`，同名 409，每人上限 20 |
| PUT/DELETE | `/api/print-presets/{id}` | 改名 / 覆盖参数（字段可选）/ 删除，仅限本人 |

#### 扫描异步任务要点(issue #111)

- **改走 `scanimage` 子进程,不用 hplip 的 `hp-scan`**:hp-scan 依赖容器内 HPLIP daemon,即使补启 `dbus-daemon --system --fork` 仍报 `SANE: Error during device I/O (code=9)`。Debian 标准 SANE 栈(`sane-utils` + `libsane-hpaio`)在同宿主同硬件已验证可用。
- 与驱动任务不同,扫描**允许并发**——scanimage 无 apt/dpkg 全局锁,同一台扫描仪的独占由 SANE 后端负责。
- 硬超时 5 分钟(`scanJobTimeout`),context 派生自 `context.Background()`,绕开 http.Server `WriteTimeout=120s` 掐子进程。
- PDF 走 **PNG → gs 合成**:scanimage 先出临时 PNG,再 `gs -sDEVICE=pdfwrite -dSAFER -dNOPAUSE -dBATCH` 合成 PDF,最后清理临时文件。scanimage 有些后端不支持直接 `--format=pdf`,统一走这一条路径规避驱动差异。
- 内存态任务表保留 1 小时,过期只删内存条目,不动 `scan_records` 表与磁盘文件——那属于用户可见资产。
- 落盘目录由 `SCAN_DIR` 决定(默认 `scans/`;AIO 镜像里 `docker-compose.yml` 设为 `/scans` 并挂 `./.scans` 持久卷)。**所有下载/删除都用 `os.OpenInRoot(scanDir, ...)` 收敛**,阻挡 `../` 逃逸。
- 命令行注入面:device 名先经 `scanimage -L` 白名单校验,resolution 走 `strconv.Atoi` 环回,mode/format 白名单(Color/Gray/Lineart × png/jpeg/pdf),全部走变参 `exec.CommandContext` 无 shell。
- 🚫 **escl (eSCL/AirScan) 后端必须显式传扫描区域**(issue #112):不指定 `--tl-x/--tl-y/--br-x/--br-y` 时 br-x/br-y 会被 rounded 到 0,`sane_start` 报 `Invalid argument`。`runScanimage` 会先跑 `scanimage -A` 探测,存在 `br-x/br-y` range 时补一对左上 0 / 右下 max;老 backend(hpaio 无 br-x/br-y)不追加,行为保持既有。`-A` 输出被污染时通过 `isNumericScanValue` 正则守卫拒绝拼进命令行。
- **设备发现 TTL 缓存 + 启动预热**(issue #111 复测反馈):`scanimage -L` 因为要遍历多种 SANE 后端,单次 ~17s,每次切进扫描页都等这段时间太慢。`scanDeviceCacheGet` 给结果加一层进程内 TTL 缓存(默认 60s,`SCAN_DEVICES_CACHE_TTL` 覆盖,`<=0` 禁用回退旧行为)。`GET /api/scan/devices?force=1`(前端【刷新设备】使用)绕过缓存;probe/options/jobs 的设备存在性校验命中缓存,不承担"发现设备"职责。启动后 `startScanPrewarm` 后台跑一次真调,让首访直接拿热数据。失败不覆盖已有 entry,避免"刷新失败反而丢热数据";单飞用 `sync.Mutex` + `chan struct{}`,并发请求只发一次子进程。

#### `/api/printers` 返回形状

每项：`name` / `uri` / `info` / `location` / `makeAndModel`。`info` 即 CUPS Web 界面里的「描述」（`printer-info`），前端下拉靠它区分同型号队列（issue #101）。

- `ipp.ListPrinters` 优先走 IPP `CUPS-Get-Printers`（只点名 4 个 `requested-attributes`，否则每台机器回上百个属性）；操作被拒或对端不是 CUPS 时退回抓 `/printers` HTML 页面，此时 `info`/`location`/`makeAndModel` 为空串。
- 🚫 必须遍历 `rsp.Groups` 里的 `TagPrinterGroup`：`goipp` 的 `Message.Printer` 只是被压平的第一组，直接用它会在多队列时静默只返回一台。
- 🚫 URI 一律按配置的主机 + 队列名拼（TCP 用 `CUPS_HOST`，Unix Socket 用占位主机 `localhost:631`），**不要**用响应里的 `printer-uri-supported` —— 那是 cupsd 自报的主机名，容器/跨网段场景下浏览器与服务端未必解析得到。
- CUPS 建队列不带 `-D` 时 `printer-info` 默认等于队列名，前端 `printerLabel()` 会在两者相同时省略描述，不重复显示。

#### CUPS Unix Socket（issue #120）

- `CUPS_SOCKET_PATH` 非空时优先于 `CUPS_HOST`，必须是绝对路径，启动时检查语法；socket 可以由外部 cupsd 稍后创建。连接失败不回退 TCP。
- `internal/ipp/connection.go` 统一处理请求 URI，Socket 模式保留队列路径并归一化为 `http://localhost:631`，IPP 属性使用 `ipp://`；任务列表的 `job-printer-uri` 也归一化，供取消操作使用。历史记录和定时任务的旧 TCP URI 复用该连接层。
- socket 路径只读部署环境变量，禁止用户请求指定路径。Socket 模式仅允许根、打印机列表、打印机/类队列路径；禁止凭据、查询参数、fragment 和路径穿越。`newSafeClient` 固定 dial Unix Socket，保留超时、禁止重定向和响应大小限制；主机/IP SSRF 策略仅适用于 TCP。
- `docker-compose.web.socket.yml` 为宿主机 socket 目录挂载示例，跨容器可共享运行目录卷。必须挂载整个目录，以适应 cupsd 重建 socket；目录/socket 权限仍由部署方管理。AIO 默认 TCP 启动及本地驱动命令不变。

### 管理员接口（`/api/admin/*`）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/api/admin/users` | 用户列表 / 创建 |
| PUT/DELETE | `/api/admin/users/{id}` | 更新 / 删除（`admin` 禁止） |
| GET | `/api/admin/print-records` | 全站记录 |
| GET/PUT | `/api/admin/settings` | 系统设置 |
| POST | `/api/admin/cleanup` | 手动清理 |
| GET | `/api/admin/drivers` | 驱动列表 + 状态 |
| POST | `/api/admin/drivers/install` | **异步**安装，`202` + `jobId` |
| POST | `/api/admin/drivers/remove` | **异步**卸载，`202` + `jobId` |
| GET | `/api/admin/drivers/detect` | 扫描打印机推荐驱动 |
| GET | `/api/admin/drivers/ppds` | 候选 PPD 列表（`?deviceUri=&deviceId=&manufacturer=&model=&limit=8`） |
| POST | `/api/admin/drivers/upload` | 上传 `.ppd` / `.deb`（**同步**，64MB 上限） |
| POST | `/api/admin/drivers/setup` | **异步**一键设置，`202` + `jobId` |
| GET | `/api/admin/drivers/jobs/{id}` | 轮询任务状态 |

#### 驱动异步任务要点

- `install` / `remove` / `setup` **必须异步**（编译型驱动耗时可达十几分钟，全局 `WriteTimeout=120s` 会 kill 同步进程）。handler 立刻 `202` + `jobId`，命令跑在 `context.Background()` goroutine（硬超时 30min），前端轮询 `jobs/{id}`。
- **单飞**：同一时刻只允许一个驱动任务（apt/dpkg 全局锁），已有任务时返回 `409` + 正在跑的 `jobId`。
- 任务只存内存，保留 1 小时，进程重启即丢。

> 异步模型的完整设计理由与请求/响应形状见 [docs/driver-management.md](docs/driver-management.md#异步任务模型driver_handlersgo)。

`drivers[]` 每项是 `DriverStatus`：`name` / `displayName` / `description` / `arch` / `needCompile` / `installed` / `installedAt` / `installedArch` / `supported` / `hasScript`。`installed` 以 `manifest.txt` 是否存在为唯一判据。

`customDebs[]` 每项是 `CustomDebPackage`：`filename` / `installedAt` / `installedArch` / `sizeBytes`。纯信息性条目。

### `/api/print` 表单字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 待打印文件 |
| `printer` | string | 打印机 URI |
| `sides` | `one-sided` / `two-sided-long-edge` / `two-sided-short-edge` | 单面 / 长边 / 短边，优先于 `duplex` |
| `duplex` | `"true"` / `"false"` | 兼容旧客户端：缺少 `sides` 时，true 为长边双面 |
| `color` | `"true"` / `"false"` | 彩色 |
| `copies` | int | 份数 |
| `orientation` | `portrait` / `landscape` | 方向 |
| `paper_size` | `A4` / `A3` / `5inch`…`10inch` | 纸张尺寸 |
| `driver_options` | JSON string | `pageSize` / `mediaType` / `quality`，从 `/api/printer-options` 选取，服务端校验并保存快照；覆盖对应传统参数 |
| `paper_type` | `plain` / `photo` / … / `auto` | 纸张类型 |
| `media_source` | string | 进纸盒（`auto` 不发送） |
| `print_scaling` | `auto` / `auto-fit` / `fit` / `fill` / `none` / 纯数字 `10`–`400` | 缩放；纯数字＝自定义百分比 |
| `page_range` | string | 页码范围 |
| `page_set` | `all` / `odd` / `even` | 页面子集（`all` 不发送） |
| `mirror` | `"true"` / `"false"` | 镜像 |
| `compress_images` | `"true"` / `"false"` | 图片压缩，默认关闭；每张图片编码大小不超过 800,000 字节，仅图片转换生效 |
| `number_up` | `1`–`16` | N-up（`1` 不发送） |
| `number_up_layout` | `lrtb` / `rltb` / `tblr` / `tbrl` | N-up 排布 |
| `page_border` | `single` / `none` | N-up 边框 |

## 🗄️ 数据库

SQLite，`WAL` + `foreign_keys`；迁移在 `store.go::migrate()` 中用幂等 SQL + `addColumnIfMissing` 增量加列。

### `users`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER PK | 自增 |
| `username` | TEXT UNIQUE | 登录名 |
| `password_hash` | TEXT | bcrypt |
| `role` | TEXT | `admin` / `user` |
| `protected` | INTEGER | `1` = 受保护 |
| `contact_name` / `phone` / `email` | TEXT | 联系信息 |
| `created_at` / `updated_at` | TEXT | RFC3339 UTC |

### `print_jobs`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER PK | 自增 |
| `user_id` | INTEGER FK | 提交者 |
| `printer_uri` / `filename` / `stored_path` | TEXT | 打印机 / 文件 |
| `pages` | INTEGER | 页数 |
| `job_id` | TEXT | IPP Job ID |
| `status` | TEXT | `queued` / `printed` |
| `is_duplex` / `is_color` / `mirror` | INTEGER | 布尔参数 |
| `copies` / `number_up` | INTEGER | 数值参数 |
| `orientation` / `paper_size` / `paper_type` / `media_source` | TEXT | 纸张参数 |
| `print_scaling` / `page_range` / `page_set` | TEXT | 页面参数 |
| `watermark_text` | TEXT | 水印 |
| `number_up_layout` / `page_border` | TEXT | N-up 参数 |
| `created_at` | TEXT | RFC3339 UTC |

> 除 `is_duplex` / `is_color` 外，其余打印参数列为 Issue #68 新增（完整参数快照落库，供「重新打印」预填）。`page_set` 存用户原始选择（`even-reverse` 等），不是重排后的值。

### `scan_records`(issue #111)

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER PK | 自增 |
| `user_id` | INTEGER FK | 扫描者(级联删除) |
| `device` | TEXT | SANE URI(如 `hpaio:/usb/...`) |
| `mode` | TEXT | Color / Gray / Lineart |
| `resolution` | INTEGER | dpi(50–4800) |
| `source` | TEXT | ADF / Flatbed / …;空串表示用设备默认 |
| `format` | TEXT | png / jpeg / pdf |
| `filename` / `stored_path` | TEXT | 相对 `SCAN_DIR` |
| `size_bytes` | INTEGER | 产物大小,失败为 0 |
| `status` | TEXT | `running` / `succeeded` / `failed` / `cancelled` |
| `err_msg` | TEXT | 失败原因 |
| `created_at` / `finished_at` | TEXT | RFC3339 UTC |

索引 `idx_scan_records_user_time (user_id, created_at)`。管理员可看全表,普通用户按 `username` 过滤。

### `settings`

KV 表。当前键：`retention_days`（`0` = 永久）、`session_hash_key` / `session_block_key`。

### `print_presets`（issue #128）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER PK | 自增 |
| `user_id` | INTEGER FK | 归属用户（级联删除） |
| `name` | TEXT | 名称，`UNIQUE(user_id, name)`，1–64 字符 |
| `settings_json` | TEXT | 前端参数快照，JSON 对象 ≤16KB |
| `created_at` / `updated_at` | TEXT | RFC3339 UTC |

- 「上次设置」只存浏览器 localStorage `print_settings_last`（标准模式打印成功后写入，不含水印文字与页码范围），命名预置才落库；guest 共享账号禁用预置。
- 服务端只校验「JSON 对象 + 大小」，字段白名单在 `frontend/src/utils/print-settings.js::sanitizePrintSettings`，套用时缺失/非法字段回到出厂默认；真正的可信校验仍在 `/api/print`。新增打印参数时记得同步白名单与 `DEFAULT_PRINT_SETTINGS`。
- 🚫 `driverOptions` 是队列私有的 attribute/value，快照里与 `printerUri` 成对保存，只在同一台打印机上恢复；换打印机时清空并递增 `driverRematchKey`，由 `PrinterMediaOptions` 按纸张重新匹配，不能跨队列直接套用（`/api/print` 会拒绝）。

### 照片定时队列（issue #121）

- `scheduled_prints` 复用现有打印参数和触发策略，幂等新增 `files_json`（按顺序存原图及缩略图相对路径）、`next_file_index`、`submitted_count`、`loop`、`start_date`、`history_json`（最近 20 张照片的执行结果），以及 `photos_per_run`（每次张数，默认 1）。
- 创建沿用 multipart：旧 `file` 字段保持单文件语义；新 `files` 字段表示照片队列（1–100 张 JPG/PNG/GIF），每次按 `photos_per_run` 提交 1–100 张（默认 1）；两者不能混用。队列支持 daily/weekly/monthly，`start_date` 为服务器本地日期，`loop=true` 循环，否则完成后停用。
- 自动执行、手动执行、编辑、删除共享任务互斥；手动执行用 Background goroutine，页面通过列表的 `running` 字段轮询。每张成功提交后立即保存进度，中途失败停止当前批次，失败/错过窗口不推进该照片；手动队列执行消费下一次计划，保留暂停状态。
- 修改打印参数或暂停时保留下次执行点；恢复时只重算已过期的执行点，避免撤销提前打印后的计划顺延。编辑页面保留未展示的打印参数，列表忽略较早请求的迟到响应。
- 提交前将队列持久化为 `enabled=0, last_status=running`，成功或明确失败后原子写入结果、进度并恢复计划；服务中断、结果落库失败或 IPP 提交结果不确定时保留暂停，防止自动重复提交不确定结果。
- 原图和缩略图存于 `uploads/scheduled/`，沿用历史清理豁免；缩略图读取、删除用 `os.OpenInRoot` / `os.OpenRoot` 限制路径。原有单文件任务不受队列完成规则影响。

## 🔐 认证与安全

1. **启动**：`auth.SetupSecureCookie` 从 settings 读取/生成密钥
2. **登录**：写 `session`（HttpOnly，加密+签名）+ `csrf_token`（非 HttpOnly）
3. **鉴权链**：`RequireSession` → `RequireAdmin`（管理员）→ `ValidateCSRF`（非 GET）
4. **登出**：两条 cookie `MaxAge=-1`
5. **默认管理员**：`bootstrap.go` 保证 `admin/admin` 存在且 `protected=1`；`Username == "admin"` 判定保护（禁止改名/改角色/删除）

### Cookie 属性

两条 cookie 统一 `Path=/`、`SameSite=Lax`、`MaxAge=86400`，`Domain` 不设（host-only）。
签发与清除都必须走 `auth.NewCSRFCookie()` / `auth.SetSession()` / `auth.ClearSession()`，
🚫 不要在 handler 里手搓 `http.Cookie` —— 属性漂移过一次（旧的登出路径漏了 `Secure`/`SameSite`）。

`Secure` 由 `COOKIE_SECURE` 三态控制：缺省 `auto`（逐请求看 `r.TLS` 或 `X-Forwarded-Proto`
第一跳）/ `true` 恒开 / `false` 恒关。因此 `CookieSecure(r)` 带 `*http.Request` 参数，
不能缓存成进程级常量。

### 跨源防护（`middleware.CrossOriginProtection`）

包 Go 标准库 `http.CrossOriginProtection`（基于 `Sec-Fetch-Site`，缺失时退化为
`Origin` vs `r.Host`），外加两处项目特有的适配：

- **`X-Forwarded-Host` 比对**：反代不写 `proxy_set_header Host $host` 时 `r.Host` 是上游
  地址，Origin 与它天然不等 → 整站 POST 被 403。`proxyOriginAllowed()` 只在
  `Sec-Fetch-Site` **缺失**时补一次 XFH 比对救回这类请求。
  🚫 不要把这条放行扩大到 `Sec-Fetch-Site` 有值的情况 —— 那才是真正的跨源判定。
  默认信任 XFH 不削弱安全（能伪造头的客户端本来就能让标准库 fail-open），论证见 docs。
- **拒绝响应必须是 JSON**：标准库默认吐 `text/plain`，而前端把「响应不是合法 JSON」的
  登录失败兜底显示成「用户名或密码错误」，导致反代配错被谎报成密码错误（issue #99）。
  `denyCrossOrigin` 返回 `{"error","code":"cross_origin_blocked","origin","host"}` 并打
  含全部判定依据的日志。同理，前端 `LoginView.vue` 的失败文案按状态码分级，
  🚫 不要再加「解析失败就当密码错误」的兜底。

`TRUSTED_ORIGINS`（逗号分隔完整 origin）是浏览器判定 `same-site`/`cross-site` 时的唯一
逃生阀 —— 那种情况改请求头没用。

### 登录限流（`login_limiter.go`）

键 `clientIP|username`，5 次失败锁 15 分钟，仅进程内存。`clientIP()` 取
`X-Forwarded-For` 第一跳且**无条件信任**，所以后端端口不应直接暴露公网。

> 反代下的完整排查表、两类 403 的分别修法、Caddy/Traefik 配置、子路径挂载限制见
> [docs/reverse-proxy.md](docs/reverse-proxy.md)。

## 🖨️ 打印流水

`printHandler` 流程：接收 multipart → 落盘 `uploads/YYYYMMDD/` → 类型识别 & 转换 → 页数统计 → 插入 `queued` 记录 → IPP 提交 → 回写 `printed`。

- `pdf` → `normalizePDF`（gs → LibreOffice → passthrough）
- `office` → LibreOffice `--convert-to pdf`
- `ofd` → `java -jar /ofd-converter.jar`
- `image` → `gofpdf`（默认保留原始像素，显式压缩时限制 3000px / 每图 800KB；旋转使用 PDF 矩阵）
- `text` → `gofpdf` + 内嵌中文字体

> ⚠️ 标准化管线只解决 CUPS 老驱动兼容性问题，gs 会破坏空壳 CJK 字体导致 pdf.js 预览错位。管线原理与 cidfmap 机制见 [docs/pdf-pipeline.md](docs/pdf-pipeline.md)。

### 双面翻页方式（issue #129）

- 普通/批量打印、文档/照片重打、定时任务统一传递 `sides` 三态；JSON 同名。旧 `duplex` / `isDuplex` 布尔值仍兼容，显式 `sides` 优先且只允许三个标准 keyword，非法值返回 400。
- `print_jobs` 和 `scheduled_prints` 增量增加 `sides` 列；空串表示旧记录，按 `is_duplex` 还原为单面或长边。写入时同步布尔值，重打预填及定时任务编辑、暂停/恢复保留翻页方式。
- 奇数页、偶数页、偶数倒序按手动双面处理，提交与历史均强制单面（issue #109）；倒序重排前的原始 `page_set` 必须参与判断。

### 自定义百分比缩放（`pdf_scale.go`）

`print_scaling` 为纯数字时走 `resolveCustomScaling`：gs 预缩放 PDF 内容 → 发给 CUPS 的 `print-scaling` 换成 `none`。
打印与重打（`reprintHandler`）两条路径都必须调用它。

- 🚫 **纯数字绝不能透传给 IPP**：`print-scaling` 是 keyword，`"40"` 不是合法取值。gs 失败/非 PDF 时退回空串（打印机默认），不是原样下发。
- 🚫 gs 缩放**不要**用 `-dFIXEDMEDIA` + 固定 `-dDEVICEWIDTHPOINTS`：横向页会被塞进纵向纸并偏移。现在的 `Install` 过程逐页读 `currentpagedevice /PageSize`，纸张尺寸原样保留。
- ⚠️ `-sOutputFile` 必须排在 `-f` **之前**——`-f` 之后的参数会被 gs 当成输入文件名，放末尾会直接报 `requires an output file`（Issue #98 的根因）。
- 前端预览用等比 CSS transform 同步这个百分比（`PrintPreview.vue` / `PdfCanvas.vue`），其余缩放模式交给 CUPS，预览不模拟。

### 驱动打印选项（issue #122）

- `internal/ipp/printer_options.go` 通过现有安全连接读取队列 PPD，解析 PageSize / MediaType / 质量控件（可能叫 Ink 等）；缺失类别退回标准 IPP。无边距依赖 ImageableArea 或 media-col 四边距为零，不能用通用开关假定硬件支持。
- `driver_options_json` 在 `print_jobs` 与 `scheduled_prints` 中保存完整选择；服务端只接受该队列上报的 attribute/value，客户端尺寸与标签不能作为信任来源。打印、重打、定时执行均复用校验和 IPP 编码。
- 纸张转换与预览使用 `custom:<宽mm>x<高mm>`；无边距图片转换将应用默认 10mm 边距改为 0。已经生成的 PDF 不自动裁掉内容白边。
- 图片 `print_scaling=fill` 在生成 PDF 时等比放大并居中裁切，单图按内容区、多图按各格子裁切；`fit` 完整显示。普通打印、转换、原图重打、定时任务共用排版，前端切换填充时必须使旧 PDF 缓存失效。已有 PDF 不按照片重新裁切。
- 多张队列仍逐张提交，在下一张开始前持久化成功进度；整个批次只推进一个调度点。磁盘上的任务在批次运行期间保持暂停，避免中断后重复打印不确定结果。详见 [docs/printer-options.md](docs/printer-options.md)。

### 照片拍摄日期水印（issue #124）

- `photo_date_watermark`（multipart）/ `photoDateWatermark`（JSON）默认关闭，在打印记录和定时任务中持久化。普通打印、重打、定时执行共用图片转 PDF 绘制逻辑。
- 只读取 JPEG/PNG EXIF `DateTimeOriginal`，不以修改时间或当前日期替代；缺失或无效时跳过。当前浏览器预览和打印均上传原图，后端在处理前提取日期；自行预压缩的客户端须用 `/api/convert` 的 `photo_dates` 数组按顺序传递，后端严格校验日期与数量。
- 日期在图片旋转、适应/填充排版后按每张可见区域右下角定位；开启/关闭须使旧 PDF 缓存失效。已生成 PDF 的日期直接保留，重打不重复叠加。详见 [docs/printer-options.md](docs/printer-options.md)。

### 照片原图与打印 PDF

- `/api/print` 保留单文件 `file`，支持照片数组 `files`（1–100 张，禁止混用），复用 `rotations` / `per_page` / `invert` / `compress_images`。预览 PDF 不代替原图上传。
- `print_jobs.photo_layout_json` 保存有序原图及排版快照，`rendered_path` 保存应用提交的 PDF。照片重打从原图生成并复制独立资产，清理一条历史不能损坏其他历史。定时打印历史另存原图，任务源文件仍由任务管理。
- 照片的自定义纸张以短边在前为方向基准，再应用 `orientation`；转换、预览、重打和定时执行共用这一约定。
- 原图下载沿用 `/api/print-records/{id}/file?source=<索引>`，提交 PDF 用 `?variant=rendered`，均限本人或管理员，并通过 `os.OpenInRoot` 限制路径。历史到期/手动清理须覆盖全部关联资产，保护 `scheduled/` 共享文件。

## 🧹 维护任务

`maintenance.go` 每小时：读 `retention_days`（`0` 跳过）→ 删过期 `print_jobs` + 文件 → `VACUUM` + `wal_checkpoint`。管理员可 `POST /api/admin/cleanup` 手动触发。

## 🧩 驱动管理

相关文件：`driver_handlers.go`、`driver_registry.go`、`scripts/driver/*.sh`、`DriversView.vue`。

### 目录约定

| 路径 | 内容 |
| --- | --- |
| `/opt/cups-drivers/scripts/install-<name>.sh` | 安装脚本（构建期 COPY，不执行） |
| `/usr/local/bin/{driver-install,driver-list,driver-remove,restore-drivers}` | 管理命令 |
| `/opt/cups-drivers/libexec/capture-debs` | apt `DPkg::Pre-Install-Pkgs` 钩子（归档 apt 要装的 `.deb`） |
| `/opt/cups-drivers/baseline-packages.txt` | **构建期**生成的镜像自带包名快照（镜像层，**不在挂载卷内**） |
| `/opt/cups-drivers/data/<driver>/manifest.txt` | 文件清单 = **"已安装"唯一标记**（纯路径列表，包级模式下可能为空文件） |
| `/opt/cups-drivers/data/<driver>/packages.txt` | 存在即包级/混合模式。`<pkg> <version> <arch> <相对路径>` |
| `/opt/cups-drivers/data/<driver>/packages/*.deb` | 归档的 `.deb` 原件 |
| `/opt/cups-drivers/data/<driver>/metadata.txt` | `driver=` / `installed_at=` / `file_count=` / `arch=` / `manifest_version=` / `restore_mode=` / `package_count=` / `package_bytes=` |
| `/opt/cups-drivers/data/<driver>/<绝对路径镜像>` | 文件级产物副本 |
| `/opt/cups-drivers/data/custom-ppd/` | 上传 `.ppd`（写 manifest，可恢复） |
| `/opt/cups-drivers/data/custom-deb/packages/` | 上传 `.deb`（写 `packages.txt`，**重启自动重装**） |

`docker-compose.yml` 把 `./.drivers` 挂到 `/opt/cups-drivers/data`。**不挂卷 = 重启丢驱动。**

### 两条持久化通道（按驱动来源分流）

| 通道 | 适用 | 恢复 | 卸载 |
| --- | --- | --- | --- |
| **包级** | dpkg 包来源（`escpr2`(deb) / `canon-ufr2` / `konica-bizhub` / `epson-cn` / `gutenprint` / `custom-deb`） | `dpkg -i` 归档的 `.deb` | `apt-get purge`（dry-run 校验后）或 `dpkg -P --force-depends` |
| **文件级** | 非包来源（`canon-capt` / `hp-laserjet1020` / `sharp` / `foo2zjs-firmware` / `custom-ppd` / `escpr2`(源码分支)） | 按 manifest `cp -aT` | 按 manifest `rm -f` |

**为什么必须分流**：厂商 deb 普遍把产物装在 `/opt/<vendor>/`（Epson escpr2 的 298 个文件全在 `/opt`、Konica 的 filter 真身在 `/opt/km`）、`/usr/bin`（Canon UFR II 的渲染引擎 `cnrsdrvufr2`）、`/usr/share/<vendor>/`（gutenprint 的 370 个机型 XML、Canon 的 356 个 ICC），这些全在文件级路径白名单之外 —— 只靠文件级快照会得到空 manifest（`exit 1`，但文件其实已进容器）或残缺快照（UI 显示"已安装"而重启后驱动失效）。

`.deb` 原件的捕获有四层，前面命中就不用后面：apt 钩子（`capture-debs`，含全部传递依赖）→ 厂商脚本用 `DRIVER_PKG_DIR` 主动交接 → `/var/cache/apt/archives/` 打捞 → `apt-get download`。

> 🚫 `capture-debs` **必须永远 `exit 0`** —— `DPkg::Pre-Install-Pkgs` 返回非零会让 apt 中止**整个安装事务**。
>
> 🚫 厂商脚本的交接行必须 `|| true` 且不新增 `trap`，以免破坏"退出码 0/3/其他"和"只允许一个 EXIT trap"两条约定。

### 退出码约定

| 退出码 | 含义 | 行为 |
| --- | --- | --- |
| `0` | 成功 | 写 manifest |
| `3` | 架构不支持 | 不写 manifest，`exit 3` |
| 其他非零 | 真失败 | 不写 manifest，透传 |

**退出码 0 但文件与包归档同时为空才判失败**（拒绝写 manifest）。只要归档到了 `.deb`，即使所有文件都落在白名单外也算成功 —— 那种驱动照样能完整恢复。

### 架构探测

统一用 `dpkg --print-architecture`（**不要用 `dpkg-architecture`**，runtime 无 `dpkg-dev`）。Go 侧 `currentDebArch()` 映射 `GOARCH` → Debian 命名。multiarch 用 `detect_multiarch_libdir()`（glob `/usr/lib/*-linux-gnu*`，拿不到返回空串）。

### manifest 白名单速查

**ALLOW**：`/usr/lib/cups`、`/usr/share/cups`、`/usr/share/ppd`、`/usr/share/foomatic`、`/lib/firmware`、`/usr/lib/firmware`、`/usr/lib/<multiarch>`。

**DENY**：`/usr/bin/*`、`/usr/sbin/*`、`/bin/*`、`/sbin/*`、`/usr/local/{bin,sbin}/*`、`/etc/*`、`/var/*`、`/usr/include/*`、`/opt/cups-drivers/*`、`/tmp/*`、`/usr/share/{doc,man,locale,info}/*`、`/usr/share/cups/doc-root/*`、`*/pkgconfig/*`、`*.a`、`*.o`、`*.la`。

> ⚠️ **白名单在 `driver-install.sh` / `driver-remove.sh` / `restore-drivers.sh` 三处各有一份，必须永久保留全部三份**——remove/restore 侧是给存量被污染快照兜底的。🚫 不要因为 install 侧已过滤就删掉另外两处。详见 [docs/driver-management.md](docs/driver-management.md#-manifest-白名单为什么必须存在且三处都要有)。

### baseline 归属守卫（路径白名单的结构性补丁）

路径白名单有个**按路径分不开**的盲区：multiarch 目录（`/usr/lib/<triplet>`）既是驱动共享库的家、也是系统库的家。厂商 deb 的依赖被 apt 解析时会把无关系统库拖进来（老 `install-epson-cn.sh` 的 `apt-get -f install` 会拉进整套 Qt5/X11/GL —— 那只是 GUI 工具的依赖），它们正好落在白名单**内** → 被当成驱动产物写进 manifest → `driver-remove` 逐条 `rm` 就把系统库删了，`restore-drivers` 每次开机还用旧副本覆盖回去。

按**包属主**就分得干干净净：构建期把当时已安装的全部包名存进 `baseline-packages.txt`，三个脚本据此把"镜像自带包拥有的文件"一律排除（install 侧不记录，remove 侧不删，restore 侧不覆盖）。

实现要点：包名读进 bash 关联数组做 O(1) 判断（避免对两千多个 `.list` 文件各 fork 一次 grep）→ 只 `cat` baseline 包自己的 `/var/lib/dpkg/info/*.list`（multiarch 包的清单名是 `<pkg>:<arch>.list`，比对前要去掉 `:arch`）→ 与 manifest 求一次 `comm -12` 交集，之后逐条查是 O(1)。

> ⚠️ 这份守卫同样**三处各有一份**，理由与三份路径白名单相同。
>
> ⚠️ `baseline-packages.txt` 必须留在**镜像层**且生成于**所有 apt 安装之后**。放进 `/opt/cups-drivers/data` 会被 volume 挂载覆盖，守卫就降级成"只做路径白名单"。
>
> 🚫 **CUPS 自身的包绝不能进驱动快照**（`driver-install.sh` 的 `PKG_NAME_DENY_PATTERNS` 里列了 `cups` / `cups-core-drivers` / `cups-ppdc` / `libcups*` 等）。本镜像的 CUPS 是源码编译的（2.4.19，overlay tar 解包进 `/usr`），apt 侧只装了 `cups-daemon` / `cups-client` / `cups-filters`，**故意没有 `cups` 元包**。某些驱动包（`printer-driver-gutenprint`、Konica 的 deb）硬依赖 `cups` 元包，一旦让 apt 去满足就会装上 Debian 的 `cups-core-drivers` —— 那里面正是 `/usr/lib/cups/backend/{usb,socket,lpd,...}` 和一批 filter，**直接覆盖源码编译的同名文件**。实测 Konica 那条路径会让 564 个 CUPS 自身的文件被算进驱动快照。各 `install-*.sh` 用 `dpkg -i --force-depends` 从根上避免，DENY 再兜一层。

### AIO 编译脚本约定

> ⚠️ 编译型脚本**只允许一个 `trap _cleanup EXIT`**（bash 同信号只保留最后注册的 handler）。AIO 模式下只 `apt-get clean`，**绝不 `rm -rf /var/lib/apt/lists/*`**。详见 [docs/driver-management.md](docs/driver-management.md#-aio-编译脚本的单一-exit-trap约定)。

### 上传自定义驱动

- **`.ppd`**：校验 → 装到 `/usr/share/cups/model/custom/` → 写 manifest（可恢复）
- **`.deb`**：`dpkg -i`（失败补依赖后**必须再 `dpkg -i` 一次**）→ 归档到 `custom-deb/packages/` **并登记 `packages.txt`** → 容器启动时由 `restore-drivers` 幂等 `dpkg -i` 自动重装。仍**不写 manifest**（文件级恢复对 `.deb` 无意义，真正的安装动作在 maintainer script 里）
  - 存量只归档未登记的 `.deb` 会被 restore 按 glob **自动收养**，无需用户操作
  - 装不上的坏包会每次开机重试 → 出口是删掉宿主 `./.drivers/custom-deb/packages/` 下的对应文件
- 🔐 上传 `.deb` = 容器内 root RCE（dpkg maintainer script）。接口受三重鉴权保护，**管理员密码等同容器 root 凭据**
- 大小上限必须用 `http.MaxBytesReader`（`ParseMultipartForm(n)` 的 `n` 是 maxMemory 不是 body 上限）

> 上传机制完整说明见 [docs/driver-management.md](docs/driver-management.md#上传自定义驱动)。

### `lpinfo` 检测与一键设置

- 用 `lpinfo -l -v` **长格式**（短格式无厂商型号）；按 caps 加 `--timeout`/`--include-schemes`；独立超时 context（不挂 `r.Context()`）
- 型号优先级（修正）：`req.Manufacturer/Model`（lpinfo make-and-model，最可信）→ `device-id` MFG/MDL → URI 路径。🚫 不要把 URI 解析排最前（usb URI 的厂商常是裸 "HP" 甚至 "Unknown"）
- PPD 匹配走**打分引擎**（`ppd_match.go` 纯函数 + `ppd_query.go` 副作用层）：型号归一化 → 分层 tier 打分 → 来源偏好（custom > vendor > hplip > everywhere > foomatic > gutenprint > generic）→ cups-driverd 指纹加分 → 稳定排序 Top-N
- `GET /api/admin/drivers/ppds` 返回候选列表（不走后台 job，不占单飞锁，并发闸 4）
- `setup` 三态决策树：显式 `ppdUri` > `everywhere`（driverless）> 自动 Top-1 > **报错**（绝不静默建 raw）
- ⚠️ **`lpadmin` 不传 `-m` 建的是 raw 队列（无 PPD），不是 IPP Everywhere。** 真正的 driverless 要显式 `-m everywhere`。raw 队列拿不到 PPD 选项 → `/api/printer-info` 的 `mediaSourceSupported` 为空 → 前端进纸盒下拉消失
- 队列名去重（`uniquePrinterName`，`-2`…`-50` 后缀）；同 device-uri 已有队列时拒绝覆盖
- `lpadmin` 后验证（`lpstat -p` + `lpoptions -l`），PPD 未生效时 `isNew` 队列回滚 `lpadmin -x`

> 解析细节与历史翻车见 [docs/driver-management.md](docs/driver-management.md#lpinfo-检测格式假设与型号解析优先级)。

## 🚀 容器启动流程（`entrypoint.sh`）

1. `restore-drivers`（恢复驱动快照：先包级 `dpkg -i` 归档的 `.deb`，再文件级 `cp -a`）
2. CUPS 管理员用户 + tzdata
3. CUPS 配置还原（空卷时从 `/etc/cups-bak/` 复制）+ **对存量卷幂等补** `ssl/` 目录与 `ReadyPaperSizes`
4. HP 1020 PPD Letter → A4 修补（issue #48）
5. HP host-based 固件上传（后台）
6. dbus + avahi + ipp-usb（后台，允许失败）
7. cupsd + watchdog
8. 等 cupsd 就绪（`lpstat -r`，30 × 1s）
9. HP 1020 队列 `media-default=A4`（后台）
10. `exec /cups-web`（PID 1）

> ⚠️ cupsd 必须在 watchdog 子 shell **内部前台**启动（`wait` 只能等自己的子进程，否则 127 重启风暴）。🚫 不要把 `cupsd -f` 挪到子 shell 外面。
>
> ⚠️ `restore-drivers` 必须永远 `exit 0`（驱动恢复是尽力而为，不能阻塞启动，否则用户连 Web UI 都进不去无法自救）。它现在会跑 `dpkg -i`，因此必须 `DEBIAN_FRONTEND=noninteractive` + `--force-confold` + `timeout` + 临时 `policy-rc.d`（详见 docs）。
>
> ⚠️ **第 3 步的两条补丁必须在 `if [ ! -f cupsd.conf ]` 之外**：那个 if 只看 `cupsd.conf` 一个文件，存量 `./.etc` 卷里只要有它，整块还原就被跳过，新基线里加的东西存量用户永远拿不到。
>
> 🚫 **`lpadmin -o media=` 设的是 `media-default`，不是 `media-ready`。** iPhone AirPrint 面板的纸张**列表**读 `media-ready`，而 CUPS 的 `media-ready` 只由 cupsd.conf 的 `ReadyPaperSizes` ∩ PPD 尺寸决定（`scheduler/printers.c` 里 `load_ppd` 是唯一写入点），跟 `media-default` 无关。第 9 步只负责"面板默认勾选 A4"，issue #82 的真正修法是 `ReadyPaperSizes A4,A3,A5,A6,EnvDL`（值用 **PPD 尺寸名**，不是 PWG 名 `iso_a4_210x297mm`）。不配它时 cupsd 按 locale 兜底成 `Letter,Legal,Tabloid,4x6,Env10`（容器无 `LANG` → locale 为 C → 走 Letter 分支），A4 永远不出现。
>
> 完整设计理由见 [docs/container-startup.md](docs/container-startup.md)。

## 🔧 开发环境

### 本地搭建

```bash
# 前端
cd frontend && bun install && bun run dev    # :5173，代理 /api → :8090

# 后端
go mod download
go build -o bin/cups-web ./cmd/server && ./bin/cups-web    # :8080
```

### Makefile

```bash
make all            # 前端 dist + Go 二进制（必须先前端再后端）
make frontend       # 仅前端
make build          # 仅后端（禁止裸 go build ./cmd/server）
make docker-build   # AIO 镜像
make docker-build-web # 仅 Web 镜像（cups-web:web）
```

### Vite 开发代理

`/api → http://localhost:8090`。本地调试：后端 `LISTEN_ADDR=:8090 go run ./cmd/server`，前端 `bun run dev`。

### 构建产物分包

`vue-vendor`（vue/router）、`ui-vendor`（nuxt-ui/reka-ui/vueuse）、`pdf-vendor`（pdfjs-dist）。

## 🚢 部署

### docker-compose

单服务 `cups`（AIO），`image: hanxi/cups-web:latest`，`network_mode: host`（mDNS 组播需要，CUPS 直接占用宿主 `631`，Web 由 `LISTEN_ADDR` 决定、默认 `:1180`，issue #107）。

| 配置 | 为什么 |
| --- | --- |
| `network_mode: host` + `hostname: CUPS` | mDNS/DNS-SD 组播（5353/udp）桥接下出不去进不来：发现不了局域网网络打印机、AirPrint 广播不出去（issue #107）。avahi 以 `CUPS.local` 广播 |
| `LISTEN_ADDR=:1180` | host 网络无端口映射，Web 直接监听宿主 1180，与旧桥接时代一致 |
| `user: root` | cupsd / lpadmin / dpkg / 写系统路径 |
| `security_opt: [apparmor:unconfined]` | PVE LXC AppArmor DENIED（issue #91） |
| `./.etc:/etc/cups`、`./.data:/data`、`./.uploads:/uploads` | 持久化 |
| **`./.drivers:/opt/cups-drivers/data`** | **驱动快照持久化**（删 = 重启丢驱动） |
| **`./.scans:/scans`** + `SCAN_DIR=/scans` | **扫描产物持久化**（issue #111）；不挂则容器重启后历史扫描件全丢，DB 里的记录会指向不存在的文件 |
| `/dev/bus/usb:/dev/bus/usb` + `device_cgroup_rules` | USB 热插拔（issue #81） |
| `/run/udev:/run/udev:ro` | libusb 设备属性（可选） |

> ⚠️ **不要挂载宿主 `/run/dbus/system_bus_socket`**：旧版为借宿主 avahi 广播 AirPrint 挂过它（issue #94），但宿主没装 avahi 时容器内自己的 dbus-daemon 会因 socket 路径被占而起不来，avahi 跟着失效（issue #107）。现已移除，host 网络下容器内自启 dbus + avahi 即可。

### Docker 构建

七个阶段：`frontend-build`（node:20-slim）、`java-builder`（BUILDPLATFORM 锁 amd64）、`builder`（golang:1.26）、`cups-builder`（源码编译 CUPS）、`document-runtime`（共享转换依赖/字体）、`web-runtime`（仅 Web）和 `runtime`（默认 AIO）。覆盖 `linux/amd64` + `arm64` + `arm/v7`。`web-runtime` 不依赖 `cups-builder`，构建仅 Web 镜像时不会编译 CUPS。

> 🚨 `cups-builder` 的 `ca-certificates` 请勿删除（wget TLS 校验，删了 CI 直接崩）。
>
> ⚠️ **`/etc/cups/*` 来自 apt 的 `cups-daemon`（trixie 2.4.10），不是源码编译那份。** `cups-builder` 打的 `cups-compiled.tar` 只含 `/usr` 路径，整个 `/etc` 都不在里面。所以：读 `cupsd.conf` 的 Location/Policy 出厂内容要按 **Debian 版**理解；而 `/usr/lib/cups/**`（filter、backend、`daemon/cups-driverd`）是**源码编译的 2.4.19**。这也是为什么绝不能让 apt 装 `cups-core-drivers` —— 它会用 Debian 版覆盖 overlay 解包的那批文件。
>
> ⚠️ `cupsd` **不会自己创建** `/etc/cups/ssl`（源码里那处 `cupsdCheckPermissions` 的 `create_dir` 传的是 0，`lstat` 失败时既不 mkdir 也不报错），缺了它要到第一次 ipps 握手才炸 `Unable to create server credentials`，而 AirPrint 客户端优先挑 `_ipps._tcp`。以前它之所以存在纯粹是因为 Debian 的 `cups-daemon` 把它作为 package-owned 空目录发布 —— 那是别人的实现细节，现在 Dockerfile 与 entrypoint 各显式建一次。
>
> 构建目标与共享依赖设计、三架构镜像选型史、HOME/LibreOffice profile 见 [docs/docker-build.md](docs/docker-build.md)。

### CI/CD

- **`build-release.yml`**：7 平台交叉编译。v* tag → 正式 Release；master 分支 → 滚动 `dev` prerelease（git 标签 `dev`，不用分支名，避免重名冲突）
- **`docker-publish.yml`**：`master` / `v*` tag → 两种目标的三架构镜像。AIO 使用 `vX.Y.Z` + `latest` / `dev`，仅 Web 使用 `vX.Y.Z-web` + `latest-web` / `dev-web`；两者 `VERSION` 一致。开头有 `Free disk space` 步骤。

### 版本管理

`./bump-version.sh patch|minor|major`

## 🎯 常见开发任务 / 调试 / 代码风格

> 新增 API、修改 DB、新增前端页面、新增文件类型、新增驱动的步骤模板，以及调试命令、Go/Vue 风格、Git 提交约定，见 [docs/conventions.md](docs/conventions.md)。
>
> 🚫 **commit message 禁止 `Co-Authored-By` 及任何 AI 署名行**，中文撰写。

**维护者**：涵曦（<im.hanxi@gmail.com>）
