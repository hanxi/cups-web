# Printer Options and Photo Batches (Issue #122)

Standard printing, reprints, and scheduled jobs share driver-provided paper sizes, media types, and print quality controls. For a CUPS queue, the application first reads `/printers/<queue>.ppd` over the existing secure HTTP or Unix socket connection. When the PPD or a category is unavailable, it queries standard IPP capabilities. The UI shows the reason when discovery fails and offers **Refresh options** to retry.

## Capabilities Endpoint

`GET /api/printer-options?uri=<queue-uri>` uses existing browser session or API key authentication. Its 60-second cache holds up to 64 targets. `force=1` bypasses the cache, and a failed query preserves any existing entry.

- `paperSizes`, `mediaTypes`, and `qualities` contain arrays of options.
- `defaults` may include the driver's `pageSize`, `mediaType`, or `quality` default.
- `source` is `ppd` or `ipp`. Standard IPP can fill in categories the PPD omits.

Each option includes `attribute`, `value`, and `label`. Paper options also include `widthMM`, `heightMM`, and `borderless`. Standard IPP `media-col` options retain the four margins in hundredths of a millimeter, in bottom, left, right, top order. The PPD label uses the `zh_CN` translation if available, then `zh`, then the original driver label.

For quality, the PPD parser prefers `cupsPrintQuality`, `PrintQuality`, and `Quality`, followed by vendor-specific controls labeled Print Quality, such as Epson's `Ink`, and finally `Resolution`. It exposes only paper, media, and quality controls. It treats the PPD as declarations and does not execute its code.

A PPD `ImageableArea` that covers the whole sheet qualifies as borderless, including negative margins that extend the printable area beyond the paper.

The IPP fallback explicitly requests `media-col-database`. Complete dimensions and margins yield standard and borderless options. Otherwise it parses PWG dimensions in `media-supported`. Quality uses IPP enumeration values 3, 4, and 5. CUPS enforces the driver's constraints on the final paper, media, and quality combination.

## Submission and Option Snapshots

`POST /api/print` and `POST /api/scheduled-prints` accept a `driver_options` JSON string in multipart requests.

```json
{
  "pageSize": {"attribute": "PageSize", "value": "actual driver value"},
  "mediaType": {"attribute": "MediaType", "value": "actual driver value"},
  "quality": {"attribute": "Ink", "value": "actual driver value"}
}
```

Copy selections from the capabilities endpoint. Each field is optional. When quality is omitted, the printer default applies. `POST /api/print-records/{id}/reprint` and `PUT /api/scheduled-prints/{id}` use a JSON `driverOptions` object. The server matches `attribute` and `value` against the target queue and replaces client-provided dimensions and labels with the reported values. Unsupported choices return an error and require an explicit correction.

When using a driver-provided paper or media choice, omit the corresponding legacy `media` or `media-type` mapping to avoid conflicts. PPD options keep their original attribute names and values. IPP quality uses the enum type and borderless `media-col` uses the collection type. IPP success responses include `0x0001` (attributes substituted) and `0x0002` (conflicting attributes adjusted). Both accept the job and advance photo progress.

An idempotent migration adds `driver_options_json TEXT NOT NULL DEFAULT '{}'` to `print_jobs` and `scheduled_prints`. The stored snapshot pre-fills reprint and edit controls. Older clients can still send `paper_size` and `paper_type`. Editing or pausing a scheduled job with an unchanged queue and choices reuses its trusted snapshot, so offline printers can still be paused. Execution checks that the options still exist.

Conversion and preview use the same paper dimensions. Changing the paper or orientation after converting an image or text invalidates the cached PDF. Custom dimensions use `custom:<width-mm>x<height-mm>`. Borderless image PDF conversion removes the app's 10 mm inset. Standard sizes keep the app inset without recalculating it from hardware margins. `/api/convert` with `borderless=true` uses the same layout for the preview.

### Photo Fit and Fill

`/api/convert` accepts optional `print_scaling` for a single image in `file` or multiple images in `files`. Without it, conversion keeps the existing full-image layout. Standard printing, batch conversion, original-photo reprints, and scheduled photo queues share the layout rules.

- `fit` scales proportionally to keep the full photo visible. Different proportions can leave white space.
- `fill` scales proportionally and crops to the center to cover the content area. Borderless sizes cover the sheet. Standard sizes cover the area inside the app's 10 mm inset on all four sides.
- Multi-photo layouts apply per-photo rotation first, then fit or fill within each cell's proportions. Cells keep 4 mm gaps. A PDF clipping path limits the visible photo area without re-encoding the image.

