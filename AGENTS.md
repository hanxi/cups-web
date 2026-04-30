# CUPS Web 开发者指南

本文档面向开发者，介绍项目架构、API、开发流程与扩展方式。用户文档请参阅 [README.md](README.md)。

## 📦 项目概述

- **项目定位**：基于 CUPS 的 Web 打印管理工具，前后端分离
- **技术栈**：Go 1.26（后端）+ Vue 3（前端）+ SQLite（存储）+ IPP（打印协议）
- **部署形态**：单二进制（前端通过 `go:embed` 打包进可执行文件），或 Docker 镜像（内置 LibreOffice + Java 17 + OFD 转换器）

## 🛠️ 技术栈

### 后端

| 组件 | 版本 / 说明 |
| --- | --- |
| Go | 1.26（见 `go.mod`） |
| HTTP 路由 | `github.com/gorilla/mux` |
| 会话管理 | `github.com/gorilla/securecookie` |
| 数据库 | `modernc.org/sqlite`（纯 Go，无 CGO） |
| 打印协议 | `github.com/OpenPrinting/goipp`（IPP） |
| PDF 解析 | `rsc.io/pdf`（页数读取）、`github.com/phpdave11/gofpdf`（PDF 生成） |
| 图像缩放 | `golang.org/x/image/draw`（CatmullRom，用于大图下采样） |
| 加密 | `golang.org/x/crypto/bcrypt` |

### 前端

| 组件 | 版本 / 说明 |
| --- | --- |
| 框架 | Vue 3.5 + Vue Router（hash 模式） |
| 构建 | Vite 7 |
| UI 库 | `@nuxt/ui` v4（含自带的 Tailwind 主题） |
| 样式 | Tailwind CSS v4 |
| 图标 | `@iconify-json/lucide` |
| PDF 处理 | `pdfjs-dist`（预览，PDF 生成统一交由后端 `/api/convert`） |
| HEIC 兼容 | `heic2any` |
| 包管理 | Bun（推荐） |

### 外部依赖

| 依赖 | 作用 |
| --- | --- |
| CUPS | 打印服务，通过 IPP 通信 |
| LibreOffice（headless） | Office 文档 → PDF；同时作为 PDF 标准化的兜底链路 |
| Java 17 + `ofd-converter.jar` | OFD 文档 → PDF（基于 `ofdrw`） |
| Ghostscript (`gs`) | PDF 标准化首选链路：统一降级到 PDF 1.4 并强制嵌入所有字体，解决 `UniGB-UCS2-H` 等外部 CMap 编码导致的打印乱码（本地 macOS 需要 `brew install ghostscript`） |

## 📁 项目结构

```text
cups-web/
├── cmd/server/                    # 后端主程序
│   ├── main.go                    # 入口与路由注册
│   ├── app.go                     # 全局变量（appStore、uploadDir）
│   ├── bootstrap.go               # 默认 admin 初始化
│   ├── auth_handlers.go           # 登录 / 登出 / session / csrf / me
│   ├── admin_handlers.go          # 管理员：用户 / 系统设置
│   ├── user_handlers.go           # /api/me
│   ├── print_handlers.go          # /api/print（主打印入口）
│   ├── print_records_handlers.go  # 打印记录查询、文件下载
│   ├── printer_info_handler.go    # 打印机属性查询（IPP Get-Printer-Attributes）
│   ├── convert_handler.go         # /api/convert（文档 → PDF 转换）
│   ├── convert_utils.go           # 调 LibreOffice / OFD 转换器的工具
│   ├── estimate_handler.go        # /api/estimate（预估页数）
│   ├── file_utils.go              # 文件保存、文件类型识别、页数统计
│   ├── pdf_utils.go               # 图片 / 文本 → PDF 的渲染
│   ├── fonts.go                   # 中文字体加载（内嵌 assets/fonts）
│   ├── maintenance.go             # 后台维护任务（按保留天数清理）
│   └── assets/fonts/              # 打包进二进制的字体资源
├── internal/
│   ├── auth/session.go            # securecookie 会话 + CSRF cookie
│   ├── middleware/csrf.go         # RequireSession / RequireAdmin / ValidateCSRF
│   ├── ipp/client.go              # IPP 客户端：列表、属性、提交打印
│   ├── server/static.go           # 静态资源嵌入服务（SPA fallback）
│   └── store/                     # 数据层
│       ├── store.go               # DB 打开 + 迁移
│       ├── users.go               # users CRUD
│       ├── prints.go              # print_jobs CRUD
│       └── settings.go            # settings KV 存取
├── frontend/
│   ├── embed.go                   # go:embed dist/** → frontend.FS
│   ├── src/
│   │   ├── main.js                # Vue app 入口
│   │   ├── App.vue                # 顶层布局：header / router-view / footer
│   │   ├── router/index.js        # hash 路由 + session 缓存守卫
│   │   ├── views/                 # LoginView / PrintView / AdminView
│   │   ├── components/            # 业务组件
│   │   ├── utils/                 # api / file / format 工具
│   │   └── index.css              # 全局样式
│   ├── package.json
│   └── vite.config.js
├── ofd-converter/                 # Java 子项目：OFD → PDF
│   ├── pom.xml
│   └── src/
├── cups/                          # CUPS 服务镜像
│   ├── Dockerfile
│   └── entrypoint.sh
├── .github/workflows/             # CI：多平台二进制构建与发布
├── .aone_copilot/plans/           # 历史开发计划（只增不改的档案）
├── data/                          # 运行时数据库（.gitignore）
├── uploads/                       # 运行时上传目录（.gitignore）
├── Dockerfile                     # Web 镜像多阶段构建
├── docker-compose.yml             # cups + web 组合
├── Makefile                       # 构建脚本
├── bump-version.sh                # 语义化版本打 tag 脚本
├── go.mod / go.sum
├── README.md                      # 用户文档
└── AGENTS.md                      # 本文档
```

