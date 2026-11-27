# Add a Windows-shared printer

Use this guide when a printer is connected to a Windows PC and you want to submit jobs from cups-web. The chain is **cups-web → CUPS (IPP) → Windows printer share (SMB, TCP 445) → printer**. The Windows PC must be on and connected to the network while printing.

The examples use a Windows host named `WINPC`, the example IP `192.0.2.50`, and an `EPSON L380 Series` printer. The Windows **printer name** is `EPSON L380 Series`. The **share name** and the **CUPS queue name** are both `EPSON-L380` here. Replace the host, IP, and queue names with your own values. Run Compose commands from the deployment directory. The example service is named `cups`.

## 1. Share the printer on Windows

Print a test page from Windows first. On a trusted LAN, use the Private network profile and enable **File and printer sharing**. In **Settings → Printers & scanners → Printer properties → Sharing**, share the printer and note the share name. You can also connect over SSH and run the following commands in PowerShell as a Windows administrator. Check the exact printer name and any existing share. Keep an existing share name if other clients use it. [Microsoft printer sharing guide](https://support.microsoft.com/en-us/windows/hardware/printer/share-a-printer-as-a-network-printer)

```powershell
Get-Printer | Select-Object Name, PortName, Shared, ShareName
Get-NetConnectionProfile | Select-Object InterfaceAlias, NetworkCategory
Get-Service Spooler, LanmanServer

# Run after confirming the printer and share names.
Set-Printer -Name 'EPSON L380 Series' -Shared $true -ShareName 'EPSON-L380'
Get-Printer -Name 'EPSON L380 Series' | Select-Object Name, Shared, ShareName
net share
```

Use a standard Windows account that has permission to print. If you need a dedicated local account, check that the name is available first. The example prompts for its password. Check effective permissions under **Printer properties → Security** and grant the account **Print** permission if necessary, preserving existing entries. The Windows print account, the CUPS administrator, and the cups-web login are credentials for their respective services. [Microsoft `New-LocalUser` reference](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.localaccounts/new-localuser)

```powershell
$password = Read-Host 'Windows SMB printing password' -AsSecureString
$account = New-LocalUser -Name 'cups_print' -Password $password -Description 'SMB printing from CUPS'
$users = Get-LocalGroup -SID 'S-1-5-32-545'  # The built-in Users group has a language-independent SID.
if (-not (Get-LocalGroupMember -Group $users | Where-Object SID -eq $account.SID)) {
    Add-LocalGroupMember -Group $users -Member "$env:COMPUTERNAME\cups_print"
}
```

## 2. Prepare the CUPS SMB backend and printer driver

Work **where `cupsd` runs**. For an all-in-one deployment, this is the `cups` container. In a web-only deployment, configure the external CUPS server that cups-web reaches through `CUPS_HOST` or `CUPS_SOCKET_PATH`.

- **SMB backend.** On Debian, install `smbclient`, which provides `smbspool` and `/usr/lib/cups/backend/smb`. Install the package in a derived container image when the base image lacks it so that container recreation keeps the backend. [Debian file list](https://packages.debian.org/trixie/amd64/smbclient/filelist)
- **Driver.** Find your printer model with `lpinfo -m`. The Windows share receives print jobs. CUPS still uses a Linux driver to produce data that the printer can read. Select a matching driver or PPD for the `smb://` queue. The L380 was tested with the vendor's `amd64` package `epson-inkjet-printer-201601w`. The project's **Epson China driver** (`epson-cn`) supplies the same package. If the driver is missing after container recreation, install it from the Drivers page or run `docker compose exec -T cups driver-install epson-cn`. Keep the `.drivers` mount for staged driver restoration on recreation. [Driver management](driver-management.md)

If your AIO image lacks the SMB backend, create `image/Dockerfile` beside the Compose file. Use the base version already deployed in `FROM` to avoid upgrading cups-web at the same time.

```dockerfile
FROM hanxi/cups-web:latest
RUN apt-get update \
    && apt-get install -y --no-install-recommends smbclient \
    && rm -rf /var/lib/apt/lists/*
```

Set the `cups` service's `image` to `cups-web-local:smb` and add `build: ./image`. Retain mounts such as `./.etc:/etc/cups` and `./.drivers:/opt/cups-drivers/data`. After making the network choice in the next section, run `docker compose up -d --build cups`.

## 3. Choose the Docker network

Connecting to a Windows host by IP or DNS name only requires the `cups` container to reach TCP 445 on Windows. For web-only access to the AIO service, remove `network_mode: host` and add the following mapping under `cups`. This assumes `LISTEN_ADDR=:1180`. CUPS port `631` stays internal to the container. Port `445` is the destination of an outbound connection. If you use AirPrint advertising or automatic LAN printer discovery, keep the default host networking to support mDNS. [Docker Compose networking](https://docs.docker.com/compose/how-tos/networking/) · [AirPrint and discovery](../../README.en.md#airprint-or-lan-printer-discovery-fails)

```yaml
ports:
  - "1180:1180"
```

From the `cupsd` environment, check the backend and verify shared-printer authentication. `smbclient` prompts for the Windows account password. The `quit` command checks the session and share without submitting a print job.

```bash
docker compose exec -T cups test -x /usr/lib/cups/backend/smb
docker compose exec -T cups lpinfo -m | grep -i 'L380'
docker compose exec cups smbclient //192.0.2.50/EPSON-L380 -U 'WINPC\cups_print' -t 5 -c quit
```

## 4. Add the CUPS queue and configure authentication

In **Administration → Add Printer**, select **Windows Printer via SAMBA**, enter the device address below, and choose the matching model. Update the connection through **Modify Printer** if the queue already exists. Replace `PASSWORD` with the account password and percent-encode any reserved URI characters. Enter the complete device URI over a protected CUPS administration connection. CUPS saves the URI with its credentials in `/etc/cups/printers.conf`, which corresponds to `./.etc/printers.conf` in an AIO Compose deployment. Restrict access to the file and its backups. [Samba device URI syntax](https://www.samba.org/samba/docs/current/man-html/smbspool.8.html)

```text
smb://cups_print:PASSWORD@WINPC/192.0.2.50/EPSON-L380
```

If the queue still prompts for a password on each job, verify that the SMB credential is saved, then run `docker compose exec -T cups lpadmin -p EPSON-L380 -o auth-info-required=none`. This allows the backend to use the stored credential.

On a Linux Docker host, reach a bridged container's CUPS administration page through a temporary SSH tunnel. Run `docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' cups` on the Docker host to get the IP of a container attached to a single network. From the machine running your browser, run `ssh -N -L 127.0.0.1:1631:CONTAINER_IP:631 DOCKER_HOST`, substituting the container IP and SSH host. Open <http://127.0.0.1:1631/admin/> and switch to HTTPS if CUPS prompts you. Close the tunnel when finished.

**Agent or unattended setup**

1. Use `Get-Printer`, `lpstat -p`, and `lpinfo -m` to identify the target before creating an account or changing a queue. In an SSH script, read the password from standard input in PowerShell and convert it to `SecureString`. For `smbclient //192.0.2.50/EPSON-L380 -A /dev/stdin -t 5 -c quit`, pass `username`, `password`, and `domain` on three `key = value` lines through standard input.
2. If needed, create the queue with `lpadmin -p EPSON-L380 -E -v smb://192.0.2.50/EPSON-L380 -m MODEL_ID`, substituting the first column of the matching `lpinfo -m` row for `MODEL_ID`.
3. Use a libcups client with administrative access in the `cupsd` environment. Point `cupsSetServer` at the local Unix socket (`/run/cups/cups.sock` in AIO) and send `CUPS-Add-Modify-Printer` (`0x4003`) to `/admin/` with `cupsDoRequest`. Set the operation attribute `printer-uri` to the existing queue URI, such as `ipp://localhost/printers/EPSON-L380`. Set the printer attribute `device-uri` to the authenticated SMB URI, and set `auth-info-required` to `none` so the backend uses the saved queue credential. Check the IPP response and verify that the configuration was saved before printing. [CUPS IPP extensions](https://openprinting.github.io/cups/doc/spec-ipp.html#CUPS_ADD_MODIFY_PRINTER) · [libcups API](https://openprinting.github.io/cups/doc/cupspm.html)
4. Pass passwords through standard input or a protected credential source. Report the queue name, status, and verification result. Keep the full credential-bearing URI out of command arguments, logs, Compose files, temporary files, and commits.

## 5. Verify and troubleshoot

Check the service and queue in order. The last command submits one real test page. Replace `LISTEN_ADDR` and the queue name if your deployment uses different values.

```bash
curl -fsS http://127.0.0.1:1180/api/version
docker compose exec -T cups lpstat -p EPSON-L380
docker compose exec -T cups lp -d EPSON-L380 -o page-ranges=1 /usr/share/cups/data/default-testpage.pdf
```

Record the job ID from `lp`. For `EPSON-L380-1`, inspect the job with the command below. On Windows, use `Get-PrintJob -PrinterName 'EPSON L380 Series'` to check for pending jobs. `job-completed-successfully` means CUPS handed the job off successfully. Confirm the printed page at the printer, then refresh cups-web and select the queue.

```bash
docker compose exec -T cups ipptool -tv -d job-id=1 \
  ipp://localhost:631/jobs/1 /usr/share/cups/ipptool/get-job-attributes.test
```

| Symptom | What to check |
| --- | --- |
| The SMB backend is missing or `smb://` is unsupported | Check `smbclient` and `backend/smb` where `cupsd` runs, including after container recreation. |
| The host can connect but the container times out | Run `smbclient` inside the container. Check the route, Windows firewall, and reachability of TCP 445. |
| `NT_STATUS_LOGON_FAILURE`, `NT_STATUS_ACCESS_DENIED`, or `Held for authentication` | Check the Windows host name, username, password, and printer permissions. `session setup failed` points to sign-in. `tree connect failed` points to the share name or permissions. Verify with `smbclient` first, then check the queue credential and `auth-info-required`. |
| The share is reachable, but pages contain garbage or filters fail | Check the Linux driver, model, CPU architecture, and filter. For the L380, use the vendor's L380 driver or PPD. |
| cups-web reports submission but no page comes out | Check CUPS job status and the Windows queue. IPP acceptance, SMB handoff, and physical output are separate stages to verify. |
