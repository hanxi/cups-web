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
| 包管理 | 本地开发推荐 Bun（`bun install` / `bun run dev`）；CI 与 Docker 镜像统一用 npm（`npm ci` + `npm run build`），以同时覆盖 `linux/arm/v7` 架构——Bun 官方不支持 32-bit ARM（见部署章节） |

### 外部依赖

| 依赖 | 作用 |
| --- | --- |
| CUPS | 打印服务，通过 IPP 通信 |
| LibreOffice（headless） | Office 文档 → PDF；同时作为 PDF 标准化的兜底链路 |
| Java 17 + `ofd-converter.jar` | OFD 文档 → PDF（基于 `ofdrw`） |
| Ghostscript (`gs`) | PDF 标准化首选链路：统一降级到 PDF 1.4 兼容性（主要面向 CUPS/老打印机对新版 PDF 解析能力弱的场景）。**注意：`gs pdfwrite` 会对原 PDF 的每个字体对象强行加上 subset 前缀（`CCGWER+` 之类 6 位随机码）并重建字体字典**，对"空壳 Type0 字体 + `UniGB-UCS2-H` 外部 CMap"（Acrobat 导出的准考证/国标表格最常见的形态）是**破坏性改造**：原 PDF 的 `/BaseFont /#ba#da#cc#e5`（宋体 GBK 字节转义）会被改写成 `/BaseFont /BPCXJX+#cb#ce#cc#e5`，让 pdf.js 等渲染器误以为有内嵌字形可用、走内嵌路径却拿不到真实 FontFile，字宽表 vs 字形度量对不上导致"先正确一闪、再错位挤压"。因此该链路**不是 PDF 预览乱码的解药**，只在 CUPS 驱动确认无法解析原字体字典时才有收益。本地 macOS 需要 `brew install ghostscript`；Docker 镜像里给 gs 配了**三层中文字体兜底**（见 `Dockerfile` 注释）：①`/etc/ghostscript/cidfmap.local` 把 GBK 字节 BaseFont（宋/黑/楷/仿宋 × Regular/Bold，共 8 条）精准映射到 `arphic-uming` / `arphic-ukai` / `wqy-zenhei` 这三套**纯 TrueType**字体，并由 `pdf_normalize.go::cidfmapPreambleArgs` 在每次 gs 调用时显式用 `-I/etc/ghostscript -c "(cidfmap.local) .runlibfile"` 加载，不依赖任何"Debian 自动合并"约定；②`fonts-droid-fallback` 作为 cidfmap 未命中时的 Adobe-GB1 CID 兜底（Debian 把 gs 依赖的 `DroidSansFallback.ttf` 剥离到独立包）；③`fonts-noto-cjk` 等 Unicode 字形包仅服务 LibreOffice 渲染 Office 文档，不参与 CIDFSubst 路径。之所以只用 arphic/wqy 而不用 Noto CJK OTC，是因为 gs 10.x 对 CFF-based OpenType Collection 的 CIDFont 子字体索引偶有坑，纯 TrueType 最稳。|

## 📁 项目结构

