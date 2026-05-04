# 使用 debian:trixie-slim（Debian 13），与主 Dockerfile runtime 阶段的 base 镜像对齐。
#
# 历史说明：之前固定 bookworm 是因为 trixie 对 armhf 做了 64-bit time_t ABI 迁移，
# 把 libcups2/libcupsimage2/libpng16-16 等包重命名为 t64 后缀版本。现在的做法是：
# ① 不再显式安装这些低级库包——它们都是 cups/cups-filters 的依赖，apt 会自动
#    根据架构拉取正确的包名（如 libcups2t64、libpng16-16t64 等）；
# ② printer-driver-gutenprint 在 trixie armhf 上已无 binary（gutenprint 源码包
#    未对 armhf 构建），因此改为条件安装——仅在 amd64/arm64 上装入。
FROM debian:trixie-slim

ARG TARGETARCH
ARG TARGETVARIANT

LABEL maintainer="涵曦 <im.hanxi@gmail.com>"
LABEL description="Debian-based CUPS server (compiled from OpenPrinting/cups source) with Epson L380 official driver preinstalled"

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai
ENV CUPSADMIN=print
ENV CUPSPASSWORD=print

# CUPS 版本：从 OpenPrinting/cups 的 GitHub Releases 拉取源码编译。
# 默认锁定到官方 2.4.x 稳定分支最新版——2.4.x 维持 libcups2 ABI 稳定，
# 与 Debian trixie 的 cups-filters 可直接链接复用；
# 3.x 系列已切换到 CMake 构建并移除了经典驱动模型（printer-driver-* 大量失效），
# 如需切到 3.x 必须同步改写下方的 configure/make 构建步骤。
# 版本来源：https://github.com/OpenPrinting/cups/releases
ARG CUPS_VERSION=2.4.19

# ────────────────────────────────────────────────────────────────────────────
# 第一阶段：安装编译依赖 + 运行时依赖 + cups-filters。
# ────────────────────────────────────────────────────────────────────────────
# 这里故意不单独 `apt install cups`——cups-filters 会把 apt 版 cups 作为依赖拉进来，
# 由它负责创建 lp/lpadmin 用户组、/etc/cups 目录骨架和 systemd unit 文件等；
# 随后第二阶段用源码编译出的二进制（同样 --prefix=/usr）覆盖掉 apt 版的
# libcups.so.2 / cupsd / cups-client 等文件，既保留 Debian 侧的集成脚手架，
# 又替换成 OpenPrinting 上游的最新版本，且 libcups2 ABI 兼容让 cups-filters
# 和所有 printer-driver-* 可以继续工作。
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      # ── 源码编译工具链（第二阶段使用，构建完成后不清理，
      #    因为后续 ESC/P-R 2 驱动也需要） ──
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
      printer-driver-all \
      printer-driver-cups-pdf \
      printer-driver-escpr \
      printer-driver-foo2zjs \
      foomatic-db-compressed-ppds \
      openprinting-ppds \
      hpijs-ppds \
      hp-ppd \
      hplip \
      avahi-daemon && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# ────────────────────────────────────────────────────────────────────────────
