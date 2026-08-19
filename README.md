# 🖨️ CUPS Web — 网页打印管理

<div align="center">

[![GitHub Release](https://img.shields.io/github/v/release/hanxi/cups-web?style=flat-square&logo=github&color=blue)](https://github.com/hanxi/cups-web/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/hanxi/cups-web?style=flat-square&logo=docker)](https://hub.docker.com/r/hanxi/cups-web)
[![Docker Image Size](https://img.shields.io/docker/image-size/hanxi/cups-web/latest?style=flat-square&logo=docker&color=066da5)](https://hub.docker.com/r/hanxi/cups-web)
[![GitHub Stars](https://img.shields.io/github/stars/hanxi/cups-web?style=flat-square&logo=github)](https://github.com/hanxi/cups-web/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/hanxi/cups-web?style=flat-square&logo=github)](https://github.com/hanxi/cups-web/network/members)
[![GitHub Issues](https://img.shields.io/github/issues/hanxi/cups-web?style=flat-square&logo=github)](https://github.com/hanxi/cups-web/issues)
[![GitHub Last Commit](https://img.shields.io/github/last-commit/hanxi/cups-web?style=flat-square&logo=github)](https://github.com/hanxi/cups-web/commits)
[![GitHub Downloads](https://img.shields.io/github/downloads/hanxi/cups-web/total?style=flat-square&logo=github&color=success)](https://github.com/hanxi/cups-web/releases)
[![License](https://img.shields.io/github/license/hanxi/cups-web?style=flat-square&color=blue)](LICENSE)

[![Go Version](https://img.shields.io/github/go-mod/go-version/hanxi/cups-web?style=flat-square&logo=go)](https://golang.org)
[![Vue 3](https://img.shields.io/badge/Vue-3.5-4FC08D?style=flat-square&logo=vue.js)](https://vuejs.org)
[![Vite](https://img.shields.io/badge/Vite-7-646CFF?style=flat-square&logo=vite)](https://vitejs.dev)
[![Nuxt UI](https://img.shields.io/badge/Nuxt%20UI-v4-00DC82?style=flat-square&logo=nuxt.js)](https://ui.nuxt.com)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-v4-38B2AC?style=flat-square&logo=tailwind-css)](https://tailwindcss.com)
[![CUPS](https://img.shields.io/badge/CUPS-IPP-orange?style=flat-square)](https://www.cups.org)
[![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?style=flat-square&logo=sqlite)](https://www.sqlite.org)

🏠 [GitHub](https://github.com/hanxi/cups-web) • 🐳 [Docker Hub](https://hub.docker.com/r/hanxi/cups-web) • 📖 [开发文档](AGENTS.md) • 💬 [微信群](https://github.com/hanxi/cups-web/issues/36) • 💰 [赞赏支持](https://afdian.com/a/imhanxi)

</div>

基于 CUPS 的网页版打印管理工具。通过浏览器上传文件、远程提交打印任务，支持多用户管理与打印记录追踪，适合家庭和小型办公室使用。

## 📸 界面预览

<div align="center">
<table>
  <tr>
    <td align="center"><img src="screenshots/print1.png" width="400" alt="文件上传"><br/><b>文件上传</b></td>
    <td align="center"><img src="screenshots/print2.png" width="400" alt="打印机选择"><br/><b>打印机选择</b></td>
  </tr>
  <tr>
    <td align="center"><img src="screenshots/preview.png" width="400" alt="预览"><br/><b>实时预览</b></td>
    <td align="center"><img src="screenshots/admin.png" width="400" alt="管理后台"><br/><b>管理后台</b></td>
  </tr>
</table>
</div>

## ✨ 功能特性

### 打印能力

- **多格式支持**：PDF、图片（JPG/PNG/GIF/HEIC）、Office 文档（doc/docx/xls/xlsx/ppt/pptx）、OFD、纯文本
- **自动转换**：Office 文档通过 LibreOffice 转 PDF；OFD 通过内置 Java 转换器（基于 ofdrw）转 PDF；文本/图片在服务端渲染为 PDF
- **多图片合并打印**：一次选择多张图片自动合并为一份 PDF
- **打印选项**：份数、单双面、彩色/黑白、纸张大小、纸张类型、页面方向、页码范围、缩放、镜像打印
- **实时预览**：支持 PDF 预览、纸张方向的可视化预览、页数估算

### 打印机驱动

镜像内预装了 Debian `printer-driver-all` 等通用驱动包，覆盖大部分常见打印机。对于特定品牌打印机，提供**按需手动安装**的驱动脚本和 Web 管理界面：

**预装通用驱动**（开箱即用）：

- `printer-driver-all`：Debian 维护的驱动 meta 包，包含 splix、c2050、m2300w、ptouch 等
- `printer-driver-cups-pdf`：虚拟 PDF 打印机
- `printer-driver-escpr`：Epson ESC/P-R 标准款（大部分 Epson 喷墨老机型）
- `printer-driver-foo2zjs`：ZjStream / Hiperc / OAKT 协议（部分 HP / Konica / Minolta 老款激光机）
- `printer-driver-brlaser`：Brother 老款激光机
- `foomatic-db-compressed-ppds` + `openprinting-ppds`：海量 PPD 库
- `hplip` + `hpijs-ppds` + `hp-ppd`：HP 全系打印套件
- `ipp-usb` + CUPS 内置 driverless：IPP Everywhere / AirPrint / Mopria 自动识别

**可选厂商驱动**（通过 Web 界面或命令行按需安装，安装后自动持久化）：

| 驱动 | 命令名 | 架构 | 适用机型 |
| --- | --- | --- | --- |
| Canon UFR II | `canon-ufr2` | amd64 / arm64 | i-SENSYS LBP/MF、imageCLASS、imageRUNNER 等 |
| Canon CAPT | `canon-capt` | 全架构 🔧 | LBP2900 / LBP2900B |
| HP LaserJet 1020 固件 | `hp-laserjet1020` | 全架构 | HP LaserJet 1020 / 1020 Plus |
| HP foo2zjs 固件 | `foo2zjs-firmware` | 全架构 🔧 | HP LaserJet 1000/1005/1018/P1005/P1006/P1505 |
| Epson ESC/P-R 2 | `escpr2` | amd64 / armhf / arm64 | ET-18100, L8050, L8160, WF-7840 等 |
| Epson 国行驱动 | `epson-cn` | 仅 amd64 | L380, L455 等国行机型 |
| Konica Minolta bizhub | `konica-bizhub` | amd64 / arm64 | bizhub 3000MF |
| Sharp PostScript | `sharp` | 全架构 | MX-C2622R 等 PostScript 打印机 |
| Gutenprint | `gutenprint` | amd64 / arm64 | 大量 Epson/Canon/HP 老机型 |

> 🔧 = 需要在容器内现场编译，耗时较长（几分钟到十几分钟，ARM 小主机更慢）。其他驱动是下载 + 解包安装，通常几十秒内完成。
>
> Epson ESC/P-R 2 在 amd64 / armhf 上直接安装预编译包，在 **arm64 上会回退到源码编译**，也需要几分钟。
>
> 「架构」列就是实际的硬限制：厂商没有提供对应架构的二进制时，Web 界面会把该驱动的「安装」按钮**禁用**并提示原因，不会让你点一个必然失败的按钮。

### 驱动管理（Web 界面）

驱动管理页面**仅管理员可见**（登录后导航栏的「驱动」入口）：

- **自动检测**：扫描 USB / 网络打印机，自动匹配推荐驱动
- **一键安装**：检测到打印机后一键安装驱动，并自动 `lpadmin` 添加到 CUPS（默认纸张设为 A4）
- **驱动列表**：查看所有可用驱动的安装状态、安装时间与支持架构，一键安装 / 卸载
- **上传自定义驱动**：支持上传 PPD 文件（`.ppd`）或 Debian 包（`.deb`），仅这两种扩展名
- **驱动持久化**：安装的驱动文件自动快照到 `.drivers` 持久卷，容器重建后自动恢复

#### 安装过程是异步的，请耐心等待

点击「安装」后接口会**立刻返回**，真正的安装在后台执行，页面上会出现一个进度卡片，每 2 秒刷新一次并**实时展示编译 / 安装日志**：

- 需要编译的驱动（Canon CAPT、HP foo2zjs 固件、arm64 上的 Epson ESC/P-R 2）可能要**几分钟到十几分钟**，页面一直显示滚动日志是正常的，**不是卡住了**
- 这期间请**不要刷新页面、不要重复点击**。刷新虽然不会中断后台安装，但页面上的实时日志会丢失，只能改用 `docker compose logs -f cups` 观察
- 后台任务的硬超时是 30 分钟，页面等待上限 35 分钟

#### 同一时刻只能跑一个驱动任务

apt / dpkg 本身持有全局锁，并发安装只会互相失败。因此后端限制**同时只允许一个驱动任务**（安装 / 卸载 / 一键安装并添加），任务进行中再发起第二个会被直接拒绝，并提示「已有驱动任务正在执行，请等待其完成后重试」；有任务在跑时上传 `.deb` 也会被同样拒绝。

#### `.ppd` 会自动恢复，`.deb` 重启后需要重新安装

| 上传类型 | 容器重启后 |
| --- | --- |
| `.ppd` | ✅ 自动恢复（文件被快照到 `.drivers`，启动时还原到 `/usr/share/cups/model/custom`） |
| `.deb` | ❌ **需要重新上传安装**（只做归档记录，界面上会列出装过哪些包并提示重装） |

原因是 `.deb` 的真正安装动作发生在包内的安装脚本里，光把文件拷回来并不能让驱动生效。界面上的提示原文是：「上传的 .deb 包不会随容器重启自动恢复，重启后需要重新上传安装。」

> ⚠️ **上传 `.deb` 的安全风险**：安装 `.deb` 时 dpkg 会以 **root 身份执行包内的安装脚本**，等价于在容器里执行任意代码，并且会改动容器的系统状态。这是有意保留给管理员的能力，但也意味着**管理员账号密码等同于容器的 root 凭据**。请只上传来源可信的 `.deb`，并且**不要在不可信的多人环境下开放管理员账号**（普通 `user` 角色看不到也调不到驱动接口）。每次上传都会把上传者用户名写进容器日志，便于事后审计。

> ⚠️ **驱动持久化依赖 `./.drivers` 卷**：手动安装的第三方驱动全部快照在这个目录里。**删掉它（或忘记挂这个卷）= 丢失所有手动安装的驱动**，重启后需要在「驱动」页面重新装一遍。备份时别漏了它。

也可以通过命令行安装驱动（命令行是同步执行的，会一直占用终端直到结束）：

```bash
# 查看可用驱动
docker exec cups driver-list

# 安装驱动
docker exec cups driver-install canon-ufr2

# 查看已安装驱动
docker exec cups driver-list --installed

# 卸载驱动
docker exec cups driver-remove canon-ufr2
```

### 用户与权限

- **多用户系统**：支持 `admin` / `user` 两种角色
- **默认管理员**：首次启动自动创建 `admin/admin`，`admin` 账号受保护无法被删除或重命名
- **打印记录**：完整保存每次打印的文件、页数、份数、双面/彩色选项、状态等

### 管理后台

- **用户管理**：创建、编辑、删除用户；修改角色与联系信息
- **打印记录查询**：可按用户名、时间范围过滤
- **数据保留策略**：按天数自动清理过期打印记录和对应文件（每小时巡检一次）

### 安全

- **Session 认证**：基于 Gorilla `securecookie`（加密 + 签名），密钥自动生成并持久化到数据库
- **CSRF 防护**：对所有非 GET/HEAD/OPTIONS 请求校验 `X-CSRF-Token`
- **密码安全**：bcrypt 加密存储

## 🛠️ 技术栈

- **后端**：Go 1.26 · Gorilla Mux · SQLite（`modernc.org/sqlite`，纯 Go 实现，无需 CGO）
- **打印协议**：[OpenPrinting/goipp](https://github.com/OpenPrinting/goipp)（IPP）
- **前端**：Vue 3 · Vite 7 · [Nuxt UI v4](https://ui.nuxt.com/) · Tailwind CSS v4 · Vue Router（hash 模式）
- **文档转换**：LibreOffice（Office → PDF）· [ofdrw](https://github.com/ofdrw/ofdrw)（OFD → PDF，Java 21）
- **打印服务**：[CUPS](https://www.cups.org/)（源码编译 2.4.x，覆盖 apt 版本）

## 🚀 快速开始

提供两种部署方式：

- [Docker 部署](#docker-部署)（推荐，一键拉起 CUPS + Web）
- [二进制部署](#二进制部署)（适合已有 CUPS 服务的场景）

---

## Docker 部署

### 前置要求

- Docker 与 Docker Compose
- USB 打印机（若使用本地打印机）

### 1. 创建 `docker-compose.yml`

```yaml
services:
  cups:
    image: hanxi/cups-web:latest
    container_name: cups
    user: root
    security_opt:
      - apparmor:unconfined
    environment:
      - CUPSADMIN=${CUPSADMIN:-print}
      - CUPSPASSWORD=${CUPSPASSWORD:-print}
      - TZ=${TZ:-Asia/Shanghai}
    ports:
      - "631:631"
      - "1180:8080"
    volumes:
      - ./.etc:/etc/cups
      - ./.data:/data
      - ./.uploads:/uploads
      - ./.drivers:/opt/cups-drivers/data
      - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket
      - /dev/bus/usb:/dev/bus/usb
      - /run/udev:/run/udev:ro
    device_cgroup_rules:
      - 'c 189:* rmw'
    restart: unless-stopped
```

也可直接下载仓库内的 `docker-compose.yml`：

```bash
wget https://raw.githubusercontent.com/hanxi/cups-web/master/docker-compose.yml
```

### 2. 配置环境变量（可选）

在同目录创建 `.env`：

```bash
CUPSADMIN=print
CUPSPASSWORD=your_cups_password
TZ=Asia/Shanghai
```

### 3. 启动服务

```bash
docker compose up -d
```

### 4. 安装打印机驱动（按需）

大部分打印机靠镜像预装的通用驱动即可直接使用，只有特定品牌机型才需要这一步。

通过 Web 界面安装（推荐）：
1. 访问 <http://localhost:1180>，使用 `admin/admin` 登录
2. 点击导航栏「驱动」进入驱动管理页面（仅管理员可见）
3. 点击「扫描打印机」自动检测已连接的打印机
4. 对检测到的打印机点击「一键安装并添加」

> ⏳ 安装是**后台异步**执行的，页面上会实时滚动安装日志。需要编译的驱动可能要十几分钟，请不要刷新页面或重复点击；同一时刻只能有一个驱动任务在跑。详见 [驱动管理（Web 界面）](#驱动管理web-界面)。

或通过命令行安装：
```bash
docker exec cups driver-install canon-ufr2
```

### 5. 配置打印机

如果没有通过上面的自动检测添加打印机，也可以手动配置：

访问 CUPS 管理界面：<http://localhost:631>，使用 CUPS 管理员账号登录并添加打印机。

> ⚠️ **重要**：添加打印机后，必须在 CUPS 管理后台将其设为 **Shared（共享）** 状态，否则 Web 端无法发现该打印机。

### 6. 访问 Web

浏览器打开 <http://localhost:1180>，使用默认账号登录：

- 用户名：`admin`
- 密码：`admin`

> ⚠️ **首次登录请立即修改默认密码**。

---

## 二进制部署

适合已有 CUPS 服务的场景。

> ⚠️ **裸二进制部署的能力边界**：二进制里只有 Web 服务本身，**CUPS 需要你自己在宿主机上安装并配置好**（含打印机驱动）。镜像特有的功能在这里都不可用：
>
> - **驱动管理页面用不了**：驱动的安装 / 卸载脚本（`driver-install` 等）和持久化目录 `/opt/cups-drivers` 是**镜像内置**的。裸二进制环境下这些文件不存在，「驱动」页面里所有驱动都会显示为未安装、「安装」按钮被禁用并提示「当前镜像缺少该驱动的安装脚本」；如果强行调用接口，任务会以 `no such file or directory` 失败。请直接用宿主机的包管理器（`apt install printer-driver-*`）或 CUPS 管理界面装驱动。
> - **自动检测打印机**依赖 `lpinfo`（`cups-client` 包），缺失时「扫描打印机」会报 `failed to detect printers`。
> - **Office / OFD / PDF 标准化**依赖 LibreOffice、Java、Ghostscript，需要自行安装（见下文）。

### 1. 下载二进制

从 [GitHub Releases](https://github.com/hanxi/cups-web/releases) 下载对应平台的二进制：

| 平台 | 架构 | 文件名 |
| --- | --- | --- |
| Linux | amd64 | `cups-web-linux-amd64` |
| Linux | arm64 | `cups-web-linux-arm64` |
| Linux | armv7 | `cups-web-linux-armv7` |
| Linux | loong64 | `cups-web-linux-loong64` |
| macOS | amd64 | `cups-web-darwin-amd64` |
| macOS | arm64 | `cups-web-darwin-arm64` |
| Windows | amd64 | `cups-web-windows-amd64.exe` |

```bash
wget https://github.com/hanxi/cups-web/releases/latest/download/cups-web-linux-amd64
chmod +x cups-web-linux-amd64
```

### 2. 配置并运行

```bash
export CUPS_HOST=localhost:631
export DB_PATH=./data/cups-web.db
export UPLOAD_DIR=./uploads
export LISTEN_ADDR=:8080

./cups-web-linux-amd64
```

或使用命令行参数（优先级高于环境变量）：

```bash
./cups-web-linux-amd64 -addr :8080
```

> ⚠️ **OFD 打印仅在 Docker 镜像中开箱即用**。二进制部署若需支持 OFD，需要另行安装 Java 运行时（镜像内为 Java 21）并把 `ofd-converter.jar` 放到 `/ofd-converter.jar`（或手动改源码中的路径）。

### 3. 访问 Web

浏览器打开 <http://localhost:8080>，使用 `admin/admin` 登录。

---

## ⚙️ 配置说明

### 环境变量

| 变量名 | 说明 | 默认值 |
| --- | --- | --- |
| `LISTEN_ADDR` | Web 服务监听地址 | `:8080` |
| `DB_PATH` | SQLite 数据库路径 | `data/cups-web.db` |
| `UPLOAD_DIR` | 上传文件目录 | `uploads` |
| `CUPS_HOST` | CUPS 服务地址（Docker 内默认 `localhost`） | `localhost` |
| `CUPSADMIN` | CUPS 管理员用户名 | `print` |
| `CUPSPASSWORD` | CUPS 管理员密码 | `print` |
| `TZ` | 时区 | `Asia/Shanghai` |

> 💡 `.env.example` 只列了 Docker 部署常用的三个：`CUPSADMIN` / `CUPSPASSWORD` / `TZ`。这三个在镜像里都已有内置默认值（`print` / `print` / `Asia/Shanghai`），不写 `.env` 也能启动。
>
> 💡 `DB_PATH` / `UPLOAD_DIR` 的默认值是**相对路径**，镜像内工作目录是 `/`，所以容器里实际落在 `/data/cups-web.db` 与 `/uploads/`，正好对应下面的卷映射，通常不需要显式设置。
>
> 💡 `CUPS_HOST` 单容器化后默认就是 `localhost`（cupsd 与 Web 同容器），一般无需设置；只有把 Web 指向**另一台机器**上的 CUPS 时才需要，可写 `host` 或 `host:port`（省略端口时自动补 `631`）。

### 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-addr` | 监听地址，优先级高于 `LISTEN_ADDR` |

### 默认端口

- CUPS：`631`（管理界面 + IPP 协议）
- Web：容器内 `8080`，`docker-compose.yml` 默认映射到宿主机 `1180`

### 数据持久化目录

Docker 默认卷映射：

| 宿主机路径 | 容器路径 | 说明 |
| --- | --- | --- |
| `./.etc` | `/etc/cups` | CUPS 配置（打印机、PPD 等） |
| `./.data` | `/data` | cups-web 数据库 |
| `./.uploads` | `/uploads` | 上传的原始文件与转换后 PDF |
| `./.drivers` | `/opt/cups-drivers/data` | 手动安装的打印机驱动快照（⚠️ 删除即丢失全部手动装的驱动） |

此外还有两个**非数据类**的挂载，用于 USB 打印机识别与热插拔：

| 宿主机路径 | 容器路径 | 说明 |
| --- | --- | --- |
| `/dev/bus/usb` | `/dev/bus/usb` | 以**目录**方式挂载（而不是 `devices:`），这样打印机后开机时新建的设备节点能实时传播进容器 |
| `/run/udev` | `/run/udev`（只读） | 让 libusb 读到设备属性，改善识别；宿主机没有该目录时可以删掉这一行 |
| `/run/dbus/system_bus_socket` | `/run/dbus/system_bus_socket` | 共享宿主机的 D-Bus system bus socket，让容器内的 CUPS 能通过宿主机的 avahi-daemon 广播 AirPrint 服务（[Issue #94](https://github.com/hanxi/cups-web/issues/94)）。需要宿主机安装并运行 `avahi-daemon`，详见 [AirPrint 搜不到打印机](#airprint-搜不到打印机) |

### 其他 compose 选项说明

| 选项 | 为什么需要 |
| --- | --- |
| `user: root` | 容器内要运行 cupsd、`lpadmin`、`dpkg`（驱动安装），还要往 `/usr/lib/cups`、`/usr/share/ppd` 等系统路径写驱动文件 |
| `security_opt: [apparmor:unconfined]` | 解除 AppArmor 限制（[Issue #91](https://github.com/hanxi/cups-web/issues/91)）。PVE (Proxmox VE) LXC 等环境下会出现 `apparmor="DENIED"` 导致打印失败；单容器化后它同时也保护 LibreOffice / OFD 转换子进程不被拦截 |
| `device_cgroup_rules: ['c 189:* rmw']` | 放开 USB 字符设备（major 189）的 cgroup 权限，配合 `/dev/bus/usb` 目录挂载实现 USB 打印机热插拔（[Issue #81](https://github.com/hanxi/cups-web/issues/81)）。若你的 Docker 环境不支持该字段，改用 `privileged: true` |

---

## 📖 使用指南

### 支持的文件格式

| 类型 | 扩展名 | 处理方式 |
| --- | --- | --- |
| PDF | `.pdf` | 直接打印 |
| 图片 | `.jpg` `.jpeg` `.png` `.gif` `.heic` | 转换为 PDF（支持多张合并） |
| Office | `.doc` `.docx` `.xls` `.xlsx` `.ppt` `.pptx` | 通过 LibreOffice 转换 |
| OFD | `.ofd` | 通过 ofdrw 转换 |
| 文本 | `.txt` `.md` `.html` | 服务端渲染为 PDF |

### 打印流程

1. 选择打印机
2. 上传文件（支持多图）
3. 预览转换后的 PDF、调整打印参数
4. 确认提交，系统自动落库并下发到 CUPS

### 管理员功能

- **用户管理**：创建、编辑、删除；默认 `admin` 账号不可删除、不可改名、角色固定
- **打印记录**：查看全站记录，按用户名/日期过滤，下载原始文件
- **系统设置**：数据保留天数（`0` 表示永久保留）
- **驱动管理**：自动检测打印机、安装/卸载驱动、上传自定义 PPD/deb（后台异步执行 + 实时日志，同时只跑一个任务）

---

## 🔧 进阶配置

### 使用 HTTPS

通过反向代理（例如 Nginx）提供 HTTPS：

```nginx
server {
    listen 443 ssl;
    server_name example.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:1180;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 修改端口

编辑 `docker-compose.yml`：

```yaml
services:
  cups:
    ports:
      - "你的CUPS端口:631"
      - "你的Web端口:8080"
```

### 数据备份

```bash
cp ./.data/cups-web.db /backup/location/
tar -czf uploads-backup.tar.gz ./.uploads/
tar -czf cups-config-backup.tar.gz ./.etc/
tar -czf drivers-backup.tar.gz ./.drivers/
```

> 💡 `.drivers` 一定要一起备份——它是所有手动安装的第三方驱动的唯一副本。注意它是**按架构**快照的：把 amd64 上备份的 `.drivers` 恢复到 arm64 机器上，驱动列表会提示「安装于 amd64，与当前架构不符，建议卸载重装」。

---

## ❓ 常见问题

### 忘记管理员密码怎么办？

删除数据库文件后重启即可重置为默认 `admin/admin`（**会丢失全部数据**）：

```bash
docker compose down
rm ./.data/cups-web.db
docker compose up -d
```

### Web 端看不到打印机？

1. 检查打印机是否在 CUPS 中正常列出（<http://localhost:631>）
2. 确认打印机设置为 **Shared**
3. 重启容器：`docker compose restart cups`

### 打印机后开机就识别不到？（USB 热插拔）

使用最新的 `docker-compose.yml`（volume 目录挂载 `/dev/bus/usb` + `device_cgroup_rules`）即可支持热插拔。若你的 Docker 环境不支持 `device_cgroup_rules`，改用 `privileged: true` 即可。

### AirPrint 搜不到打印机？

手机 / iPad 的 AirPrint 通过 mDNS（Bonjour）在局域网发现打印机。Docker 默认的 bridge 网络无法广播 mDNS 多播包，需要借助宿主机的 avahi-daemon 来广播（[Issue #94](https://github.com/hanxi/cups-web/issues/94)）。

**方法一（推荐）：宿主机安装 avahi-daemon + 共享 D-Bus**

1. 在宿主机上安装 avahi-daemon：
   ```bash
   # Debian / Ubuntu
   sudo apt install avahi-daemon
   sudo systemctl enable --now avahi-daemon
   ```
2. 确认 `docker-compose.yml` 中已挂载 D-Bus socket（最新版已包含）：
   ```yaml
   volumes:
     - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket
   ```
3. 重启容器：`docker compose up -d`

原理：容器内的 CUPS 通过挂载的 D-Bus socket 与宿主机的 avahi-daemon 通信，由宿主机的 avahi 在局域网广播打印机服务，手机即可发现。

**方法二：使用 host 网络模式**

将 `docker-compose.yml` 改为 `network_mode: host`（删掉 `ports:` 配置）：
```yaml
services:
  cups:
    image: hanxi/cups-web:latest
    network_mode: host
    # ... 其他配置不变，删掉 ports 部分
```

容器直接使用宿主机网络栈，avahi 可以直接在局域网广播。缺点是无法自定义端口映射，CUPS 固定占用 631、Web 固定占用 8080。

### 安装的驱动丢失了？

确认 `docker-compose.yml` 中已配置驱动持久化卷：
```yaml
volumes:
  - ./.drivers:/opt/cups-drivers/data
```

驱动数据保存在 `.drivers` 目录中，容器重建后自动恢复。**删掉该目录就等于丢失全部手动安装的驱动**，需要在「驱动」页面重新安装。

另外，通过「上传自定义驱动」装的 **`.deb` 包本身不会自动恢复**（只有 `.ppd` 会），容器重启后需要重新上传安装一次，界面上会列出装过哪些包作为提醒。

### 驱动安装一直在转圈，是卡住了吗？

大概率没有。安装是后台异步执行的，页面上的进度卡片每 2 秒刷新一次并实时显示日志：

- 需要编译的驱动（Canon CAPT、HP foo2zjs 固件、arm64 上的 Epson ESC/P-R 2）在 ARM 小主机上十几分钟都属正常
- 只要日志还在增长就说明在正常推进；**不要刷新页面或重复点击**
- 如果不放心，另开终端看容器日志：`docker compose logs -f cups`
- 后台任务硬超时 30 分钟，页面等待上限 35 分钟；真的超时会明确报错

### 提示「已有驱动任务正在执行」？

apt / dpkg 有全局锁，所以同一时刻只允许一个驱动任务（安装 / 卸载 / 一键安装并添加）；有任务在跑时上传 `.deb` 也会被拒绝。等当前任务在页面上显示完成后再操作即可。

### 某个驱动的「安装」按钮是灰的？

两种原因，鼠标悬停在按钮上会显示具体提示：

- **当前架构 xxx 不支持**：厂商没有提供该 CPU 架构的二进制（例如 Epson 国行驱动仅 amd64、Canon UFR II 仅 amd64/arm64、Gutenprint 在 armhf 上无包）。请改用预装的通用驱动或 driverless/IPP Everywhere
- **当前镜像缺少该驱动的安装脚本**：通常出现在裸二进制部署下（驱动脚本是镜像内置的），或使用了裁剪过的镜像

### 在 PVE LXC 容器中部署时失败？（AppArmor DENIED）

在服务中添加 `security_opt: [apparmor:unconfined]`（最新 `docker-compose.yml` 已包含）。PVE LXC 容器还需开启 `Nesting` 和 `Keyctl` 功能。

### Office / OFD 转换失败？

- 转换有 **60 秒超时**，复杂文档可能超时
- 确认文档本身未损坏；可尝试本地先另存为 PDF 再上传
- 查看日志：`docker compose logs -f cups`

### 如何查看日志？

```bash
docker compose logs -f cups
```

---

## 🤝 贡献

欢迎提 Issue 和 Pull Request。开发者文档请参阅 [AGENTS.md](AGENTS.md)。

## 📈 Star History

<div align="center">

[![Star History Chart](https://star-history.dera.page/svg?repos=hanxi/cups-web&type=Date)](https://star-history.dera.page/#hanxi/cups-web&Date)

如果这个项目对你有帮助，欢迎点击右上角的 ⭐ **Star** 让更多人发现它！

</div>

## 💖 支持项目

如果这个项目对你有帮助，欢迎通过以下方式支持：

### ⭐ Star 项目

点击右上角的 ⭐ Star 按钮，让更多人发现这个项目。

### 💰 赞赏支持

- [💝 爱发电](https://afdian.com/a/imhanxi) — 持续支持项目发展
- 扫码请作者喝杯奶茶 ☕

<p align="center">
  <img src="https://i.v2ex.co/7Q03axO5l.png" alt="赞赏码" width="300">
</p>

感谢你的支持！❤️

## 📄 许可证

本项目采用 MIT 许可证，详见 [LICENSE](LICENSE)。
