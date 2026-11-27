# Driver Management

> This document expands the driver management section of [AGENTS.en.md](../../AGENTS.en.md). It explains persistence, allowlist failures, the `EXIT` trap convention, architecture detection, uploads, and `lpinfo` parsing. See the agent guide for the directory, exit-code, and path rule tables.

Related files are `cmd/server/driver_handlers.go`, `cmd/server/driver_registry.go`, `scripts/driver/*.sh`, and `frontend/src/views/DriversView.vue`. The Drivers page uses `/drivers` and is available to administrators.

## Driver Persistence Without `CAP_SYS_ADMIN`

Persistence uses ordinary files and package management. No overlay or mount operations are required. Drivers take one of two paths based on their source.

### File-Level Persistence

This path captures files from source builds, manual copies, ZIP archives, and firmware generation.

1. Snapshot `MONITORED_DIRS` before installation with `find \( -type f -o -type l \)`. It covers `/usr/lib/cups`, `/usr/share/cups`, `/usr/share/ppd`, `/lib/firmware`, `/usr/share/foomatic`, and the detected multiarch directory `/usr/lib/<triplet>`. Record installed packages and versions with `dpkg-query -W`.
2. Run `install-<name>.sh` with `CUPS_AIO=1` and `DRIVER_PKG_DIR` pointing to the package handoff directory.
3. Take another snapshot and use `comm -13` to find new files. Add allowlisted files from newly installed packages using `dpkg -L`.
4. Apply the path allowlist and baseline ownership guard. Exclude files owned by skipped packages or covered by archived `.deb` files. Copy the remaining entries to `/opt/cups-drivers/data/<driver>/<absolute-path>` with `cp -aT`. Write `manifest.txt` and `metadata.txt`, then run `ldconfig`.
5. At startup, `restore-drivers` reads the manifest, creates parent directories with `mkdir -p`, copies entries back with `cp -aT --remove-destination`, and runs `ldconfig`.

### Package-Level Persistence

For dpkg sources, archive the original `.deb` in `packages/` and register it in `packages.txt`. Install it with `dpkg -i` on startup and purge it by package name on removal. dpkg uses its own `.list` to determine package files, so this path does not depend on the file allowlist.

Package capture tries these sources in order and stops after a match.

| Layer | Method | Source | Limitation |
| --- | --- | --- | --- |
| A | apt's `DPkg::Pre-Install-Pkgs` hook calls `capture-debs` | Packages installed by `apt-get install` | Needs write access to `/etc/apt/apt.conf.d/`. |
| B | Vendor script copies downloaded packages to `$DRIVER_PKG_DIR` | Vendor `.deb` downloads via curl or wget | Needs one handoff line in the script. |
| C | Recover files from `/var/cache/apt/archives/` | Works offline | The script may have run `apt-get clean`. |
| D | `apt-get download <pkg>=<ver>` | Final fallback | Requires network and package indexes. |

Layer A receives the exact files apt is about to install, including transitive dependencies. It runs before any `apt-get clean` and avoids downloading the package again or guessing a version.

> `capture-debs` must always `exit 0`. A nonzero hook exit aborts the entire apt transaction. When `/run/cups-drivers/capture-target` is missing, the hook must quietly do nothing. Even if `SIGKILL` leaves a hook drop-in after `driver-install` exits, subsequent apt operations can continue.

> The vendor-script handoff is `[ -n "${DRIVER_PKG_DIR:-}" ] && cp -a "$DEB" "$DRIVER_PKG_DIR/" 2>/dev/null || true`. The guard lets image builds and manual invocations behave as before. The `|| true` preserves exit codes `0`, `3`, and other failures, and the handoff needs no new `trap`.

Keep a captured package only if it is still installed when `install-*.sh` exits and absent from `baseline-packages.txt`. This automatically removes build tools purged by an AIO script's `EXIT` trap without maintaining a dependency list. Keeping image packages out of the archive prevents a restore from downgrading system components. The package-name denylist also excludes toolchain packages when purge fails.

## Why dpkg Packages Need Package-Level Persistence

Inspection of the artifacts from all nine installer scripts found that five dpkg-sourced drivers had incomplete file snapshots. The other four drivers came from non-package sources and retained their functional files.

