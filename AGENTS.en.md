# CUPS Web Developer Guide

[简体中文](AGENTS.md) · English

This guide covers the project architecture, API, development workflow, and extension points. For user instructions, see [README.en.md](README.en.md).

> 📚 **In-depth documentation** lives in [docs/en/](docs/en/README.md). This guide keeps the rules and contracts easy to scan.

## Keeping Chinese and English documentation in sync

- When adding or updating documentation, maintain both versions in the same change. Pair `README.md` with `README.en.md`, `AGENTS.md` with `AGENTS.en.md`, and `docs/<path>.md` with `docs/en/<path>.md`. Preserve the relative filename and directory structure in the English tree.
- Synchronize feature descriptions, examples, links, screenshot references, and navigation. When a heading changes, check section links in both languages. Keep code, commands, paths, and configuration names consistent.
- Keep both languages current for the website home page, documentation navigation, and language switcher. Verify with `npm ci --prefix website` and `npm run build --prefix website`. After merging to `master`, the [GitHub Pages workflow](.github/workflows/pages.yml) rebuilds and publishes the site.

## 📦 Project overview

- **Purpose**: A web print management tool built on CUPS, with a separate frontend and backend.
- **Stack**: Go 1.26 backend, Vue 3 frontend, SQLite storage, and IPP printing.
- **Deployment**: A single binary embeds the frontend via `go:embed` and connects to external CUPS. The **all-in-one (AIO) Docker image** runs `cupsd` and `cups-web` together with LibreOffice, Java 21, the OFD converter, Ghostscript, and printing drivers.
- **Web-only image**: The `web-runtime` target in the same `Dockerfile` retains document conversion dependencies and connects to external CUPS via `CUPS_HOST` or `CUPS_SOCKET_PATH`. These tags end in `-web` and use `docker-compose.web.yml`. This target ships without cupsd, printer drivers, or scanning utilities.

> ⚠️ The repository previously used separate CUPS and cups-web containers in the `cups/` directory. The unified deployment uses the root `Dockerfile` and `entrypoint.sh`, build scripts in `scripts/build/`, and driver scripts in `scripts/driver/`.

## 🛠️ Technology stack

### Backend

| Component | Purpose |
| --- | --- |
| Go 1.26 | See `go.mod` |
| `gorilla/mux` | HTTP routing |
| `gorilla/securecookie` | Session management |
| `modernc.org/sqlite` | Pure Go SQLite without CGO |
| `OpenPrinting/goipp` | IPP protocol |
| `rsc.io/pdf` + `phpdave11/gofpdf` | PDF parsing and generation |
| `golang.org/x/image/draw` | Catmull–Rom downsampling of large images |
| `golang.org/x/crypto/bcrypt` | Password hashing |

### Frontend

| Component | Purpose |
| --- | --- |
| Vue 3.5 + Vue Router | Hash-based routing |
| Vite 7 | Build |
| `@nuxt/ui` v4 + Tailwind CSS v4 | UI and styles |
| `pdfjs-dist` | PDF preview. The backend `/api/convert` endpoint generates PDFs. |
| Bun (local) / npm (CI and Docker) | Package management. npm covers `linux/arm/v7` because Bun lacks 32-bit ARM support. |

### External dependencies

CUPS provides IPP communication, LibreOffice converts Office documents to PDFs, Java 21 and `ofd-converter.jar` convert OFD documents to PDFs, Ghostscript normalizes PDFs, and `dpkg`/`apt-get` install drivers at runtime.

> See [architecture.md](docs/en/architecture.md) for details on LibreOffice's writable home directory, Ghostscript font changes, and the runtime image's lack of `dpkg-dev`.

## 📁 Project layout

```text
cups-web/
├── cmd/server/                    # Backend executable
│   ├── main.go                    # Entry point and route registration
│   ├── app.go                     # Globals
│   ├── bootstrap.go               # Default admin initialization
│   ├── auth_handlers.go           # Login, logout, session, and CSRF
│   ├── login_limiter.go           # Failed login rate limiting
│   ├── admin_handlers.go          # Administrator users, settings, and cleanup
│   ├── user_handlers.go           # /api/me
│   ├── print_handlers.go          # /api/print (primary print entry point)
│   ├── print_records_handlers.go  # Print record listing, downloads, and reprints
│   ├── printer_info_handler.go    # Printer attributes
│   ├── convert_handler.go         # /api/convert
│   ├── convert_utils.go           # LibreOffice and OFD conversion helpers
│   ├── compose_handler.go         # /api/compose (multi-page layout)
│   ├── estimate_handler.go        # /api/estimate (page count estimate)
│   ├── preview_handler.go         # /api/preview-page (PDF page to PNG fallback)
│   ├── driver_handlers.go         # /api/admin/drivers/* and background jobs
│   ├── driver_registry.go         # Driver registry
│   ├── file_utils.go              # File storage, type detection, and page count
│   ├── pdf_utils.go               # Image and text to PDF conversion
│   ├── pdf_compose.go             # Multi-page layout
│   ├── pdf_reorder.go             # Page reordering and tests
│   ├── watermark.go               # Watermarks
│   ├── pdf_normalize.go           # PDF normalization pipeline
│   ├── fonts.go                   # Chinese font loading
│   ├── font_handlers.go           # /api/admin/fonts/* (list, upload, delete, and refresh)
│   ├── font_parser.go             # SFNT (TTF/OTF/TTC) name-table parsing
│   ├── font_preflight.go          # Missing font detection and print blocking
│   ├── maintenance.go             # Background maintenance
│   └── version.go                 # Build-time version
├── internal/
│   ├── auth/session.go            # securecookie sessions and CSRF
│   ├── middleware/                # CSRF and security middleware
│   ├── ipp/                       # IPP client and URI validation
│   ├── server/static.go           # Embedded assets and SPA fallback
│   └── store/                     # Data layer (users, prints, and settings)
├── frontend/                      # Vue 3 frontend (go:embed dist)
├── ofd-converter/                 # Java OFD to PDF converter
├── scripts/
│   ├── build/install-cups.sh       # Build CUPS from source
│   └── driver/                    # Driver commands, installers, and capture-debs.sh apt hook
├── docker-fonts/                  # Build-time fonts and Ghostscript/fontconfig configuration
├── entrypoint.sh                  # AIO container startup
├── Dockerfile                     # Shared converter dependencies and AIO/Web-only targets
├── docker-compose.yml             # Single AIO service
├── docker-compose.web.yml         # Web-only service with external CUPS
└── Makefile                       # Build targets
```

## 🔌 HTTP API

All routes have the `/api` prefix. Protected routes use `RequireSession` and `ValidateCSRF`. Administrator routes also use `RequireAdmin`. The public routes are listed below.

> **CSRF contract**: After successful login, the server issues a non-HttpOnly `csrf_token` cookie. The frontend supplies the `X-CSRF-Token` header on non-GET requests.