```text
cups-web/
├── cmd/server/                    # 后端主程序
│   ├── main.go                    # 入口与路由注册
│   ├── app.go                     # 全局变量（appStore、uploadDir）
│   ├── bootstrap.go               # 默认 admin 初始化
│   ├── auth_handlers.go           # 登录 / 登出 / session / csrf / me
│   ├── admin_handlers.go          # 管理员：用户 / 系统设置 / 手动清理
│   ├── user_handlers.go           # /api/me
│   ├── print_handlers.go          # /api/print（主打印入口）
│   ├── print_records_handlers.go  # 打印记录查询、文件下载
│   ├── printer_info_handler.go    # 打印机属性查询（IPP Get-Printer-Attributes）
│   ├── convert_handler.go         # /api/convert（文档 → PDF 转换）
│   ├── convert_utils.go           # 调 LibreOffice / OFD 转换器的工具
│   ├── estimate_handler.go        # /api/estimate（预估页数）
│   ├── file_utils.go              # 文件保存、文件类型识别、页数统计
│   ├── pdf_utils.go               # 图片 / 文本 → PDF 的渲染
│   ├── pdf_normalize.go           # PDF 标准化管线（gs → LibreOffice → passthrough）
│   ├── pdf_normalize_test.go      # PDF 标准化相关的本地测试用例
│   ├── fonts.go                   # 中文字体加载（内嵌 assets/fonts）
│   ├── maintenance.go             # 后台维护任务（按保留天数清理）
│   ├── version.go                 # 构建期注入的版本号（-ldflags -X main.Version）
│   └── assets/fonts/              # 打包进二进制的字体资源
├── internal/
│   ├── auth/session.go            # securecookie 会话 + CSRF cookie
│   ├── middleware/csrf.go         # RequireSession / RequireAdmin / ValidateCSRF
│   ├── ipp/client.go              # IPP 客户端：列表、属性（含 stateDurationSeconds 状态持续时间计算）、提交打印
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
│   │   ├── views/                 # LoginView / PrintView（含批量打印） / AdminView（含手动清理）
│   │   ├── components/            # 业务组件
│   │   ├── utils/                 # api / file / format 工具
│   │   └── index.css              # 全局样式
│   ├── package.json
│   └── vite.config.js
├── ofd-converter/                 # Java 子项目：OFD → PDF
│   ├── pom.xml
│   └── src/
├── cups/                          # CUPS 服务镜像
│   ├── Dockerfile                 # 瘦身版：apt 装依赖 + COPY scripts/ + 调用各 sh
│   ├── entrypoint.sh              # 容器启动脚本（avahi/ipp-usb/cupsd）
│   └── scripts/                   # 镜像构建脚本（版本号、URL 硬编码在脚本内）
│       ├── install-cups.sh        # 源码编译 OpenPrinting/cups 2.4.x
│       ├── install-escpr2.sh      # 编译 Epson ESCPR2 驱动（仅从仓库 Release 镜像下载）
│       ├── install-gutenprint.sh  # 安装 printer-driver-gutenprint（跳过 armhf）
│       ├── install-epson-cn.sh    # Epson 国行专有 .deb（仅 amd64）
│       ├── install-canon-ufr2.sh  # Canon UFR II/UFRII LT 官方驱动 .deb（amd64 + arm64，armhf 跳过）
│       ├── install-konica-bizhub.sh # 柯尼卡美能达 bizhub 3000MF 驱动 .deb（amd64 + arm64，从仓库 Release 镜像下载 tar.gz）
│       └── cleanup-build.sh       # 清理编译工具链 + apt-mark manual 运行时库
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
| GET | `/api/version` | 返回二进制构建期通过 `-ldflags -X main.Version` 注入的版本号；前端 footer 展示用（Issue #26） |

### 已登录用户接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me` | 当前用户信息（id / username / role） |
| GET | `/api/printers` | 列出 CUPS 中的打印机 |
| GET | `/api/printer-info?uri=<uri>` | 查询打印机属性（状态、队列任务数等） |
| POST | `/api/estimate` | 上传文件，返回估算页数 |
| POST | `/api/convert` | 上传文件，返回转换后的 PDF 流；支持单文件（`file` 字段，PDF / Office / OFD / 图片 / 文本）与多图合并（`files` 字段，多张图合成单个 PDF） |
| POST | `/api/print` | 提交打印任务（前端支持批量模式：选择混合文件类型时逐个转换并提交） |
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
| POST | `/api/admin/cleanup` | 手动触发清理过期打印记录与文件（同维护任务逻辑） |

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
| `media_source` | string（打印机上报的纸盒关键字，如 `tray-1` / `main` / `manual`） | 进纸盒（对应 IPP `media-source`，映射到 CUPS 驱动 PPD 的 `InputSlot`）；可选值由 `/api/printer-info` 返回的 `mediaSourceSupported` 动态决定，`auto`（默认）不发送到 IPP（Issue #75） |
| `print_scaling` | `auto` / `auto-fit` / `fit` / `fill` / `none` | 缩放策略 |
| `page_range` | string | 页码范围，如 `1-5 8 10-12` |
| `page_set` | `all` / `odd` / `even` | 页面子集（仅打奇数页 / 仅打偶数页）；在 `page_range` 截出的页序基础上再过滤，典型场景是**手动双面打印**——先打奇数页，把纸翻面放回后再打偶数页。对应 CUPS 的 `page-set` 属性（由 `pdftopdf` filter 处理），`all` 视为默认值、不会发送到 IPP 请求。前端留空或选「全部页」等同于 `all` |
| `mirror` | `"true"` / `"false"` | 镜像打印 |
| `number_up` | `1` / `2` / `4` / `6` / `9` / `16` | 一张多页（N-up），每张纸缩排的逻辑页数；`1`（默认）= 关闭，不发送到 IPP。由 CUPS `pdftopdf` filter 原生处理，对应 IPP `number-up`（Issue #78） |
| `number_up_layout` | `lrtb` / `rltb` / `tblr` / `tbrl` | N-up 的页面排布顺序（横向 Z 形 / 纵向 N 形），对应 IPP `number-up-layout`；仅 `number_up > 1` 时生效 |
| `page_border` | `single` / `none` | N-up 时是否为每个小页绘制边框，对应 IPP `page-border`；仅 `number_up > 1` 时生效 |

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