| Driver | Source | Result with the old file-only implementation |
| --- | --- | --- |
| `escpr2` (amd64/armhf) | Vendor `.deb` | All 298 package files were under `/opt/epson-inkjet-printer-escpr2/`, outside the allowlist. The `postinst` only made a directory symlink under `/usr/share/ppd/`, which `find -type f` skipped. The resulting empty snapshot caused `exit 1` even though the driver was active. With no manifest, the UI allowed repeated downloads. |
| `epson-cn` | Two vendor `.deb` files | On trixie, `apt-get -f install` selected renamed t64 Qt5 packages and their X11/GL dependencies, 146 new packages in a test. Shared libraries in the allowlisted multiarch directory entered the manifest. Removing the driver could delete system libraries, and restoring it could overwrite current libraries with older copies. All 26 files under `/opt/epson-inkjet-printer-201601w/` were left out. |
| `canon-ufr2` | Vendor `.deb` | Only 418 of 1,294 files reached the manifest. Missing files included `/usr/bin/cnrsdrvufr2` and `cnpdfdrv`, the rendering engines invoked by `rastertoufr2`, nine `/usr/lib/lib*ufr2*.so` libraries, 356 ICC files under `/usr/share/caepcm/`, and six halftone tables under `/usr/share/ufr2filterr/`. The queue looked normal and every job failed. |
| `gutenprint` | apt | The snapshot missed roughly 370 XML model definitions under `/usr/share/gutenprint/5.3/xml/` from `libgutenprint-common`. `lpinfo -m` listed no Gutenprint models even though the UI reported an installation. |
| `konica-bizhub` | Vendor `.deb` | Only one PPD reached the manifest. It omitted `/opt/km/`, including `lpdwrapper` and `rawtobr3`, and the `/usr/lib/cups/filter/km_BIZHUB3000MF` symlink created by `postinst`. The symlink was under an allowed directory, but `find -type f` and `dpkg -L` missed it. Jobs reported `filter failed`. |
| `canon-capt` | Source build | Complete. The installer copied the filter and PPD into allowlisted paths. |
| `hp-laserjet1020` | pyppd output | Complete. Files were in `/lib/firmware/hp/` and `/usr/share/cups/model/HP/`. |
| `sharp` | ZIP archive | Complete. Four PPDs were in `/usr/share/cups/model/Sharp/`. |
| `foo2zjs-firmware` | Firmware generation | Functionally complete. Two model alias symlinks were missing. The upstream `hplj*` script's `FWMODEL` mapping still loaded `sihpP1005/1006.dl` for P1007 and P1008. |

Package-level verification installed each driver, removed and recreated its test container, and checked the result. ESC/P-R 2 recovered the filter, shared library, 280 PPDs, and the `postinst` symlink. Canon UFR II recovered the engines, 356 ICC files, six halftone tables, 27 `.so` files, and 414 PPDs, with no missing `ldd` dependencies. Gutenprint recovered 318 XML files and listed 3,590 models. Konica restored the symlink into `/opt/km/.../lpdwrapper`. Epson restored its driver with no Qt5 packages installed.

> Validate by recreating the test container. `docker restart` preserves the writable layer, so the installed files would stay present without testing persistence.

### Keep CUPS Packages Out of Driver Snapshots

The image overlays source-built CUPS 2.4.19 from `cups-compiled.tar` onto `/usr`. The apt installation includes `cups-daemon`, `cups-client`, and `cups-filters` and omits the `cups` metapackage.

`printer-driver-gutenprint` declares `Depends: cups, cups-client, cups-filters | ghostscript-cups`. The Konica package declares `Depends: cups | cupsd`. Letting apt satisfy those dependencies adds Debian's `cups-core-drivers`, `cups-ppdc`, `cups-server-common`, and the `cups` metapackage. `cups-core-drivers` supplies `/usr/lib/cups/backend/{usb,socket,lpd,dnssd,snmp,mdns}` and several filters, which overwrite the compiled files and mix CUPS 2.4.19 and Debian 2.4.10 components.

In a Konica test, 564 CUPS files entered the driver's snapshot. `driver-remove konica-bizhub` then removed the USB and socket backends needed by every printer. Package-level restoration would also overwrite CUPS again on each boot.