# 第二阶段：从 OpenPrinting/cups 源码编译并覆盖安装到 /usr。
# ────────────────────────────────────────────────────────────────────────────
# configure 选项尽量贴齐 Debian 打包约定：
#   --prefix=/usr            覆盖 apt 版 /usr/bin/cupsd、/usr/sbin/cupsd 等；
#   --libdir=.../multiarch   Debian 多架构系统把 apt 版 libcups.so.2 装到
#                            /usr/lib/<triplet>/（ldconfig 里优先级高于 /usr/lib），
#                            如果我们用默认 libdir=/usr/lib 会被 apt 版遮蔽，
#                            所以显式用 dpkg-architecture 取 triplet 路径，
#                            让源码版 libcups/libcupsimage 彻底替换 apt 版；
#   --sysconfdir=/etc        沿用 /etc/cups 配置目录（cupsd.conf、ppd/、ssl/ 等）；
#   --localstatedir=/var     沿用 /var/{cache,log,spool}/cups 运行时目录；
#   --with-cups-user=lp      与 Debian cups-daemon 预置的系统用户一致；
#   --with-cups-group=lp     同上，避免 cupsd 启动时权限错乱；
#   --with-system-groups=lpadmin  管理员组，与 cups-daemon 预置保持一致；
#   --enable-libusb/--enable-avahi/--enable-dbus/--enable-gnutls
#                            开启 USB 直连、mDNS 发现、DBus 通知、TLS，
#                            对应容器内网络打印 + Airprint/Mopria 场景。
#
# 注：GitHub releases 的 tarball 已带 configure 脚本（发布前 bootstrap 过），
# 不需要再跑 autoreconf；若切到 master 分支 HEAD 源码需先 `./autogen.sh`。
RUN mkdir -p /tmp/cups-build && cd /tmp/cups-build && \
    wget -q -O cups.tar.gz \
      "https://github.com/OpenPrinting/cups/releases/download/v${CUPS_VERSION}/cups-${CUPS_VERSION}-source.tar.gz" && \
    tar xzf cups.tar.gz --strip-components=1 && \
    MULTIARCH="$(dpkg-architecture -qDEB_HOST_MULTIARCH)" && \
    ./configure \
      --prefix=/usr \
      --libdir=/usr/lib/${MULTIARCH} \
      --sysconfdir=/etc \
      --localstatedir=/var \
      --with-cups-user=lp \
      --with-cups-group=lp \
      --with-system-groups=lpadmin \
      --enable-libusb \
      --enable-avahi \
      --enable-dbus \
      --enable-gnutls && \
    make -j"$(nproc)" && \
    make install && \
    # 刷新动态链接器缓存，让后续步骤链接到源码编译出的 libcups.so.2
    ldconfig && \
    cd / && rm -rf /tmp/cups-build && \
    # 打印源码构建出的 CUPS 版本号，便于 CI 日志核对
    /usr/sbin/cupsd -V || true

# ────────────────────────────────────────────────────────────────────────────
# Epson ESC/P-R 2 驱动：Debian 官方仓库不提供此包，从 Epson 官方源码编译安装。
# ────────────────────────────────────────────────────────────────────────────
# 支持 ET-18100, L8050, L8160, WF-7840 等新款 Epson 喷墨打印机的完整功能（含无边距打印）。
# 版本更新时需同时修改 ESCPR2_VERSION 和 ESCPR2_UUID（从 Epson 下载中心获取新版本的 URL）。
#
# 注：这里不再单独 `apt install libcups2-dev libcupsimage2-dev`——
# 上一阶段 `make install` 已把 cups/cupsimage 的头文件与 .so 放入 /usr/include 与 /usr/lib，
# ESC/P-R 2 的 configure 会直接通过 cups-config 找到新编译出的 libcups，
# 避免 apt 版 -dev 包回踩覆盖源码装好的 /usr/include/cups/*.h。
#
# ⚠️ Best-effort 策略：
# Epson 下载中心（download-center.epson.com）的 UUID 会定期轮换，对非浏览器 UA
# 也会做拦截，历史上出现过 wget 返回 HTTP 403/404（exit code 8）的情况；
# 而 ESCPR2 仅对个别新款机型必需，老机型用 Debian 自带 printer-driver-escpr
# 就能覆盖。所以这里做成"失败不中断 build"——下载/编译任一步失败时仅打印
# WARNING，让整个镜像继续往下装其他驱动与配置，最终产物对非 ESCPR2 型号不受影响。
# 排查思路：若 CI 构建日志里看到 "escpr2 build skipped"，多半是 Epson 又换
# UUID 了，去 https://download-center.epson.com 下载页按 F12 看 "下载" 按钮
# 指向的新 UUID，更新 ESCPR2_UUID/ESCPR2_VERSION 重新构建即可。
ARG ESCPR2_VERSION=1.2.39
ARG ESCPR2_UUID=12bbe234-329d-4b3c-922c-e497f25f5e32
# 用完整的 Chrome UA 串，比单一 "Firefox" 更不容易被 Epson CDN 的 UA 过滤规则拦
ARG ESCPR2_UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
# 用 subshell 隔离 set -e 语境：子 shell 内任何一步失败就整个失败，
# 外层用 `|| echo WARNING` 捕获并降级为警告，不让 docker build 整体 abort。
RUN ( set -e; \
      mkdir -p /tmp/escpr2-build && cd /tmp/escpr2-build && \
      ESCPR2_URL="https://download-center.epson.com/f/module/${ESCPR2_UUID}/epson-inkjet-printer-escpr2-${ESCPR2_VERSION}-1.tar.gz" && \
      echo "[escpr2] downloading ${ESCPR2_URL}" && \
      wget --tries=3 --timeout=60 --retry-connrefused \
           --user-agent="${ESCPR2_UA}" \
           -O escpr2.tar.gz "${ESCPR2_URL}" && \
      mkdir src && cd src && \
      tar xzf ../escpr2.tar.gz --strip-components=1 && \
      ./configure --prefix=/usr --disable-static && \
      make -j"$(nproc)" && \
      make install && \
      echo "[escpr2] installed version ${ESCPR2_VERSION}" \
    ) || echo "[escpr2] WARNING: ESC/P-R 2 driver build skipped (download or compile failed). Non-ESCPR2 Epson printers are unaffected; Debian printer-driver-escpr still covers most older models. Update ESCPR2_UUID/ESCPR2_VERSION build args if newer models are required."; \
    rm -rf /tmp/escpr2-build

