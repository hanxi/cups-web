#!/usr/bin/env bash
# HP LaserJet 1020 / 1020 Plus 固件安装脚本。
#
# 背景（issue #40）：
# HP LaserJet 1020 系列属于"host-based"打印机（也称 GDI 打印机），打印机内部
# 没有 ROM 存储固件，每次上电后必须由主机上传固件（sihp1020.dl）才能工作。
# Debian 的 printer-driver-foo2zjs 包已提供了驱动过滤器（foo2zjs / foo2zjs-wrapper），
# 但固件文件本身因版权限制不包含在 Debian 包内，需要额外下载。
#
# 本脚本负责：
#   ① 从本仓库 GitHub Releases 镜像下载 sihp1020.dl 固件文件；
#   ② 放置到 /usr/share/foo2zjs/firmware/ 目录（foo2zjs 标准固件路径）。
#
# 固件上传到 USB 设备的动作由 entrypoint.sh 在容器启动时完成——
# 检测到 HP 1020 USB 设备（VID:PID = 03f0:2b17）后，将固件写入对应的
# USB 设备节点。
#
# ────────────────────────────────────────────────────────────────────
# 下载策略
# ────────────────────────────────────────────────────────────────────
# 与 install-escpr2.sh / install-konica-bizhub.sh 同模式：只从本仓库自维护的
# GitHub Releases 镜像（tag = cups-driver）下载，避免 HP 官方下载链路在 CI 里
# 的不稳定性。fail-fast：下载失败立即非零退出。
# 升级/替换固件：在本仓库 cups-driver release 上传新版文件，修改下方 URL。

set -eo pipefail

# ────────────────────────────────────────────────────────────────────
# 配置
# ────────────────────────────────────────────────────────────────────
FW_FILENAME="sihp1020.dl"
FW_MIRROR_URL="https://github.com/hanxi/cups-web/releases/download/cups-driver/${FW_FILENAME}"
FW_INSTALL_DIR="/usr/share/foo2zjs/firmware"

# ────────────────────────────────────────────────────────────────────
# 下载 & 安装
# ────────────────────────────────────────────────────────────────────
mkdir -p "${FW_INSTALL_DIR}"

echo "[hp-laserjet1020] downloading firmware from ${FW_MIRROR_URL}"
curl -fL --retry 3 --retry-delay 3 -o "${FW_INSTALL_DIR}/${FW_FILENAME}" "${FW_MIRROR_URL}"

# 校验文件非空
if [ ! -s "${FW_INSTALL_DIR}/${FW_FILENAME}" ]; then
    echo "[hp-laserjet1020] FATAL: downloaded firmware file is empty"
    exit 1
fi

echo "[hp-laserjet1020] installed firmware: ${FW_INSTALL_DIR}/${FW_FILENAME} ($(wc -c < "${FW_INSTALL_DIR}/${FW_FILENAME}") bytes)"