## 🔌 HTTP API

所有接口以 `/api` 为前缀。除登录/登出/csrf/session 外的接口均需通过 `RequireSession` 与 `ValidateCSRF` 两个中间件；管理员接口再叠加 `RequireAdmin`。

> **CSRF 约定**：登录成功后服务端会下发 `csrf_token` Cookie（非 HttpOnly，前端可读）；前端在所有非 GET 请求上带 `X-CSRF-Token` 头，与 Cookie 值一致方可通过。

### 公开接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/login` | 账号密码登录，成功后下发 session + csrf cookie |
| POST | `/api/logout` | 清除 session 与 csrf cookie |
| GET | `/api/csrf` | 手动刷新 csrf token |
| GET | `/api/session` | 查询当前会话（未登录返回 401） |

### 已登录用户接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me` | 当前用户信息（id / username / role） |
| GET | `/api/printers` | 列出 CUPS 中的打印机 |
| GET | `/api/printer-info?uri=<uri>` | 查询打印机属性（状态、队列任务数等） |
| POST | `/api/estimate` | 上传文件，返回估算页数 |
| POST | `/api/convert` | 上传文件，返回转换后的 PDF 流；支持单文件（`file` 字段，PDF / Office / OFD / 图片 / 文本）与多图合并（`files` 字段，多张图合成单个 PDF） |
| POST | `/api/print` | 提交打印任务 |
| GET | `/api/print-records` | 查询自己的打印记录（可带 `start` / `end`） |
| GET | `/api/print-records/{id}/file` | 下载打印记录对应的原始文件 |

### 管理员接口（`/api/admin/*`）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/admin/users` | 列出所有用户 |
| POST | `/api/admin/users` | 创建用户 |
| PUT | `/api/admin/users/{id}` | 更新用户 |
| DELETE | `/api/admin/users/{id}` | 删除用户（`admin` 账号禁止） |
| GET | `/api/admin/print-records` | 查询全站打印记录（可带 `username` / `start` / `end`） |
| GET | `/api/admin/settings` | 读取系统设置 |
| PUT | `/api/admin/settings` | 更新系统设置（`retentionDays`） |

### `/api/print` 表单字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 待打印文件（multipart） |
| `printer` | string | 打印机 URI |
| `duplex` | `"true"` / `"false"` | 是否双面 |
| `color` | `"true"` / `"false"` | 是否彩色 |
| `copies` | int | 份数 |
| `orientation` | `portrait` / `landscape` | 页面方向 |
| `paper_size` | `A4` / `A3` / `5inch` / `6inch` / `7inch` / `8inch` / `10inch` | 纸张尺寸 |
| `paper_type` | `plain` / `photo` / `glossy` / `matte` / `envelope` / `cardstock` / `labels` / `auto` | 纸张类型 |
| `print_scaling` | `auto` / `auto-fit` / `fit` / `fill` / `none` | 缩放策略 |
| `page_range` | string | 页码范围，如 `1-5 8 10-12` |
| `mirror` | `"true"` / `"false"` | 镜像打印 |

