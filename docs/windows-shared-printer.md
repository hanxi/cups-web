# 添加 Windows 共享打印机

适用于打印机通过 USB 等方式接在 Windows 电脑上，准备从 cups-web 提交打印的场景。链路为 **cups-web → CUPS（IPP）→ Windows 打印共享（SMB，TCP 445）→ 打印机**。打印时 Windows 需要开机且联网。

以下以 Windows 主机 `WINPC`、示例 IP `192.0.2.50` 和 `EPSON L380 Series` 为例。Windows 的**打印机名称**是 `EPSON L380 Series`，**共享名**和 **CUPS 队列名**在本例都取 `EPSON-L380`。操作前替换 IP、主机名和队列名称。Compose 命令在部署目录执行，示例服务名为 `cups`。

## 1. 在 Windows 开启共享

先从 Windows 本机确认打印机可以打印。可信局域网使用「专用网络」配置，并开启「文件和打印机共享」。在「设置 → 打印机和扫描仪 → 打印机属性 → 共享」启用共享并记录共享名。也可通过 SSH 连接 Windows，使用有管理员权限的 PowerShell 执行以下操作。先查看准确的队列名称和已有共享名，复用已有共享可以避免影响其他客户端。[Microsoft 打印机共享指南](https://support.microsoft.com/en-us/windows/hardware/printer/share-a-printer-as-a-network-printer)

```powershell
Get-Printer | Select-Object Name, PortName, Shared, ShareName
Get-NetConnectionProfile | Select-Object InterfaceAlias, NetworkCategory
Get-Service Spooler, LanmanServer

# 确认目标队列及共享名后执行。
Set-Printer -Name 'EPSON L380 Series' -Shared $true -ShareName 'EPSON-L380'
Get-Printer -Name 'EPSON L380 Series' | Select-Object Name, Shared, ShareName
net share
```

为 CUPS 使用一个有打印权限的 Windows 标准账号。需要新建账号时，先确认账号名可用，再创建专用本地用户。以下命令交互式读取密码。检查「打印机属性 → 安全」中的有效权限，按需给该账号授予「打印」，保留现有权限设置。打印账号、CUPS 管理员和 cups-web 登录账号各自用于对应服务。[Microsoft `New-LocalUser` 手册](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.localaccounts/new-localuser)

```powershell
$password = Read-Host 'Windows SMB printing password' -AsSecureString
$account = New-LocalUser -Name 'cups_print' -Password $password -Description 'SMB printing from CUPS'
$users = Get-LocalGroup -SID 'S-1-5-32-545'  # 内置 Users 组，SID 不随界面语言变化。
if (-not (Get-LocalGroupMember -Group $users | Where-Object SID -eq $account.SID)) {
    Add-LocalGroupMember -Group $users -Member "$env:COMPUTERNAME\cups_print"
}
```

## 2. 准备 CUPS 的 SMB 后端和打印驱动

在**运行 `cupsd` 的环境**中操作。AIO 部署在 `cups` 容器内处理。仅 Web 部署在外部 CUPS 上处理，cups-web 仍通过 `CUPS_HOST` 或 `CUPS_SOCKET_PATH` 使用外部 CUPS。

- **SMB 后端**。在 Debian 上安装 `smbclient`，它提供 `smbspool` 和 `/usr/lib/cups/backend/smb`。容器里缺少后端时，用派生镜像安装，确保重建后依然可用。[Debian 软件包文件清单](https://packages.debian.org/trixie/amd64/smbclient/filelist)
- **驱动**。用 `lpinfo -m` 查找实际型号。Windows 共享负责接收打印作业，CUPS 仍需用 Linux 驱动生成打印机可识别的数据。对 `smb://` 队列选用匹配型号的驱动或 PPD。Epson L380 实测使用 `amd64` 的原厂 `epson-inkjet-printer-201601w` 软件包，项目内的「Epson 国行驱动」（`epson-cn`）提供同一软件包。重建容器后如果缺少驱动，在 cups-web 的「驱动」页安装，或运行 `docker compose exec -T cups driver-install epson-cn`。保留 `.drivers` 挂载，供重建后恢复驱动。[驱动管理](driver-management.md)

如果 AIO 镜像缺少 SMB 后端，可在 Compose 文件旁建立 `image/Dockerfile`。将 `FROM` 换成实际部署使用的基础镜像版本可避免同时升级 cups-web。

```dockerfile
FROM hanxi/cups-web:latest
RUN apt-get update \
    && apt-get install -y --no-install-recommends smbclient \
    && rm -rf /var/lib/apt/lists/*
```

将 `cups` 服务的 `image` 改为本地派生镜像名 `cups-web-local:smb`，加上 `build: ./image`，保留 `./.etc:/etc/cups`、`./.drivers:/opt/cups-drivers/data` 等原有挂载。完成下一节的网络选择后执行 `docker compose up -d --build cups`。

## 3. 选择 Docker 网络

SMB 直连 Windows 的 IP 或 DNS 名称只需要 `cups` 容器能向 Windows 建立 TCP 445 连接。只用 cups-web 网页打印时，删除 `network_mode: host`，在 `cups` 服务下配置以下映射。示例假定 `LISTEN_ADDR=:1180`。CUPS 的 `631` 保留给容器内的 cups-web，Windows 的 `445` 是出站连接的目标端口，两者都无需在宿主机发布。若依赖 AirPrint 广播或自动发现局域网打印机，沿用仓库的 host 网络配置以支持 mDNS。[Docker Compose 网络文档](https://docs.docker.com/compose/how-tos/networking/) · [AirPrint 与自动发现](../README.md#airprint-搜不到打印机--发现不了局域网网络打印机)

```yaml
ports:
  - "1180:1180"
```

启动后从 `cupsd` 所在环境检查后端和共享认证。`smbclient` 会交互式提示输入 Windows 账号密码。`quit` 验证能否登录并进入指定共享，不提交打印作业。

```bash
docker compose exec -T cups test -x /usr/lib/cups/backend/smb
docker compose exec -T cups lpinfo -m | grep -i 'L380'
docker compose exec cups smbclient //192.0.2.50/EPSON-L380 -U 'WINPC\cups_print' -t 5 -c quit
```

## 4. 在 CUPS 添加队列并配置认证

在 CUPS 的「Administration → Add Printer」选择「Windows Printer via SAMBA」，填写以下设备地址并选择匹配的型号。已有队列时，在对应队列的「Modify Printer」中更新连接。将 `PASSWORD` 替换为实际密码，URI 中的保留字符需要做百分号编码。仅在受保护的 CUPS 管理连接中输入完整设备地址。含凭据的 URI 由 CUPS 保存到 `/etc/cups/printers.conf`，AIO 部署对应宿主机的 `./.etc/printers.conf`，应与备份一同限制访问。[Samba 设备 URI 语法](https://www.samba.org/samba/docs/current/man-html/smbspool.8.html)

```text
smb://cups_print:PASSWORD@WINPC/192.0.2.50/EPSON-L380
```

如果已有队列仍要求每个作业提供密码，确认已保存 SMB 凭据后，运行 `docker compose exec -T cups lpadmin -p EPSON-L380 -o auth-info-required=none`，让 SMB 后端使用队列内的凭据。

Linux Docker 宿主机可通过 SSH 临时访问桥接容器的 CUPS 管理页，无需发布 `631`。在宿主机运行 `docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' cups` 获取单网络部署的容器 IP。从浏览器所在机器执行 `ssh -N -L 127.0.0.1:1631:CONTAINER_IP:631 DOCKER_HOST`（替换 `CONTAINER_IP` 和 SSH 主机 `DOCKER_HOST`），打开 <http://127.0.0.1:1631/admin/>，必要时按 CUPS 提示切换 HTTPS。配置完成后关闭隧道。

**Agent 或无人值守配置**

1. 先用 `Get-Printer`、`lpstat -p` 和 `lpinfo -m` 核对目标，再创建账号或修改指定队列。SSH 自动化可让 PowerShell 从标准输入读入密码并转换成 `SecureString`。通过 `smbclient //192.0.2.50/EPSON-L380 -A /dev/stdin -t 5 -c quit` 验证认证时，将 `username`、`password`、`domain` 作为三行 `键 = 值` 经标准输入传入。
2. 可先用 `lpadmin -p EPSON-L380 -E -v smb://192.0.2.50/EPSON-L380 -m MODEL_ID` 建立队列。将 `MODEL_ID` 换成 `lpinfo -m` 输出中匹配型号的第一列。
3. 在 `cupsd` 所在环境使用有管理权限的 libcups 客户端。先用 `cupsSetServer` 指向本地 Unix socket（AIO 为 `/run/cups/cups.sock`），再通过 `cupsDoRequest` 向 `/admin/` 发送 `CUPS-Add-Modify-Printer`（`0x4003`）。操作属性 `printer-uri` 指向已有队列，如 `ipp://localhost/printers/EPSON-L380`。打印机属性 `device-uri` 填含凭据的 SMB URI，同时将 `auth-info-required` 设为 `none`，由后端使用队列凭据完成认证。检查 IPP 状态和配置落盘后再测试打印。[CUPS IPP 扩展](https://openprinting.github.io/cups/doc/spec-ipp.html#CUPS_ADD_MODIFY_PRINTER) · [libcups 接口](https://openprinting.github.io/cups/doc/cupspm.html)
4. 通过标准输入或受保护的凭据来源传递密码。报告队列名、状态和验证结果，避免在命令参数、日志、Compose 文件、临时文件或提交记录中输出完整设备 URI。

## 5. 验证与排查

按顺序检查。第一条确认 Web 可访问，第二条确认 CUPS 队列已启用，第三条会实际提交一页测试作业。若 `LISTEN_ADDR` 或队列名有修改，请替换示例值。

```bash
curl -fsS http://127.0.0.1:1180/api/version
docker compose exec -T cups lpstat -p EPSON-L380
docker compose exec -T cups lp -d EPSON-L380 -o page-ranges=1 /usr/share/cups/data/default-testpage.pdf
```

记录 `lp` 返回的作业 ID。例如 `EPSON-L380-1` 可用下面的命令查看状态。Windows 上通过 `Get-PrintJob -PrinterName 'EPSON L380 Series'` 查看是否还有作业等待。CUPS 的 `job-completed-successfully` 表示后端已成功交接，实际出纸需要在打印机旁确认。最后刷新 cups-web，确认列表中可选中该队列。

```bash
docker compose exec -T cups ipptool -tv -d job-id=1 \
  ipp://localhost:631/jobs/1 /usr/share/cups/ipptool/get-job-attributes.test
```

| 现象 | 检查方法 |
| --- | --- |
| 添加时找不到 SMB，或提示不支持 `smb://` | 在 `cupsd` 所在环境检查 `smbclient` 和 `backend/smb`。容器重建后也要检查。 |
| 宿主机可连接，容器超时 | 从容器内重跑 `smbclient`，核对容器到 Windows 的路由、Windows 防火墙和 `445` 的连通性。 |
| `NT_STATUS_LOGON_FAILURE`、`NT_STATUS_ACCESS_DENIED` 或 `Held for authentication` | 核对 Windows 主机名、账号、密码和队列打印权限。`session setup failed` 指向登录过程，`tree connect failed` 指向共享名或共享权限。用同一账号先跑通 `smbclient`，再检查 CUPS 队列凭据及 `auth-info-required`。 |
| 能连接共享，但乱码或出现过滤器错误 | 核对 Linux 驱动与实际型号、CPU 架构以及驱动过滤器。L380 选择原厂 L380 驱动或 PPD。 |
| cups-web 报提交成功，但没有出纸 | 查看 CUPS 作业状态和 Windows 队列。IPP 接受作业、SMB 交接和实际出纸是三个独立的验证点。 |