Switching fill mode after conversion invalidates the cached PDF. Stale conversion responses cannot overwrite the current selection. Original-photo previews reflect fit, fill, and the app inset. A first-image preview of a multi-photo layout is illustrative. The combined PDF displays the complete layout. Other modes continue using the existing layout and CUPS handling, and custom percentages still use the existing scaling pipeline.

The PDF generated from a fill layout contains an internal layout marker. Standard printing, reprints, and scheduled runs send `print-scaling=none` when the paper, orientation, and dimensions of every page still match. This prevents CUPS from reapplying fill and cropping a page that already contains the intended crop and date. The print record and scheduled job still store the user's `fill` preference. The marker remains in the converted PDF, including after downloading and re-uploading it. Normal PDF uploads, older PDFs without the marker, and PDFs printed with a changed paper size or orientation continue using the requested scaling mode.

Existing white margins in PDF content are preserved. A record with the original photo can generate a new fit or fill layout on reprint. Records with only a PDF print the existing pages. To change that photo layout, upload the original again.

## Photo Quality and Original Files

Standard and batch photo printing, multi-photo layouts, and scheduled photo jobs all submit an app-generated PDF to CUPS. With **Image compression** disabled, original pixels are preserved, including images whose long side exceeds 3,000 pixels. Unchanged JPEG pixels allow the original JPEG encoding to be embedded directly. Rotation uses a PDF matrix without re-encoding. Color inversion uses a lossless PNG intermediate. With compression enabled, the backend keeps the long side at or below 3,000 pixels and each encoded image at or below 800,000 bytes, reducing quality or dimensions as needed. The limits govern embedded images. The overall PDF may exceed these sizes.

The browser uploads selected originals for preview conversion and printing. The backend uses the same layout parameters. The preview PDF is for display, and the original photos remain the source. Batch photo printing also sends originals directly to the print endpoint. Keeping originals increases upload and storage size. Deployment upload limits still apply and return an error on overflow. HEIC continues through the browser's existing JPEG conversion. Native HEIC support and color management are outside this change.

Photo paper starts with portrait dimensions, short edge first, and then applies the user's chosen orientation. Preview, printing, reprints, and scheduled jobs agree even if the driver reports the long edge first. Existing PDFs keep their page dimensions.

`POST /api/print` still accepts a single `file` and also accepts `files` with 1–100 photos. Use one form of input per request. Photo parameters match `/api/convert`. `rotations` is a comma-delimited list aligned with the files, with values from `0,90,180,270`. `per_page` supports 1, 2, 4, 6, or 9. `invert` and `compress_images` are also saved. Multi-photo prints may provide `name` for the combined document. A reprint prefers the originals and can change paper, orientation, fit or fill, and the date overlay. Rotation and photos per page default to the saved snapshot.

An idempotent migration adds `photo_layout_json` to `print_jobs` for original filenames, order, rotation, photos per page, inversion, and compression. `rendered_path` stores the PDF submitted to CUPS, including text watermarks and percentage scaling applied by the app. List responses return filenames and parameters and omit disk paths. Downloads use the existing owner or administrator authorization.

- `GET /api/print-records/{id}/file?source=0` downloads the first original. Other zero-based indexes follow file order.
- `GET /api/print-records/{id}/file?variant=rendered` downloads the PDF submitted to CUPS. CUPS may still apply page ranges, copy counts, and driver choices.
- Requests without parameters preserve the previous behavior. Multi-photo records default to the combined PDF.
- A reprint creates independent copies of the originals, so deleting old history leaves the new record intact. Automatic and manual cleanup remove attached originals, conversion files, and rendered PDFs. Shared files belonging to scheduled jobs remain protected. Scheduled print history archives its own originals so newer history remains reprintable after the job is deleted.

Older databases and clients still work. Older records without a photo manifest use the previous reprint path. Originals cannot be recovered from a PDF-only record.

## Photo Taken Date (Issue #124)

**Print date taken** is available under **Advanced options** for standard photo printing and on scheduled photo jobs. It defaults to off. When enabled, the app draws an orange date with a light shadow in the lower-right corner of each photo's final visible area, formatted as `2026年10月3日`. Fit anchors the date to the photo edge. Fill anchors it to the cropped content area. A multi-photo layout draws a separate date inside each photo. Select **Convert to PDF** to preview the result. Changing the setting invalidates the cached PDF.

The app reads `DateTimeOriginal` from JPEG EXIF or PNG `eXIf` and keeps the year, month, and day without a time zone conversion. Missing, malformed, or invalid metadata skips the date and still prints the image. The app does not substitute file or execution timestamps. GIF usually lacks EXIF. HEIC converted by the browser skips the overlay if that conversion loses the date. Existing EXIF orientation handling stays the same.