1. **接收**：解析 multipart 表单（上限 512MB），提取 `file` + 打印参数
2. **落盘**：`saveUploadedFile` 将上传文件按日期分目录保存到 `uploads/YYYYMMDD/` 下，文件名做安全化处理
3. **类型识别 & 转换**（`detectFileKind`）：
   - `pdf` → **PDF 标准化管线**（`diagnosePDF` 诊断日志 → `normalizePDF`：Ghostscript `pdfwrite -dCompatibilityLevel=1.4 -dEmbedAllFonts=true` 优先（两档 strict `/prepress` → lenient `-dNEWPDF=false -dPDFSTOPONERROR=false` 重试）→ LibreOffice `--convert-to pdf` 兜底 → passthrough 最终降级）。**该管线只解决"CUPS 老驱动拒绝 PDF-1.7 新语法"这一类真正的兼容性故障**，对"预览显示"不会有帮助：gs 会把空壳 CJK 字体改写成带 subset 前缀的假嵌入字体，反而让浏览器 pdf.js 在预览时出现错位（详见前端 `PdfCanvas.vue` 的 `getDocument` 参数注释）。因此 `/api/convert` 预览入口应该**优先让 pdf.js 直接读原始 PDF**，只在真实打印前做最小化标准化。
   - `office` → `convertOfficeToPDF`（调 `libreoffice --headless --convert-to pdf`）
   - `ofd` → `convertOFDToPDF`（调 `java -jar /ofd-converter.jar`）
   - `image` → `convertImageToPDF`（用 `gofpdf` 渲染；长边超过 3000px 的大图会先经 `downscaleImageIfNeeded` 下采样到 3000px 并以 JPEG Q85 重编码再嵌入 PDF，避免把手机端 10MB+ 原图整张塞进 PDF 导致移动端预览/下载超时，见 [Issue #22](https://github.com/hanxi/cups-web/issues/22)；PNG 透明像素会被合成到白底以符合打印预期）
   - `text` → `convertTextToPDF`（用 `gofpdf` + 内嵌中文字体渲染）
4. **页数统计**：`countPDFPages` / `countPDFPagesWithFallback` / `estimateTextPages`；PDF 页数读取失败时走 `normalizePDF` 再重试，仍失败则以 1 页兜底而非直接 400
5. **持久化**：在 `print_jobs` 插入一条 `queued` 记录
6. **提交打印**：`ipp.SendPrintJob` 构造 `Print-Job` IPP 请求并发出
7. **回写状态**：成功后更新为 `printed` 并回填 `job_id`

转换或标准化后的 PDF 以 `<原文件>.print.pdf` 副文件形式存到 `uploads/`，维护任务清理时会连同原文件一起删除。`/api/convert` 对 PDF 也会走同一条 `normalizePDF` 管线，让前端 `PdfCanvas` 预览与最终打印使用完全相同的字节流。

> ⚠️ 已知副作用：Acrobat 导出的"空壳 Type0 + `UniGB-UCS2-H`"字体字典（`/BaseFont /#ba#da#cc#e5` 这种裸宋体名，准考证/国标表格常见）经 gs 改写为"subset 前缀 + FontFile2 假内嵌"后（`/BaseFont /CCGWER+#ba#da#cc#e5`），**pdf.js** 预览会出现"每 3-4 字错 1 字"的挤压错位（浏览器原生 PDF 引擎因有系统字体兜底不受影响）。之所以仍然共用 `normalizePDF`，是因为"预览与打印看到同一份字节流"的一致性比这类特殊 PDF 的预览准确性更重要——前端只使用 `pdfjs-dist` 在 canvas 里渲染预览（见 `frontend/src/components/print/PdfCanvas.vue`），遇到上述错位时用户可以忽略，不影响打印。

> 🖨️ 打印纸面中文字形的配套：Docker 镜像通过 `Dockerfile` 里写入的 `/etc/ghostscript/cidfmap.local` 把宋/黑/楷/仿宋（Regular + Bold，共 8 条 GBK 字节 BaseFont）显式映射到 `arphic-uming` / `arphic-ukai` / `wqy-zenhei` 三套 TrueType 字体，让 gs pdfwrite 在重建字体字典时能按字体名落到不同字形上，而不是全部坍缩成单一 `DroidSansFallback` 无衬线体。加载方式在 `pdf_normalize.go::cidfmapPreambleArgs` 实现——每次 gs 调用都显式传入 `-I/etc/ghostscript -c "(cidfmap.local) .runlibfile" -f`，不依赖 Debian gs 的自动合并约定（不同版本行为差异大）；cidfmap 文件不存在时 preamble 为空，兼容 macOS 本地开发。由于 arphic/wqy 都是**单字重字库**，gs 也不做 synthetic bold，Bold 变体只能通过"换字体制造视觉粗细差"——当前策略是宋体 Bold / 仿宋 Bold → `wqy-zenhei`（本镜像最粗的中文字体），黑体/楷体的 Bold 与 Regular 同源、视觉一致，属字库本身限制。诊断方式：`gs -dPDFDEBUG -I/etc/ghostscript -c "(cidfmap.local) .runlibfile" -dNOPAUSE -dBATCH -sDEVICE=pdfwrite -sOutputFile=/tmp/out.pdf <in.pdf> 2>&1 | grep -E "Substituting|CIDFSubst"`，命中 cidfmap 会看到 `Substituting font ... from /usr/share/fonts/truetype/...`；未命中才回落到 `DroidSansFallback`。新增映射条目时，GBK 字节 → PostScript name 换算关系：宋体=`cb ce cc e5`、黑体=`ba da cc e5`、楷体=`bf ac cc e5`、仿宋=`b7 c2 cb ce`，CSI 固定用 `[(GB1) 2]`。

### HTTP 超时

`cmd/server/main.go` 的 `http.Server` 配置为 `ReadTimeout = WriteTimeout = IdleTimeout = 120s`。之所以放宽到 2 分钟，是因为 `/api/convert` 与 `/api/print` 在移动端场景需要：上传 10MB+ 原图 → 服务端下采样/标准化 → 回传 PDF，整条链路在 4G 网络下 15s 远远不够（[Issue #22](https://github.com/hanxi/cups-web/issues/22)）。如果未来要对个别接口设置更激进的独立超时，建议用 `http.TimeoutHandler` 包住具体子路由，而不是再调低全局值。

## 🧹 维护任务

`maintenance.go::startMaintenance` 启动一个 goroutine，每小时执行一次：

1. 读取 `retention_days`；为 `0` 时直接跳过
2. 按 `created_at < now - retentionDays` 删除 `print_jobs` 记录
3. 同步删除 `uploads/` 下的原文件与 `.print.pdf` 副文件
4. 若有删除发生：执行 `VACUUM` 回收空间 + `PRAGMA wal_checkpoint(TRUNCATE)`

管理员也可通过 `POST /api/admin/cleanup` 手动触发同一清理逻辑（`adminCleanupHandler` → `cleanupOldPrints`），前端管理页面的"立即清理"按钮即调用该接口。

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

> **构建规范**：编译后端**必须**使用 `make build`（或等效的 `go build -ldflags='...' -o bin/cups-web ./cmd/server`），**禁止**裸执行 `go build ./cmd/server` —— 后者会在项目根目录生成名为 `server` 的垃圾文件（Go 默认用包目录名作为输出文件名），而非正确的 `bin/cups-web`。如果只需做语法/类型检查而不生成二进制，使用 `go vet ./cmd/server`。

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

`Dockerfile` 有三个构建阶段，**全部覆盖 `linux/amd64` + `linux/arm64` + `linux/arm/v7` 三架构**：

1. `frontend-build`（`node:20-slim` + `npm`）：`npm ci` + `vite build` 出 Vite dist
2. `java-builder`（`debian:bookworm-slim` + apt `openjdk-17-jdk-headless` + `maven`）：构建 `ofd-converter.jar`
3. `builder`（`golang:1.26`）：`go build` 输出二进制（`CGO_ENABLED=0`）

运行阶段使用 `debian:bookworm-slim`，装上 LibreOffice（core/writer/calc/impress）+ JRE 17 + 中文字体（`fonts-noto-cjk`、`fonts-wqy-zenhei`、`fonts-arphic-*`），以 `nonroot` 用户运行。

> 💡 **关于三架构覆盖的基础镜像选型**：最初 `frontend-build` 用 `oven/bun`、`java-builder` 用 `maven:3.9-eclipse-temurin-17`，但这两个基础镜像都不支持 32-bit ARM：
> - `oven/bun`：Bun 官方明确不支持 32-bit ARM（[oven-sh/bun#5060](https://github.com/oven-sh/bun/issues/5060) "Closed as not planned"，仅 arm64/x64）。**替代方案**：切到 `node:20-slim`（官方 manifest 覆盖 `amd64`/`arm32v7`/`arm64v8`），用 `npm ci` + `npm run build` 替换 `bun install` + `bun run build`；前端 `package.json` 里 scripts 全是标准 Vite/Node 命令，不依赖 bun 专有 API，迁移无业务代码改动。代价是必须维护 `frontend/package-lock.json`（和 `bun.lock` 并存；`npm ci` 要求 lockfile 与 `package.json` 严格一致，开发时如果用 `bun add` / `bun remove` 改了依赖，需同步跑一次 `npm install` 更新 `package-lock.json` 再提交，否则 CI 会在 `npm ci` 阶段挂掉）。
> - `maven:3.9-eclipse-temurin-17`：Eclipse Temurin 对 "Linux ARM 32-bit Hard-Float" 仅 JDK 8/11 有二进制，JDK 17/21/25 [官方明确 Not Supported](https://adoptium.net/supported-platforms)；Maven 官方镜像同样没有 armhf manifest。**现用方案**：`FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS java-builder`，把 java-builder 阶段**固定跑在 host 本地架构**（GitHub Actions 上永远是 amd64），apt 装 `openjdk-17-jdk-headless`，Maven 用 Apache 官方 3.9.x tarball；产物 `ofd-converter.jar` 是纯 Java 字节码，在 runtime 阶段被各架构的 JRE 直接 `COPY --from=java-builder` 过来复用，跨架构通吃。**为什么必须锁 `BUILDPLATFORM`**：QEMU 用户态模拟 armhf 下，OpenJDK 17 不稳定——Maven 无论是用 Debian 的 `apt install maven` 还是用 Apache 官方 tarball 启动都会随机抛 `java.lang.ClassNotFoundException: org.apache.maven.cli.MavenCli`，堆栈完全一致（只差 classworlds 版本行号：Debian 包版的 `SelfFirstStrategy.java:50` vs tarball 版的 `:42`），说明问题在 JVM 层（QEMU 下的 ClassLoader / JIT 稳定性），不是 Maven 安装方式能救的；Adoptium 官方放弃 JDK 17+ armhf 二进制也印证了"ARM 32-bit 上的现代 JVM 本来就是薄弱环节"。让 java-builder 锁 amd64 就彻底绕开了这堵墙，也是 Docker 官方推荐的 multi-arch Java 最佳实践（纯字节码跨架构是 JVM 的第一性原理）。**为什么顶部需要 `# syntax=docker/dockerfile:1`**：`BUILDPLATFORM` 是 BuildKit 前端注入的自动变量，旧 buildx 环境若缺失该声明会静默把它当成空，`--platform=$BUILDPLATFORM` 退化成默认 target，java-builder 又会落回 QEMU。**为什么 `FROM debian:bookworm-slim AS runtime` 不加 `--platform`**：runtime 阶段要装 LibreOffice/JRE/中文字体并真正被各架构的 Docker 节点拉取运行，必须跟随 `TARGETPLATFORM` 生成三份镜像；锁 amd64 会让 arm64/armhf 节点拉到 amd64 层、QEMU 模拟整个 runtime，完全跑偏。**Maven 为什么仍用 tarball 而不是 `apt install maven`**：虽然 host amd64 上 `apt install maven` 不会触发 QEMU 坑，但 Debian 包依赖 `dpkg triggers + update-alternatives` 更新软链（[carlossg/docker-maven#213](https://github.com/carlossg/docker-maven/issues/213)），换 base 镜像或升级系统时偶有兼容性问题；Apache tarball 的 `lib/` 自包含所有 jar，不依赖任何 OS 打包细节，一劳永逸。tarball URL 走 dlcdn.apache.org → archive.apache.org 的 fallback 链（前者只保留 current release，后者永久归档），升级 Maven 时只需改 `Dockerfile` 里的 `MAVEN_VERSION`。

### CI/CD

`.github/workflows/` 会在 push 到任何分支和 tag 时，针对 7 个平台交叉编译二进制（`linux/amd64`、`linux/arm64`、`linux/armv7`、`linux/loong64`、`darwin/amd64`、`darwin/arm64`、`windows/amd64`），tag push 时自动创建 Release。CI 使用的 Go 版本（`setup-go` 的 `go-version`）与 `go.mod` 保持一致（当前均为 `1.26`），升级 `go.mod` 时请同步 CI。

> 💡 补充说明：
> - `linux/armv7` 使用 `GOARCH=arm` + `GOARM=7`，覆盖树莓派 2/3、主流 ARM SBC 等 32 位硬浮点设备；matrix 里通过 `goarm` 字段声明，Build 步骤已把 `GOARM` 透传到 `env`（其他非 arm 目标此字段为空不生效）。
> - `linux/loong64` 依赖 `modernc.org/sqlite` ≥ `v1.34`（`v1.29.0` 尚未支持 loong64 架构）。
> - 由于全仓严格 `CGO_ENABLED=0`，新增其他 modernc 已支持的架构（`riscv64` / `s390x` / `ppc64le` 等）只需往 `build-release.yml` 的 matrix 里加一行 `goos/goarch/suffix`，无需额外工具链。

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

### Git 提交

- Commit message 使用中文，格式 `feat:` / `fix:` / `refactor:` 等前缀 + 简要描述
- **禁止**在 commit message 中添加 `Co-Authored-By` 或任何 AI 署名行

## 📚 相关资源

- [CUPS 官方文档](https://www.cups.org/documentation.html)
- [IPP 规范](https://www.pwg.org/ipp/)
- [Nuxt UI v4](https://ui.nuxt.com/)
- [Tailwind CSS v4](https://tailwindcss.com/)
- [Vue 3 文档](https://vuejs.org/)
- [ofdrw](https://github.com/ofdrw/ofdrw)

---

**维护者**：涵曦（<im.hanxi@gmail.com>）