## 🗄️ 数据库

SQLite，启用 `WAL` + `foreign_keys`；迁移逻辑在 `internal/store/store.go` 的 `migrate()` 中，**使用幂等 SQL**，支持老库热升级（通过 `addColumnIfMissing` 增量加列）。

### `users`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER PK | 自增主键 |
| `username` | TEXT UNIQUE | 登录名 |
| `password_hash` | TEXT | bcrypt 哈希 |
| `role` | TEXT | `admin` / `user` |
| `protected` | INTEGER | `1` 表示受保护（默认 `admin` 账号） |
| `contact_name` | TEXT | 联系人 |
| `phone` | TEXT | 电话 |
| `email` | TEXT | 邮箱 |
| `created_at` / `updated_at` | TEXT | RFC3339 UTC |

### `print_jobs`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER PK | 自增主键 |
| `user_id` | INTEGER FK → users | 提交者 |
| `printer_uri` | TEXT | 目标打印机 URI |
| `filename` | TEXT | 原始文件名 |
| `stored_path` | TEXT | 相对 `uploads/` 的路径 |
| `pages` | INTEGER | 页数 |
| `job_id` | TEXT | IPP 返回的 Job ID |
| `status` | TEXT | `queued` / `printed` |
| `is_duplex` | INTEGER | 是否双面 |
| `is_color` | INTEGER | 是否彩色 |
| `created_at` | TEXT | RFC3339 UTC |

### `settings`

KV 表：`key TEXT PRIMARY KEY` + `value TEXT`。

当前使用的键：

- `retention_days`：打印记录保留天数（`0` = 永久）
- `session_hash_key` / `session_block_key`：securecookie 的密钥（首次启动自动生成并持久化）

## 🔐 认证与安全

### 会话流程

1. **启动时**：`auth.SetupSecureCookie` 从 `settings` 读取 / 生成 `session_hash_key` + `session_block_key`（各 32 字节），构造 `securecookie.SecureCookie`
2. **登录**：校验密码后写入两条 cookie：
   - `session`（HttpOnly，加密+签名，编码 `{userId, username, role, expires}`）
   - `csrf_token`（非 HttpOnly，前端 JS 可读）
3. **鉴权中间件链**：
   - `RequireSession`：解出 session
   - `RequireAdmin`：再校验 `role == admin`
   - `ValidateCSRF`：对非 GET/HEAD/OPTIONS 请求比对 `X-CSRF-Token` 头与 cookie
4. **登出**：`ClearSession` 将两条 cookie 都设为 `MaxAge=-1`

### 默认管理员

`bootstrap.go::ensureDefaultAdmin` 保证始终存在一个 `admin` 用户：

- 若不存在：创建 `admin/admin` 且 `protected=1`
- 若存在但角色/保护位异常：纠正为 `admin` + `protected=1`
- 代码中通过 `Username == "admin"` 判定保护逻辑（禁止改名、改角色、删除）

## 🖨️ 打印流水

`printHandler`（`cmd/server/print_handlers.go`）是核心入口，流程：

