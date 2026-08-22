# CUPS Web 开发者指南

本文档面向开发者，介绍项目架构、API、开发流程与扩展方式。用户文档请参阅 [README.md](README.md)。

> 📚 **深度文档**：原理说明、故障案例与历史决策已移至 [docs/](docs/README.md)，本文件只保留可快速扫读的规则与契约。

## 📦 项目概述

- **项目定位**：基于 CUPS 的 Web 打印管理工具，前后端分离
- **技术栈**：Go 1.26（后端）+ Vue 3（前端）+ SQLite（存储）+ IPP（打印协议）
- **部署形态**：单二进制（前端 `go:embed`）连接外部 CUPS；**单容器（AIO）Docker 镜像**（`cupsd` + `cups-web` 同容器，内置 LibreOffice + Java 21 + OFD 转换器 + Ghostscript + 打印驱动生态）

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
│   └── driver/                    # 驱动管理命令 + 安装脚本
├── docker-fonts/                  # 构建期字体与 gs/fontconfig 配置
├── entrypoint.sh                  # AIO 容器启动脚本
├── Dockerfile                     # 五阶段构建
├── docker-compose.yml             # 单服务 AIO
└── Makefile                       # 构建脚本
```

## 🔌 HTTP API

所有接口以 `/api` 为前缀。除登录/登出/csrf/session 外均需 `RequireSession` + `ValidateCSRF`；管理员接口再叠加 `RequireAdmin`。

> **CSRF 约定**：登录成功后下发 `csrf_token` Cookie（非 HttpOnly）；前端非 GET 请求带 `X-CSRF-Token` 头。

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
| POST | `/api/estimate` | 估算页数 |
| POST | `/api/convert` | 文档 → PDF（单文件 `file` / 多图 `files`） |
| POST | `/api/print` | 提交打印 |
| POST | `/api/compose` | 多页拼版 |
| GET | `/api/print-records` | 打印记录 |
| GET | `/api/print-records/{id}/file` | 下载原始文件 |
| POST | `/api/print-records/{id}/reprint` | 重打参数预填 |

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
| `duplex` | `"true"` / `"false"` | 双面 |
| `color` | `"true"` / `"false"` | 彩色 |
| `copies` | int | 份数 |
| `orientation` | `portrait` / `landscape` | 方向 |
| `paper_size` | `A4` / `A3` / `5inch`…`10inch` | 纸张尺寸 |
| `paper_type` | `plain` / `photo` / … / `auto` | 纸张类型 |
| `media_source` | string | 进纸盒（`auto` 不发送） |
| `print_scaling` | `auto` / `auto-fit` / `fit` / `fill` / `none` / 纯数字 `10`–`400` | 缩放；纯数字＝自定义百分比 |
| `page_range` | string | 页码范围 |
| `page_set` | `all` / `odd` / `even` | 页面子集（`all` 不发送） |
| `mirror` | `"true"` / `"false"` | 镜像 |
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

### `settings`

KV 表。当前键：`retention_days`（`0` = 永久）、`session_hash_key` / `session_block_key`。

## 🔐 认证与安全

1. **启动**：`auth.SetupSecureCookie` 从 settings 读取/生成密钥
2. **登录**：写 `session`（HttpOnly，加密+签名）+ `csrf_token`（非 HttpOnly）
3. **鉴权链**：`RequireSession` → `RequireAdmin`（管理员）→ `ValidateCSRF`（非 GET）
4. **登出**：两条 cookie `MaxAge=-1`
5. **默认管理员**：`bootstrap.go` 保证 `admin/admin` 存在且 `protected=1`；`Username == "admin"` 判定保护（禁止改名/改角色/删除）

## 🖨️ 打印流水

`printHandler` 流程：接收 multipart → 落盘 `uploads/YYYYMMDD/` → 类型识别 & 转换 → 页数统计 → 插入 `queued` 记录 → IPP 提交 → 回写 `printed`。

- `pdf` → `normalizePDF`（gs → LibreOffice → passthrough）
- `office` → LibreOffice `--convert-to pdf`
- `ofd` → `java -jar /ofd-converter.jar`
- `image` → `gofpdf`（大图先下采样到 3000px）
- `text` → `gofpdf` + 内嵌中文字体

> ⚠️ 标准化管线只解决 CUPS 老驱动兼容性问题，gs 会破坏空壳 CJK 字体导致 pdf.js 预览错位。管线原理与 cidfmap 机制见 [docs/pdf-pipeline.md](docs/pdf-pipeline.md)。

### 自定义百分比缩放（`pdf_scale.go`）

`print_scaling` 为纯数字时走 `resolveCustomScaling`：gs 预缩放 PDF 内容 → 发给 CUPS 的 `print-scaling` 换成 `none`。
打印与重打（`reprintHandler`）两条路径都必须调用它。

- 🚫 **纯数字绝不能透传给 IPP**：`print-scaling` 是 keyword，`"40"` 不是合法取值。gs 失败/非 PDF 时退回空串（打印机默认），不是原样下发。
- 🚫 gs 缩放**不要**用 `-dFIXEDMEDIA` + 固定 `-dDEVICEWIDTHPOINTS`：横向页会被塞进纵向纸并偏移。现在的 `Install` 过程逐页读 `currentpagedevice /PageSize`，纸张尺寸原样保留。
- ⚠️ `-sOutputFile` 必须排在 `-f` **之前**——`-f` 之后的参数会被 gs 当成输入文件名，放末尾会直接报 `requires an output file`（Issue #98 的根因）。
- 前端预览用等比 CSS transform 同步这个百分比（`PrintPreview.vue` / `PdfCanvas.vue`），其余缩放模式交给 CUPS，预览不模拟。

## 🧹 维护任务

`maintenance.go` 每小时：读 `retention_days`（`0` 跳过）→ 删过期 `print_jobs` + 文件 → `VACUUM` + `wal_checkpoint`。管理员可 `POST /api/admin/cleanup` 手动触发。

## 🧩 驱动管理

相关文件：`driver_handlers.go`、`driver_registry.go`、`scripts/driver/*.sh`、`DriversView.vue`。

### 目录约定

| 路径 | 内容 |
| --- | --- |
| `/opt/cups-drivers/scripts/install-<name>.sh` | 安装脚本（构建期 COPY，不执行） |
| `/usr/local/bin/{driver-install,driver-list,driver-remove,restore-drivers}` | 管理命令 |
| `/opt/cups-drivers/data/<driver>/manifest.txt` | 文件清单 = **"已安装"唯一标记** |
| `/opt/cups-drivers/data/<driver>/metadata.txt` | `driver=` / `installed_at=` / `file_count=` / `arch=` |
| `/opt/cups-drivers/data/<driver>/<绝对路径镜像>` | 产物副本 |
| `/opt/cups-drivers/data/custom-ppd/` | 上传 `.ppd`（写 manifest，可恢复） |
| `/opt/cups-drivers/data/custom-deb/packages/` | 上传 `.deb`（**不写 manifest**，重启后需手动重装） |

`docker-compose.yml` 把 `./.drivers` 挂到 `/opt/cups-drivers/data`。**不挂卷 = 重启丢驱动。**

### 退出码约定

| 退出码 | 含义 | 行为 |
| --- | --- | --- |
| `0` | 成功 | 写 manifest |
| `3` | 架构不支持 | 不写 manifest，`exit 3` |
| 其他非零 | 真失败 | 不写 manifest，透传 |

**退出码 0 但无新文件也判失败**（拒绝写 manifest）。

### 架构探测

统一用 `dpkg --print-architecture`（**不要用 `dpkg-architecture`**，runtime 无 `dpkg-dev`）。Go 侧 `currentDebArch()` 映射 `GOARCH` → Debian 命名。multiarch 用 `detect_multiarch_libdir()`（glob `/usr/lib/*-linux-gnu*`，拿不到返回空串）。

### manifest 白名单速查

**ALLOW**：`/usr/lib/cups`、`/usr/share/cups`、`/usr/share/ppd`、`/usr/share/foomatic`、`/lib/firmware`、`/usr/lib/firmware`、`/usr/lib/<multiarch>`。

**DENY**：`/usr/bin/*`、`/usr/sbin/*`、`/bin/*`、`/sbin/*`、`/usr/local/{bin,sbin}/*`、`/etc/*`、`/var/*`、`/usr/include/*`、`/opt/cups-drivers/*`、`/tmp/*`、`/usr/share/{doc,man,locale,info}/*`、`/usr/share/cups/doc-root/*`、`*/pkgconfig/*`、`*.a`、`*.o`、`*.la`。

> ⚠️ **白名单在 `driver-install.sh` / `driver-remove.sh` / `restore-drivers.sh` 三处各有一份，必须永久保留全部三份**——remove/restore 侧是给存量被污染快照兜底的。🚫 不要因为 install 侧已过滤就删掉另外两处。详见 [docs/driver-management.md](docs/driver-management.md#-manifest-白名单为什么必须存在且三处都要有)。

### AIO 编译脚本约定

> ⚠️ 编译型脚本**只允许一个 `trap _cleanup EXIT`**（bash 同信号只保留最后注册的 handler）。AIO 模式下只 `apt-get clean`，**绝不 `rm -rf /var/lib/apt/lists/*`**。详见 [docs/driver-management.md](docs/driver-management.md#-aio-编译脚本的单一-exit-trap约定)。

### 上传自定义驱动

- **`.ppd`**：校验 → 装到 `/usr/share/cups/model/custom/` → 写 manifest（可恢复）
- **`.deb`**：`dpkg -i`（失败补依赖后**必须再 `dpkg -i` 一次**）→ 归档到 `custom-deb/packages/`，**不写 manifest**（重启后需手动重装）
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

1. `restore-drivers`（恢复驱动快照）
2. CUPS 管理员用户 + tzdata
3. CUPS 配置还原（空卷时从 `/etc/cups-bak/` 复制）
4. HP 1020 PPD Letter → A4 修补（issue #48）
5. HP host-based 固件上传（后台）
6. dbus + avahi + ipp-usb（后台，允许失败）
7. cupsd + watchdog
8. 等 cupsd 就绪（`lpstat -r`，30 × 1s）
9. AirPrint A4 `media-ready` 修补（issue #82，后台）
10. `exec /cups-web`（PID 1）

> ⚠️ cupsd 必须在 watchdog 子 shell **内部前台**启动（`wait` 只能等自己的子进程，否则 127 重启风暴）。🚫 不要把 `cupsd -f` 挪到子 shell 外面。
>
> ⚠️ `restore-drivers` 必须永远 `exit 0`（驱动恢复是尽力而为，不能阻塞启动，否则用户连 Web UI 都进不去无法自救）。
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
```

### Vite 开发代理

`/api → http://localhost:8090`。本地调试：后端 `LISTEN_ADDR=:8090 go run ./cmd/server`，前端 `bun run dev`。

### 构建产物分包

`vue-vendor`（vue/router）、`ui-vendor`（nuxt-ui/reka-ui/vueuse）、`pdf-vendor`（pdfjs-dist）。

## 🚢 部署

### docker-compose

单服务 `cups`（AIO），`image: hanxi/cups-web:latest`，端口 `631:631` + `1180:8080`。

| 配置 | 为什么 |
| --- | --- |
| `user: root` | cupsd / lpadmin / dpkg / 写系统路径 |
| `security_opt: [apparmor:unconfined]` | PVE LXC AppArmor DENIED（issue #91） |
| `./.etc:/etc/cups`、`./.data:/data`、`./.uploads:/uploads` | 持久化 |
| **`./.drivers:/opt/cups-drivers/data`** | **驱动快照持久化**（删 = 重启丢驱动） |
| `/dev/bus/usb:/dev/bus/usb` + `device_cgroup_rules` | USB 热插拔（issue #81） |
| `/run/udev:/run/udev:ro` | libusb 设备属性（可选） |
| `/run/dbus/system_bus_socket:/run/dbus/system_bus_socket` | 共享宿主机 D-Bus system bus socket，让 CUPS 通过宿主机 avahi 广播 AirPrint（issue #94） |

### Docker 构建

五阶段：`frontend-build`（node:20-slim）→ `java-builder`（BUILDPLATFORM 锁 amd64）→ `builder`（golang:1.26）→ `cups-builder`（源码编译 CUPS）→ `runtime`（trixie-slim）。覆盖 `linux/amd64` + `arm64` + `arm/v7`。

> 🚨 `cups-builder` 的 `ca-certificates` 请勿删除（wget TLS 校验，删了 CI 直接崩）。
>
> 五阶段设计理由、三架构镜像选型史、HOME/LibreOffice profile 见 [docs/docker-build.md](docs/docker-build.md)。

### CI/CD

- **`build-release.yml`**：7 平台交叉编译 + tag 自动 Release。Go 版本与 `go.mod` 一致（`1.26`）。
- **`docker-publish.yml`**：`master` / `v*` tag → 三架构镜像。开头有 `Free disk space` 步骤。

### 版本管理

`./bump-version.sh patch|minor|major`

## 🎯 常见开发任务 / 调试 / 代码风格

> 新增 API、修改 DB、新增前端页面、新增文件类型、新增驱动的步骤模板，以及调试命令、Go/Vue 风格、Git 提交约定，见 [docs/conventions.md](docs/conventions.md)。
>
> 🚫 **commit message 禁止 `Co-Authored-By` 及任何 AI 署名行**，中文撰写。

**维护者**：涵曦（<im.hanxi@gmail.com>）
