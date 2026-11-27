# Docker Builds and Deployment

> This document expands the deployment section of [AGENTS.en.md](../../AGENTS.en.md). It covers build targets and shared dependencies, base image selection for three architectures, key `Dockerfile` decisions, and CI/CD. See the agent guide for the Compose reference table.

## Multi-Stage Docker Build

The root `Dockerfile` defines two runtime targets for `linux/amd64`, `linux/arm64`, and `linux/arm/v7`. The default `runtime` target combines CUPS and the Web app in one image. `web-runtime` runs cups-web with an external CUPS server. The build has seven stages.

1. **`frontend-build`** uses `node:20-slim` with `npm ci` and `npm run build` to produce the Vite distribution.
2. **`java-builder`** uses `--platform=$BUILDPLATFORM debian:trixie-slim`, apt's `openjdk-21-jdk-headless`, and an Apache Maven tarball to build `ofd-converter.jar`.
3. **`builder`** uses `golang:1.26` to run `go build` with `CGO_ENABLED=0` and `-ldflags -X main.Version=$VERSION`.
4. **`cups-builder`** runs `scripts/build/install-cups.sh` on `debian:trixie-slim` to build OpenPrinting CUPS from source (`CUPS_VERSION=2.4.19`, `--prefix=/usr`, and `--libdir=/usr/lib/<multiarch>`). It packs the results at `/tmp/cups-compiled.tar`.
5. **`document-runtime`** uses `debian:trixie-slim` to supply LibreOffice, `openjdk-21-jre`, Ghostscript, fonts, fontconfig, and cidfmap to both runtime targets. It also provides writable locations for HOME and the LibreOffice profile.
6. **`web-runtime`** inherits `document-runtime`, copies the Go binary and OFD converter, sets `ENTRYPOINT ["/cups-web"]`, and exposes only 8080. It omits the CUPS service, print drivers, SANE scanning stack, and driver management scripts.
7. **`runtime`**, the final stage, inherits `document-runtime`, installs the CUPS ecosystem (`cups-filters`, `cups-daemon`, `printer-driver-*`, `hplip`, `avahi-daemon`, `ipp-usb`, and others) and the scanning stack. It overlays the source-built CUPS files on apt's version and runs the AIO `entrypoint.sh`.

The AIO image runs as root. Its container runs `cupsd`, `lpadmin`, and `dpkg` to install drivers at runtime and writes driver files under `/usr/lib/cups`, `/usr/share/ppd`, and `/lib/firmware`. `docker-compose.yml` explicitly sets `user: root`.

### Web-Only Image

`make docker-build-web` is equivalent to `docker build --target web-runtime --build-arg VERSION=... -t cups-web:web .`. BuildKit runs only the stages the selected target needs, so `web-runtime` skips `cups-builder`. LibreOffice may bring in the `libcups` shared library for document conversion. The image still runs without the `cupsd` service.

