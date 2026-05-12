# 使用 debian:trixie-slim（Debian 13），与主 Dockerfile runtime 阶段的 base 镜像对齐。
#
# 历史说明：之前固定 bookworm 是因为 trixie 对 armhf 做了 64-bit time_t ABI 迁移，
# 把 libcups2/libcupsimage2/libpng16-16 等包重命名为 t64 后缀版本。现在的做法是：
# ① 不再显式安装这些低级库包——它们都是 cups/cups-filters 的依赖，apt 会自动
#    根据架构拉取正确的包名（如 libcups2t64、libpng16-16t64 等）；
# ② printer-driver-gutenprint 在 trixie armhf 上已无 binary（gutenprint 的
#    源码包未对 armhf 构建），因此改为条件安装——见 scripts/install-gutenprint.sh。
#
# 构建步骤拆分到 scripts/ 目录的独立 sh 脚本，版本号、URL 等配置硬编码在脚本里，
# 不通过 ARG 透传——升级版本时直接改对应脚本即可。
FROM debian:trixie-slim

ARG TARGETARCH
ARG TARGETVARIANT

LABEL maintainer="涵曦 <im.hanxi@gmail.com>"
LABEL description="Debian-based CUPS server (compiled from OpenPrinting/cups source) with Epson L380 official driver + Brother driverless/laser support preinstalled"

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai
ENV CUPSADMIN=print
ENV CUPSPASSWORD=print

# ────────────────────────────────────────────────────────────────────
# 第一阶段：apt 安装编译依赖 + 运行时依赖 + cups-filters。
# ────────────────────────────────────────────────────────────────────
# 这里故意不单独 `apt install cups`——cups-filters 会把 apt 版 cups 作为依赖拉进来，
# 由它负责创建 lp/lpadmin 用户组、/etc/cups 目录骨架和 systemd unit 文件等；
# 随后第二阶段用源码编译出的二进制（同样 --prefix=/usr）覆盖掉 apt 版的
# libcups.so.2 / cupsd / cups-client 等文件，既保留 Debian 侧的集成脚手架，
# 又替换成 OpenPrinting 上游的最新版本，且 libcups2 ABI 兼容让 cups-filters
# 和所有 printer-driver-* 可以继续工作。
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      # ── 源码编译工具链（CUPS / ESCPR2 都用到，构建完成后由 cleanup-build.sh 清理） ──
      build-essential \
      autoconf \
      automake \
      libtool \
      pkg-config \
      # ── CUPS 源码 configure 需要的 -dev 头文件 ──
      libavahi-client-dev \
      libavahi-common-dev \
      libdbus-1-dev \
      libgnutls28-dev \
      libkrb5-dev \
      libldap-dev \
      libpam0g-dev \
      libssl-dev \
      libsystemd-dev \
      libusb-1.0-0-dev \
      zlib1g-dev \
      # ── 通用工具 ──
      wget \
      curl \
      gnupg2 \
      ca-certificates \
      lsb-release \
      apt-utils \
      usbutils \
      # ── CUPS 打印生态（cups-filters 会拉入 apt 版 cups 作为依赖） ──
      cups-filters \
      cups-client \
      cups-daemon \
      cups-ipp-utils \
      printer-driver-all \
      printer-driver-cups-pdf \
      printer-driver-escpr \
      printer-driver-foo2zjs \
      # ── Brother 兼容驱动 ──
      # Brother 官方未把任何 Linux 驱动上传 Debian 主仓库，用户常见的 Brother 打印机
      # （含新款 DCP-T425W、HL-Lxxxx 等）在 CUPS 里出现 "IPP Everywhere (color)" 选项
      # 靠的是 **driverless** 路径（IPP Get-Printer-Attributes 自动生成 PPD），
      # 需要两个关键组件：
      #   ① ipp-usb —— 把 USB 连接的 IPP Everywhere 打印机暴露为本地 http://localhost
      #     的 IPP 端点，让 CUPS/avahi 像网络打印机一样发现它（USB 直连机型必须）；
      #   ② cups-filters 内置的 cups-browsed + CUPS 自带的 everywhere 模型（已在上面
      #     --enable-avahi --enable-dbus 编译进 cupsd）。
      # 同时补上社区维护的 printer-driver-brlaser，覆盖大量老 Brother 激光机（HL-L2300D
      # 等），对 DCP-T425W 这类新墨仓机型虽然用不上（它们走 driverless），但能同时
      # 改善其他 Brother 用户体验，包体很小，issue #32。
      ipp-usb \
      printer-driver-brlaser \
      foomatic-db-compressed-ppds \
      openprinting-ppds \
      hpijs-ppds \
      hp-ppd \
      hplip \
      avahi-daemon && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# ────────────────────────────────────────────────────────────────────