The installer scripts use `dpkg -i --force-depends` to skip the metapackage dependency. The driver filters need libcups or libcupsimage, which are already present. As a safeguard, `driver-install.sh` rejects packages such as `cups`, `cups-core-drivers`, `cups-ppdc`, and `libcups*` via `PKG_NAME_DENY_PATTERNS` and explicitly removes their owned files from the manifest. The baseline guard alone cannot catch packages added at runtime.

## Baseline Package Ownership Guard

The path allowlist matches prefixes. Multiarch directories such as `/usr/lib/<triplet>` contain both driver and system libraries, so prefix matching alone cannot distinguish the Qt5 libraries pulled in by `epson-cn` from driver-owned files.

At build time, `dpkg-query -W -f '${Package}\n'` writes the installed package names to `/opt/cups-drivers/baseline-packages.txt`. The install, remove, and restore scripts exclude files owned by those packages from recording, deletion, and overwriting respectively.

- Load baseline package names into a Bash associative array for O(1) lookups. This avoids launching `grep` separately for more than 2,000 `.list` files.
- Read only the baseline packages' `/var/lib/dpkg/info/*.list` files. Strip `:arch` from `<pkg>:<arch>.list` before matching package names.
- Compute the manifest intersection once with `comm -12` and put the result in an associative array. Individual checks then avoid rescanning the entire ownership index.

In the same container, a runtime Qt5 package is outside the baseline and can be purged as a package. A recreated container starts with the baseline dpkg database. Qt5 copies from old file snapshots are unowned and can be deleted, while files owned by the image remain protected.

> Keep the guard in all three scripts so remove and restore also protect users with old, contaminated snapshots. Generate `baseline-packages.txt` after all apt installs and keep it in the image layer. The `/opt/cups-drivers/data` volume would hide a copy stored there. If the baseline file is missing, the guard warns once and falls back to the path allowlist.

## Handling Symlinks Throughout the Lifecycle

Several drivers depend on symlinks, including a Konica filter link created by `postinst`, foo2zjs model aliases, and Epson's `/usr/share/ppd/<pkg> -> /opt/.../ppds` directory link. Each stage needs to recognize symlinks.

| Location | Required behavior | Failure otherwise |
| --- | --- | --- |
| Both `find` calls in `driver-install.sh` | `\( -type f -o -type l \)` | The manifest misses symlinks. `dpkg -L` cannot recover links created by `postinst`. |
| Snapshot source check in `driver-install.sh` | `[ -e ] \|\| [ -L ]` | `-f` follows symlinks and silently skips links to directories. |
| Restore source check in `restore-drivers.sh` | `[ -e ] \|\| [ -L ]` | Directory symlinks are again skipped if the check uses `-f`. |
| Copy in `restore-drivers.sh` | `cp -aT --remove-destination` | An existing destination link to a directory would be followed and the source copied into that directory. `--remove-destination` alone was insufficient in tests. `-T` (`--no-target-directory`) forces `cp` to treat the path as the target. |
| Deletion check in `driver-remove.sh` | `[ -f ] \|\| [ -L ]` | Some symlinks count as missing and remain under `/usr/lib/cups/filter/`, where CUPS encounters the dangling links. |

With `-T`, `cp` fails if the destination is an actual directory. Manifest entries should never be directories, so this increments `error_count` and produces a warning.

## Choose the Multiarch `--libdir` for Source Builds

The source-build branch in `install-escpr2.sh` runs on architectures such as arm64 that lack a vendor binary. It used to pass only `--prefix=/usr`. `escprlib/Makefile.am` declares `lib_LTLIBRARIES = libescpr2.la`, so the library installs in `$(libdir)`. Autoconf's default `${exec_prefix}/lib` becomes plain `/usr/lib`, outside the `/usr/lib/cups`, `/usr/lib/firmware`, and multiarch allowlist. The snapshot missed `libescpr2.so.1.0.0` and the restored filter failed to start.

Pass `--libdir=<multiarch-directory>` explicitly, using the Debian shared library layout. Follow `driver-install.sh::detect_multiarch_libdir` and glob `/usr/lib/*-linux-gnu*` in the runtime. `dpkg-architecture` is provided by `dpkg-dev`, which the runtime omits. If discovery fails, keep the autoconf default and warn instead of guessing. Plain `/usr/lib` must stay outside the allowlist to protect system libraries from snapshot removal.

