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
- **打印选项**：份数、单双面、彩色/黑白、纸张大小（含驱动支持的无边距规格）、介质类型、打印质量、页面方向、页码范围、缩放、镜像打印
- **实时预览**：支持 PDF 预览、纸张方向的可视化预览、页数估算
- **记住设置与打印预置**（[Issue #128](https://github.com/hanxi/cups-web/issues/128)）：标准打印成功后自动记住本次参数（按浏览器保存，不含水印文字和页码范围），下次进入自动恢复；「打印设置 → 预置」可把当前参数保存为多组命名预置，一键套用、重命名、覆盖或删除，预置按账号保存可跨设备使用（访客模式不可用）；「恢复默认设置」一键回到出厂默认
- **照片定时队列**：在「定时 → 新建定时任务」选择「照片按顺序打印」，批量上传 JPG / PNG / GIF，拖动或用箭头排序，共用打印参数；每天、每周或每月按设定张数提交下一批（1–100 张，默认 1 张），全部提交后结束，也可循环打印。页面显示进度、下一张和最近执行，支持暂停、恢复及提前打印下一批。

定时任务按服务器本地时区执行，开始日期也按该时区解释。每张成功提交后保存进度；批次中途失败会停止，已成功的照片不会重复提交，下一次从失败照片继续。宕机期间错过时间不会消耗照片；「打印下一批」会提前执行下一次计划，暂停中的任务仍保持暂停。照片与进度保存在现有数据库和上传目录中，容器重启可保留；任务删除时同时清理原图和缩略图。进度表示 CUPS 已接受任务，不表示已经出纸。如果提交过程中服务中断或返回结果不确定，队列保持暂停并显示「结果待确认」，核实打印机后再恢复。

打印页面、重打弹窗和定时任务均会读取目标队列的驱动选项，可用「刷新选项」重新获取。无边距只在驱动或 IPP 明确提供对应规格时显示；图片转 PDF 时会去掉应用额外添加的边距，照片比例与纸张不同时仍可能留白，可先裁剪照片。驱动选项读取失败时显示原因并保留已有选择。接口和兼容约定见 [打印机选项文档](docs/printer-options.md)。

### 扫描能力（[Issue #111](https://github.com/hanxi/cups-web/issues/111)）

AIO 镜像内置 `scanimage` + `libsane-hpaio`，Web 界面「扫描」入口对所有登录用户开放：

- **单页平板扫描**：MVP 支持单张扫描；ADF / 双面暂未接入
- **输出格式**：PNG（无损）/ JPEG（较小）/ PDF（先扫成 PNG，再用 Ghostscript 合成，规避不同 SANE 后端对 `--format=pdf` 支持不一致的问题）
- **可选参数**：模式（Color / Gray / Lineart）、分辨率（75–600 dpi）、来源（Flatbed / ADF，按设备实际能力显示）
- **异步任务 + 实时日志**：提交后立即返回 `jobId`，页面每 1.5 秒轮询进度并展示 `scanimage` / `gs` 的实时输出；硬超时 5 分钟
- **历史记录持久化**：扫描件默认存到 `SCAN_DIR`（`docker-compose.yml` 里挂到 `./.scans:/scans`），元数据落 `scan_records` 表，普通用户只看得到自己的记录，管理员可看全站
- **设备可用性探测**：`scanimage -L` 会同时列出 `hpaio:` 与 `hpljm1005:` / `escl:` 等多个后端，但 `hpaio:` 在部分 HP 一体机（如 M1005）上打开就报 SANE `Error during device I/O`。选中设备后前端会异步调 `/api/scan/devices/probe` 探测一次；打不开时给出红字提示，请换 `hpljm1005:` / `escl:` 等零配置后端再试

> ⚠️ **不用 hplip 自带的 `hp-scan`**：容器里 HPLIP daemon 与 dbus 依赖不稳定，即便补启 `dbus-daemon --system --fork` 仍报 `SANE: Error during device I/O (code=9)`。改走标准 SANE 栈的 `scanimage` 子进程，同宿主同硬件（如 HP LaserJet M1005 MFP）已验证可用。
>
> 📌 **当前镜像标签**：扫描功能与设备探测已合并到 master 分支，但 Docker Hub 上的 `hanxi/cups-web:master` 尚未重建。当前请拉 `hanxi/cups-web:dev`（每次 master push 自动更新）体验，或从源码 `make all` 自行构建；等 `master`/正式版镜像更新后再切回 `latest`。

### 打印机驱动

AIO 镜像内预装了 Debian `printer-driver-all` 等通用驱动包，覆盖大部分常见打印机。对于特定品牌打印机，提供**按需手动安装**的驱动脚本和 Web 管理界面：

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

#### `.ppd` 与 `.deb` 都会自动恢复

| 上传类型 | 容器重启后 |
| --- | --- |
| `.ppd` | ✅ 自动恢复（文件被快照到 `.drivers`，启动时还原到 `/usr/share/cups/model/custom`） |
| `.deb` | ✅ 自动重新安装（原件归档到 `.drivers/custom-deb/packages/`，启动时用 `dpkg -i` 装回来） |

`.deb` 的真正安装动作发生在包内的安装脚本里，光把文件拷回来并不能让驱动生效 —— 所以它走的是"归档整个包、重启时重新安装一遍"的路子，而不是逐个文件还原。重复安装是安全的：已经装上且版本不更旧的包会被跳过。

> 💡 如果某个上传的 `.deb` 始终装不上，它会在每次容器启动时重试一遍（日志里能看到告警）。想彻底移除它，删掉宿主 `./.drivers/custom-deb/packages/` 下对应的文件即可。

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

### API 密钥（[Issue #113](https://github.com/hanxi/cups-web/issues/113)）

面向第三方系统（微信 / 飞书 / 钉钉机器人、家庭自动化脚本等）直接调用打印相关 API，不再依赖浏览器 cookie / CSRF。

- **入口**：登录后顶部导航「密钥」，或直接访问 `/#/api-keys`。
- **管理**：可设备注名、有效期（永不过期 / 7 / 30 / 90 / 365 天），支持删除。明文密钥仅在创建时返回一次，请立即保存。
- **调用**：请求头带 `Authorization: Bearer <密钥>` 或 `X-API-Key: <密钥>`，无需再走登录/CSRF。密钥继承其归属用户的角色（`user` 不能访问管理接口）。
- **限制**：`guest` 访客账号禁止签发或使用密钥；API 密钥自身不能用来创建或删除其他密钥（须在浏览器 UI 上操作）。

```bash
# 列出打印机
curl -H "Authorization: Bearer cw_XXXXXXXX" https://your-host/api/printers
```

## 🛠️ 技术栈

- **后端**：Go 1.26 · Gorilla Mux · SQLite（`modernc.org/sqlite`，纯 Go 实现，无需 CGO）
- **打印协议**：[OpenPrinting/goipp](https://github.com/OpenPrinting/goipp)（IPP）
- **前端**：Vue 3 · Vite 7 · [Nuxt UI v4](https://ui.nuxt.com/) · Tailwind CSS v4 · Vue Router（hash 模式）
- **文档转换**：LibreOffice（Office → PDF）· [ofdrw](https://github.com/ofdrw/ofdrw)（OFD → PDF，Java 21）
- **打印服务**：[CUPS](https://www.cups.org/)（源码编译 2.4.x，覆盖 apt 版本）

## 🚀 快速开始

提供三种部署方式：

- [Docker 部署](#docker-部署)（推荐，一键拉起 CUPS + Web）
- [仅 Web 镜像](#仅-web-镜像外部-cups)（已有 CUPS 服务，或搭配其他 CUPS 镜像）
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
    hostname: CUPS
    # mDNS/DNS-SD 依赖局域网组播，桥接模式下发现不了网络打印机、
    # AirPrint 也广播不出去，必须用 host 网络（issue #107）
    network_mode: host
    security_opt:
      - apparmor:unconfined
    environment:
      - CUPSADMIN=${CUPSADMIN:-print}
      - CUPSPASSWORD=${CUPSPASSWORD:-print}
      - TZ=${TZ:-Asia/Shanghai}
      # host 网络下 Web 直接监听宿主端口，默认 1180（与旧版端口映射一致）
      - LISTEN_ADDR=${LISTEN_ADDR:-:1180}
      # 扫描件输出目录，落在下面 ./.scans:/scans 卷里
      - SCAN_DIR=${SCAN_DIR:-/scans}
    volumes:
      - ./.etc:/etc/cups
      - ./.data:/data
      - ./.uploads:/uploads
      - ./.drivers:/opt/cups-drivers/data
      - ./.scans:/scans
      - /dev/bus/usb:/dev/bus/usb
      - /run/udev:/run/udev:ro
    device_cgroup_rules:
      - 'c 189:* rmw'
    # 特权模式：ARM / 嵌入式平台上 libusb 需要完整 sysfs 访问才能枚举 USB
    # 打印机（issue #110）。如果你确认 x86 平台不需要，可注释掉这行。
    privileged: true
    restart: unless-stopped
```

也可直接下载仓库内的 `docker-compose.yml`：

```bash
wget https://raw.githubusercontent.com/hanxi/cups-web/master/docker-compose.yml
```

> 💡 镜像标签：`latest` = 最新正式发行版（v* tag）；`dev` = master 分支最新构建（每次 push 更新，尝鲜用）。`docker-compose.yml` 默认用 `latest`，想试 dev 版本时把 `image: hanxi/cups-web:latest` 改成 `image: hanxi/cups-web:dev` 即可。

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

> 💡 备选 CUPS 镜像：[tzishue/cloud-printer](https://hub.docker.com/r/tzishue/cloud-printer)，提供较丰富的打印机驱动。

---

## 仅 Web 镜像（外部 CUPS）

如果已有 CUPS 服务，可以使用仅包含 cups-web 服务的镜像。它保留 Web 界面、多用户、打印记录和 Office / OFD / PDF 文档转换能力，通过 `CUPS_HOST`（TCP）或 `CUPS_SOCKET_PATH`（Unix Socket）连接外部 CUPS。

| 标签 | 内容 |
| --- | --- |
| `hanxi/cups-web:latest` / `dev` / `vX.Y.Z` | CUPS + Web 一体化镜像 |
| `hanxi/cups-web:latest-web` / `dev-web` / `vX.Y.Z-web` | 仅 Web 镜像，连接外部 CUPS |

先在外部 CUPS 中安装驱动、添加并共享打印机，确保 Web 容器能够访问其 IPP 服务。打印机配置、驱动安装和 AirPrint 广播由外部 CUPS 负责；仅 Web 镜像不提供本地 USB 扫描和驱动管理工具，无需 USB 挂载或特权模式。

在 `.env` 中设置外部 CUPS 地址，例如：

```dotenv
CUPS_HOST=192.168.1.100:631
```

使用仓库中的 [docker-compose.web.yml](docker-compose.web.yml) 启动：

```bash
docker compose -f docker-compose.web.yml up -d
```

浏览器访问 <http://localhost:1180>，首次使用 `admin/admin` 登录并修改密码。数据库和上传文件分别持久化到 `./.data`、`./.uploads`。

`CUPS_HOST` 应填写 Web 容器可访问的主机地址或同一 Docker 网络中的 CUPS 服务名，格式为 `host:port`；容器内的 `localhost` 指向 Web 容器自身。若与其他 CUPS 容器配合，请将两个服务加入同一 Docker 网络，并按该 CUPS 镜像的说明配置服务。

### 通过 Unix Socket 连接

同一宿主机上可以共享 CUPS 的运行目录，通过 Unix Socket 通信，无需为 Web 连接开放 CUPS TCP 端口。在外部 CUPS 的 `cupsd.conf` 中启用：

```conf
Listen /var/run/cups/cups.sock
```

连接宿主机 CUPS 时，使用仓库中的 [Socket Compose 配置](docker-compose.web.socket.yml)：

```bash
docker compose -f docker-compose.web.yml -f docker-compose.web.socket.yml up -d
```

默认将宿主机 `/var/run/cups` 只读挂载到 Web 容器中的同一路径；宿主机目录不同时，在 `.env` 中设置 `CUPS_SOCKET_DIR`。该配置自动设置 `CUPS_SOCKET_PATH=/var/run/cups/cups.sock`，无需设置 `CUPS_HOST`。

跨容器部署时，在现有 CUPS Compose 项目中让两个服务挂载同一个运行目录卷。例如，将下列配置合并到已有服务配置中（替换卷名及服务名）：

```yaml
services:
  cupsd:
    volumes:
      - cups_runtime:/var/run/cups
  web:
    image: hanxi/cups-web:latest-web
    ports:
      - "1180:8080"
    environment:
      CUPS_SOCKET_PATH: /var/run/cups/cups.sock
    volumes:
      - cups_runtime:/var/run/cups:ro
      - ./.data:/data
      - ./.uploads:/uploads
    restart: unless-stopped
volumes:
  cups_runtime:
```

挂载包含 socket 的**整个目录**，避免 cupsd 重建 socket 后单文件挂载失效。Web 进程仍需具备该目录的遍历权限和 socket 的连接权限；只读挂载不替代 Unix 文件权限检查。

`CUPS_SOCKET_PATH` 必须是进程可见的绝对文件路径，设置后优先于 `CUPS_HOST`。Socket 不存在、权限不足或连接失败时，请求报错且服务端日志记录原因，不会回退到 TCP。接口中的打印机 URI 保持 HTTP/IPP 格式，Socket 模式下使用 `localhost:631` 作为占位主机，实际连接只走配置的 socket。历史打印记录和定时任务中的队列地址也通过同一 socket 连接；切换 CUPS 服务时需确保原队列名仍存在。

Socket 模式的目标由部署配置固定，`PRINTER_HOST_ALLOWLIST` / `PRINTER_BLOCK_PRIVATE` 仅作用于 TCP 模式。外部 CUPS 的访问策略继续生效；驱动、队列配置和 AirPrint 广播仍由外部 CUPS 管理。

也可以本地构建（尚未发布新标签时可使用此方式）：

```bash
make docker-build-web
# 将 docker-compose.web.yml 的 image 改为 cups-web:web，再执行上面的启动命令。
```

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
| `CUPS_SOCKET_PATH` | CUPS Unix Socket 的绝对路径；非空时优先于 `CUPS_HOST` | 空（使用 TCP） |
| `CUPSADMIN` | CUPS 管理员用户名 | `print` |
| `CUPSPASSWORD` | CUPS 管理员密码 | `print` |
| `TZ` | 时区 | `Asia/Shanghai` |
| `COOKIE_SECURE` | 会话 / CSRF cookie 的 `Secure` 属性：`auto`（缺省，按每个请求的实际协议判定）/ `true` 恒开 / `false` 恒关 | `auto` |
| `TRUSTED_ORIGINS` | 额外放行的来源，逗号分隔（形如 `https://print.example.com`）。仅在跨源防护误拦时需要 | 空 |
| `PRINTER_HOST_ALLOWLIST` | 打印机 URI 的主机白名单，逗号分隔。收紧 SSRF 面 | 空（不限制） |
| `PRINTER_BLOCK_PRIVATE` | `true` 时拒绝指向私网地址的打印机 URI | `false` |
| `OFD_CONVERTER_JAR` | OFD → PDF 转换器 jar 路径 | `/ofd-converter.jar` |
| `SCAN_DIR` | 扫描件输出目录，Docker 内挂到 `./.scans` | `scans` |

> 💡 `.env.example` 列了 AIO 部署常用的 `CUPSADMIN` / `CUPSPASSWORD` / `TZ`，以及仅 Web 镜像使用的 `CUPS_HOST` / `CUPS_SOCKET_PATH` 注释示例。AIO 的前三项都有内置默认值（`print` / `print` / `Asia/Shanghai`），不写 `.env` 也能启动；仅 Web 部署需配置外部 CUPS 的 TCP 地址或 socket。
>
> 💡 `DB_PATH` / `UPLOAD_DIR` 的默认值是**相对路径**，镜像内工作目录是 `/`，所以容器里实际落在 `/data/cups-web.db` 与 `/uploads/`，正好对应下面的卷映射，通常不需要显式设置。
>
> 💡 AIO 镜像的 `CUPS_HOST` 默认就是 `localhost`（cupsd 与 Web 同容器），一般无需设置；仅 Web 镜像在 TCP 模式下需要设置外部 CUPS 地址，可写 `host` 或 `host:port`（省略端口时自动补 `631`）。Unix Socket 模式见上面的部署示例。
>
> 💡 `COOKIE_SECURE` 缺省的 `auto` 会在直连 HTTPS（`r.TLS`）或反代上报 `X-Forwarded-Proto: https` 时自动给 cookie 加 `Secure`，HTTP 内网部署行为不变。若代理错误地上报了 `https` 而浏览器实际走 HTTP，会出现「登录后立刻掉线」，此时显式设 `COOKIE_SECURE=false`。
>
> 💡 `TRUSTED_ORIGINS` 通常**不需要**设置。反向代理下的跨源误拦已由 `X-Forwarded-Host` 自动处理，详见 [docs/reverse-proxy.md](docs/reverse-proxy.md)。

### 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-addr` | 监听地址，优先级高于 `LISTEN_ADDR` |

### 默认端口

- CUPS：`631`（管理界面 + IPP 协议）。Docker 部署使用 host 网络模式（[Issue #107](https://github.com/hanxi/cups-web/issues/107)），cupsd 直接占用宿主机 `631`
- Web：二进制部署默认监听 `8080`；Docker 部署由 `LISTEN_ADDR` 环境变量决定，`docker-compose.yml` 默认 `:1180`（与旧版桥接时代的端口映射保持一致）

### 数据持久化目录

Docker 默认卷映射：

| 宿主机路径 | 容器路径 | 说明 |
| --- | --- | --- |
| `./.etc` | `/etc/cups` | CUPS 配置（打印机、PPD 等） |
| `./.data` | `/data` | cups-web 数据库 |
| `./.uploads` | `/uploads` | 上传的原始文件与转换后 PDF |
| `./.drivers` | `/opt/cups-drivers/data` | 手动安装的打印机驱动快照（⚠️ 删除即丢失全部手动装的驱动） |
| `./.scans` | `/scans` | 扫描件存储目录，路径由 `SCAN_DIR` 决定；元数据落 `.data/cups-web.db` 的 `scan_records` 表 |

此外还有两个**非数据类**的挂载，用于 USB 打印机识别与热插拔：

| 宿主机路径 | 容器路径 | 说明 |
| --- | --- | --- |
| `/dev/bus/usb` | `/dev/bus/usb` | 以**目录**方式挂载（而不是 `devices:`），这样打印机后开机时新建的设备节点能实时传播进容器 |
| `/run/udev` | `/run/udev`（只读） | 让 libusb 读到设备属性，改善识别；宿主机没有该目录时可以删掉这一行 |

> ⚠️ **从旧版升级注意**：旧版 compose 还挂载了 `/run/dbus/system_bus_socket`（[Issue #94](https://github.com/hanxi/cups-web/issues/94)，借用宿主机 avahi 广播 AirPrint），新版已**移除**（[Issue #107](https://github.com/hanxi/cups-web/issues/107)）：宿主机没装 avahi-daemon 时，该挂载会让容器内自己的 dbus/avahi 起不来，网络打印机发现与 AirPrint 广播全部失效。host 网络模式下容器内自启的 avahi 已能直接在局域网发现和广播，无需依赖宿主机任何服务。请同步删除你本地 compose 里的这一行。

### 其他 compose 选项说明

| 选项 | 为什么需要 |
| --- | --- |
| `network_mode: host` | mDNS/DNS-SD 依赖局域网组播（5353/udp），桥接模式下组播出不去也进不来：容器发现不了局域网网络打印机，手机也搜不到 AirPrint（[Issue #107](https://github.com/hanxi/cups-web/issues/107)）。代价是无法自定义端口映射，CUPS 固定占用宿主 `631`、Web 由 `LISTEN_ADDR` 决定；宿主若自装 avahi-daemon 会与容器内 avahi 抢 5353 端口，且宿主 avahi 无法替容器广播（两条独立 D-Bus 总线），需停掉宿主侧 |
| `hostname: CUPS` | avahi 以 `CUPS.local` 在局域网广播 AirPrint / IPP Everywhere 服务 |
| `LISTEN_ADDR=:1180` | host 网络下没有端口映射，让 Web 直接监听宿主 `1180`，与旧桥接时代一致，书签 / 反代配置无需改动 |
| `user: root` | 容器内要运行 cupsd、`lpadmin`、`dpkg`（驱动安装），还要往 `/usr/lib/cups`、`/usr/share/ppd` 等系统路径写驱动文件 |
| `security_opt: [apparmor:unconfined]` | 解除 AppArmor 限制（[Issue #91](https://github.com/hanxi/cups-web/issues/91)）。PVE (Proxmox VE) LXC 等环境下会出现 `apparmor="DENIED"` 导致打印失败；单容器化后它同时也保护 LibreOffice / OFD 转换子进程不被拦截 |
| `device_cgroup_rules: ['c 189:* rmw']` | 放开 USB 字符设备（major 189）的 cgroup 权限（[Issue #81](https://github.com/hanxi/cups-web/issues/81)） |
| `privileged: true` | 特权模式。ARM / 嵌入式平台（树莓派、Amlogic、各类 TV Box 跑 Armbian）上 libusb 需要完整 sysfs 才能枚举 USB 打印机（[Issue #110](https://github.com/hanxi/cups-web/issues/110)）。x86 平台若介意安全隔离可注释掉，仅靠上方 `device_cgroup_rules` 通常已足够 |

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

### 反向代理与 HTTPS

通过反向代理（例如 Nginx）对外提供服务：

```nginx
server {
    listen 443 ssl;
    server_name example.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # 打印文件可能很大，不放开会 413 Request Entity Too Large
    client_max_body_size 200m;

    location / {
        proxy_pass http://localhost:1180;

        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;

        # 文档转换与驱动安装耗时较长，默认 60s 会中途断开
        proxy_read_timeout    300s;
        proxy_send_timeout    300s;
        proxy_request_buffering off;
    }
}
```

> ⚠️ **上面五个 `proxy_set_header` 都不能省。** 漏掉 `Host` / `X-Forwarded-Host` 会让跨源防护把登录请求当成跨站请求拒掉，页面上表现为**登录提示密码错误**（issue #99）；漏掉 `X-Forwarded-Proto` 会让 HTTPS 部署下的 cookie 拿不到 `Secure` 标记。
>
> 📖 反代下的登录失败排查、Caddy / Traefik 配置、子路径挂载的限制，见 [docs/reverse-proxy.md](docs/reverse-proxy.md)。

### 修改端口

新版默认 host 网络模式（[Issue #107](https://github.com/hanxi/cups-web/issues/107)），**没有端口映射可改**：

- **Web 端口**：改 compose 里的 `LISTEN_ADDR`（默认 `:1180`），或在 `.env` 中设置：
  ```bash
  LISTEN_ADDR=:你的Web端口
  ```
- **CUPS 端口**：cupsd 固定监听 `631`。确需修改时编辑 `./.etc/cupsd.conf` 中的 `Listen 631` 后重启容器（不推荐，IPP/AirPrint 生态默认按 631 工作）
- 若坚持桥接模式（放弃网络打印机发现与 AirPrint 广播），可改回 `ports: ["你的CUPS端口:631", "你的Web端口:8080"]` 并删除 `network_mode: host` 与 `LISTEN_ADDR`

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

使用最新的 `docker-compose.yml` 即可支持热插拔（volume 目录挂载 `/dev/bus/usb` + `device_cgroup_rules` + `privileged: true`）。如果在 x86 平台介意安全隔离，可注释掉 `privileged: true`，仅靠 `device_cgroup_rules` 通常已足够。

### AirPrint 搜不到打印机 / 发现不了局域网网络打印机？

手机 / iPad 的 AirPrint 广播、以及 CUPS 发现局域网网络打印机（`dnssd://xxx._ipp._tcp.local`），都依赖 mDNS 组播（5353/udp）。Docker 默认的 bridge 网络下组播出不去也进不来，因此新版 `docker-compose.yml` 默认使用 `network_mode: host`，由容器内自启的 avahi-daemon 直接在局域网广播和发现（[Issue #107](https://github.com/hanxi/cups-web/issues/107)）。

排查步骤：

1. **确认使用新版 compose**：必须有 `network_mode: host`、且**没有** `ports:` 和 `/run/dbus/system_bus_socket` 挂载。
   > ⚠️ 旧版为借用宿主机 avahi 广播而挂载了宿主机 D-Bus socket（[Issue #94](https://github.com/hanxi/cups-web/issues/94)）。宿主机没装 avahi-daemon 时，该挂载反而让容器内自己的 dbus/avahi 起不来，发现与广播全部失效——升级时务必删掉这一行。
2. **看容器日志**：`docker logs cups | grep -i 'dbus\|avahi'`。新版 entrypoint 在 dbus/avahi 启动失败时会打 `WARN`，按提示排查。
3. **宿主机自装了 avahi-daemon？** 容器内 avahi 会与它抢 5353 端口而起不来。注意宿主机 avahi **无法**替容器内的 CUPS 广播或发现（容器与宿主机是两条独立的 D-Bus 总线），必须停掉宿主机侧：`sudo systemctl disable --now avahi-daemon`，然后重启容器。
4. **路由器/交换机屏蔽组播**：部分 AP 开了「组播增强/IGMP 隔离」会丢 mDNS 包，尝试关闭或在同一 AP 下复测。

验证容器内 avahi 是否正常工作：

```bash
# 应能列出局域网打印机的 _ipp._tcp / _pdl-datastream._tcp 等服务
docker exec cups avahi-browse -art
# 应能解析出 .local 域名（依赖镜像内的 libnss-mdns）
docker exec cups getent hosts CUPS.local
```

### 安装的驱动丢失了？

确认 `docker-compose.yml` 中已配置驱动持久化卷：
```yaml
volumes:
  - ./.drivers:/opt/cups-drivers/data
```

驱动数据保存在 `.drivers` 目录中，容器重建后自动恢复。**删掉该目录就等于丢失全部手动安装的驱动**，需要在「驱动」页面重新安装。

通过「上传自定义驱动」装的 `.ppd` 与 `.deb` 都会自动恢复：`.ppd` 按文件还原，`.deb` 则归档整个包、启动时用 `dpkg -i` 重新安装一遍（已装且版本不更旧的会跳过）。界面上会列出装过哪些包。

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
