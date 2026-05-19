#!/bin/bash
# cups-web 容器入口脚本
#
# 职责：在 Go 二进制 /cups-web 启动前执行一次性初始化逻辑。
# 当前包含：
#   1. 扫描仪网络配置——读取 /scan/config.json，向 SANE 后端配置文件写入 net <ip>
#
# 设计说明：
#   - 本脚本由 Dockerfile ENTRYPOINT 指定，PID 1 由 exec 接管给 /cups-web，
#     确保 Go 进程正确接收 SIGTERM 等信号（Docker stop 优雅退出）。
#   - /scan/config.json 通过 docker-compose.yml 的 volume 挂载提供（运行时可改），
#     也可在构建时 COPY 进镜像作为默认值。
#   - 扫描仪配置写入 /etc/sane.d/*.conf 需要 root 权限；docker-compose.yml
#     已设置 user: root，直接 docker run 时需传 --user root。
#   - 配置写入是原子性的：用标记注释（SCAN_MARKER）标识托管行，每次启动时
#     先清除所有旧的托管行，再写入当前 config.json 的内容。修改配置后重启
#     容器即可生效，旧配置不会残留。
#   - 后续如有其他初始化逻辑（环境变量处理、目录准备等），追加到 exec 之前即可。

set -e

# ── 扫描仪网络配置 ──────────────────────────────────────────────────
# SANE 的网络扫描仪需要在对应后端配置文件（如 /etc/sane.d/epsonds.conf）中
# 添加 `net <ip>` 行，scanimage -L 才能发现设备。
# 不同后端配置文件名不同（epsonds.conf、epson2.conf、fujitsu.conf 等），
# 由 config.json 的 backend 字段指定，脚本自动映射到 /etc/sane.d/<backend>.conf。
# 同时向 /etc/sane.d/net.conf（通用网络后端）追加相同条目，确保兼容性。

CONFIG="/scan/config.json"
# 标记注释：所有由本脚本写入的行都带此前缀，用于下次启动时精准清除旧配置
SCAN_MARKER="# managed by cups-web entrypoint"

# cleanManagedLines <file>
# 移除指定文件中所有由本脚本托管的行（带 SCAN_MARKER 前缀）。
# 使用临时文件原子替换，避免 sed -i 在某些文件系统上的问题。
cleanManagedLines() {
  local file="$1"
  if [ -f "$file" ]; then
    local tmp="${file}.tmp"
    grep -vF "$SCAN_MARKER" "$file" > "$tmp" 2>/dev/null || true
    mv "$tmp" "$file"
    echo "[scan] cleaned managed lines from $file"
  fi
}

if [ -f "$CONFIG" ] && command -v jq >/dev/null 2>&1; then
  count=$(jq '.scanners | length' "$CONFIG" 2>/dev/null || echo 0)
  if [ "$count" -gt 0 ] 2>/dev/null; then
    echo "[scan] configuring $count scanner(s) from $CONFIG"

    # 收集本次配置涉及的所有后端，用于清理对应的 conf 文件
    backends=()
    for i in $(seq 0 $((count - 1))); do
      backend=$(jq -r ".scanners[$i].backend // empty" "$CONFIG")
      if [ -n "$backend" ]; then
        backends+=("$backend")
      fi
    done

    # 去重后端列表
    unique_backends=($(printf '%s\n' "${backends[@]}" | sort -u))

    # 第一步：清除所有相关 conf 文件中的旧托管行
    for backend in "${unique_backends[@]}"; do
      cleanManagedLines "/etc/sane.d/${backend}.conf"
    done
    cleanManagedLines "/etc/sane.d/net.conf"

    # 第二步：写入当前配置
    for i in $(seq 0 $((count - 1))); do
      backend=$(jq -r ".scanners[$i].backend // empty" "$CONFIG")
      ip=$(jq -r ".scanners[$i].ip // empty" "$CONFIG")

      # 跳过字段不完整的条目
      if [ -z "$backend" ] || [ -z "$ip" ]; then
        echo "[scan] skipping entry $i: missing backend or ip"
        continue
      fi

      conf="/etc/sane.d/${backend}.conf"
      net_line="net ${ip}"
      managed_line="${net_line}  ${SCAN_MARKER}"

      # 写入后端配置文件（如 /etc/sane.d/epsonds.conf）
      if [ -f "$conf" ]; then
        echo "$managed_line" >> "$conf"
        echo "[scan] appended '$net_line' to $conf"
      else
        # 后端配置文件不存在时主动创建，SANE 仍能识别
        echo "[scan] warning: $conf not found, creating it"
        echo "$managed_line" > "$conf"
        echo "[scan] created $conf with '$net_line'"
      fi

      # 同步写入 net.conf（通用网络后端），确保 scanimage -L 能通过 net 后端发现
      net_conf="/etc/sane.d/net.conf"
      if [ -f "$net_conf" ]; then
        echo "$managed_line" >> "$net_conf"
        echo "[scan] appended '$net_line' to $net_conf"
      fi
    done
  else
    echo "[scan] no scanners configured in $CONFIG"
    # 配置为空时也要清理旧的托管行
    if [ -d /etc/sane.d ]; then
      for f in /etc/sane.d/*.conf; do
        cleanManagedLines "$f"
      done
    fi
  fi
else
  # config.json 缺失或 jq 未安装时静默跳过，不影响服务启动
  if [ ! -f "$CONFIG" ]; then
    echo "[scan] $CONFIG not found, skipping scanner configuration"
  else
    echo "[scan] jq not available, skipping scanner configuration"
  fi
fi

# ── 启动 Go 服务 ────────────────────────────────────────────────────
# exec 替换当前进程，使 /cups-web 成为 PID 1，正确接收容器停止信号。
exec /cups-web "$@"
