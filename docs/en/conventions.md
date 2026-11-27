# Development Conventions and Common Tasks

> This guide supplements [AGENTS.en.md](../../AGENTS.en.md) with implementation checklists, debugging commands, and code conventions.

## Common Development Tasks

### Add an API Endpoint

1. Create `xxx_handler.go` under `cmd/server/` and export the handler.
2. Register the route in the appropriate subrouter (`api`, `protected`, or `admin`) in `main.go`.
3. Add the client call to `frontend/src/utils/api.js` and use it from the view.
4. For a write endpoint, confirm the frontend `fetch` includes the `X-CSRF-Token` header.

### Change the Database Schema

1. Change or add the model in `internal/store/`.
2. In `store.go::migrate()`, append `CREATE TABLE IF NOT EXISTS ...` for a new table, or call `addColumnIfMissing(ctx, db, "<table>", "<column_def>")` to add a column to an existing table.
3. Update the affected CRUD functions.
4. Use `sqlite3 data/cups-web.db` to check that the migration works with a fresh database and with old data.

### Add a Frontend Page

1. Add a `.vue` single-file component to `frontend/src/views/` using the Composition API.
2. Add a route in `frontend/src/router/index.js`. Use `meta: { requiresAuth: true }` for authenticated pages and add `requiresAdmin: true` for administrator pages.
3. Add a navigation entry in the `App.vue` header when needed. The Print, Drivers, and Admin entries for administrators appear as segmented controls on desktop and in the hamburger menu on mobile.

### Add a Supported File Type

1. Add a `fileKind` in `file_utils.go::detectFileKind`.
2. Implement conversion in `convert_utils.go` or `pdf_utils.go`.
3. Handle the type in the `switch kind` in `print_handlers.go`.
4. Update `estimateHandler` and `convertHandler`. Cover both the single-file `file` and multi-file `files` inputs in `convertHandler`.

### Add a Printer Driver

See [Driver management](driver-management.md) for the implementation details and known issues.

1. **Add the installation script.** Use `scripts/driver/install-<name>.sh`, where `<name>` is the canonical driver name. The Dockerfile's `COPY scripts/driver/install-*.sh /opt/cups-drivers/scripts/` automatically includes the script.
2. **Use the expected exit codes.** Return `0` for success, `3` when the current architecture is unsupported, and another nonzero exit code for failure. Returning `0` for an unsupported architecture creates a manifest and makes the UI report a false installation. Detect the architecture with `dpkg --print-architecture`. The runtime image omits `dpkg-dev` and therefore `dpkg-architecture`.
3. **Use a single exit trap for source builds.** When building or installing build dependencies, use one `trap _cleanup EXIT`. Put temporary directory cleanup and `apt-get purge -y --auto-remove ${BUILD_DEPS}` in branches of `_cleanup()`. Use `CUPS_AIO` to distinguish runtime container installation and image builds. Both `driver-install` and Go's `runDriverCommand` supply `CUPS_AIO=1`. Keep `/var/lib/apt/lists/*` in AIO mode so later driver installations can use the apt index.
4. **Choose a persistence mechanism.**
   - Use **package-level snapshots** for vendor `.deb` files and packages installed by `apt-get install`. After downloading a vendor package, hand the original file to `driver-install` before `dpkg -i`:
     ```bash
     # The archive attempt preserves installation outcomes and exit codes.
     # The guard preserves behavior for builds and manual runs.
     if [ -n "${DRIVER_PKG_DIR:-}" ]; then
         cp -a "${DEB_PATH}" "${DRIVER_PKG_DIR}/" 2>/dev/null || true
     fi
     ```
     Apt's `capture-debs` hook automatically captures apt-installed packages and their transitive dependencies. Avoid resolving the vendor package's `cups` metapackage dependency through `apt-get -f install`. Debian's `cups-core-drivers` would overwrite components built from CUPS source. Use `dpkg -i --force-depends`. Keep archive cleanup under the existing trap.
   - Use **file-level snapshots** for assets produced by a source build, manual copy, or unzip. Keep outputs in an allowed path such as `/usr/lib/cups`, `/usr/share/cups`, `/usr/share/ppd`, `/usr/share/foomatic`, `/lib/firmware`, `/usr/lib/firmware`, or `/usr/lib/<multiarch>`. Autoconf defaults `--libdir` to the bare `/usr/lib`. Explicitly move shared libraries to the multiarch directory so they are captured.
   - `driver-install` fails when both the file manifest and the package archive are empty.