## Path Allowlist for Install, Remove, and Restore

The earlier unfiltered dpkg diff counted the `build-essential` toolchain installed by AIO source builds, including `gcc`, `binutils`, and `libc6-dev`. `/usr/bin/gcc`, `/usr/share/man/**`, and `/etc/**` entered the manifest. `driver-remove` used `rm -f` on every entry and deleted critical system files. `restore-drivers` could overwrite newer system files with older snapshots.

The same rules now apply in `driver-install.sh::_is_monitored_path`, `driver-remove.sh::_is_removable_path`, and `restore-drivers.sh::_is_restorable_path`.

**ALLOW** requires a match under `/usr/lib/cups`, `/usr/share/cups`, `/usr/share/ppd`, `/usr/share/foomatic`, `/lib/firmware`, `/usr/lib/firmware`, or the detected `/usr/lib/<multiarch-triplet>` for proprietary `.so` files.

**DENY** overrides the allowlist. It includes `/usr/bin/*`, `/usr/sbin/*`, `/bin/*`, `/sbin/*`, `/usr/local/bin/*`, `/usr/local/sbin/*`, `/etc/*`, `/var/*`, `/usr/include/*`, `/opt/cups-drivers/*`, `/tmp/*`, `/usr/share/{doc,man,locale,info}/*`, `/usr/share/cups/doc-root/*`, `*/pkgconfig/*`, `*.a`, `*.o`, and `*.la`. `/usr/share/cups/doc-root/*` is CUPS's built-in web interface. Executables restored by this file-level path belong to `/usr/lib/cups/filter/` and `/usr/lib/cups/backend/`. Package-level drivers restore other binary locations through dpkg.

Keep all three guards. Users of previous releases may still have `.drivers` manifests containing `/usr/bin/gcc` or other system files. Remove and restore must warn and skip those entries without calling `rm` or `cp`, then report `skipped_count`.

## One `EXIT` Trap per AIO Build Script

Bash keeps the most recently registered handler for a signal. Earlier build scripts installed two `trap ... EXIT` handlers, one to uninstall AIO build dependencies and one to delete the build directory. The second replaced the first.

- In `install-canon-capt.sh` and `install-foo2zjs-firmware.sh`, the toolchain cleanup was replaced. `build-essential` and `gcc` stayed installed and appeared in the driver diff. Before path filtering, removing the driver could remove system GCC.
- In `install-escpr2.sh`, temporary directory cleanup was replaced. Build directories of tens of megabytes remained in the container's `/tmp`.

Each script now registers one `trap _cleanup EXIT`. `_cleanup()` performs every applicable cleanup, including `rm -rf "${BUILD_DIR}"` and `apt-get purge -y --auto-remove ${BUILD_DEPS}` when `_AIO_DEPS_INSTALLED=1`. Preserve the original exit status with `local rc=$?` and `return $rc`. Use the same single-trap pattern in new source-build driver scripts.

In AIO mode, `_cleanup` may run `apt-get clean`. Keep `/var/lib/apt/lists/*` so a second driver install in the same running container still has indexes. The `if [ "${CUPS_AIO:-0}" != "1" ]` guard on index cleanup in `install-*.sh` restricts deletion to image builds.

## Installer Exit Codes

`driver-install.sh` documents the following contract for `install-*.sh`.

| Code | Meaning | `driver-install` behavior |
| --- | --- | --- |
| `0` | Installation succeeded | Continue with the diff and manifest. |
| `3` | No vendor binary for the CPU architecture | Log a Chinese explanation, call `discard_driver_data` to remove an empty data directory if present, skip the manifest, and exit 3. |
| Other nonzero code | Download, compilation, or dpkg failed | Call `discard_driver_data` and propagate the original status. |

Earlier unsupported branches returned `0` and produced an installed marker even though the driver was unavailable. The current `exit 3` branches include `install-gutenprint.sh` on armhf/armel, `install-canon-ufr2.sh` outside amd64/arm64, `install-epson-cn.sh` outside amd64, and `install-konica-bizhub.sh` outside amd64/arm64.

An exit of `0` with no new artifacts is also a failure. If the filtered diff leaves `new-files.txt` empty, `driver-install.sh` logs the error, calls `discard_driver_data`, and exits 1 without writing `manifest.txt`. This keeps the UI status consistent with the installed files.