# 拷贝构建脚本，统一赋予可执行权限。
# ────────────────────────────────────────────────────────────────────
# 每个脚本独立一个 RUN，缓存粒度与原来一行 RUN 等价；脚本内容不变时层级缓存复用。
COPY scripts/ /tmp/build-scripts/
RUN chmod +x /tmp/build-scripts/*.sh

# ────────────────────────────────────────────────────────────────────
# 第二阶段：从 OpenPrinting/cups 源码编译并覆盖安装到 /usr。
# ────────────────────────────────────────────────────────────────────
# 详见 scripts/install-cups.sh
RUN /tmp/build-scripts/install-cups.sh

# ────────────────────────────────────────────────────────────────────
# Epson ESC/P-R 2 驱动：Debian 官方仓库不提供此包，从 Epson 官方源码编译安装。
# ────────────────────────────────────────────────────────────────────
# 详见 scripts/install-escpr2.sh（版本号、镜像 URL 都硬编码在脚本里）
RUN /tmp/build-scripts/install-escpr2.sh

# ────────────────────────────────────────────────────────────────────
# Canon CAPT (LBP2900/LBP2900B) 开源驱动：从源码编译安装。
# ────────────────────────────────────────────────────────────────────
# issue #43。基于逆向工程的开源 CAPT 协议实现（GPL-3.0），覆盖 Canon LBP2900/
# LBP2900B 等走 CAPT 协议的旧款 Canon 激光打印机。纯 C 源码，全架构编译。
# 详见 scripts/install-canon-capt.sh
RUN /tmp/build-scripts/install-canon-capt.sh

# ────────────────────────────────────────────────────────────────────
# 清理编译工具链。
# ────────────────────────────────────────────────────────────────────
# 详见 scripts/cleanup-build.sh
RUN /tmp/build-scripts/cleanup-build.sh

# ────────────────────────────────────────────────────────────────────
# printer-driver-gutenprint：仅 amd64/arm64 安装。
# ────────────────────────────────────────────────────────────────────
# 详见 scripts/install-gutenprint.sh
RUN /tmp/build-scripts/install-gutenprint.sh

# ────────────────────────────────────────────────────────────────────
# Epson 国行私有驱动（仅 amd64）：best-effort 安装。
# ────────────────────────────────────────────────────────────────────
# 详见 scripts/install-epson-cn.sh（UUID/.deb 文件名硬编码在脚本里）
RUN /tmp/build-scripts/install-epson-cn.sh

# ────────────────────────────────────────────────────────────────────
# Canon UFR II / UFRII LT 官方驱动（amd64 + arm64）：best-effort 安装。
# ────────────────────────────────────────────────────────────────────
# 覆盖 i-SENSYS LBP/MF、imageCLASS、imageRUNNER (iR) 等所有走 UFR II /
# UFRII LT 协议的 Canon 激光机（issue #34）。Canon 官方 tarball 从 v6.30
# 起已为 amd64/arm64 提供预编译 .deb；armhf/armel 没有 32-bit ARM 二进制，
# 由脚本入口判断架构后 skip。
# 详见 scripts/install-canon-ufr2.sh（tarball URL/.deb 文件名硬编码在脚本里）
RUN /tmp/build-scripts/install-canon-ufr2.sh

# ────────────────────────────────────────────────────────────────────
# 柯尼卡美能达 bizhub 3000MF 黑白激光打印机驱动（amd64 + arm64）：best-effort 安装。
# ────────────────────────────────────────────────────────────────────
# issue #35。原始驱动来自柯尼卡美能达国产化平台（银河麒麟/UOS）的 .7z 包，
# 已重新打成 tar.gz 上传到本仓库自维护的 GitHub Releases 镜像（与 escpr2
# 共用 cups-driver tag），脚本走 tar.gz 链路。loongarch64/mips64el 镜像
# 本身不发布；armhf/armel 厂商无包，由脚本入口 skip。
# 详见 scripts/install-konica-bizhub.sh（镜像 URL/.deb 文件名硬编码在脚本里）
RUN /tmp/build-scripts/install-konica-bizhub.sh

# ────────────────────────────────────────────────────────────────────
# HP LaserJet 1020 / 1020 Plus 固件：下载 sihp1020.dl 到 foo2zjs 标准路径。
# ────────────────────────────────────────────────────────────────────
# issue #40。HP 1020 系列是 host-based 打印机，每次上电需由主机上传固件。
# Debian 的 printer-driver-foo2zjs 已提供驱动过滤器，但固件文件因版权不含在
# Debian 包内。本脚本从本仓库 GitHub Releases 镜像下载固件并放到
# /usr/share/foo2zjs/firmware/，entrypoint.sh 在容器启动时负责检测 USB
# 设备并上传固件。全架构通用（固件是二进制 blob，与 CPU 架构无关）。
# 详见 scripts/install-hp-laserjet1020.sh
RUN /tmp/build-scripts/install-hp-laserjet1020.sh

# 构建脚本使命完成，清理临时目录
RUN rm -rf /tmp/build-scripts

# ────────────────────────────────────────────────────────────────────
# cupsd.conf：放开监听 + 浏览，并备份默认配置供 entrypoint 还原。
# ────────────────────────────────────────────────────────────────────
RUN sed -i 's/Listen localhost:631/Listen 0.0.0.0:631/' /etc/cups/cupsd.conf && \
    sed -i 's/Browsing Off/Browsing On/' /etc/cups/cupsd.conf && \
    sed -i 's/<Location \/>/<Location \/>\n  Allow All/' /etc/cups/cupsd.conf && \
    sed -i 's/<Location \/admin>/<Location \/admin>\n  Allow All\n  Require user @SYSTEM/' /etc/cups/cupsd.conf && \
    sed -i 's/<Location \/admin\/conf>/<Location \/admin\/conf>\n  Allow All/' /etc/cups/cupsd.conf && \
    echo "ServerAlias *" >> /etc/cups/cupsd.conf && \
    echo "DefaultEncryption Never" >> /etc/cups/cupsd.conf
RUN cp -rp /etc/cups /etc/cups-bak
VOLUME [ "/etc/cups" ]

EXPOSE 631

COPY entrypoint.sh /
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]
