#!/bin/bash
set -e

# ══════════════════════════════════════════════════════════════
# 1. Restore persisted drivers
# ══════════════════════════════════════════════════════════════
# 驱动恢复是"尽力而为"的操作：快照目录可能只读、可能是旧版本写坏的快照、
# 目标路径可能已被别的包占用成目录。这些都不该阻塞 cupsd / cups-web 启动
# （否则容器起不来，用户连 Web UI 都进不去，无法自救删掉坏驱动）。
# restore-drivers 自身已不使用 set -e 并会汇总失败数，这里再加一层 `|| true`
# 语义的兜底，防止它因意外信号/非零退出把 set -e 的 entrypoint 带崩。
/usr/local/bin/restore-drivers || echo "[entrypoint] WARN: restore-drivers 部分失败，继续启动"

# ══════════════════════════════════════════════════════════════
# 1b. Custom fonts initialization (外部挂载字体加载与缓存刷新)
# ══════════════════════════════════════════════════════════════
# 用户可通过挂载卷（如 ./.data/fonts 或单独挂载至 /data/fonts）传入自定义中文字体。
# 启动时检测并将字体软链接到系统字体路径，刷新 fontconfig，并动态修补 Ghostscript cidfmap。
# 容错：目录不存在或为空时跳过，失败不阻断启动。
FONT_SRC="/data/fonts"
FONT_TARGET="/usr/share/fonts/truetype/custom"
if [ -d "$FONT_SRC" ] && [ "$(find "$FONT_SRC" -maxdepth 1 -type f \( -iname '*.ttf' -o -iname '*.ttc' -o -iname '*.otf' \) 2>/dev/null | head -1)" ]; then
    echo "[entrypoint] 发现挂载字体，同步至系统字体目录..."
    mkdir -p "$FONT_TARGET"
    cp -snf "$FONT_SRC"/*.* "$FONT_TARGET/" 2>/dev/null || cp -f "$FONT_SRC"/*.* "$FONT_TARGET/" 2>/dev/null || true
    fc-cache -f "$FONT_TARGET" 2>/dev/null || true

    # 若挂载了真实 simsun/simhei 等字体，同步修补 Ghostscript cidfmap
    GS_CIDFMAP="/etc/ghostscript/cidfmap.local"
    if [ -f "$GS_CIDFMAP" ]; then
        if [ -f "$FONT_TARGET/simsun.ttf" ]; then
            sed -i 's|/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simsun.ttf) /SubfontID 0|g' "$GS_CIDFMAP" 2>/dev/null || true
        fi
        if [ -f "$FONT_TARGET/simhei.ttf" ]; then
            sed -i 's|/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simhei.ttf) /SubfontID 0|g' "$GS_CIDFMAP" 2>/dev/null || true
        fi
        if [ -f "$FONT_TARGET/simkai.ttf" ]; then
            sed -i 's|/usr/share/fonts/truetype/arphic/ukai.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simkai.ttf) /SubfontID 0|g' "$GS_CIDFMAP" 2>/dev/null || true
        fi
        if [ -f "$FONT_TARGET/simfang.ttf" ]; then
            sed -i '/^\/#b7#c2#cb#ce[, ]/s|/usr/share/fonts/truetype/custom/simsun.ttf) /SubfontID 0|/usr/share/fonts/truetype/custom/simfang.ttf) /SubfontID 0|' "$GS_CIDFMAP" 2>/dev/null || true
            sed -i '/^\/#b7#c2#cb#ce[, ]/s|/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simfang.ttf) /SubfontID 0|' "$GS_CIDFMAP" 2>/dev/null || true
        fi

        GS_INIT_DIR=$(find /usr/share/ghostscript -path "*/Resource/Init" -type d 2>/dev/null | head -1)
        if [ -n "$GS_INIT_DIR" ]; then
            cp -f "$GS_CIDFMAP" "$GS_INIT_DIR/cidfmap" 2>/dev/null || true
        fi
    fi
fi