## Architecture Detection

The runtime image ships without `dpkg-dev`. Older code ran `dpkg-architecture` and defaulted to `x86_64-linux-gnu` when it failed on ARM. The nonexistent path prevented closed-source `.so` files from entering the snapshot. `driver-list.sh` fell back to `uname -m` and compared `aarch64` against `metadata.txt`'s `arch=arm64`, reporting an architecture mismatch for every installed driver.

- Use `dpkg --print-architecture` for Debian names (`amd64`, `arm64`, `armhf`). `driver-install.sh::detect_deb_arch` uses `uname -m` only when dpkg itself is missing and only for diagnostics.
- `detect_multiarch_libdir()` uses `dpkg-architecture` when available on a build or development host. Otherwise it globs `/usr/lib/*-linux-gnu*`. If neither yields a directory, it returns an empty string and the caller skips that directory.
- `driver_registry.go::currentDebArch()` maps `GOARCH` to the same names (`amd64`→`amd64`, `arm64`→`arm64`, `arm`→`armhf`, `386`→`i386`) and returns unknown names unchanged. `DriverMeta.Arch`, `metadata.txt`'s `arch=`, and shell checks can then be compared directly. The binary is cross-compiled with `CGO_ENABLED=0`, and `GOARCH` is the runtime architecture.

## Uploading Custom Drivers

### `.ppd`

Validate that the first 256 bytes contain `*PPD-Adobe`. Write `/usr/share/cups/model/custom/<name>.ppd` and archive a copy with the equivalent path under `custom-ppd/`. Append its filename idempotently via `appendManifestLine` and write `metadata.txt`. `restore-drivers` restores it at startup.

### `.deb`

If `dpkg -i` fails, run `apt-get install -y -f --no-install-recommends` to resolve dependencies, then run `dpkg -i` again. The earlier handler returned after repairing dependencies without retrying installation.

On success, archive the original to `custom-deb/packages/` and register it in `packages.txt` with `<pkg> <version> <arch> <filename>`. Read package name, version, and architecture with `dpkg-deb -f`. If metadata is unavailable, use the filename as the fallback package name. At startup, `restore-drivers` uses the package path and `dpkg -i`. It skips a package already installed at the same or a newer version.

The upload leaves `manifest.txt` unwritten. A file manifest would copy the `.deb` to an absolute path without running its maintainer scripts. Package restoration installs the archive and runs those scripts.

Previously the upload archived the file without registering it, and users had to upload it again after each restart. `restore-drivers` now discovers existing unregistered files via `packages/*.deb`, so no manual migration is required. An invalid package can be retried on every boot. Remove the file from `./.drivers/custom-deb/packages/` or remove its entry from `packages.txt` to stop the retries. The API exposes this through `customDebNotice`.

### Administrator Trust Boundary

Uploading a `.deb` lets its maintainer scripts, including `preinst` and `postinst`, execute as root inside the container. The endpoint requires `RequireSession`, `RequireAdmin`, and `ValidateCSRF` and logs the uploader's username for audit. Protect administrator credentials like container root credentials.

### Filenames and Request Size

- `safeUploadFilename` strips Windows backslash path prefixes, applies `filepath.Base`, and rejects `.`, `..`, hidden filenames, and separators. It applies regardless of any normalization performed by the standard library's multipart parser.
- `r.ParseMultipartForm(n)` uses `n` as the **memory threshold**, and Go spools larger data to temporary files. The old `ParseMultipartForm(50 << 20)` with `// 50 MB limit` did not cap upload size. Use `r.Body = http.MaxBytesReader(w, r.Body, driverUploadMaxBytes)` for a hard request limit. Keep `maxMemory` lower, 8 MB for this endpoint, so large valid packages can spool to disk.

> Existing `print_handlers.go`, `convert_handler.go`, `estimate_handler.go`, and `compose_handler.go` calls to `ParseMultipartForm(512 << 20)` allow up to 512 MB in memory and do not set a hard request-body limit. A future change should use `MaxBytesReader` and a smaller `maxMemory` for those handlers too.

## `lpinfo` Discovery Format and Model Priority