5. **Register the driver** in `driver_registry.go::driversRegistry`. Set `Name` to `<name>`. Set `DisplayName`, `Description`, `Arch` (`{"all"}` or a list of Debian architecture names), `NeedCompile`, and `MatchPatterns`. The UI uses `Arch` to enable installation, and `/drivers/detect` uses the case-insensitive `(?i)` regular expressions to recommend a driver. Generic drivers can leave `MatchPatterns` empty.
6. **Use the repository's GitHub Releases mirror** under the `cups-driver` tag for third-party downloads. Vendor endpoints from Epson and Sharp often return HTTP 403 in CI because of user-agent and TLS-fingerprint checks. Exit with a nonzero status promptly when a download fails.
7. **Verify installation and removal.** Run `driver-list` inside the container and check that the available driver shows `Restore: package/files/hybrid` as appropriate. Run `driver-install <name>`. Inspect `/opt/cups-drivers/data/<name>/manifest.txt` and confirm that it excludes system files, especially CUPS's backends and filters. After `driver-remove <name>`, confirm `lpstat -r` succeeds and `/usr/lib/cups/backend/*` remains present.
8. **Verify restoration with a fresh container.**
   ```bash
   docker rm -f <ct> && docker run -d --name <ct> -v "$PWD/.drivers:/opt/cups-drivers/data" <image>
   ```
   `docker restart` preserves the writable layer and gives a false pass when the original installation is still present. In the new container, confirm that the filter, shared libraries, PPD, and vendor data directory are restored. Check `ldd` for missing dependencies.

## Debugging and Testing

### Backend Tests

```bash
go test ./...                # Run all tests.
go test -cover ./...         # Include coverage.
go vet ./...                 # Run static checks.
```

> The repository mainly relies on manual tests and logs. `test/` contains temporary test cases outside CI. Add a `_test.go` file when introducing a core module.

### Frontend Checks

```bash
cd frontend
bun run build                # Check the production build and syntax.
bun run dev                  # Start local development.
```

### Inspect the Database

```bash
sqlite3 data/cups-web.db
.tables
SELECT * FROM users;
SELECT id, filename, status, is_duplex, is_color, created_at FROM print_jobs ORDER BY id DESC LIMIT 20;
SELECT * FROM settings;
```

## Code Style

### Go

- Follow Go naming conventions and format with `gofmt`.
- Use `appStore.WithTx(ctx, readOnly, func(tx) error { ... })` to set transaction boundaries in handlers.
- Use `writeJSONError(w, status, msg)` for errors and `writeJSON(w, v)` for successful responses.
- Store relative file paths after `filepath.ToSlash`. Use `filepath.FromSlash` and `filepath.Join(uploadDir, ...)` to resolve them on access.

### Vue

- Use single-file components and the `<script setup>` Composition API.
- Prefer `@nuxt/ui` components. The global prefix is `U` in `vite.config.js`.
- Use Tailwind utilities and Nuxt UI semantic classes such as `bg-default` and `text-muted` for light and dark themes.
- `cachedSession` in `router/index.js` keeps routes from refetching `/api/session` on every navigation.

### Git Commits

- Write commit messages in Chinese using a prefix such as `feat:`, `fix:`, or `refactor:` followed by a brief description.
- Omit `Co-Authored-By` and AI attribution lines.

## References

- [CUPS documentation](https://www.cups.org/documentation.html)
- [IPP specifications](https://www.pwg.org/ipp/)
- [Nuxt UI v4](https://ui.nuxt.com/)
- [Tailwind CSS v4](https://tailwindcss.com/)
- [Vue 3 documentation](https://vuejs.org/)
- [ofdrw](https://github.com/ofdrw/ofdrw)