# ══════════════════════════════════════════════════════════════
# 2. CUPS admin user setup (from cups/entrypoint.sh)
# ══════════════════════════════════════════════════════════════
if [ $(grep -ci $CUPSADMIN /etc/shadow) -eq 0 ]; then
    useradd -r -G lpadmin -M $CUPSADMIN

    # add password
    echo $CUPSADMIN:$CUPSPASSWORD | chpasswd

    # add tzdata
    ln -fs /usr/share/zoneinfo/$TZ /etc/localtime
    dpkg-reconfigure --frontend noninteractive tzdata
fi

# ══════════════════════════════════════════════════════════════
# 3. CUPS config restore (from cups/entrypoint.sh)
# ══════════════════════════════════════════════════════════════
# restore default cups config in case user does not have any
if [ ! -f /etc/cups/cupsd.conf ]; then
    cp -rpn /etc/cups-bak/* /etc/cups/
fi

# ── 下面两条对**存量卷**做幂等修补 ────────────────────────────────────────
# ⚠️ 上面那个 if 只看 cupsd.conf 一个文件。挂载的 ./.etc 卷里如果已经有一份（旧版本
# 留下的、或用户从别处拷来的）cupsd.conf，整块还原就被跳过 —— 于是新版镜像在基线里
# 加的东西，存量用户永远拿不到。所以需要在 if **之外**单独补。

# ① ssl 目录：cupsd 自己不会建（见 Dockerfile 里的说明），缺了它 ipps/AirPrint
#    握手会失败并在 error_log 刷 "Unable to create server credentials"。
#    无条件保证存在，代价只是一次 mkdir。
mkdir -p /etc/cups/ssl 2>/dev/null || true
chmod 700 /etc/cups/ssl 2>/dev/null || true

# ② ReadyPaperSizes：iPhone AirPrint 面板的纸张列表读 IPP media-ready，而 CUPS 的
#    media-ready 只由 ReadyPaperSizes ∩ PPD 尺寸决定（跟 media-default 无关，所以
#    `lpadmin -o media=` 修不了它）。不配时按 locale 兜底成 Letter,Legal,... → A4
#    永远不出现（issue #82）。
#    只在用户完全没写过这一项时追加，绝不覆盖用户的显式配置。
if [ -f /etc/cups/cupsd.conf ] && ! grep -qiE '^[[:space:]]*ReadyPaperSizes' /etc/cups/cupsd.conf; then
    echo "ReadyPaperSizes A4,A3,A5,A6,EnvDL" >> /etc/cups/cupsd.conf
    echo "[entrypoint] 已为存量配置追加 ReadyPaperSizes（A4 系列），让 AirPrint 面板能选到 A4（issue #82）"
fi

# ③ /etc/cups/ppd 权限对齐（issue #59）
# ── 复现确认（容器实测，非推断）────────────────────────────────────────
#   - 容器默认（非挂载卷）场景：cupsd（root）写出的 PPD 是 640 root:lp，
#     lp 组本可读，不会报错。
#   - 但 docker-compose 把宿主目录 bind-mount 到 /etc/cups 后，卷内 PPD 副本
#     的 owner/group 来自宿主（group 通常不是容器内的 lp / gid=7，或文件是
#     从别的环境迁移来的）。CUPS 处理任务时降权到 lp 去读 PPD，group 不匹配
#     → lp 读不到，打印报
#       "Failed to open PPD: /etc/cups/ppd/<printer>.ppd"。
#   - 修复：cupsd 启动前把 ppd 目录与文件对齐到 root:lp + 组可读。
#     chmod/chown 会写穿 bind-mount 改宿主 inode（已实测：宿主侧 owner 也
#     变成 root:lp，容器内 lp 读取由 Permission denied 转为成功）。
#   - 幂等：已是 root:lp 640 的文件不受影响；失败不阻塞启动。
if [ -d /etc/cups/ppd ]; then
    chown -R root:lp /etc/cups/ppd 2>/dev/null || true
    chmod 775 /etc/cups/ppd 2>/dev/null || true
    find /etc/cups/ppd -maxdepth 1 -name '*.ppd' -type f -exec chmod 640 {} + 2>/dev/null || true
    echo "[entrypoint] 已对齐 /etc/cups/ppd 权限为 root:lp（组可读），修复挂载卷下降权后读不到 PPD 的问题（issue #59）"
fi

# ══════════════════════════════════════════════════════════════
# 4. HP 1020 PPD Letter→A4 patch (from cups/entrypoint.sh)
# ══════════════════════════════════════════════════════════════
# ── 已添加 HP 1020 打印机的默认纸张 Letter → A4 一次性修补 ──
# issue #48：foo2zjs 上游 HP-LaserJet_1020 PPD 的 *DefaultPageSize 是 Letter。
# 苹果设备走 AirPrint（IPP）时按 media-default 渲染首屏纸张，Letter 默认会让
# 国内常用的 A4 在 iPhone 打印面板里被折叠/隐藏，用户反映"无 A4 选项"。
#
# 已通过 install-hp-laserjet1020.sh 在 /usr/share/cups/model/HP/ 安装了 A4-default
# 变体 PPD，新加的打印机可以直接选这版。但已经按"(recommended)"加好的存量打印机
# 不会自动迁移——它们的 PPD 副本在 /etc/cups/ppd/<printer>.ppd，仍是 Letter 默认。
#
# 这里在 cupsd 启动前对存量副本做一次性原地修补：
#   - 仅处理 foo2zjs HP 1020 PPD（用 *Product 和 *FoomaticIDs 双重指纹）
#   - 仅当当前 *DefaultPageSize 是 Letter（用户没在 CUPS UI 里改过默认纸张）
#   - 仅当 *PageSize 列表里确实声明了 A4
#   - 三个条件都满足才把四组 *Default*: Letter 同步改成 A4
# 任何一条不满足就跳过，不会覆盖用户的显式选择，也不会处理非 HP 1020 的 PPD。
# 修改前先备份成 .bak-cupsweb-issue48，方便用户回退。
if [ -d /etc/cups/ppd ]; then
    for ppd in /etc/cups/ppd/*.ppd; do
        [ -f "$ppd" ] || continue
        grep -q '^\*Product:[[:space:]]*"(HP LaserJet 1020)"' "$ppd" || continue
        grep -q '^\*FoomaticIDs:[[:space:]]\+HP-LaserJet_1020[[:space:]]\+foo2zjs-z1' "$ppd" || continue
        grep -q '^\*DefaultPageSize:[[:space:]]\+Letter[[:space:]]*$' "$ppd" || continue
        grep -q '^\*PageSize A4' "$ppd" || continue

        cp -p "$ppd" "${ppd}.bak-cupsweb-issue48"
        sed -i -E '
            s/^\*DefaultPageSize:[[:space:]]+Letter[[:space:]]*$/\*DefaultPageSize: A4/;
            s/^\*DefaultPageRegion:[[:space:]]+Letter[[:space:]]*$/\*DefaultPageRegion: A4/;
            s/^\*DefaultImageableArea:[[:space:]]+Letter[[:space:]]*$/\*DefaultImageableArea: A4/;
            s/^\*DefaultPaperDimension:[[:space:]]+Letter[[:space:]]*$/\*DefaultPaperDimension: A4/
        ' "$ppd"
        echo "[entrypoint] patched $ppd: HP 1020 default paper Letter → A4 (issue #48; backup at ${ppd}.bak-cupsweb-issue48)"
    done
fi

# ══════════════════════════════════════════════════════════════
# 5. HP host-based firmware monitor
# ══════════════════════════════════════════════════════════════
# 容器没有 udev daemon，启动时的一次性 hplj 调用覆盖不了后开机/热插拔，
# 也覆盖不了运行中刚安装固件的场景。参考 tzishue/cloud-printer 的修复脚本，
# 持续检查 IEEE-1284 固件状态，按完整 USB URI 向未加载固件的受支持 HP 型号
# 上传 /lib/firmware/hp/sihp*.dl；有 FWVER/FIRMWARE 或状态未知时跳过。
# 使用镜像已有的 CUPS USB backend，不依赖 /dev/usb/lpN 或额外绑定 usblp。
# 独立后台进程，探测/上传有超时，失败不阻塞 cupsd / cups-web 启动。
mkdir -p /var/log/cups
if [ -x /usr/local/bin/hp-firmware-monitor ]; then
    echo "[entrypoint] starting HP firmware monitor; log: /var/log/cups/hp-firmware.log"
    /usr/local/bin/hp-firmware-monitor >> /var/log/cups/hp-firmware.log 2>&1 &
fi

# ══════════════════════════════════════════════════════════════
# 6. Start dbus + avahi + ipp-usb (from cups/entrypoint.sh)
# ══════════════════════════════════════════════════════════════
# ── 后台拉起 avahi-daemon 与 ipp-usb：用于 driverless / IPP Everywhere 发现 ──
# 其中 ipp-usb 负责把 USB 直连的 IPP Everywhere 打印机（如 Brother DCP-T425W）
# 暴露成本地 http://localhost 的 IPP 端点，让 CUPS 能把它识别为
# "IPP Everywhere (color)" 机型。两者均允许缺失（某些架构 ipp-usb 可能未安装，
# 或容器未拿到 USB 设备），失败不影响 cupsd 启动。
if command -v avahi-daemon >/dev/null 2>&1; then
    # avahi-daemon 依赖 system dbus，由容器内自带的 dbus-daemon 提供。
    # ⚠️ 不要把宿主 /run/dbus/system_bus_socket 挂进来（issue #107）：
    # socket 路径被宿主文件占用后这里的 dbus-daemon 起不来，avahi 跟着失效，
    # CUPS 既发现不了 dnssd:// 网络打印机，AirPrint 也无法广播。
    # 启动失败不再完全静默，打一条 WARN 便于排查（issue #107）。
    mkdir -p /var/run/dbus
    if ! err=$(dbus-daemon --system --fork 2>&1); then
        echo "[entrypoint] WARN: dbus-daemon 启动失败，avahi 不可用——网络打印机发现与 AirPrint 广播将失效（检查是否误挂了宿主 dbus socket）：${err}"
    elif ! err=$(avahi-daemon --daemonize --no-chroot 2>&1); then
        echo "[entrypoint] WARN: avahi-daemon 启动失败——无法发现 dnssd:// 网络打印机，AirPrint 不广播：${err}"
    fi
fi
if command -v ipp-usb >/dev/null 2>&1; then
    # ipp-usb 默认走 systemd，容器里直接前台 --no-fork 失败，用后台模式；
    # 拿不到 USB（未挂 /dev/bus/usb）时会自动退出，不影响 cupsd。
    mkdir -p /var/log/ipp-usb /var/lock/ipp-usb
    (ipp-usb >/var/log/ipp-usb/ipp-usb.log 2>&1 &) || true
fi

# ══════════════════════════════════════════════════════════════
# 6b. 外部 CUPS 冲突检测（issue #79）
# ══════════════════════════════════════════════════════════════
# 场景：单容器镜像 (cups-web + 内嵌 cupsd) 与 host 网络模式配合时，如果
# 宿主机自己也装了 cupsd 并已在跑（用户在飞牛 OS 命令行手动装过 CUPS，
# 或宿主本身自带并启用的 CUPS），宿主的 cupsd 已经占了 631/tcp。
# 容器内的 cupsd 拉起时 bind 631 会失败 → watchdog 短命重试到 fast-fails
# 上限后放弃 → 每次容器启动都刷一屏错误日志；此外 restore-drivers 里的
# dpkg -i / lpadmin 若配合失败可能拖慢启动。
#
# 检测到 631 已被**容器外**的进程占用时，直接让路：跳过内嵌 cupsd 与
# 后续依赖 cupsd 的探活/AirPrint 修补，前台 cups-web 依旧启动并通过
# CUPS_HOST=localhost:631 走宿主 cupsd。此时 host network 让二者共处
# 同一网络命名空间，功能上等价于 CUPS_HOST=<宿主IP>。
#
# ── 判定方式 ──────────────────────────────────────────────────
# 只判"当前是否已经有进程 listen 在 631"，不区分是不是自己的 cupsd——
# 因为 6b 是在 cupsd 启动**之前**跑的，容器内还没有本地 cupsd 在监听，
# 任何监听者都是外部（宿主）的。优先 ss，fallback 到 netstat，再 fallback
# 到 bash /dev/tcp 尝试连接。任何一步判定"被占用"就跳过内嵌 cupsd。
# 检测本身失败（工具都不可用）则保守假设"未被占用"，走原路径。
SKIP_CUPSD=0
_port631_in_use() {
    if command -v ss >/dev/null 2>&1; then
        ss -Hltn 'sport = :631' 2>/dev/null | grep -q ':631'
        return $?
    fi
    if command -v netstat >/dev/null 2>&1; then
        netstat -ltn 2>/dev/null | grep -qE '[[:space:]]:::631[[:space:]]|[[:space:]]0\.0\.0\.0:631[[:space:]]|[[:space:]]127\.0\.0\.1:631[[:space:]]'
        return $?
    fi
    # bash 内置 /dev/tcp：能连上说明有人 listen；连不上视为空闲。
    # 用 timeout 兜底避免个别环境 /dev/tcp 阻塞。
    if timeout 1 bash -c '</dev/tcp/127.0.0.1/631' 2>/dev/null; then
        return 0
    fi
    return 1
}

if _port631_in_use; then
    SKIP_CUPSD=1
    export CUPS_HOST="${CUPS_HOST:-localhost:631}"
    echo "[entrypoint] WARN: 检测到 631/tcp 已被宿主机 cupsd 占用（host 网络模式下等同于容器外 CUPS 在跑）。"
    echo "[entrypoint] WARN: 跳过内嵌 cupsd 启动，cups-web 将通过 CUPS_HOST=${CUPS_HOST} 使用宿主机 CUPS（issue #79）。"
    echo "[entrypoint] WARN: 若你希望改用容器内 cupsd，请先停掉宿主机的 cups 服务再重启本容器。"
fi

# ══════════════════════════════════════════════════════════════
# 7. Start cupsd in background with auto-restart watchdog
# ══════════════════════════════════════════════════════════════
# ── 为什么 cupsd 必须在 watchdog 子 shell **内部**前台启动 ────────────────
# bash 的 `wait` 只能等待**当前 shell 自己的子进程**。如果在主 shell 里
# `cupsd -f &` 拿到 PID，再在子 shell 里 `wait $CUPSD_PID`，那个 PID 对子
# shell 来说是"兄弟进程"而不是子进程，bash 会立刻返回 127（not a child of
# this shell）而不是阻塞等待。老实现用 `|| true` 把这个错误吞掉，于是
# 循环立即往下走 → sleep 2 → 再拉一个 cupsd → 631 端口已被占用，新进程
# 秒退 → 每 2 秒 fork 一次，形成重启风暴、日志刷满。
#
# 正确做法：让 cupsd 在 watchdog 子 shell 内以**前台**方式运行。这样它就是
# 子 shell 的直接子进程，子 shell 会在 cupsd 真正退出时才继续，`$?` 也是
# cupsd 的真实退出码。整个子 shell 再 `&` 到后台，不阻塞后续启动步骤。
# ⚠️ 后人修改注意：不要把 `/usr/sbin/cupsd -f` 挪到子 shell 外面再配
# `wait`，那会退回到上面描述的 127 死循环。
#
# ── 失败退避（防止配置错误导致无限刷屏）────────────────────────────────
# cupsd 若因配置错误（cupsd.conf 语法错、端口被宿主占用、权限问题）启动即退，
# 无限重试只会刷满日志。这里统计"短命退出"（存活 < CUPSD_MIN_UPTIME 秒）的
# 连续次数，达到 CUPSD_MAX_FAST_FAILS 次就打印醒目错误并彻底放弃重启；
# 只要有一次存活超过阈值（说明是偶发崩溃而非配置问题）就把计数器清零。
CUPSD_MIN_UPTIME=5
CUPSD_MAX_FAST_FAILS=5
if [ "$SKIP_CUPSD" != "1" ]; then
(
    fast_fails=0
    while true; do
        start_ts=$SECONDS
        /usr/sbin/cupsd -f
        rc=$?
        uptime=$((SECONDS - start_ts))

        if [ "$uptime" -lt "$CUPSD_MIN_UPTIME" ]; then
            fast_fails=$((fast_fails + 1))
        else
            fast_fails=0
        fi

        if [ "$fast_fails" -ge "$CUPSD_MAX_FAST_FAILS" ]; then
            echo "[entrypoint] ERROR: cupsd exited ${fast_fails} times within ${CUPSD_MIN_UPTIME}s (last rc=${rc})."
            echo "[entrypoint] ERROR: 这通常是 cupsd 配置错误（/etc/cups/cupsd.conf）或 631 端口被占用，"
            echo "[entrypoint] ERROR: 而不是偶发崩溃。放弃自动重启，请检查上方 cupsd 日志后重启容器。"
            break
        fi

        echo "[entrypoint] cupsd exited (rc=${rc}, uptime=${uptime}s), restarting in 2s..."
        sleep 2
    done
) &
fi

# ══════════════════════════════════════════════════════════════
# 8. Wait for cupsd to be ready (max 30s)
# ══════════════════════════════════════════════════════════════
# 跳过内嵌 cupsd 时（见 6b），本地不会启动 cupsd；此时 lpstat -r 走 IPP
# 打到宿主 cupsd 会成功但意义不大，直接跳过省 30s 等待窗口。
if [ "$SKIP_CUPSD" != "1" ]; then
for i in $(seq 1 30); do
    lpstat -r >/dev/null 2>&1 && break
    sleep 1
done
fi

# ══════════════════════════════════════════════════════════════
# 8b. PdftopsRenderer: PDF→PostScript 渲染器改用 Ghostscript (issue #105)
# ══════════════════════════════════════════════════════════════
# cups-filters 的 pdftops 滤镜默认用 Poppler 渲染 PDF→PostScript，对某些 PDF
# 会失败导致任务卡在 processing。这里分两步修复：
#   ① 修补存量卷中缺失 PdftopsRenderer 的 cups-browsed.conf（影响新发现的队列）
#   ② 对所有已存在的打印机队列设置 pdftops-renderer=gs（影响存量队列）
# 幂等：已设过则跳过。失败不阻塞启动。

# ① 存量卷修补：Dockerfile 已在镜像基线写入 PdftopsRenderer gs，但挂载的
#   ./.etc 卷可能还是旧配置，这里补上。
if [ -f /etc/cups/cups-browsed.conf ] && ! grep -q '^[[:space:]]*PdftopsRenderer' /etc/cups/cups-browsed.conf; then
    echo "" >> /etc/cups/cups-browsed.conf
    echo "# Use Ghostscript for PDF->PostScript conversion (issue #105)" >> /etc/cups/cups-browsed.conf
    echo "PdftopsRenderer gs" >> /etc/cups/cups-browsed.conf
    echo "[entrypoint] 已为存量 cups-browsed.conf 追加 PdftopsRenderer gs（issue #105）"
fi

# ② 对所有已存在的打印机队列设置 pdftops-renderer=gs
#    lpoptions -p NAME -o pdftops-renderer=gs 写入 per-printer 默认选项，
#    对手动通过 CUPS Web UI / lpadmin 添加的本地队列生效。
#    跳过内嵌 cupsd 时（issue #79），lpoptions 会打到宿主 cupsd —— 不能
#    擅自修改用户宿主机上的打印队列配置，整段短路。
if [ "$SKIP_CUPSD" != "1" ]; then
(
    set +x
    for p in $(lpstat -e 2>/dev/null); do
        current=$(lpoptions -p "$p" -l 2>/dev/null | grep 'pdftops-renderer' | head -1)
        if ! echo "$current" | grep -q '\bgs\b'; then
            if lpoptions -p "$p" -o pdftops-renderer=gs 2>/dev/null; then
                echo "[entrypoint] set pdftops-renderer=gs on $p (issue #105)"
            else
                echo "[entrypoint] WARN: failed to set pdftops-renderer=gs on $p (issue #105)"
            fi
        fi
    done
) &

# ══════════════════════════════════════════════════════════════
# 9. AirPrint A4 media-ready patch (from cups/entrypoint.sh)
# ══════════════════════════════════════════════════════════════
# ── 把 HP 1020 队列的默认纸张设成 A4 ────────────────────────────────────
# issue #48 把 PPD 的 *DefaultPageSize 改成 A4；这里再把**队列**的默认媒体
# (IPP media-default) 也设成 A4，让 iOS 打印面板打开时预选 A4 而不是 Letter。
#
# ⚠️ 更正一处长期的误解：本段注释以前写着"CUPS 随之把 media-ready 通告成 A4"，
# **这句不成立**。按 CUPS 源码（scheduler/printers.c 的 load_ppd），`media-ready`
# 的唯一写入点是拿 `ReadyPaperSizes` 去和 PPD 支持的尺寸求交集，跟 media-default
# 完全无关。`lpadmin -o media=` 只写 media-default。
# 也就是说 issue #82 抱怨的"iPhone 纸张列表里没有 A4"并不是靠这一句修好的——
# 真正起作用的是 cupsd.conf 里的 `ReadyPaperSizes`（见本脚本第 3 步与 Dockerfile）。
# 这一句保留，因为它确实负责"面板默认勾选 A4"这件事，只是别再把它当成
# media-ready 的解法。
#
# 手法：cupsd 起来后对存量 HP 1020 队列执行
#   lpadmin -p NAME -o media=iso_a4_210x297mm
# lpadmin 需要 cupsd 在线(走 IPP)，所以放后台等 cupsd 就绪后执行，
# 不阻塞启动；exec 替换父进程不会杀掉已 fork 的后台子 shell。
# 命中条件与 issue #48 的一次性 PPD 修补一致(HP 1020 foo2zjs 双重指纹 +
# PageSize 列表含 A4)，队列名由 /etc/cups/ppd/<printer>.ppd 文件名反推。
# 仅当当前默认纸张仍是 A4(即用户没在 CUPS UI 里显式改过)时才动，避免覆盖
# 用户的手动选择。任何一台失败只告警不影响其他队列与 cupsd。
(
    set +x
    if [ -d /etc/cups/ppd ]; then
        for ppd in /etc/cups/ppd/*.ppd; do
            [ -f "$ppd" ] || continue
            grep -q '^\*Product:[[:space:]]*"(HP LaserJet 1020)"' "$ppd" || continue
            grep -q '^\*FoomaticIDs:[[:space:]]\+HP-LaserJet_1020[[:space:]]\+foo2zjs-z1' "$ppd" || continue
            grep -q '^\*PageSize A4' "$ppd" || continue
            grep -q '^\*DefaultPageSize:[[:space:]]\+A4[[:space:]]*$' "$ppd" || continue
            printer_name="$(basename "$ppd" .ppd)"
            if lpadmin -p "$printer_name" -o media=iso_a4_210x297mm 2>/dev/null; then
                echo "[entrypoint] set media-default=A4 on $printer_name (issue #48/#82)"
            else
                echo "[entrypoint] WARN: failed to set media-default=A4 on $printer_name (issue #48/#82)"
            fi
        done
    fi
) &
fi   # end: SKIP_CUPSD 保护 8b/9 两段——跳过内嵌 cupsd 时不改宿主队列（issue #79）

# ══════════════════════════════════════════════════════════════
# 10. Start cups-web as foreground process (PID 1)
# ══════════════════════════════════════════════════════════════
exec /cups-web