`GET /api/admin/drivers/detect` uses `lpinfo -l -v`. The older implementation ran short-format `lpinfo -v`, which contains only `<class> <uri>`, and looked for a quoted make-and-model. Network printer models therefore parsed as empty. `checkHasDriver(" ")` always matched through `strings.Contains(desc, " ")`, and `findBestPPD("")` selected the first unrelated PPD from `lpinfo -m`.

`parseLpinfoDevices` and `buildDetectedPrinter` use these rules.

- Start each block at `Device:`. Split subsequent lines at their first `=` and ignore unrecognized keys. Field order or indentation changes should not break the parser.
- Drop backend-only records such as `network socket`, `direct hp`, `ipp`, `lpd`, `beh`, or `dnssd`. They lack a complete URI containing `://` and would otherwise add five or six false devices. Skip virtual devices including `cups-pdf`, `cups-brf`, and `file:///dev/null`.
- Prefer `make-and-model`, then `device-id` fields `MFG`/`MDL` or `MANUFACTURER`/`MODEL`, then `info`, then the URI path for `usb://vendor/model`. Remove a duplicate manufacturer prefix. `splitMakeAndModel` treats CUPS's `Unknown` as empty and uses a single token as the model.
- `ppd_match.go::ScorePPDCandidates` stops non-generic matching when `len(compact) < 2`, including one-character parse remnants. Omitting `-m` from `lpadmin` creates a raw queue without a PPD. Older comments described this as driverless or IPP Everywhere. A raw queue lacks PPD options, so `/api/printer-info` returns an empty `mediaSourceSupported` list and the paper tray selector disappears. With no match, explicitly use `-m everywhere` when supported or return an error.
- `ppd_match.go` implements pure matching and `ppd_query.go` wraps its external calls. `lpinfo -m` uses a 10-minute cache, explicitly invalidated after driver installation or removal.

## One-Click Setup (`/drivers/setup`)

The request fields follow `/detect`: `{deviceUri, driverName?, manufacturer?, model?, deviceId?, ppdUri?, printerName?, allowRaw?}`.

If `manifest.txt` is missing, run `driver-install`. Determine the manufacturer and model from `req.Manufacturer/Model`, `parseDeviceID(deviceId)`, or `parseDeviceURI(uri)` in that order. Choose `-m` by preferring explicit `ppdUri`, then `everywhere`, then the best automatically scored PPD, otherwise report an error. Use `uniquePrinterName` to avoid an existing queue name. Run `lpadmin -p <name> -E -v <uri> [-m <ppd>]`, set the default paper to A4, and verify with `lpstat -p` and `lpoptions -l`. If the PPD fails to take effect, use `lpadmin -x` to roll back a new (`isNew`) queue.

## Asynchronous Jobs (`driver_handlers.go`)

Keep `install`, `remove`, and `setup` asynchronous. `main.go` gives `http.Server` a global `WriteTimeout = 120s`, but source-build drivers such as `canon-capt`, `foo2zjs-firmware`, and arm64 `escpr2` can spend several minutes installing `build-essential` and running `make`. The synchronous `exec.CommandContext(r.Context(), ...)` approach killed `make` when a request timed out and canceled its context, leaving partial builds and dependencies that the exit trap could not clean up.

The handler now immediately returns `202` with a `jobId`. It runs the command in a background goroutine derived from `context.Background()`, with `driverJobTimeout = 30 * time.Minute`. The frontend polls `jobs/{id}` for incremental logs. A locked `safeBuffer` holds stdout and stderr so users see build output while the job runs.

`startDriverJob` checks the job table under a lock. It allows one driver operation at a time because apt and dpkg use global locks. If another job is active, the endpoint returns `409` with the active `jobId` so clients can poll it.

```json
{ "error": "已有驱动任务正在执行，请等待其完成后重试", "jobId": "…" }
```

The error means a driver job is already running. A synchronous `.deb` upload also checks `runningDriverJobID()` and returns `409` when it is nonempty, avoiding contention for dpkg's lock.

`driverJobRetention = time.Hour`. `pruneDriverJobsLocked` removes jobs that finished more than an hour ago when a new job starts. Jobs are in memory and disappear when the process restarts. The frontend's timeout messages account for this. `randomToken()` produces an opaque uppercase base32 `jobId`, and the route accepts `{id:[A-Za-z0-9]+}`.
