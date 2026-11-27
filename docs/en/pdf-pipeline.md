# PDF Normalization and the Print Pipeline

> This document expands the print pipeline in [AGENTS.en.md](../../AGENTS.en.md) and describes normalization, Ghostscript's cidfmap configuration, and known side effects.

## Print Pipeline Overview

`printHandler` in `cmd/server/print_handlers.go` is the main entry point.

1. **Receive the request**. Parse the multipart form with a 512 MB memory threshold and extract `file` and the print parameters.
2. **Save the file**. `saveUploadedFile` sanitizes the filename and saves it in `uploads/YYYYMMDD/`.
3. **Detect and convert the file type** using `detectFileKind`.
   - `pdf` goes through [PDF normalization](#pdf-normalization-with-normalizepdf).
   - `office` uses `convertOfficeToPDF`, which runs `libreoffice --headless --convert-to pdf`.
   - `ofd` uses `convertOFDToPDF`, which runs `java -jar /ofd-converter.jar`.
   - `image` uses `convertImageToPDF` and renders with `gofpdf`. `downscaleImageIfNeeded` reduces images whose long side exceeds 3,000 pixels to 3,000 pixels and re-encodes them as JPEG at quality 85 before embedding them. This prevents 10 MB or larger phone originals from causing preview and download timeouts on mobile. Transparent PNG pixels are composited onto white for printing. [Issue #22](https://github.com/hanxi/cups-web/issues/22)
   - `text` uses `convertTextToPDF`, which renders with `gofpdf` and embedded Chinese fonts.
4. **Count pages**. Use `countPDFPages`, `countPDFPagesWithFallback`, or `estimateTextPages`. When PDF page counting fails, normalize the PDF and retry. If the retry fails, use one page as the fallback and keep processing the request.
5. **Persist the job**. Insert a `queued` row in `print_jobs`.
6. **Submit the job**. `ipp.SendPrintJob` builds and sends the IPP `Print-Job` request.
7. **Update the status**. On success, set `printed` and record `job_id`.

Converted or normalized PDFs are saved in `uploads/` as `<original-file>.print.pdf`. Maintenance deletes them alongside their originals. `/api/convert` also normalizes PDFs with `normalizePDF` so the frontend `PdfCanvas` preview and the final print path can use the same bytes.

## Optional Image Compression (Issue #119)

Standard printing offers **Image compression** for selected images. It is off by default. When enabled, the browser compresses each image to 800,000 bytes or less before upload and PDF conversion for preview and printing. The setting also applies to images in mixed-file batch jobs. Toggling it invalidates cached image conversions. The next conversion or print uses the new setting.

`POST /api/convert` (`file` for a single image or `files` for multiple images) and `POST /api/print` (`file` for an image) accept the multipart field `compress_images=true`. After the existing downsampling, rotation, and color inversion, the server checks the encoded size. It tries JPEG quality 85, 75, 65, 55, and 45 in order. If the image is still too large, the server proportionally reduces its dimensions and retries. Transparency is composited onto white. Compression failure returns an error and stops the image from being submitted. With compression off, existing behavior applies.

The limit applies to each encoded image. The overall PDF size and printer memory use can differ because the CUPS driver transforms the print data. Confirm whether this resolves HL1110 failures using the actual driver, sample image, and logs. When browser compression is enabled, the browser submits the generated PDF and history reprints reuse that PDF. For direct API uploads of original images, `compress_images` applies only to that conversion and is not persisted, so reprinting the original image does not inherit it. Existing PDFs, Office files, and images in scheduled jobs are outside this feature.

## PDF Normalization with `normalizePDF`

After `diagnosePDF` writes diagnostic logs, `normalizePDF` tries these options in order.

1. **Ghostscript `pdfwrite`**, with `-dCompatibilityLevel=1.4 -dEmbedAllFonts=true`. Try strict `/prepress`, followed by the lenient `-dNEWPDF=false -dPDFSTOPONERROR=false` run.
2. **LibreOffice `--convert-to pdf`** as a fallback.
3. **Pass through the original** as the final fallback.

This pipeline addresses compatibility issues such as older CUPS drivers rejecting new PDF 1.7 syntax. Ghostscript may rewrite placeholder CJK fonts with a subset prefix and misleading embedding information, which can cause spacing errors in browser pdf.js previews. See the `getDocument` argument comments in `PdfCanvas.vue`. The preferred preview behavior is to let pdf.js read the original PDF and apply minimal normalization before actual printing.

## Ghostscript cidfmap Loading for Chinese Glyphs

The image gives Ghostscript the mappings in `docker-fonts/cidfmap.local`. These map Regular and Bold Song, Hei, Kai, and FangSong fonts, represented as eight GBK-byte BaseFont names, to the TrueType families `arphic-uming`, `arphic-ukai`, and `wqy-zenhei`. When `pdfwrite` rebuilds a font dictionary, it can use different glyphs for each family instead of defaulting all of them to the sans-serif `DroidSansFallback`.

Both loading paths still serve separate use cases. Read the comments in `Dockerfile` and `cmd/server/pdf_normalize.go` before changing either one.

### 1. Automatic `Resource/Init/cidfmap` Loading in Docker

At build time, `Dockerfile` copies `docker-fonts/cidfmap.local` to `/etc/ghostscript/cidfmap.local`. It uses `sed` to change mapping targets when `simsun/simhei/simkai/simfang.ttf` fonts are available. It finds the Ghostscript init directory with `find /usr/share/ghostscript -path "*/Resource/Init"` and copies the same file to `Resource/Init/cidfmap`. Ghostscript loads it at startup without additional command-line arguments. Trixie's Ghostscript 10.05.1 provides `FAPIcidfmap` and leaves `cidfmap` available for this configuration, so no merge is necessary. A build-time `test -s` and `grep -cE '^/#'` check requires exactly eight mappings. A different count fails the build.

### 2. Compatibility Search Path in `cidfmapPreambleArgs()`

`tryGhostscriptRun` in `pdf_normalize.go` appends `cidfmapPreambleArgs()` to its Ghostscript command. When `/etc/ghostscript/cidfmap.local` (`cidfmapSystemPath`) exists, the function returns a single `-I<dir>` search path argument. When the file is absent, as in local macOS development, the function returns `nil` and the command is unchanged.

Earlier versions explicitly loaded the mapping with `-c "(cidfmap.local) .runlibfile" -f`. Automatic loading from `Resource/Init/cidfmap` made that redundant. `pdf_normalize_test.go::TestCidfmapPreambleArgs` checks both cases. A missing file must return `nil`, and an existing one must return exactly one `-I` argument pointing to the mapping directory.

### Font Mappings and Diagnostics

The arphic and wqy fonts each come in one weight, and Ghostscript does not synthesize bold. To distinguish weights visually, Song Bold and FangSong Bold use `wqy-zenhei`, the heaviest Chinese font in the image. Hei and Kai use the same font for both weights and therefore look identical across Regular and Bold.

```bash
gs -dPDFDEBUG -dNOPAUSE -dBATCH -sDEVICE=pdfwrite -sOutputFile=/tmp/out.pdf <in.pdf> 2>&1 | grep -E "Substituting|CIDFSubst"
```

A matching cidfmap prints `Substituting font ... from /usr/share/fonts/truetype/...`. If no mapping matches, Ghostscript falls back to `DroidSansFallback`.

For new mappings, the GBK bytes for the PostScript names are `cb ce cc e5` for Song, `ba da cc e5` for Hei, `bf ac cc e5` for Kai, and `b7 c2 cb ce` for FangSong. Use `[(GB1) 2]` for CSI. Update the `expect 8` build check in `Dockerfile` when the count changes.

## Known Side Effect with Placeholder CJK Fonts and pdf.js

> Acrobat can export a placeholder Type0 font dictionary using `UniGB-UCS2-H` and a bare CJK name such as `/BaseFont /#ba#da#cc#e5`. Examination admission slips and national-standard forms often use this format. Ghostscript can rewrite it as `/BaseFont /CCGWER+#ba#da#cc#e5` with a subset prefix and apparent `FontFile2` embedding. **pdf.js** may then position roughly one character in every three or four incorrectly. A browser's native PDF engine can fall back to system fonts. The preview uses `pdfjs-dist` with a canvas in `frontend/src/components/print/PdfCanvas.vue`. Where the preview and print paths share `normalizePDF`, they use the same bytes for consistency. This visual problem with the preview does not affect printing.

## Custom Chinese Fonts and Layout Fidelity

Custom fonts preserve line breaks and layout for business documents and official forms using fonts such as 仿宋_GB2312 or 方正小标宋简体.

### 1. External Mount and Startup Synchronization

- **Mount path.** Mount the host font directory at `/data/fonts`. In the default Compose setup, put fonts in `./.data/fonts/`.
- **Entrypoint sync, step 1b.** Startup scans `/data/fonts` for `.ttf`, `.ttc`, and `.otf` files, links them into `/usr/share/fonts/truetype/custom`, and runs `fc-cache -f`.
- **Ghostscript mapping.** When original Windows fonts (`simsun.ttf`, `simhei.ttf`, `simkai.ttf`, or `simfang.ttf`) are mounted, the script adjusts `/etc/ghostscript/cidfmap.local` and copies it to `$GS_INIT_DIR/cidfmap`. `pdfwrite` can use the original glyphs and preserve their weight and metrics.

### 2. Font Management in the Web UI (`/fonts`)

- **Direct access.** Administrators can open `/#/fonts` from **Fonts** in desktop and mobile navigation.
- **Binary metadata parsing in `font_parser.go`.** The SFNT `name` table decoder supports TrueType, OpenType, and TrueType Collections. It reads UTF-16BE names for ID 1 (Family), 2 (Subfamily), 4 (Full Name), and 16 (Preferred Family). It can display `Arial` even when the file is named `1.ttf`. Chinese names can display as `仿宋 (FangSong)`, and collections expand to entries such as `宋体 (SimSun) / 新宋体 (NSimSun)`.
- **Immediate changes.** A successful Web upload or deletion synchronizes system symlinks, runs `fc-cache -f`, updates cidfmap, and calls `InvalidateSystemFontCache()`. The container can keep running.

### 3. Font Preflight

- **When it runs.** `/api/print` and `/api/convert` check `.docx` files and PDFs with incompletely embedded fonts. The server extracts the document's required families and compares them against registered fonts, common aliases, and GBK hexadecimal BaseFont names.
- **Blocking behavior.** If a font is missing and fallback is disabled, the endpoint returns `422 Unprocessable Entity` with code `MISSING_FONTS`. The UI lists the missing fonts and offers **Cancel**, **Allow fallback this time**, and **Always allow fallback**. This prevents faulty layouts from printing without notice.
- **Global preference.** The administrator can allow automatic fallback for missing fonts in **Admin → System settings**. The switch is persisted as `settings.allow_font_fallback` and disables strict preflight blocking when enabled.

## HTTP Timeouts

`http.Server` in `cmd/server/main.go` uses `ReadTimeout = WriteTimeout = IdleTimeout = 120s`. Mobile `/api/convert` and `/api/print` requests can involve uploading a 10 MB or larger photo, server-side downsampling or normalization, and downloading a PDF. A 15-second timeout was too short over 4G. Use `http.TimeoutHandler` on the relevant routes if individual endpoints need shorter limits, so long uploads can keep the global timeout. [Issue #22](https://github.com/hanxi/cups-web/issues/22)