- Multipart requests to `POST /api/convert`, `POST /api/print`, and `POST /api/scheduled-prints` use `photo_date_watermark=true`.
- JSON for `PUT /api/scheduled-prints/{id}` and `POST /api/print-records/{id}/reprint` uses `photoDateWatermark: true`. List responses include the boolean.
- `/api/convert` also accepts `photo_dates`, a JSON string array such as `["2026-10-03", ""]`. Dates correspond to `file` or `files` in order. An empty string means no valid date. The array length must match the file count, and dates must be valid `YYYY-MM-DD` values. Without the array, the server reads original metadata. The current browser uploads original images for server-side EXIF extraction. Clients that precompress images can supply dates before compression strips the metadata. This field controls display only and cannot identify a source file.

An idempotent migration adds `photo_date_watermark INTEGER NOT NULL DEFAULT 0` to `print_jobs` and `scheduled_prints`. It stores the setting for prints, reprints, and scheduled jobs. Existing records default to off. When an older client edits a scheduled job and omits the field, the stored setting is preserved. Scheduled execution reads persisted originals one at a time. Drawing the date leaves submission progress and scheduling rules intact.

Reprints can toggle the setting when originals are available. A generated PDF keeps the date already drawn on its page. A reprint of that PDF neither adds the date again nor removes it. Other document types ignore the switch. The date and an optional custom text watermark can both appear.

## Photos per Scheduled Run

Creation accepts `photos_per_run` in multipart, and edits use JSON `photosPerRun`. The range is 1–100, with 1 as the default for older requests and jobs. The database uses `photos_per_run INTEGER NOT NULL DEFAULT 1`. This setting is independent of `copies`. A run of three photos and two copies sends three jobs, each for two copies.

- A non-looping queue submits the remaining photos and finishes if fewer than configured remain.
- A looping queue returns to its first photo at the end and submits until the configured count is reached. A run may repeat photos when the count exceeds the queue length.
- The scheduler saves progress after every accepted photo. A failure stops the batch, and the next run starts with the failed photo.
- Automatic and manual execution use the same batch logic. The batch advances the schedule by one occurrence. A manually triggered batch preserves the previous paused state.
- Before submitting each photo, persist `enabled=0, last_status=running`. The job stays paused while successful photo progress is persisted and resumes its schedule after the final definite result is saved. Interruptions, database failures, disconnects after upload, malformed responses, and HTTP gateway errors keep it paused. A definite connection failure or valid IPP rejection records a failure and allows the next scheduled run. Users can resume after checking the printer.
- Recent execution history records the latest 20 photo results. `submittedCount` and print records indicate CUPS acceptance. They do not verify physical output.

## Automatic Photo Queue Orientation

New scheduled photo queues default to **Automatic** orientation. Users can choose portrait or landscape instead. Existing jobs keep their settings. Single-file schedules do not offer automatic orientation. Switching from a photo queue with automatic orientation to a single file selects portrait.

Multipart creation and JSON editing use `orientation`. Photo queues can send `auto`, while single-file jobs return 400 for it. The existing `scheduled_prints.orientation` column stores the value.

Automatic execution and **Print next batch** read the pixel dimensions of each original. Photos wider than tall use landscape. All others, including square photos, use portrait. The generated PDF, IPP parameters, and print history agree on the actual orientation. The queue itself retains `auto`. File-read or conversion errors stop the batch at the failed photo.

The setting uses actual pixel dimensions, including changes to the dimensions of an image a user has already rotated and saved. It does not interpret JPEG EXIF orientation or add a rotation step.

## Duplex Binding Edges (Issue #129)

Standard printing, batch printing, reprints, and scheduled jobs support single-sided, long-edge duplex, and short-edge duplex. Multipart and JSON requests accept `sides` as `one-sided`, `two-sided-long-edge`, or `two-sided-short-edge`, matching the IPP `sides` attribute. Explicit `sides` takes precedence over the old boolean. Invalid values return 400. If `sides` is omitted, the server reads `duplex` or `isDuplex` for a scheduled update. True means long-edge duplex, and false means single-sided.

Print history and scheduled jobs store the binding edge so a reprint or edit restores the original choice. Pause and resume preserve the value. Old records that lack an explicit binding can recover only the old boolean and cannot infer short-edge duplex. Print presets and the last-used settings keep the frontend's existing three-value `duplex` string.

Odd pages, even pages, and reverse even pages still use the manual duplex workflow. The actual submission and its history entry are single-sided so CUPS does not insert blank separator pages.