`docker-compose.web.yml` uses a bridge network, maps host port 1180 to container port 8080, and persists `/data` and `/uploads`. For TCP, configure the external `CUPS_HOST`. For a Unix socket, set `CUPS_SOCKET_PATH` and share the CUPS runtime directory. A nonempty socket configuration takes precedence, and a connection failure stops the request without trying TCP. In combination with `docker-compose.web.socket.yml`, the default read-only mount comes from `/var/run/cups` on the host. `CUPS_SOCKET_DIR` overrides the host directory. The external CUPS server handles queues, drivers, USB access, and AirPrint advertising. The Web container ships without those management commands or local scanning tools. [Shared CUPS runtime volume example](../../README.en.md#connect-through-a-unix-socket)

The Web-only target sets `SCAN_DEVICES_CACHE_TTL=0` to skip scanner cache prewarming. The OFD converter's Maven Shade configuration excludes dependency JAR signatures (`META-INF/*.SF`, `*.DSA`, `*.RSA`, and `*.EC`). Merging JARs and rewriting the manifest invalidates the original signatures. If present, Java rejects the merged JAR with `Invalid signature file digest for Manifest main attributes`. Both images use the corrected converter.

### Why Debian CUPS Packages Are Overlaid with a Source Build

`cups-filters` pulls in Debian's `cups` packages as dependencies. The packages establish the `lp` and `lpadmin` groups, the `/etc/cups` configuration skeleton, and systemd units. OpenPrinting's `make install` under the same `--prefix=/usr` overlays `cupsd`, `libcups.so.2`, and client binaries. This keeps Debian's integration files and uses the selected upstream OpenPrinting version. The `libcups2` ABI keeps `cups-filters` and the `printer-driver-*` packages compatible. The runtime runs `tar xf /tmp/cups-compiled.tar -C / && ldconfig`. The archive file list discovers `libcups*` with `dpkg-architecture -qDEB_HOST_MULTIARCH`. `dpkg-dev` is available in the build stage only. [Driver architecture detection](driver-management.md#architecture-detection)

### CA Certificates in `cups-builder`

`scripts/build/install-cups.sh` downloads the CUPS tarball from GitHub Releases using `wget`. Without CA certificates, TLS verification fails with exit code 5. Because the script uses `set -euo pipefail`, CI stops with `install-cups.sh ... exit code: 5`. `debian:trixie-slim` does not come with `ca-certificates`. The old single-stage `cups/Dockerfile` inherited the package from runtime dependencies. In the separate `cups-builder` stage, declare the package explicitly.

### HOME and the LibreOffice Profile

The runtime sets `ENV HOME=/root`, `XDG_CACHE_HOME`, and `DCONF_USER_CONFIG_DIR` and creates the directories in advance.

1. Headless LibreOffice needs a writable HOME to create its user profile. Without it, `--convert-to pdf` can exit silently with status 0 and produce no PDF.
2. Docker implicitly assigns HOME only for a `USER` found in `/etc/passwd`. If a deployment chooses an arbitrary UID through Kubernetes `securityContext.runAsUser` or `docker run -u`, HOME can resolve to `/` and conversions fail. Setting `HOME` explicitly makes the path predictable and simplifies diagnosis.

## Base Images for Three Architectures

The original `frontend-build` used `oven/bun` and `java-builder` used `maven:3.9-eclipse-temurin-17`. These images lacked 32-bit ARM support. The design still applies after the move from bookworm to trixie and from JDK 17 to 21.

### Frontend Build with `node:20-slim`

Upstream Bun supports arm64 and x64 and closed 32-bit ARM support as not planned. `node:20-slim` includes `amd64`, `arm32v7`, and `arm64v8` manifests. Use `npm ci` and `npm run build` in place of `bun install` and `bun run build`. The frontend's `package.json` scripts use standard Vite and Node commands, so the application code needed no changes. [Bun issue #5060](https://github.com/oven-sh/bun/issues/5060)

Maintain `frontend/package-lock.json` alongside `bun.lock`. `npm ci` requires the lockfile and `package.json` to agree. After changing dependencies with `bun add` or `bun remove`, run `npm install`, update `package-lock.json`, and commit it with the dependency change.

### Java Build with Debian, OpenJDK, and the Maven Tarball

Eclipse Temurin provides Linux ARM 32-bit hard-float binaries for JDK 8 and 11. Its platform support table excludes JDK 17, 21, and 25. Official Maven images also lack an armhf manifest. [Adoptium supported platforms](https://adoptium.net/supported-platforms)

The current `FROM --platform=$BUILDPLATFORM debian:trixie-slim AS java-builder` runs on the build host's architecture, which is amd64 in GitHub Actions. It installs `openjdk-21-jdk-headless` and uses the official Apache Maven tarball (`MAVEN_VERSION=3.9.9`). `ofd-converter.jar` contains Java bytecode (`maven.compiler.source=1.8`). Each runtime architecture can use it with its own JRE via `COPY --from=java-builder`.

**Pin the Java stage to `BUILDPLATFORM`.** Modern OpenJDK was unreliable under QEMU user-mode armhf emulation. Both Debian Maven and the Apache tarball intermittently threw `java.lang.ClassNotFoundException: org.apache.maven.cli.MavenCli`. The traces matched except for the classworlds line (`SelfFirstStrategy.java:50` for the Debian package and `:42` for the tarball). The evidence points to JVM class loader or JIT stability under QEMU. Adoptium's lack of ARM 32-bit binaries for JDK 17 and newer illustrates the same platform limitation. Building the architecture-neutral JAR on amd64 bypasses emulation and follows Docker's multi-architecture Java pattern.

**Keep `# syntax=docker/dockerfile:1`.** BuildKit injects `BUILDPLATFORM`. Older buildx environments could interpret it as empty without the syntax declaration. `--platform=$BUILDPLATFORM` would then use the target architecture, putting the Java stage back under QEMU.

**Leave the default platform for `runtime` and `cups-builder`.** `runtime` must install the target architecture's LibreOffice, JRE, Chinese fonts, and print drivers. `cups-builder` produces native architecture-specific executables. Both need separate builds for each `TARGETPLATFORM`. An amd64 pin would ship amd64 layers to arm64 and armhf installations and force emulation of the runtime.

**Use the Maven tarball.** Installing Debian Maven on amd64 avoids the QEMU issue, but the package uses dpkg triggers and `update-alternatives` to update symlinks. That integration has had compatibility issues after base image changes or OS upgrades. The official tarball is self-contained under `lib/`. The download tries `dlcdn.apache.org` for the current release and falls back to the permanent `archive.apache.org` archive. Update `MAVEN_VERSION` in `Dockerfile` to change the version. [docker-maven issue #213](https://github.com/carlossg/docker-maven/issues/213)

## Compose Configuration

`docker-compose.yml` now has a single `cups` service using `hanxi/cups-web:latest`. It replaced the two-service `cups` and `web` configuration. In `network_mode: host`, CUPS uses host port 631 directly and `LISTEN_ADDR` controls the Web port. Compose defaults to `:1180`, matching the host side of the earlier bridge mapping `1180:8080`.

| Configuration | Purpose |
| --- | --- |
| `network_mode: host` and `hostname: CUPS` | mDNS/DNS-SD needs LAN multicast on UDP 5353. A bridge network prevents discovery of LAN printers via `dnssd://…._ipp._tcp.local` and AirPrint discovery by phones. The container's own Avahi can use the LAN with host networking. Port mappings are unavailable in this mode, and a host Avahi can contend for 5353 (issue #107). |
| `LISTEN_ADDR=:1180` | Listen directly on host port 1180 so existing bookmarks and reverse proxies keep working. `cmd/server/main.go` supports the environment variable. |
| `user: root` | Run cupsd, lpadmin, and dpkg and write drivers to system paths. |
| `security_opt: [apparmor:unconfined]` | Avoids printing failures in environments such as PVE LXC where AppArmor logs `apparmor="DENIED" … comm="jobs.cgi"` (issue #91). The setting also covers LibreOffice and OFD conversion subprocesses in the combined container. |
| `./.etc:/etc/cups`, `./.data:/data`, `./.uploads:/uploads` | Persist CUPS configuration, the database, and uploaded files. |
| `./.drivers:/opt/cups-drivers/data` | Persist driver snapshots. Removing the volume loses manually installed third-party drivers on restart. Reinstall them in the Web **Drivers** page. [Driver management](driver-management.md) |
| `/dev/bus/usb:/dev/bus/usb` and `device_cgroup_rules: ['c 189:* rmw']` | Enable USB printer hotplug (issue #81). `devices:` binds existing nodes only at startup. A directory bind mount also exposes new nodes that host udev creates for a printer powered on later. |
| `/run/udev:/run/udev:ro` | Lets libusb read device attributes to improve recognition. The mount is optional when the host lacks `/run/udev`. |

> The `/run/dbus/system_bus_socket` mount was removed in issue #107, replacing the host Avahi approach from issue #94. On a host without Avahi, the mount could occupy the container socket path and prevent the container's `dbus-daemon --system --fork` from starting. Earlier code silenced the failure with `|| true`, and Avahi stopped working. With host networking, the container's D-Bus and Avahi handle discovery and advertising directly.

## CI/CD

- **`build-release.yml`** cross-compiles binaries for seven platforms when any branch or tag receives a push (`linux/amd64`, `linux/arm64`, `linux/armv7`, `linux/loong64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`). A `v*` tag creates a stable release. A push to `master` updates the rolling prerelease under the `dev` tag, avoiding a branch/tag name collision. Keep the `setup-go` version in sync with `go.mod`, currently `1.26`.
- **`docker-publish.yml`** builds both runtime targets for three architectures in the same job on pushes to `master` or a `v*` tag. AIO keeps `vX.Y.Z` and `latest` or `dev`. Web-only images use `vX.Y.Z-web` and `latest-web` or `dev-web`. Both receive the same `VERSION` build argument without an image suffix. The AIO cache scope is `cups-web`. Web-only builds read that shared cache and save separately to `cups-web-web` so they do not replace AIO-only stages. Keep the **Free disk space** step that removes the dotnet, Android, and CodeQL caches.

`linux/armv7` uses `GOARCH=arm` and `GOARM=7` for 32-bit hard-float systems such as Raspberry Pi 2/3 and other ARM SBCs. The matrix's `goarm` field is passed through the build environment and stays empty on non-ARM targets. `linux/loong64` needs `modernc.org/sqlite` v1.34 or newer. v1.29.0 predates loong64 support. With `CGO_ENABLED=0` across the repository, architectures already supported by modernc, such as `riscv64`, `s390x`, and `ppc64le`, only need another `goos/goarch/suffix` entry in the release matrix.

## Release Versions

Create a tag with `bump-version.sh`.

```bash
./bump-version.sh patch    # Default
./bump-version.sh minor
./bump-version.sh major
```
