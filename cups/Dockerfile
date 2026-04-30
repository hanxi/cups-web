# 固定到 debian:bookworm-slim（Debian 12）而不是 debian:stable-slim：
# Debian 13 (trixie) 对 32-bit 架构（armhf / i386）做了 64-bit time_t ABI 迁移，
# 把 libcups2 / libcupsimage2 / libpng16-16 / printer-driver-gutenprint 等包
# 在 armhf 上替换成了 t64 后缀版本（`libcups2t64` / `libpng16-16t64` 等），
# 旧包名已经没有 installation candidate —— 这会让 linux/arm/v7 构建直接炸在
# apt-get install 这一步。amd64/arm64 不受影响是因为 64-bit 架构 time_t 本来
# 就是 64 位，没有 ABI bump，旧名仍保留为过渡包。
# bookworm 的 armhf 仓库上述包旧名都在，且 bookworm 也是主 Dockerfile runtime
# 阶段的 base，两个镜像 base 对齐便于运维。
# 跟随上游升级到 trixie 时，需要整体把受影响的包名改成 t64 版本并重测 armhf。
FROM debian:bookworm-slim

ARG TARGETARCH
ARG TARGETVARIANT

LABEL maintainer="涵曦 <im.hanxi@gmail.com>"
LABEL description="Debian-based CUPS server with Epson L380 official driver preinstalled"

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai
ENV CUPSADMIN=print
ENV CUPSPASSWORD=print

# Install packages directly for the current architecture (do not pre-check availability)
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      wget \
      curl \
      gnupg2 \
      ca-certificates \
      libcups2 \
      libcupsimage2 \
      cups \
      cups-client \
      cups-filters \
      libjpeg62-turbo \
      libpng16-16 \
      libtiff6 \
      libusb-1.0-0 \
      lsb-release \
      apt-utils \
      usbutils \
      printer-driver-gutenprint \
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

# Only install Epson proprietary driver on amd64; other architectures will use the Debian drivers
RUN if [ "${TARGETARCH}" = "amd64" ]; then \
    mkdir -p /tmp/epson_driver && \
    wget -q -O /tmp/epson_driver/epson-inkjet-printer-201601w_1.0.1-1_amd64.deb \
    https://download-center.epson.com.cn/f/module/81976474-cb4d-4b75-a0d8-ebc9ebde70d0/epson-inkjet-printer-201601w_1.0.1-1_amd64.deb && \
    wget -q -O /tmp/epson_driver/epson-printer-utility_1.2.2-1_amd64.deb \
    https://download-center.epson.com.cn/f/module/b04dc5b0-cb78-4ce7-98c8-06731fb74062/epson-printer-utility_1.2.2-1_amd64.deb && \
    dpkg -i /tmp/epson_driver/*.deb || apt-get install -y -f --no-install-recommends; \
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
