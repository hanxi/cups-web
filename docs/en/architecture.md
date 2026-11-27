# Architecture and Technology Stack

> This guide supplements [AGENTS.en.md](../../AGENTS.en.md) with the reasons behind the technology choices and known issues with external dependencies. The developer guide contains the quick-reference tables.

## Backend Stack

| Component | Version or purpose |
| --- | --- |
| Go | 1.26 (see `go.mod`) |
| HTTP routing | `github.com/gorilla/mux` |
| Sessions | `github.com/gorilla/securecookie` |
| Database | `modernc.org/sqlite` (pure Go, no CGO) |
| Print protocol | `github.com/OpenPrinting/goipp` (IPP) |
| PDF handling | `rsc.io/pdf` reads page counts. `github.com/phpdave11/gofpdf` creates PDFs. |
| Image resizing | `golang.org/x/image/draw` downscales large images with CatmullRom. |
| Password hashing | `golang.org/x/crypto/bcrypt` |

## Frontend Stack

| Component | Version or purpose |
| --- | --- |
| Framework | Vue 3.5 and Vue Router in hash mode |
| Build tooling | Vite 7 |
| UI library | `@nuxt/ui` v4 with its Tailwind theme |
| Styling | Tailwind CSS v4 |
| Icons | `@iconify-json/lucide` |
| PDF handling | `pdfjs-dist` renders previews. The backend creates PDFs through `/api/convert`. |
| HEIC support | `heic2any` |
| Package management | Bun is recommended for local development (`bun install`, `bun run dev`). CI and Docker use `npm ci` and `npm run build` to support `linux/arm/v7`, since Bun has no 32-bit ARM build. [Docker build](docker-build.md) |

## External Dependencies and Known Issues

### CUPS

CUPS provides the print service over IPP. In the AIO image, `scripts/build/install-cups.sh` builds the OpenPrinting source (`CUPS_VERSION=2.4.19`) and overlays the apt-installed version.

### LibreOffice (Headless)

LibreOffice converts Office documents to PDF and serves as the fallback for PDF normalization.

**Writable `HOME` is required.** The Dockerfile sets `ENV HOME=/root` and creates `~/.config/libreoffice`. With an unwritable home directory, `--convert-to pdf` can exit successfully without producing a PDF.

**Use a UTF-8 locale that exists in the image.** LibreOffice, the OFD converter, and Ghostscript run with `LANG=C.UTF-8` and `LC_ALL=C.UTF-8`, both available in Debian slim. Forcing a locale such as `zh_CN.UTF-8` when it has not been generated can cause LibreOffice to report `Error: source file could not be loaded` for a DOCX with a Chinese filename while still exiting with code 0. After conversion, the backend verifies that the result is a readable, nonempty regular file. It retains command output when the result is missing or invalid. The locale applies only to conversion subprocesses. [Issue #125](https://github.com/hanxi/cups-web/issues/125)

### Java 21 and `ofd-converter.jar`

The converter uses `ofdrw` to produce PDFs from OFD documents. The build uses `openjdk-21-jdk-headless` and the runtime uses `openjdk-21-jre`.

### Ghostscript (`gs`)

Ghostscript is the first option for PDF normalization. It converts documents to PDF 1.4 for older CUPS drivers or printers that have trouble parsing newer PDF versions.

**`gs pdfwrite` rebuilds each font dictionary and adds a six-letter subset prefix such as `CCGWER+`.** This can alter PDFs with placeholder Type 0 fonts and an external `UniGB-UCS2-H` CMap, a common pattern in Acrobat exports of Chinese admission tickets and standardized forms. For example, it can change the original `/BaseFont /#ba#da#cc#e5` (the GBK-escaped SimSun name) to `/BaseFont /BPCXJX+#cb#ce#cc#e5`. Renderers such as pdf.js can then treat the font as embedded, fail to find the actual `FontFile`, and render mismatched glyph metrics and widths. The preview may briefly render correctly before characters shift or overlap. Use normalization when the CUPS driver cannot parse the original font dictionary. [PDF pipeline](pdf-pipeline.md)

Install Ghostscript locally on macOS with `brew install ghostscript`. The Docker image has three levels of Chinese font fallback, documented in the Dockerfile.

1. `docker-fonts/cidfmap.local` maps eight GBK-byte BaseFont entries, covering regular and bold Song, Hei, Kai, and FangSong, to the TrueType families `arphic-uming`, `arphic-ukai`, and `wqy-zenhei`. If the user supplies Windows fonts such as `simsun.ttf`, build-time `sed` substitutes those fonts. The build copies the mapping to `/etc/ghostscript/cidfmap.local` and the Ghostscript `Resource/Init/cidfmap`, which Ghostscript loads on startup. [PDF pipeline](pdf-pipeline.md)
2. `fonts-droid-fallback` supplies an Adobe-GB1 CID fallback when the mapping has no match. Debian packages the Ghostscript dependency `DroidSansFallback.ttf` separately.
3. Unicode fonts such as `fonts-noto-cjk` support LibreOffice document rendering. The CIDFSubst path uses the fonts above.

Ghostscript 10.x can have problems with CIDFont face indexes in CFF-based OpenType Collections. The arphic and WQY fonts use TrueType outlines and avoid that issue.

### `dpkg` and `apt-get`

The runtime uses these commands to install third-party printer drivers. Since the runtime image omits `dpkg-dev`, use `dpkg --print-architecture` to detect the architecture. [Driver management](driver-management.md)