1. **接收**：解析 multipart 表单，提取 `file` + 打印参数
2. **落盘**：`saveUploadedFile` 将上传文件按日期分目录保存到 `uploads/YYYYMMDD/` 下，文件名做安全化处理
3. **类型识别 & 转换**（`detectFileKind`）：
   - `pdf` → **PDF 标准化管线**（`diagnosePDF` 诊断日志 → `normalizePDF`：Ghostscript `pdfwrite -dCompatibilityLevel=1.4 -dEmbedAllFonts=true` 优先（两档 strict `/prepress` → lenient `-dNEWPDF=false -dPDFSTOPONERROR=false` 重试）→ LibreOffice `--convert-to pdf` 兜底 → passthrough 最终降级），解决 Acrobat 高版本 / `UniGB-UCS2-H` 等外部 CMap 导致的预览空白与打印乱码
   - `office` → `convertOfficeToPDF`（调 `libreoffice --headless --convert-to pdf`）
   - `ofd` → `convertOFDToPDF`（调 `java -jar /ofd-converter.jar`）
   - `image` → `convertImageToPDF`（用 `gofpdf` 渲染；长边超过 3000px 的大图会先经 `downscaleImageIfNeeded` 下采样到 3000px 并以 JPEG Q85 重编码再嵌入 PDF，避免把手机端 10MB+ 原图整张塞进 PDF 导致移动端预览/下载超时，见 [Issue #22](https://github.com/hanxi/cups-web/issues/22)；PNG 透明像素会被合成到白底以符合打印预期）
   - `text` → `convertTextToPDF`（用 `gofpdf` + 内嵌中文字体渲染）
4. **页数统计**：`countPDFPages` / `countPDFPagesWithFallback` / `estimateTextPages`；PDF 页数读取失败时走 `normalizePDF` 再重试，仍失败则以 1 页兜底而非直接 400
5. **持久化**：在 `print_jobs` 插入一条 `queued` 记录
6. **提交打印**：`ipp.SendPrintJob` 构造 `Print-Job` IPP 请求并发出
7. **回写状态**：成功后更新为 `printed` 并回填 `job_id`

转换或标准化后的 PDF 以 `<原文件>.print.pdf` 副文件形式存到 `uploads/`，维护任务清理时会连同原文件一起删除。`/api/convert` 对 PDF 也会走同一条 `normalizePDF` 管线，让前端 `PdfCanvas` 预览与最终打印使用完全相同的字节流。

### HTTP 超时

`cmd/server/main.go` 的 `http.Server` 配置为 `ReadTimeout = WriteTimeout = IdleTimeout = 120s`。之所以放宽到 2 分钟，是因为 `/api/convert` 与 `/api/print` 在移动端场景需要：上传 10MB+ 原图 → 服务端下采样/标准化 → 回传 PDF，整条链路在 4G 网络下 15s 远远不够（[Issue #22](https://github.com/hanxi/cups-web/issues/22)）。如果未来要对个别接口设置更激进的独立超时，建议用 `http.TimeoutHandler` 包住具体子路由，而不是再调低全局值。

## 🧹 维护任务

`maintenance.go::startMaintenance` 启动一个 goroutine，每小时执行一次：

1. 读取 `retention_days`；为 `0` 时直接跳过
2. 按 `created_at < now - retentionDays` 删除 `print_jobs` 记录
3. 同步删除 `uploads/` 下的原文件与 `.print.pdf` 副文件
4. 若有删除发生：执行 `VACUUM` 回收空间 + `PRAGMA wal_checkpoint(TRUNCATE)`

## 🔧 开发环境

### 本地搭建

```bash
# 1. 前端
cd frontend
bun install
bun run dev       # 开发模式（Vite，默认 :5173，代理 /api → :8090）

# 2. 后端
cd ..
go mod download
go build -o bin/cups-web ./cmd/server
./bin/cups-web    # 默认监听 :8080，数据库 ./data/cups-web.db
```

### 使用 Makefile

```bash
make all            # 构建前端 dist + Go 二进制
make frontend       # 仅构建前端
make build          # 仅构建 Go 二进制
make docker-build   # 同时构建 cups 和 cups-web 镜像
make clean          # 删除 bin/cups-web
```

> **前后端整合规则**：Go 使用 `//go:embed dist/**` 将前端产物嵌入二进制，因此 **必须先构建前端** 再构建后端（CI 与 `Makefile all` 已按此顺序执行）。

### Vite 开发代理

`frontend/vite.config.js` 里配置了 `/api → http://localhost:8090` 代理，本地调试建议：

```bash
# 后端启动在 8090
LISTEN_ADDR=:8090 go run ./cmd/server

# 前端启动在 5173
cd frontend && bun run dev
```

### 构建产物分包

Vite 已配置 `manualChunks`：

- `vue-vendor`：vue / vue-router
- `ui-vendor`：`@nuxt/ui` / `reka-ui` / `@vueuse`
- `pdf-vendor`：`pdfjs-dist`（仅预览，PDF 生成已迁移到后端）

## 🚢 部署

### Docker 多阶段构建

`Dockerfile` 有三个构建阶段：

1. `frontend-build`（`oven/bun`）：构建 Vite dist
2. `java-builder`（`maven:3.9-eclipse-temurin-17`）：构建 `ofd-converter.jar`
3. `builder`（`golang:1.26`）：`go build` 输出二进制

运行阶段使用 `debian:bookworm-slim`，装上 LibreOffice（core/writer/calc/impress）+ JRE 17 + 中文字体（`fonts-noto-cjk`、`fonts-wqy-zenhei`、`fonts-arphic-*`），以 `nonroot` 用户运行。

### CI/CD

`.github/workflows/` 会在 push 到任何分支和 tag 时，针对 5 个平台交叉编译二进制（`linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`、`windows/amd64`），tag push 时自动创建 Release。

> ⚠️ 当前 CI 的 `setup-go` 仍固定为 `'1.24'`，与 `go.mod` 的 `1.26` 不一致；若改动触发了新版本语法需同步升级 CI 的 Go 版本。

### 版本管理

使用 `bump-version.sh` 打 tag：

```bash
./bump-version.sh patch    # 默认
./bump-version.sh minor
./bump-version.sh major
```

## 🎯 常见开发任务

### 新增 API 接口

1. 在 `cmd/server/` 下新建 `xxx_handler.go`，导出 handler 函数
2. 在 `main.go` 对应的 subrouter（`api` / `protected` / `admin`）中注册路由
3. 前端在 `frontend/src/utils/api.js` 中新增调用方法，并在视图中使用
4. 若是写接口，确认前端 `fetch` 会带上 `X-CSRF-Token` 头

### 修改数据库结构

1. 在 `internal/store/` 中修改或新增模型
2. 在 `store.go::migrate()` 中：
   - 新表：追加 `CREATE TABLE IF NOT EXISTS ...`
   - 旧表加字段：用 `addColumnIfMissing(ctx, db, "<table>", "<column_def>")`
3. 更新对应的 CRUD 函数
4. 本地用 `sqlite3 data/cups-web.db` 验证迁移在新库与老库上都能跑通

### 新增前端页面

1. 在 `frontend/src/views/` 新建 `.vue`，使用 Composition API
2. 在 `frontend/src/router/index.js` 添加路由；若需鉴权用 `meta: { requiresAuth: true }`，管理员页加 `requiresAdmin: true`
3. 在 `App.vue` 顶栏中按需加入导航入口（当前实现对 `admin` 角色显示「打印 / 管理」分段切换）

### 新增支持的文件类型

1. 在 `file_utils.go::detectFileKind` 加入新的 `fileKind`
2. 实现转换函数（放 `convert_utils.go` 或 `pdf_utils.go`）
3. 在 `print_handlers.go` 的 `switch kind` 中处理新类型
4. 同步更新 `estimateHandler` / `convertHandler` 中的分支（`convertHandler` 需覆盖单文件 `file` 与多文件 `files` 两种入口）

## 🧪 调试与测试

### 后端测试

```bash
go test ./...                # 全部测试
go test -cover ./...         # 带覆盖率
go vet ./...                 # 静态检查
```

> 当前仓库主要以手工测试 + 日志为主，`test/` 目录下存放临时测试用例，不参与 CI。新增核心模块时建议补 `_test.go`。

### 前端验证

```bash
cd frontend
bun run build                # 构建检查（类型与语法）
bun run dev                  # 本地调试
```

### 数据库查看

```bash
sqlite3 data/cups-web.db
.tables
SELECT * FROM users;
SELECT id, filename, status, is_duplex, is_color, created_at FROM print_jobs ORDER BY id DESC LIMIT 20;
SELECT * FROM settings;
```

## 📐 代码风格

### Go 风格

- 遵循标准 Go 命名约定与 `gofmt`
- Handler 内部通过 `appStore.WithTx(ctx, readOnly, func(tx) error { ... })` 做事务边界
- 错误响应统一使用 `writeJSONError(w, status, msg)`，成功使用 `writeJSON(w, v)`
- 文件路径：存储到 DB 的是 `filepath.ToSlash` 后的相对路径，使用时再用 `filepath.FromSlash` + `filepath.Join(uploadDir, ...)` 还原

### Vue 风格

- 单文件组件（SFC）+ `<script setup>` Composition API
- UI 组件优先用 `@nuxt/ui`（全局前缀 `U`，见 `vite.config.js`）
- 样式使用 Tailwind utility class，深色/浅色主题跟随 Nuxt UI 的 `bg-default` / `text-muted` 等语义类
- Session 信息通过 `router/index.js` 中的 `cachedSession` 缓存，避免每次路由切换都打 `/api/session`

## 📚 相关资源

- [CUPS 官方文档](https://www.cups.org/documentation.html)
- [IPP 规范](https://www.pwg.org/ipp/)
- [Nuxt UI v4](https://ui.nuxt.com/)
- [Tailwind CSS v4](https://tailwindcss.com/)
- [Vue 3 文档](https://vuejs.org/)
- [ofdrw](https://github.com/ofdrw/ofdrw)

---

**维护者**：涵曦（<im.hanxi@gmail.com>）