# ────────────────────────────────────────────────────────────────────────────
# 清理编译工具链：CUPS 与 ESC/P-R 2 都已编译安装完毕，构建依赖可以全部清理。
# ────────────────────────────────────────────────────────────────────────────
# 注意只清理 *-dev 头文件包和编译器，保留运行时动态库（如 libavahi-client3、
# libgnutls30、libssl3、libusb-1.0-0 等），它们是 cupsd 运行期必需的。
#
# ⚠️ 陷阱：源码编译出来的 cupsd 不在 apt 依赖图里，`apt-get autoremove` 不知道
# 它依赖这些运行时库，purge *-dev 后有概率把 libgnutls30 / libldap-* /
# libdbus-1-3 等作为"孤儿"一起删掉，导致 cupsd 启动时 dlopen 失败。
# 解决：先 `apt-mark manual` 把这些运行时库标记为用户显式安装，autoremove
# 就不会碰它们了。
#
# ⚠️ 包名跨版本差异：Debian trixie 升级了 OpenLDAP（libldap-2.5-0 → libldap-2.6-0）
# 并做了 t64 ABI 迁移，部分包名带 t64 后缀；不同架构（amd64/arm64/armhf）
# 实际装到的包名可能也不同。所以用 for 循环逐个 apt-mark，用
# `dpkg-query` 先检查"包已安装"再 mark，不存在的包静默跳过，保证
# 整条命令在任意 Debian 版本 / 架构下都不会因为找不到某个包而整体 exit 100。
RUN set -eux; \
    for pkg in \
      libavahi-client3 \
      libavahi-common3 \
      libdbus-1-3 \
      libgnutls30 \
      libkrb5-3 \
      libldap-2.5-0 libldap-2.6-0 libldap-2.6-0t64 \
      libpam0g libpam0t64 \
      libssl3 libssl3t64 \
      libsystemd0 \
      libusb-1.0-0 \
      zlib1g \
    ; do \
      if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "install ok installed"; then \
        apt-mark manual "$pkg"; \
      fi; \
    done; \
    # purge 同样用循环，避免某个 -dev 包在当前架构缺失时整批 abort
    for pkg in \
      build-essential \
      autoconf \
      automake \
      libtool \
      pkg-config \
      libavahi-client-dev \
      libavahi-common-dev \
      libdbus-1-dev \
      libgnutls28-dev \
      libkrb5-dev \
      libldap-dev libldap2-dev \
      libpam0g-dev \
      libssl-dev \
      libsystemd-dev \
      libusb-1.0-0-dev \
      zlib1g-dev \
    ; do \
      if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "install ok installed"; then \
        apt-get purge -y "$pkg"; \
      fi; \
    done; \
    apt-get autoremove -y; \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# printer-driver-gutenprint 在 trixie armhf 上没有 binary 包（gutenprint 的