> **API key authentication (issue #113)**: The `protected` and `admin` subrouters run `middleware.APIKeyAuth` before `RequireSession`. The middleware looks up the SHA-256 hex hash of `Authorization: Bearer cw_...` or `X-API-Key: cw_...` in `api_keys.token_hash`, then inserts the owner's `auth.Session` into the request context. `RequireSession` and `RequireAdmin` apply the usual checks. `ValidateCSRF` uses `auth.IsAPIKeyAuth` to exempt this authentication method from double-submit validation. The reserved guest account cannot issue or use keys. The handlers for `POST /api/api-keys` and `DELETE /api/api-keys/{id}` require a browser session to prevent keys issuing more keys.

### Public endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/api/login` | Log in |
| POST | `/api/logout` | Log out |
| GET | `/api/csrf` | Refresh CSRF token |
| GET | `/api/session` | Query session |
| GET | `/api/version` | Build version |

### Authenticated endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/me` | Current user information |
| GET | `/api/printers` | List printers |
| GET | `/api/printer-info?uri=<uri>` | Printer attributes |
| GET | `/api/printer-options?uri=<uri>&force=1` | Driver paper, media, and quality options. `force` is optional. |
| POST | `/api/estimate` | Estimate page count |
| POST | `/api/convert` | Convert a document to PDF (single `file` or multiple image `files`) |
| POST | `/api/preview-page` | Render a PDF page to PNG (`file` and `page`, response header `X-Page-Count`) as a browser pdf.js fallback |
| POST | `/api/print` | Submit a print job |
| POST | `/api/compose` | Compose a multi-page layout |
| GET/POST | `/api/scheduled-prints` | List or create scheduled jobs |
| PUT/DELETE | `/api/scheduled-prints/{id}` | Update or delete a scheduled job |
| POST | `/api/scheduled-prints/{id}/run` | Run a job early and asynchronously. Returns `202 {ok, status:"running"}`. |
| GET | `/api/scheduled-prints/{id}/files/{index}` | Photo queue thumbnail for the owner or an administrator |
| GET | `/api/scheduled-prints/bing/preview` | Today's Bing China wallpaper metadata and a same-origin image URL |
| GET | `/api/scheduled-prints/bing/image/{imageID}` | Preview URL for today's downloadable Bing JPEG, validated before serving |
| GET | `/api/print-records` | Print records |
| GET | `/api/print-records/{id}/file` | Download the original file |
| POST | `/api/print-records/{id}/reprint` | Prefill reprint parameters |
| GET | `/api/scan/devices` | List scanners (`scanimage -L`) |
| GET | `/api/scan/options?device=<name>` | List device options using `scanimage -A`. The MVP covers mode, resolution, and source. |
| POST | `/api/scan/jobs` | Submit an asynchronous scan. Returns `202 {jobId, recordId, filename}`. |
| GET | `/api/scan/jobs/{id}` | Poll scan status and incremental logs |
| DELETE | `/api/scan/jobs/{id}` | Cancel a running scan |
| GET | `/api/scan/records` | Scan records. Administrators can filter with `?username=`. |
| GET | `/api/scan/records/{id}/file` | Download a scan |
| DELETE | `/api/scan/records/{id}` | Delete scan record and file |
| GET | `/api/api-keys` | List the caller's API keys without plaintext (issue #113) |
| POST | `/api/api-keys` | Create a key and reveal plaintext once. Browser session only. |
| DELETE | `/api/api-keys/{id}` | Delete one of the caller's keys. Browser session only. |
| GET | `/api/print-presets` | List the caller's presets (issue #128). Guests receive 403. |
| POST | `/api/print-presets` | Create `{name, settings}`. Duplicate names return 409. Limit 20 per user. |
| PUT/DELETE | `/api/print-presets/{id}` | Owner-only rename, overwrite of optional fields, or deletion |

#### Asynchronous scan jobs (issue #111)

- Use the `scanimage` subprocess. HPLIP's `hp-scan` depends on the container's HPLIP daemon and returned `SANE: Error during device I/O (code=9)` even with `dbus-daemon --system --fork`. The standard Debian SANE stack (`sane-utils` + `libsane-hpaio`) worked on the same host and hardware.
- Scans can run concurrently. `scanimage` has no global apt/dpkg lock. The SANE backend manages exclusive access to each scanner.
- The hard timeout is five minutes (`scanJobTimeout`). Derive the context from `context.Background()` so the HTTP server's `WriteTimeout=120s` cannot terminate the subprocess.
- For PDF output, first write a temporary PNG with `scanimage`, then convert it using `gs -sDEVICE=pdfwrite -dSAFER -dNOPAUSE -dBATCH` and clean up the PNG. This path works with backends that lack `--format=pdf`.
- The in-memory job table retains entries for an hour. Expiry removes the memory entry only. User-visible data lives in `scan_records` and the stored file.
- `SCAN_DIR` controls storage and defaults to `scans/`. The AIO Compose file maps `./.scans` to `/scans`. Downloads and deletions use `os.OpenInRoot(scanDir, ...)` to guard against path traversal.
- Validate device names against `scanimage -L`, round-trip resolution via `strconv.Atoi`, and allowlist mode and format (Color/Gray/Lineart and png/jpeg/pdf). Pass separate arguments to `exec.CommandContext` without invoking a shell.
- **The eSCL/AirScan backend needs an explicit scan region** (issue #112). Without `--tl-x/--tl-y/--br-x/--br-y`, br-x/br-y round to zero and `sane_start` returns `Invalid argument`. `runScanimage` first probes with `scanimage -A`. When br-x/br-y ranges exist, pass a top-left of 0 and a bottom-right at the reported maxima. Older backends such as hpaio without these ranges keep their existing behavior. `isNumericScanValue` screens values parsed from `-A` before argument construction.
- **Device discovery cache and preload** (issue #111). `scanimage -L` takes about 17 seconds because it queries multiple SANE backends. `scanDeviceCacheGet` caches results in process for 60 seconds by default. Override via `SCAN_DEVICES_CACHE_TTL` (`<=0` disables the cache). `GET /api/scan/devices?force=1`, used by Refresh Devices, bypasses the cache. Probe, options, and jobs validate existence from the cache. `startScanPrewarm` starts a background discovery during startup. Failed refreshes retain the previous entry. A `sync.Mutex` and `chan struct{}` coalesce concurrent requests into one subprocess.

#### `/api/printers` response shape

Each item contains `name`, `uri`, `info`, `location`, and `makeAndModel`. `info` is the Description in the CUPS web UI (`printer-info`), which the frontend uses to distinguish queues for the same model (issue #101).

- `ipp.ListPrinters` tries IPP `CUPS-Get-Printers` first. It requests four specific attributes to avoid hundreds per printer. When the peer rejects the operation or is outside CUPS, it falls back to the `/printers` HTML page and returns empty `info`, `location`, and `makeAndModel`.
- Iterate all `TagPrinterGroup` entries in `rsp.Groups`. The flattened `goipp` `Message.Printer` holds the first group only, which silently omits the remaining queues.
- Construct URIs from the configured host and the queue name. In TCP mode use `CUPS_HOST`. In Unix socket mode use `localhost:631`. Avoid the response's `printer-uri-supported` because it reflects cupsd's own hostname, which can be unresolvable in containers or across networks.
- If CUPS creates a queue without `-D`, `printer-info` defaults to the queue name. `printerLabel()` omits the duplicate description.

#### CUPS Unix socket (issue #120)

- A nonempty `CUPS_SOCKET_PATH` takes precedence over `CUPS_HOST`. Set an absolute path, which the application checks at startup. External cupsd can create the socket later. A failed socket connection stays in socket mode.
- `internal/ipp/connection.go` provides centralized URI handling. Socket mode keeps queue paths and normalizes HTTP addresses to `http://localhost:631`, using `ipp://` for IPP attributes. It also normalizes job-list `job-printer-uri` values for cancellation. Historical records and scheduled jobs with TCP URIs use the same connection layer.
- The socket path comes from the deployment environment only. Socket mode allows only the root, printer list, and printer/class queue paths, and rejects credentials, queries, fragments, and path traversal. `newSafeClient` dials the Unix socket directly and keeps timeouts, redirect restrictions, and response size limits. TCP mode uses the host/IP SSRF policy.
- `docker-compose.web.socket.yml` demonstrates mounting the host socket directory. Containers can share a runtime directory volume. Mount the whole directory so cupsd can recreate the socket. The deployer manages directory and socket permissions. AIO keeps its default TCP mode and local driver commands.

### Administrator endpoints (`/api/admin/*`)

| Method | Path | Purpose |
| --- | --- | --- |
| GET/POST | `/api/admin/users` | List or create users |
| PUT/DELETE | `/api/admin/users/{id}` | Update or delete users except the protected `admin` |
| GET | `/api/admin/print-records` | Site-wide print records |
| GET/PUT | `/api/admin/settings` | System settings |
| POST | `/api/admin/cleanup` | Manual cleanup |
| GET | `/api/admin/drivers` | Driver list and status |
| POST | `/api/admin/drivers/install` | Asynchronous install. Returns `202` and `jobId`. |
| POST | `/api/admin/drivers/remove` | Asynchronous uninstall. Returns `202` and `jobId`. |
| GET | `/api/admin/drivers/detect` | Detect printers and recommend drivers |
| GET | `/api/admin/drivers/ppds` | PPD candidates (`?deviceUri=&deviceId=&manufacturer=&model=&limit=8`) |
| POST | `/api/admin/drivers/upload` | Synchronous `.ppd` or `.deb` upload, up to 64 MB |
| POST | `/api/admin/drivers/setup` | Asynchronous one-click setup. Returns `202` and `jobId`. |
| GET | `/api/admin/drivers/jobs/{id}` | Poll job status |
| GET | `/api/admin/fonts` | Custom font list and status |
| POST | `/api/admin/fonts/upload` | Synchronous upload (`.ttf`, `.ttc`, `.otf`), up to 64 MB |
| DELETE | `/api/admin/fonts/{name}` | Delete a custom font |
| POST | `/api/admin/fonts/refresh` | Sync fonts and refresh the cache |

#### Asynchronous driver jobs

- `install`, `remove`, and `setup` must run asynchronously. Compiling drivers can take more than ten minutes, which exceeds the global `WriteTimeout=120s`. Return `202` and `jobId` immediately, and run the command in a `context.Background()` goroutine with a 30-minute hard timeout. The frontend polls `jobs/{id}`.
- Allow one driver job at a time because apt/dpkg use a global lock. Return `409` with the active `jobId` if a job is already running.
- Jobs are stored in memory for one hour and cleared when the process restarts.

> See [driver management](docs/en/driver-management.md) for the asynchronous job design and request/response shapes.

Each `drivers[]` item is a `DriverStatus` with `name`, `displayName`, `description`, `arch`, `needCompile`, `installed`, `installedAt`, `installedArch`, `supported`, and `hasScript`. The presence of `manifest.txt` determines `installed`.

Each `customDebs[]` item is an informational `CustomDebPackage` with `filename`, `installedAt`, `installedArch`, and `sizeBytes`.

#### Font management and preflight

- **Storage and system synchronization**
  - `FONTS_DIR` controls storage. The default is `/data/fonts` and the fallback is `data/fonts`. Both AIO and Web-only images mount `./.data:/data`.
  - Startup (`entrypoint.sh` step 1b) and the upload, delete, and refresh endpoints call `syncAndRefreshFonts`.
    - Symlink or copy fonts into `/usr/share/fonts/truetype/custom` and clean up orphaned symlinks for files deleted from storage.
    - Run `fc-cache -f /usr/share/fonts/truetype/custom` with a 30-second timeout.
    - When basic fonts such as `simsun.ttf`, `simhei.ttf`, `simkai.ttf`, and `simfang.ttf` are present, patch `/etc/ghostscript/cidfmap.local` and sync `$GS_INIT_DIR/cidfmap` so Ghostscript's `pdfwrite` normalization uses the correct glyphs.
    - Call `InvalidateSystemFontCache()` to reset the in-process cache immediately.
- **Metadata parser (`font_parser.go`)**
  - Filename-based recognition can misidentify files such as `1.ttf`. Parse the standard SFNT (TTF/OTF/TTC) `name` table.
  - Decode Chinese names strictly as UTF-16BE for Platform ID 3 (Windows) and Platform ID 0 (Unicode).
  - Extract Name IDs 1 (Family), 2 (Subfamily), 4 (Full Name), 6 (PostScript), and 16 (Preferred Family).
  - Each `customFonts[]` entry preserves `name` as the on-disk filename. It adds `displayName` (bilingual names such as `仿宋 (FangSong)`, Western names such as `Arial`, or slash-separated TTC subfonts such as `宋体 (SimSun) / 新宋体 (NSimSun)`), `family`, `subfamily`, and `format` (`TTF`/`OTF`/`TTC`).
  - The API sorts by `displayName` in ascending case-insensitive order.
- **Font preflight blocking (`font_preflight.go`)**
  - For `.docx` or `.pdf` files with incomplete embedded fonts submitted to `/api/print` or `/api/convert`, extract referenced fonts and compare with system families, including alias mappings and Chinese GBK hex BaseFont names.
  - Missing fonts return `422 Unprocessable Entity` with `{"error":"MISSING_FONTS","missingFonts":["..."]}` when font substitution is disabled, so incorrect layouts cannot print silently.
  - `allow_font_fallback` is disabled by default and configurable globally in system settings. The frontend modal can allow substitutions once or always.
- **Upload and delete boundaries**
  - Sanitize filenames with `filepath.Base`, allow only `.ttf`, `.ttc`, or `.otf`, and reject traversal characters.
  - Each upload has a 64 MB limit.

### `/api/print` form fields

| Field | Type | Purpose |
| --- | --- | --- |
| `file` | file | File to print |
| `printer` | string | Printer URI |
| `sides` | `one-sided` / `two-sided-long-edge` / `two-sided-short-edge` | Simplex, long-edge duplex, or short-edge duplex. Takes precedence over `duplex`. |
| `duplex` | `"true"` / `"false"` | Compatibility field. Without `sides`, true means long-edge duplex. |
| `color` | `"true"` / `"false"` | Color output |
| `copies` | int | Number of copies |
| `orientation` | `portrait` / `landscape` | Orientation |
| `paper_size` | `A4` / `A3` / `5inch`…`10inch` | Paper size |
| `driver_options` | JSON string | `pageSize`, `mediaType`, and `quality` from `/api/printer-options`. The server validates and stores a snapshot. These override the corresponding legacy parameters. |
| `paper_type` | `plain` / `photo` / … / `auto` | Media type |
| `media_source` | string | Input tray. Omit `auto`. |
| `print_scaling` | `auto` / `auto-fit` / `fit` / `fill` / `none` / numeric `10`–`400` | Scaling. A number is a custom percentage. |
| `page_range` | string | Page range |
| `page_set` | `all` / `odd` / `even` | Page subset. Omit `all`. |
| `mirror` | `"true"` / `"false"` | Mirror output |
| `compress_images` | `"true"` / `"false"` | Opt-in image compression during conversion. Each encoded image is at most 800,000 bytes. |
| `number_up` | `1`–`16` | N-up. Omit `1`. |
| `number_up_layout` | `lrtb` / `rltb` / `tblr` / `tbrl` | N-up order |
| `page_border` | `single` / `none` | N-up borders |

## 🗄️ Database

SQLite uses `WAL` and `foreign_keys`. Migrations in `store.go::migrate()` use idempotent SQL and add columns incrementally with `addColumnIfMissing`.

### `users`

| Column | Type | Purpose |
| --- | --- | --- |
| `id` | INTEGER PK | Auto-increment |
| `username` | TEXT UNIQUE | Login name |
| `password_hash` | TEXT | bcrypt |
| `role` | TEXT | `admin` or `user` |
| `protected` | INTEGER | `1` indicates protected |
| `contact_name` / `phone` / `email` | TEXT | Contact information |
| `created_at` / `updated_at` | TEXT | RFC3339 UTC |

### `print_jobs`

| Column | Type | Purpose |
| --- | --- | --- |
| `id` | INTEGER PK | Auto-increment |
| `user_id` | INTEGER FK | Submitter |
| `printer_uri` / `filename` / `stored_path` | TEXT | Printer and file |
| `pages` | INTEGER | Page count |
| `job_id` | TEXT | IPP Job ID |
| `status` | TEXT | `queued` or `printed` |
| `is_duplex` / `is_color` / `mirror` | INTEGER | Boolean parameters |
| `copies` / `number_up` | INTEGER | Numeric parameters |
| `orientation` / `paper_size` / `paper_type` / `media_source` | TEXT | Media parameters |
| `print_scaling` / `page_range` / `page_set` | TEXT | Page parameters |
| `watermark_text` | TEXT | Watermark |
| `number_up_layout` / `page_border` | TEXT | N-up parameters |
| `created_at` | TEXT | RFC3339 UTC |

> Issue #68 added the print parameter columns beyond `is_duplex` and `is_color` to persist a full snapshot for reprinting. `page_set` stores the user's original choice, such as `even-reverse`, before any page reordering.

### `scan_records` (issue #111)

| Column | Type | Purpose |
| --- | --- | --- |
| `id` | INTEGER PK | Auto-increment |
| `user_id` | INTEGER FK | Scanning user, with cascading deletes |
| `device` | TEXT | SANE URI such as `hpaio:/usb/...` |
| `mode` | TEXT | Color, Gray, or Lineart |
| `resolution` | INTEGER | dpi (50–4800) |
| `source` | TEXT | ADF, Flatbed, or another source. An empty string uses the device default. |
| `format` | TEXT | png, jpeg, or pdf |
| `filename` / `stored_path` | TEXT | Relative to `SCAN_DIR` |
| `size_bytes` | INTEGER | Output file size, or 0 on failure |
| `status` | TEXT | `running`, `succeeded`, `failed`, or `cancelled` |
| `err_msg` | TEXT | Failure reason |
| `created_at` / `finished_at` | TEXT | RFC3339 UTC |

Index: `idx_scan_records_user_time (user_id, created_at)`. Administrators can view the entire table. Standard users are filtered by `username`.

### `settings`

Key-value table. Current keys are `retention_days` (`0` means keep forever), `save_history`, `max_pages_per_job`, `max_upload_bytes`, `custom_css`, `guest_mode`, `allow_font_fallback` (`0` means block on missing fonts, `1` allows automatic substitution), `session_hash_key`, and `session_block_key`.

### `print_presets` (issue #128)

| Column | Type | Purpose |
| --- | --- | --- |
| `id` | INTEGER PK | Auto-increment |
| `user_id` | INTEGER FK | Owner, with cascading deletes |
| `name` | TEXT | Name, `UNIQUE(user_id, name)`, 1–64 characters |
| `settings_json` | TEXT | Frontend settings snapshot, JSON object up to 16 KB |
| `created_at` / `updated_at` | TEXT | RFC3339 UTC |

- Last settings live in browser localStorage under `print_settings_last`. They are written after successful standard printing and omit watermark content and page ranges. Named presets persist to the database. The shared guest account cannot use presets.
- The server checks the object type and size. The field allowlist lives in `frontend/src/utils/print-settings.js::sanitizePrintSettings`. Applying a preset uses factory defaults for missing or invalid values. `/api/print` remains the authoritative validation boundary. Update the allowlist and `DEFAULT_PRINT_SETTINGS` when adding print parameters.
- `driverOptions` holds queue-specific attribute/value pairs. Store it alongside `printerUri` and restore it only for that printer. When changing printers, clear the options and increment `driverRematchKey` so `PrinterMediaOptions` matches the paper again. `/api/print` rejects options from another queue.

### Scheduled photo queues (issue #121)

- `scheduled_prints` reuses print parameters and trigger policies. The idempotently added columns are `files_json` (ordered relative original and thumbnail paths), `next_file_index`, `submitted_count`, `loop`, `start_date`, `history_json` (results for the last 20 photos), and `photos_per_run` (batch size, default 1).
- Creation uses multipart with the existing single-file `file` field or the new `files` queue field for 1–100 JPG/PNG/GIF images. Supply one of these fields. Each run submits 1–100 photos via `photos_per_run`, default 1. Queues support daily, weekly, and monthly schedules. `start_date` is local to the server. `loop=true` repeats the sequence, otherwise completion disables the queue.
- Automatic execution, manual execution, edits, and deletion share a per-job lock. Manual execution uses a background goroutine, and the UI polls the list's `running` field. Persist progress immediately after each successful submission. A failure ends the batch and the affected photo stays next if submission fails or the execution window is missed. A manual run consumes the next scheduled occurrence and preserves paused status.
- Editing print options or pausing preserves the next run time. On resume, only recalculate expired times so running early still moves the schedule forward. The edit view preserves print options it does not expose. The list ignores late responses from older requests.
- Before submitting, persist `enabled=0, last_status=running`. On success or a definitive failure, atomically persist the result and progress and restore the schedule. Keep the job paused if service is interrupted, result persistence fails, or the IPP outcome is uncertain to avoid submitting duplicates.
- Originals and thumbnails live under `uploads/scheduled/` and remain exempt from history cleanup. Restrict thumbnail reads and deletes with `os.OpenInRoot` and `os.OpenRoot`. Existing single-file schedules keep their completion behavior.

### Scheduled Bing daily wallpaper

- Create a multipart job with `source=bing` and no file. It can run once, daily, weekly, or monthly. Task responses include `source`. Existing jobs default to `upload`, and the source is fixed when editing a job.
- Preview and background runs use fixed Bing China endpoints, with the image date determined in China Standard Time. Automatic and manual runs each submit one image. A manual run keeps the scheduled occurrence. Fetch failures appear in the task results, and the task pauses if the IPP outcome is uncertain. Print history keeps the image that was submitted.

## 🔐 Authentication and security

1. **Startup** — `auth.SetupSecureCookie` reads or creates keys in settings.
2. **Login** — Write `session` (HttpOnly, encrypted, and signed) and `csrf_token` (non-HttpOnly).
3. **Authorization** — `RequireSession` → `RequireAdmin` for administrators → `ValidateCSRF` for non-GET requests.
4. **Logout** — Set both cookies to `MaxAge=-1`.
5. **Default administrator** — `bootstrap.go` ensures `admin/admin` exists with `protected=1`. `Username == "admin"` prevents renaming, changing the role, or deleting the account.

### Cookie attributes

Both cookies use `Path=/`, `SameSite=Lax`, and `MaxAge=86400`. Omit `Domain` to keep them host-only. Always use `auth.NewCSRFCookie()`, `auth.SetSession()`, or `auth.ClearSession()` to issue or clear cookies. Directly constructing `http.Cookie` in handlers caused attribute drift when the old logout path omitted `Secure` and `SameSite`.

`COOKIE_SECURE` has three modes. The default `auto` checks `r.TLS` or the first `X-Forwarded-Proto` value per request. `true` always sets `Secure` and `false` always clears it. `CookieSecure(r)` takes an `*http.Request` because its result cannot be cached process-wide.

### Cross-origin protection (`middleware.CrossOriginProtection`)

The middleware wraps the Go standard library's `http.CrossOriginProtection`, which uses `Sec-Fetch-Site` and falls back to comparing `Origin` with `r.Host`. It adds two project-specific behaviors.

- **Compare `X-Forwarded-Host` when `Sec-Fetch-Site` is absent.** If a proxy omits `proxy_set_header Host $host`, `r.Host` contains the upstream address and disagrees with Origin, blocking POST requests with 403. `proxyOriginAllowed()` checks the forwarded host only when `Sec-Fetch-Site` is missing. Keep browser-supplied cross-origin decisions authoritative when it has a value. The default forwarded-host trust does not weaken security because a client that forges headers can already make the standard library fail open. See the reverse proxy guide for the rationale.
- **Return JSON on rejection.** The standard library's `text/plain` response once caused the frontend to report invalid credentials for proxy misconfiguration (issue #99). `denyCrossOrigin` returns `{"error","code":"cross_origin_blocked","origin","host"}` and logs the decision inputs. `LoginView.vue` chooses the error message by HTTP status. Keep JSON parsing errors distinct from invalid credentials.

`TRUSTED_ORIGINS`, a comma-separated list of complete origins, is the only override for browser-classified `same-site` and `cross-site` requests.

### Login rate limiting (`login_limiter.go`)

The in-process limiter uses `clientIP|username` as its key and locks for 15 minutes after five failures. `clientIP()` unconditionally trusts the first `X-Forwarded-For` value, so keep the backend port off the public internet.

> See the [reverse proxy guide](docs/en/reverse-proxy.md) for troubleshooting both kinds of 403s, configuring Caddy or Traefik, and using subpaths.

## 🖨️ Print pipeline

`printHandler` accepts multipart data, stores the upload under `uploads/YYYYMMDD/`, detects and converts the file, counts pages, inserts a `queued` record, submits via IPP, and writes back `printed`.

- `pdf` uses `normalizePDF` (Ghostscript → LibreOffice → passthrough).
- `office` uses LibreOffice `--convert-to pdf`.
- `ofd` uses `java -jar /ofd-converter.jar`.
- `image` uses `gofpdf`. It retains original pixels unless compression is enabled, which limits images to 3000 pixels and 800 KB each. A PDF matrix applies rotation.
- `text` uses `gofpdf` with embedded Chinese fonts.

> ⚠️ Normalization addresses older CUPS driver compatibility. Ghostscript can damage placeholder CJK fonts and misalign pdf.js previews. See the [PDF pipeline](docs/en/pdf-pipeline.md) for the pipeline and cidfmap.

### Duplex binding (issue #129)

- Standard and batch printing, document and photo reprints, and scheduled jobs share the three-value `sides` field. JSON uses the same name. Legacy `duplex` and `isDuplex` booleans remain supported. Explicit `sides` takes precedence and accepts only the three standard keywords. Invalid values return 400.
- `print_jobs` and `scheduled_prints` gain a `sides` column. An empty string marks a legacy record and falls back to simplex or long-edge duplex via `is_duplex`. Store the boolean alongside the new column. Reprint prefill, schedule edits, and pause/resume preserve the binding.
- Odd, even, and reverse-even modes run as manual duplex. Both submission and history store simplex (issue #109). Use the original `page_set` before reversing page order.

### Custom percentage scaling (`pdf_scale.go`)

Numeric `print_scaling` uses `resolveCustomScaling` to scale PDF content using Ghostscript and send `print-scaling=none` to CUPS. Use it on both printing and reprinting (`reprintHandler`).

- IPP `print-scaling` accepts keywords, so omit numeric strings such as `"40"`. After a Ghostscript failure or for a non-PDF input, use the empty string for the printer default.
- Avoid fixed media and dimensions via `-dFIXEDMEDIA` and `-dDEVICEWIDTHPOINTS`. They shift landscape pages onto portrait paper. The `Install` procedure reads `currentpagedevice /PageSize` per page to preserve dimensions.
- Put `-sOutputFile` before `-f`. Ghostscript interprets later arguments as input filenames and reports `requires an output file` (issue #98).
- `PrintPreview.vue` and `PdfCanvas.vue` preview percentages with a proportional CSS transform. CUPS applies the other scaling modes.

### Driver print options (issue #122)

- `internal/ipp/printer_options.go` reads the queue's PPD using the existing safe connection. It parses PageSize, MediaType, and quality controls, which may be named Ink or similar. Missing categories fall back to standard IPP. Borderless capability requires an appropriate ImageableArea or zero margins on all four sides of media-col.
- `driver_options_json` stores the complete choice in `print_jobs` and `scheduled_prints`. Accept attribute/value pairs reported for the queue. Treat client-supplied dimensions and labels as untrusted. Print, reprint, and scheduled execution share validation and IPP encoding.
- Paper conversion and preview use `custom:<width-mm>x<height-mm>`. Borderless image conversion changes the application's default 10 mm margin to zero. Existing PDFs keep their content margins.
- Image `print_scaling=fill` proportionally enlarges and center-crops during PDF generation. It uses the content area for a single image and each cell for multiple images. `fit` preserves the full image. Standard printing, conversion, original-image reprinting, and scheduled jobs share layout logic. Switching fill in the frontend invalidates the old PDF. PDFs already generated keep their layout.
- Submit queued photos one at a time and persist each success before starting the next image. Each batch advances a single scheduling point. Keep the persisted job paused while processing a batch to avoid duplicate submissions after interruption. See [printer options](docs/en/printer-options.md).

### Photo capture date watermarks (issue #124)

- `photo_date_watermark` (multipart) or `photoDateWatermark` (JSON) is off by default and persisted in print records and schedules. Printing, reprinting, and scheduled execution share image-to-PDF rendering.
- Read JPEG/PNG EXIF `DateTimeOriginal` only and skip missing or invalid values. The browser uploads originals for preview and print, and the server extracts dates before conversion. Clients that precompress images must pass an ordered `photo_dates` array to `/api/convert`. The server strictly validates the dates and item count.
- After rotation and fit/fill layout, place each watermark at the bottom-right of the visible image. Toggling the feature invalidates the old PDF cache. Preserve dates already present in generated PDFs so reprinting does not overlay a second copy. See [printer options](docs/en/printer-options.md).

### Photo originals and submitted PDFs

- `/api/print` keeps the single `file` field and supports `files` for 1–100 photos. Use one field per request. Photo layout shares `rotations`, `per_page`, `invert`, and `compress_images`. Upload originals even when a preview PDF exists.
- `print_jobs.photo_layout_json` stores ordered originals and a layout snapshot. `rendered_path` stores the submitted PDF. Photo reprints regenerate from originals and create independent copies, so clearing one record cannot break another. Scheduled print history separately stores originals, while the schedule owns its source files.
- Custom photo sizes conventionally place the short side first, then apply `orientation`. Conversion, preview, reprinting, and scheduled execution share this convention.
- Download originals via `/api/print-records/{id}/file?source=<index>` and the submitted PDF with `?variant=rendered`. Both require ownership or administrator access and restrict paths with `os.OpenInRoot`. Retention and manual cleanup must cover every associated asset while preserving shared files under `scheduled/`.

## 🧹 Maintenance

`maintenance.go` runs hourly. Read `retention_days` and skip if it is `0`, delete expired `print_jobs` and files, then run `VACUUM` and `wal_checkpoint`. Administrators can trigger cleanup via `POST /api/admin/cleanup`.

## 🧩 Driver management

Related files are `driver_handlers.go`, `driver_registry.go`, `scripts/driver/*.sh`, and `DriversView.vue`.

### Directory conventions

| Path | Contents |
| --- | --- |
| `/opt/cups-drivers/scripts/install-<name>.sh` | Installer scripts copied at build time for runtime use |
| `/usr/local/bin/{driver-install,driver-list,driver-remove,restore-drivers}` | Management commands |
| `/opt/cups-drivers/libexec/capture-debs` | apt `DPkg::Pre-Install-Pkgs` hook that archives the `.deb` files apt is about to install |
| `/opt/cups-drivers/baseline-packages.txt` | Build-time snapshot of packages that ship in the image. Kept in the image layer outside the mounted volume. |
| `/opt/cups-drivers/data/<driver>/manifest.txt` | File path list and sole installed marker. May be empty in package mode. |
| `/opt/cups-drivers/data/<driver>/packages.txt` | Marks package or hybrid mode when present. Each row is `<pkg> <version> <arch> <relative-path>`. |
| `/opt/cups-drivers/data/<driver>/packages/*.deb` | Archived original packages |
| `/opt/cups-drivers/data/<driver>/metadata.txt` | `driver=` / `installed_at=` / `file_count=` / `arch=` / `manifest_version=` / `restore_mode=` / `package_count=` / `package_bytes=` |
| `/opt/cups-drivers/data/<driver>/<mirrored-absolute-path>` | Copy of a file-level artifact |
| `/opt/cups-drivers/data/custom-ppd/` | Uploaded `.ppd` files with a restorable manifest |
| `/opt/cups-drivers/data/custom-deb/packages/` | Uploaded `.deb` packages registered in `packages.txt` and reinstalled on startup |

`docker-compose.yml` mounts `./.drivers` at `/opt/cups-drivers/data`. This volume preserves drivers across restarts.

### Two persistence modes by driver source

| Mode | Applies to | Restore | Uninstall |
| --- | --- | --- | --- |
| **Package** | dpkg packages such as `escpr2` (deb), `canon-ufr2`, `konica-bizhub`, `epson-cn`, `gutenprint`, and `custom-deb` | `dpkg -i` on archived `.deb` files | `apt-get purge` after dry-run validation, or `dpkg -P --force-depends` |
| **File** | Non-package sources such as `canon-capt`, `hp-laserjet1020`, `sharp`, `foo2zjs-firmware`, `custom-ppd`, and `escpr2` (source branch) | `cp -aT` using the manifest | `rm -f` using the manifest |

Vendor packages often install files outside the allowed paths. All 298 Epson escpr2 files live under `/opt`, the Konica filter lives under `/opt/km`, the Canon UFR II rendering engine `cnrsdrvufr2` lives under `/usr/bin`, and the 370 Gutenprint model XML files and 356 Canon ICC profiles live under `/usr/share/<vendor>/`. A file snapshot alone can yield an empty manifest and `exit 1` even after installing files, or an incomplete snapshot that appears installed in the UI and fails after a restart.

Capture original `.deb` files in this order, stopping after the first source that works. Use the apt hook `capture-debs` for all transitive dependencies, then the vendor installer handoff through `DRIVER_PKG_DIR`, then `/var/cache/apt/archives/`, and finally `apt-get download`.

> `capture-debs` must always `exit 0`. A nonzero result from `DPkg::Pre-Install-Pkgs` aborts the entire apt transaction.
>
> The vendor installer handoff must use `|| true` and avoid adding a `trap`, preserving the exit code contract and the single EXIT trap.

### Exit code contract

| Exit code | Meaning | Behavior |
| --- | --- | --- |
| `0` | Success | Write the manifest |
| `3` | Unsupported architecture | Skip the manifest and `exit 3` |
| Any other nonzero result | Failure | Skip the manifest and return the failure |

If the installer returns 0, classify the installation as failed only when both the file snapshot and package archive are empty. An archived `.deb` can be restored successfully even when every installed file falls outside the path allowlist.

### Architecture detection

Use `dpkg --print-architecture`. The runtime lacks `dpkg-dev` and therefore `dpkg-architecture`. Go's `currentDebArch()` maps `GOARCH` to Debian names. `detect_multiarch_libdir()` matches `/usr/lib/*-linux-gnu*` and returns an empty string when no path matches.

### Manifest path allowlist

**ALLOW**: `/usr/lib/cups`, `/usr/share/cups`, `/usr/share/ppd`, `/usr/share/foomatic`, `/lib/firmware`, `/usr/lib/firmware`, and `/usr/lib/<multiarch>`.

**DENY**: `/usr/bin/*`, `/usr/sbin/*`, `/bin/*`, `/sbin/*`, `/usr/local/{bin,sbin}/*`, `/etc/*`, `/var/*`, `/usr/include/*`, `/opt/cups-drivers/*`, `/tmp/*`, `/usr/share/{doc,man,locale,info}/*`, `/usr/share/cups/doc-root/*`, `*/pkgconfig/*`, `*.a`, `*.o`, and `*.la`.

> Keep the allowlist in all three scripts: `driver-install.sh`, `driver-remove.sh`, and `restore-drivers.sh`. Removal and restoration also protect against contaminated legacy snapshots. See [driver management](docs/en/driver-management.md) for the rationale.

### Baseline package ownership guard

The multiarch directory `/usr/lib/<triplet>` contains both driver and system libraries. A path allowlist cannot distinguish ownership. When apt resolves a vendor package's dependencies, it can pull in unrelated system libraries. For example, the old `install-epson-cn.sh` used `apt-get -f install`, which pulled in Qt5, X11, and GL for a GUI utility. These libraries passed the allowlist and entered the manifest, allowing `driver-remove` to delete them and `restore-drivers` to overwrite them with stale copies on startup.

At build time, record all installed package names in `baseline-packages.txt`. The three scripts use that list to exclude files owned by image packages from recording, removal, and overwrite.

Load package names into a Bash associative array for O(1) lookup to avoid a grep subprocess for each of over two thousand `.list` files. Read only the baseline package's `/var/lib/dpkg/info/*.list` files. Multiarch manifests use `<pkg>:<arch>.list`, so strip `:arch` before matching. Intersect the paths with the driver manifest using `comm -12` once, then use O(1) checks for individual paths.

> Keep this ownership guard in all three scripts for the same reason as the path allowlist.
>
> Generate `baseline-packages.txt` after all apt installations and retain it in the image layer. A file under `/opt/cups-drivers/data` would be hidden by the volume mount, leaving only path filtering.
>
> Keep CUPS packages out of driver snapshots. `PKG_NAME_DENY_PATTERNS` in `driver-install.sh` covers `cups`, `cups-core-drivers`, `cups-ppdc`, `libcups*`, and related names. The image builds CUPS 2.4.19 from source and overlays the tar archive in `/usr`. apt installs `cups-daemon`, `cups-client`, and `cups-filters` and omits the `cups` metapackage. Packages such as `printer-driver-gutenprint` and the Konica deb depend on the metapackage. Resolving it through apt installs Debian's `cups-core-drivers` and overwrites the compiled backends in `/usr/lib/cups/backend/{usb,socket,lpd,...}` and filters. The Konica installation once caused 564 CUPS files to be captured as driver files. The `install-*.sh` scripts use `dpkg -i --force-depends` to avoid pulling in the metapackage. The denylist adds a second check.

### AIO installer build scripts

> Compiler-based scripts must register only one `trap _cleanup EXIT` because Bash keeps the last handler for each signal. In AIO mode use only `apt-get clean` and keep `/var/lib/apt/lists/*`. See [driver management](docs/en/driver-management.md) for the single-trap contract.

### Custom driver uploads

- **`.ppd`**: Validate, install under `/usr/share/cups/model/custom/`, and write a restorable manifest.
- **`.deb`**: Run `dpkg -i`. If it fails and dependencies are installed afterward, run `dpkg -i` again. Archive the package under `custom-deb/packages/` and register it in `packages.txt`. On startup, `restore-drivers` reinstalls packages idempotently with `dpkg -i`. Package maintainer scripts perform the real installation, so this path skips file manifests.
  - Restore automatically discovers older archived packages missing from the registry via glob.
  - A broken package is retried on every startup. Remove the corresponding file from `./.drivers/custom-deb/packages/` on the host to stop retrying.
- A `.deb` upload lets its maintainer scripts run as root in the container. The endpoint uses three layers of authorization, and the administrator password is equivalent to container root credentials.
- Enforce the body size with `http.MaxBytesReader`. The parameter to `ParseMultipartForm(n)` limits memory, leaving the body size unrestricted.

> See the [custom driver upload documentation](docs/en/driver-management.md).

### `lpinfo` detection and one-click setup

- Use the long format `lpinfo -l -v` for manufacturer and model. Add `--timeout` and `--include-schemes` only when supported. Use an independent timeout context, separate from `r.Context()`.
- Model precedence is `req.Manufacturer/Model` from lpinfo make-and-model, then `device-id` MFG/MDL, then the URI path. Place URI parsing last because USB URIs can report only `HP` or even `Unknown` as the manufacturer.
- PPD matching uses a scoring engine, with pure functions in `ppd_match.go` and side effects in `ppd_query.go`. Normalize model names, score tiers, apply source preference (custom > vendor > hplip > everywhere > foomatic > gutenprint > generic), add the cups-driverd fingerprint bonus, and stably sort the top N.
- `GET /api/admin/drivers/ppds` returns candidates without a background job or the driver-job lock. It has its own concurrency limit of four.
- `setup` prefers an explicit `ppdUri`, then `everywhere` for driverless printing, then the top candidate. Return an error if no candidate applies.
- `lpadmin` without `-m` creates a raw queue without a PPD. Pass `-m everywhere` explicitly for IPP Everywhere. The lack of PPD settings on raw queues leaves `mediaSourceSupported` empty in `/api/printer-info`, hiding the frontend tray selector.
- Deduplicate queue names via `uniquePrinterName` with suffixes `-2` through `-50`. Refuse to overwrite an existing queue with the same device URI.
- After `lpadmin`, verify with `lpstat -p` and `lpoptions -l`. If the PPD failed to apply, roll back an `isNew` queue with `lpadmin -x`.

> See [driver management](docs/en/driver-management.md) for parser details and failure cases.

## 🚀 Container startup (`entrypoint.sh`)

1. Restore driver snapshots with `restore-drivers`. Reinstall archived `.deb` packages with `dpkg -i` first, then restore saved files with `cp -a`.
2. Set up the CUPS administrator and tzdata.
3. Restore CUPS configuration from `/etc/cups-bak/` when the volume is empty. Ensure existing volumes also have `ssl/` and `ReadyPaperSizes`.
4. Patch the HP 1020 PPD from Letter to A4 (issue #48).
5. Upload HP host-based firmware in the background.
6. Start dbus, avahi, and ipp-usb in the background. Failures here are nonfatal.
7. Start cupsd and its watchdog.
8. Wait for cupsd via `lpstat -r`, up to 30 times at one-second intervals.
9. Set the HP 1020 queue's `media-default=A4` in the background.
10. Use `exec /cups-web` as PID 1.

> Start `cupsd -f` in the foreground inside the watchdog subshell. `wait` can only wait for its own child processes. Moving startup outside the subshell causes a restart storm with status 127.
>
> `restore-drivers` must always `exit 0`. Best-effort restoration keeps the web UI available for recovery. It runs `dpkg -i` with `DEBIAN_FRONTEND=noninteractive`, `--force-confold`, a `timeout`, and a temporary `policy-rc.d`.
>
> Keep both step 3 patches outside `if [ ! -f cupsd.conf ]`. Existing `./.etc` volumes with a configuration skip that restore block and still need new baseline additions.
>
> `lpadmin -o media=` sets `media-default`, which step 9 uses to preselect A4 in the AirPrint UI. iPhone lists available sizes from `media-ready`, which cupsd computes from the intersection of `ReadyPaperSizes` and the PPD sizes. `load_ppd` in `scheduler/printers.c` is the only writer. Issue #82 is fixed by `ReadyPaperSizes A4,A3,A5,A6,EnvDL` using PPD sizes rather than PWG names such as `iso_a4_210x297mm`. Without that directive, the container's unset `LANG` falls back to the C locale, which yields `Letter,Legal,Tabloid,4x6,Env10` and omits A4.
>
> See [container startup](docs/en/container-startup.md) for the full design.

## 🔧 Development environment

### Local setup

```bash
# Frontend
cd frontend && bun install && bun run dev    # :5173, proxies /api to :8090

# Backend
go mod download
go build -o bin/cups-web ./cmd/server && ./bin/cups-web    # :8080
```

### Makefile

```bash
make all              # Frontend dist followed by the Go binary
make frontend         # Frontend only
make build            # Backend only, preferred over a bare go build ./cmd/server
make docker-build     # AIO image
make docker-build-web # Web-only image (cups-web:web)
```

### Vite development proxy

`/api` proxies to `http://localhost:8090`. For local debugging, run `LISTEN_ADDR=:8090 go run ./cmd/server` for the backend and `bun run dev` for the frontend.

### Frontend chunks

`vue-vendor` contains Vue and the router, `ui-vendor` contains nuxt-ui, reka-ui, and vueuse, and `pdf-vendor` contains pdfjs-dist.

## 🚢 Deployment

### Docker Compose

The AIO Compose service is `cups`, with `image: hanxi/cups-web:latest` and `network_mode: host` for mDNS multicast. CUPS uses port `631` on the host and the web server uses `LISTEN_ADDR`, default `:1180` (issue #107).

| Setting | Reason |
| --- | --- |
| `network_mode: host` and `hostname: CUPS` | mDNS/DNS-SD multicast (5353/UDP) requires host networking to discover LAN printers and advertise AirPrint (issue #107). avahi advertises `CUPS.local`. |
| `LISTEN_ADDR=:1180` | With host networking, the web server listens directly on host port 1180, matching the former bridge setup. |
| `user: root` | cupsd, lpadmin, dpkg, and writes to system paths |
| `security_opt: [apparmor:unconfined]` | Works around AppArmor denials on PVE LXC (issue #91) |
| `./.etc:/etc/cups`, `./.data:/data`, and `./.uploads:/uploads` | Persist configuration, data, and uploads |
| `./.drivers:/opt/cups-drivers/data` | Persist driver snapshots across restarts |
| `./.scans:/scans` with `SCAN_DIR=/scans` | Persist scans (issue #111). Without the mount, scan records can outlive the files they reference. |
| `/dev/bus/usb:/dev/bus/usb` with `device_cgroup_rules` | USB hot-plugging (issue #81) |
| `/run/udev:/run/udev:ro` | Optional libusb device attributes |

> Avoid mounting the host's `/run/dbus/system_bus_socket`. The old AirPrint setup borrowed host avahi (issue #94), but if avahi was unavailable on the host, the mounted socket path prevented the container's own dbus-daemon and avahi from starting (issue #107). The container starts its own dbus and avahi on the host network.

### Docker build

The seven stages are `frontend-build` (node:20-slim), `java-builder` (amd64 via BUILDPLATFORM), `builder` (golang:1.26), `cups-builder` (CUPS from source), `document-runtime` (shared conversion dependencies and fonts), `web-runtime` (Web-only), and `runtime` (default AIO). Build platforms are `linux/amd64`, `arm64`, and `arm/v7`. The Web-only stage skips `cups-builder` and avoids compiling CUPS.

> Keep `ca-certificates` in `cups-builder` for wget TLS verification in CI.
>
> The Debian trixie `cups-daemon` package (2.4.10) supplies `/etc/cups/*`. `cups-compiled.tar` from the source build contains only `/usr`, so take default `cupsd.conf` Location and Policy values from Debian. Files in `/usr/lib/cups/**`, including filters, backends, and `daemon/cups-driverd`, come from compiled CUPS 2.4.19. Installing `cups-core-drivers` via apt overwrites those files with Debian's versions.
>
> Explicitly create `/etc/cups/ssl`. In the source, `cupsdCheckPermissions` passes `create_dir=0` for this path. A failed `lstat` neither creates the directory nor returns an immediate error. The first IPPS handshake then reports `Unable to create server credentials`. AirPrint clients prefer `_ipps._tcp`. Debian once supplied the empty directory as part of `cups-daemon`. The Dockerfile and entrypoint now create it explicitly.
>
> See [Docker builds](docs/en/docker-build.md) for target dependencies, the three image architectures, and the writable HOME and LibreOffice profile requirements.

### CI/CD

- `build-release.yml` cross-compiles for seven platforms. Tags matching `v*` generate formal releases. Commits on `master` update the rolling prerelease tagged `dev`, which avoids a tag/branch naming conflict.
- `docker-publish.yml` publishes both targets for three architectures on `master` or tags matching `v*`. AIO uses `vX.Y.Z`, `latest`, and `dev`. Web-only uses `vX.Y.Z-web`, `latest-web`, and `dev-web`. Both have the same `VERSION`. The workflow begins with a Free disk space step.

### Versioning

`./bump-version.sh patch|minor|major`

## 🎯 Common development tasks, debugging, and code style

> See [development conventions](docs/en/conventions.md) for templates covering new APIs, database changes, frontend pages, file types, and drivers, plus debugging commands, Go and Vue style, and Git commit rules.
>
> Write commit messages in Chinese and omit `Co-Authored-By` and any AI attribution lines.

**Maintainer**: Hanxi (<im.hanxi@gmail.com>)
