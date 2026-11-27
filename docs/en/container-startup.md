# Container Startup

> This document expands the container startup section of [AGENTS.en.md](../../AGENTS.en.md). It explains the entrypoint steps, the cupsd watchdog's exit-127 loop, and the failure handling in `restore-drivers`.

In the all-in-one (AIO) image, `entrypoint.sh` is the container's `ENTRYPOINT`. The numbers match the comment blocks in the script.

1. **`restore-drivers`** restores the `.drivers` snapshot. For each driver, install archived `.deb` packages with `dpkg -i`, then copy manifest entries with `cp -aT`. File entries override or supplement the package contents, so they must go last.
2. **CUPS administrator**. If `$CUPSADMIN` is missing from `/etc/shadow`, create the account with `useradd -r -G lpadmin` and set its password with `chpasswd`. Configure tzdata from `$TZ`.
3. **CUPS configuration**. If `/etc/cups/cupsd.conf` is missing, as with an empty mount, copy the defaults from `/etc/cups-bak/`. Ensure the `ssl/` directory and `ReadyPaperSizes` are present on every startup outside that conditional.
4. **One-time Letter-to-A4 change for HP 1020 PPDs** (issue #48). Change an existing PPD only if both its `*Product` and `*FoomaticIDs` fingerprints match, its default remains Letter, and `*PageSize A4` exists. Save a `.bak-cupsweb-issue48` backup before editing.
5. **HP host-based firmware monitor**. Start `hp-firmware-monitor` in the background. Every 15 seconds it reads device IDs through the CUPS USB backend. It sends firmware to supported HP 1000/1005/1018/1020/P1005–P1008/P1505 printers that need it, addressing each device by its full USB URI. This covers printers powered on later, USB hotplug, and firmware installed while the container runs. Logs go to `/var/log/cups/hp-firmware.log`.
6. **D-Bus, Avahi, and ipp-usb**. Start these background processes for driverless or IPP Everywhere discovery and AirPrint advertising. cupsd can still start if any are missing or fail. Start the container's own `dbus-daemon --system` and keep the host's `/run/dbus/system_bus_socket` out of the container. If the socket path is occupied, D-Bus fails and Avahi fails with it (issue #107). D-Bus and Avahi startup failures produce `WARN` logs.
7. **cupsd and its watchdog**. See the [watchdog section](#run-cupsd-in-the-foreground-inside-the-watchdog-subshell).
8. **CUPS readiness**. Poll `lpstat -r` once per second, up to 30 times.
9. **HP 1020 queue `media-default=A4`**. In the background, run `lpadmin -p NAME -o media=iso_a4_210x297mm` on matching queues to preselect A4 in the iOS print dialog. The [paper-choice list](#readypapersizes-and-media-ready) depends on `media-ready`.
10. **`exec /cups-web`**. Run the Web process in the foreground as PID 1. Replacing the parent process with `exec` leaves already-forked background subshells running.

## HP Host-Based Firmware Recovery

The monitor follows the continuous monitoring, firmware-state checks, and USB autosuspend handling in `fix-foo2zjs-printer.sh` from [tzishue/cloud-printer](https://hub.docker.com/r/tzishue/cloud-printer). Users reported that it recovered printing on the HP LaserJet 1020. The earlier startup sequence called upstream `hplj*` firmware loaders once. The container had no udev trigger for a second load when a printer powered on later, was reconnected, or received a firmware installation while running.

`scripts/driver/hp-firmware-monitor.py` uses the CUPS USB backend already present in the image. Discovery returns full device URIs and IEEE-1284 IDs. The monitor handles a supported HP LaserJet only when both the URI and device ID agree on the vendor and model. It skips devices with `FWVER` or `FIRMWARE` fields and devices with absent or incomplete IDs. It rejects a URI that matches more than one device. The monitor preserves serial and interface parameters and sends the correct firmware for each device. The P1007 uses P1005 firmware, and the P1008 uses P1006 firmware. The M1005 MFP is outside this scope.

The Drivers page still installs firmware in `/lib/firmware/hp/`. The monitor finds new files on later checks. Discovery times out after 20 seconds and each upload after 30 seconds. Attempts for the same URI are at least 30 seconds apart. The next device-ID check confirms firmware status. Uploads run serially, and `/run/hp-firmware-monitor.lock` prevents duplicate instances. Missing firmware, busy devices, and individual failures allow Web startup and later retries to continue. The monitor logs state changes only.

The monitor sets `power/control` to `on` for USB vendor ID `03f0` and supported product IDs to reduce disconnects caused by autosuspend. A read-only `/sys` still allows firmware loading. With no supported HP device in sysfs, the monitor skips USB backend discovery to reduce access to other printers. It uses the existing `/dev/bus/usb` directory mount and container permissions. It requires no `/dev/usb/lpN` devices, `usblp` module loading, or forced binding. Existing queue URIs, error policies, and user jobs are preserved.

## Apply Both Step 3 Changes Outside the Conditional

Step 3 checks only `if [ ! -f /etc/cups/cupsd.conf ]`. Any existing `cupsd.conf` in the user's `./.etc` mount, including one from an older release, a different installation, or a manual setup, skips `cp -rpn /etc/cups-bak/*`. Existing installations would therefore miss additions to the image defaults. Apply the following two idempotent changes at every startup, outside the conditional.

### The `ssl/` Directory

The call to `cupsdCheckPermissions(ServerRoot, "ssl", …)` in cupsd passes `0` for `create_dir`. The adjacent `"ppd"` call passes `1`. When `lstat` fails for `ssl/`, the function returns `1` without creating the directory or logging an error, so startup appears successful.

The first IPPS handshake exposes the issue. `cupsMakeServerCredentials()` calls `cupsFileOpen(keyfile, "w")` for `/etc/cups/ssl/<cn>.key`. Without the directory, the call fails with `ENOENT`, producing `Unable to create server credentials` and `Unable to encrypt connection` in the error log. A build with `--enable-gnutls` advertises `_ipps._tcp` through DNS-SD regardless of credential availability. macOS and iOS prefer IPPS records, so Bonjour can discover a printer that cannot accept jobs.

Previous images inherited the empty directory from Debian's `cups-daemon` package. Create the directory explicitly so image startup does not depend on that packaging detail.

### `ReadyPaperSizes` and `media-ready`

The iPhone AirPrint dialog uses IPP `media-ready` for its list of currently loaded paper sizes. In CUPS, `media-ready` is the intersection of `ReadyPaperSizes` in `cupsd.conf` and the sizes supported by the PPD. The only place the scheduler sets it is `load_ppd()` in `scheduler/printers.c`. A PPD size is eligible when its name appears in `ReadyPaperSizes`.

> `lpadmin -p X -o media=iso_a4_210x297mm` updates `media-default`, which controls the default for jobs that specify no paper size and the dialog's preselected option. It has no effect on `media-ready`. Earlier versions of this document and `entrypoint.sh` said otherwise. Issue #82 likely appeared fixed because A4 became preselected and the list stayed the same.

Without `ReadyPaperSizes`, cupsd uses the locale. When `DefaultPaperSize` is unset, locales matching `C/POSIX/en*/en_US/en_CA/fr_CA` select Letter. Other locales select A4. The Letter set is `Letter,Legal,Tabloid,4x6,Env10`, and the A4 set is `A4,A3,A5,A6,EnvDL`. The image sets neither `LANG` nor `LC_ALL`, which selects locale C and the Letter set. This leaves A4 out of the iOS dialog's paper choices.

Debian's `/etc/papersize` override also has no effect. The source build does not pass `--enable-libpaper`, so `HAVE_LIBPAPER` is undefined and that code is excluded.

Set `ReadyPaperSizes A4,A3,A5,A6,EnvDL` in `cupsd.conf`. Use **PPD size names** such as `A4`. PWG names such as `iso_a4_210x297mm` do not match the PPD lookup. The PPD cache includes the `ConfigurationFile` modification time in its validity check, so changing `cupsd.conf` invalidates the cache and reparses the PPD automatically. There is no need to clear `var/cache/*.data` manually.

## Package Restoration with `dpkg -i`

Package restoration runs first, before cupsd starts. Vendor `postinst` scripts may call `invoke-rc.d cups restart` with `|| true`. With no cupsd running, those calls have no effect. cupsd starts with the restored PPDs and filters already in place and can load them without a second reload.

This is on the container's startup path, so the following safeguards apply.

| Safeguard | Failure it prevents |
| --- | --- |
| `DEBIAN_FRONTEND=noninteractive` and `dpkg -i --force-confold` | A conflicting configuration file makes dpkg wait for interactive input indefinitely. |
| `timeout 600` | Best-effort restoration blocks startup indefinitely. |
| Temporary `policy-rc.d` that returns `101`, installed only when absent and removed afterward | A vendor `postinst` starts a second cupsd with `service cups start`, takes port 631 outside the watchdog, and causes repeated failures. |
| Fall back from `dpkg -i` to `--force-depends`, then `dpkg --configure -a --force-depends` | Canon's `cups-bsd`/`libgtk-3-0` dependencies cannot be met on trixie. `--force-depends` both unpacks and configures the package. Konica's filter symlink needs that configuration step to run its `postinst`. |

> Keep `apt-get -f install` out of restoration. Startup may have no network access, and the image's `/var/lib/apt/lists` is normally empty. When apt resolves dependencies, it can remove the vendor package it cannot install. `install-canon-ufr2.sh` documents a case where this happened.

## Run cupsd in the Foreground Inside the Watchdog Subshell

Bash `wait` works only on children of the current shell. The old implementation started `cupsd -f &` in the main shell and monitored its PID with `wait $CUPSD_PID` in a watchdog subshell. The target PID was a sibling of that subshell, so `wait` returned `127` immediately (`not a child of this shell`). `|| true` hid the error. The watchdog started another cupsd after `sleep 2`. The new instance exited immediately because port 631 was already in use, and the cycle produced a failure and log messages every two seconds.

Run `/usr/sbin/cupsd -f` in the foreground **inside** `( while true; … ) &`. cupsd is then a direct child of the watchdog subshell, which blocks until cupsd actually exits and receives its real exit code in `$?`. The whole subshell runs in the background so the rest of startup can proceed. Keep this arrangement when modifying the watchdog.

### Consecutive Early Exits

`CUPSD_MIN_UPTIME=5` and `CUPSD_MAX_FAST_FAILS=5`. An exit after less than five seconds counts as an early failure. After five consecutive early failures, the watchdog prints a prominent Chinese error suggesting invalid `cupsd.conf` syntax or a port 631 conflict, then uses `break` to stop retrying. An instance that runs for more than five seconds resets the counter, allowing recovery from occasional crashes.

## `restore-drivers` Must Always Return Zero

`entrypoint.sh` uses `set -e`, and driver restoration is its first step. Snapshots can be damaged by earlier releases, another package can occupy a destination as a directory, or a mount can be read-only. Driver restoration is best effort. If it blocked startup, users would lose access to the Web UI they need to uninstall the broken driver.

`restore-drivers.sh` uses `set -uo pipefail`. It accounts for each file, reports `total_errors` and `total_skipped`, and tells users they can uninstall and reinstall affected drivers in the Web UI. It always ends with `exit 0`. The invocation in `entrypoint.sh` is also guarded by `|| echo "[entrypoint] WARN: restore-drivers 部分失败，继续启动"` so an unexpected signal or nonzero exit cannot abort the `set -e` entrypoint.