# 链接依赖 libgutenprint9 未完成 t64 迁移），仅在 amd64/arm64 上安装。
# armhf 用户仍可通过 printer-driver-all 推荐的其他驱动覆盖大部分打印机。
RUN if [ "${TARGETARCH}" != "arm" ]; then \
      apt-get update && \
      apt-get install -y --no-install-recommends printer-driver-gutenprint && \
      apt-get clean && rm -rf /var/lib/apt/lists/*; \
    fi

# ────────────────────────────────────────────────────────────────────────────
# Epson 国行私有驱动（仅 amd64）：best-effort 安装。
# ────────────────────────────────────────────────────────────────────────────
# `epson-inkjet-printer-201601w` 与 `epson-printer-utility` 是 Epson 中国区
# 发布的**闭源专有** .deb 包（无源码，无 arm64/armhf 二进制），覆盖 L380/L455
# 等国行早期喷墨机型。对应功能大部分可以被 Debian 自带的 `printer-driver-escpr`
# 覆盖，但原厂 PPD 在墨水检测、尺寸预设等细节上更完整。
#
# ⚠️ 与 ESCPR2 同样的坑：download-center.epson.com.cn 的 UUID 会定期轮换，
# URL 失效时 wget 退出码 8（HTTP 4xx）会让整个 docker build abort。
# 这里包进 subshell + `|| echo WARNING` 做成 best-effort——下载 / dpkg 任一
# 步失败只打告警不中断镜像构建，arm64/armhf 本来就跳过，不受影响。
# 升级方法：去 https://download-center.epson.com.cn 找对应机型按 F12 抓新
# UUID，更新下面两个 ARG 即可。
ARG EPSON_PROP_DRIVER_UUID=81976474-cb4d-4b75-a0d8-ebc9ebde70d0
ARG EPSON_PROP_DRIVER_DEB=epson-inkjet-printer-201601w_1.0.1-1_amd64.deb
ARG EPSON_PROP_UTILITY_UUID=b04dc5b0-cb78-4ce7-98c8-06731fb74062
ARG EPSON_PROP_UTILITY_DEB=epson-printer-utility_1.2.2-1_amd64.deb
ARG EPSON_PROP_UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
RUN if [ "${TARGETARCH}" = "amd64" ]; then \
      ( set -e; \
        mkdir -p /tmp/epson_driver && cd /tmp/epson_driver && \
        EPSON_DRV_URL="https://download-center.epson.com.cn/f/module/${EPSON_PROP_DRIVER_UUID}/${EPSON_PROP_DRIVER_DEB}" && \
        EPSON_UTIL_URL="https://download-center.epson.com.cn/f/module/${EPSON_PROP_UTILITY_UUID}/${EPSON_PROP_UTILITY_DEB}" && \
        echo "[epson-prop] downloading ${EPSON_DRV_URL}" && \
        wget --tries=3 --timeout=60 --retry-connrefused \
             --user-agent="${EPSON_PROP_UA}" \
             -O "${EPSON_PROP_DRIVER_DEB}" "${EPSON_DRV_URL}" && \
        echo "[epson-prop] downloading ${EPSON_UTIL_URL}" && \
        wget --tries=3 --timeout=60 --retry-connrefused \
             --user-agent="${EPSON_PROP_UA}" \
             -O "${EPSON_PROP_UTILITY_DEB}" "${EPSON_UTIL_URL}" && \
        ( dpkg -i ./*.deb || apt-get install -y -f --no-install-recommends ) && \
        echo "[epson-prop] installed Epson CN proprietary driver + utility" \
      ) || echo "[epson-prop] WARNING: Epson CN proprietary driver skipped (download or dpkg failed). L380/L455 国行机型可回落到 Debian printer-driver-escpr；如需恢复此驱动请在 https://download-center.epson.com.cn 查新 UUID 并更新 EPSON_PROP_DRIVER_UUID/EPSON_PROP_UTILITY_UUID 构建参数。"; \
      rm -rf /tmp/epson_driver; \
    fi && rm -rf /var/lib/apt/lists/*

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
