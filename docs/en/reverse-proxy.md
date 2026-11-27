# Reverse Proxies and HTTPS

This guide covers origin checks, cookies, and login failures when cups-web runs behind a reverse proxy. The corresponding code lives in `internal/middleware/security.go` and `internal/auth/session.go`.

## Troubleshooting at a Glance

| Symptom | Cause | Fix |
| --- | --- | --- |
| Login returns HTTP 403 | The origin check classified the request as cross-origin | See [Diagnose the Two Types of HTTP 403](#diagnose-the-two-types-of-http-403) |
| Login succeeds and immediately returns to the login page | The cookie is marked `Secure` while the browser uses HTTP | Set `COOKIE_SECURE=false` |
| The UI reports an unreachable endpoint (404 or 405) | The proxy does not route `/api` to cups-web | Check `location` and `proxy_pass` |
| Large uploads return HTTP 413 | The proxy limits request body size | Set Nginx `client_max_body_size 200m;` |
| Conversion or driver installation disconnects | The proxy read times out | Set Nginx `proxy_read_timeout 300s;` |
| The page is blank and static files return HTTP 404 | The proxy serves cups-web under a URL subpath | See [Serving Under a URL Subpath](#serving-under-a-url-subpath) |

## Why a Proxy Error Looked Like an Incorrect Password

A common failure was that login worked on the LAN, while a reverse proxy made it appear that the same credentials were incorrect. Three parts of the request contributed. [Issue #99](https://github.com/hanxi/cups-web/issues/99)

1. **Server.** `middleware.CrossOriginProtection()` wraps Go's `http.CrossOriginProtection`. When `Sec-Fetch-Site` is absent, Go compares the host of `Origin` with `r.Host`.
2. **Proxy.** Without `proxy_set_header Host $host;`, Nginx defaults to forwarding `$proxy_host`, such as `localhost:1180`. The public host in `Origin` differs from the upstream host in `r.Host`, causing POST requests across the app to receive HTTP 403.
3. **Frontend.** The old `LoginView.vue` treated a login error with an unparseable body as an incorrect username or password. Go returned the HTTP 403 as `text/plain`, so the proxy problem appeared as a credentials error.

The backend now checks `X-Forwarded-Host` in the fallback case and returns JSON for rejections. The frontend selects its error message based on the HTTP status.

### Diagnose the Two Types of HTTP 403

The structured response includes `code: "cross_origin_blocked"` and explains the rejection. The backend logs the fields used in the decision:

```text
[cross-origin] 拒绝请求: method=POST path=/api/login host="localhost:1180" \
  origin="http://print.example.com" sec-fetch-site="" \
  x-forwarded-host="" x-forwarded-proto=""
```

**Missing `Sec-Fetch-Site` and mismatched `Origin` and `Host`.** The proxy did not forward the public host. Forward both headers:

```nginx
proxy_set_header Host             $host;
proxy_set_header X-Forwarded-Host $host;
```

Matching the hostname in `X-Forwarded-Host` to `Origin` is sufficient. The backend first checks the full host and then tries the hostname without the port. Nginx's `$host` excludes the port, while a browser includes a nondefault port in `Origin`. This fallback handles lost port information without requiring an environment variable.

**`Sec-Fetch-Site: same-site` or `cross-site`.** The browser classified the request. Changing the upstream host headers does not change that classification. Common causes include:

- The page loads over HTTP, while the API request upgrades to HTTPS because of HTTPS-First, HSTS, or a proxy redirect on `/api`.
- A page on `a.example.com` makes API requests to `b.example.com`.

Serve the page and API with the same protocol and hostname. If the deployment requires a cross-origin request, allow the specific origins:

```yaml
environment:
  - TRUSTED_ORIGINS=https://print.example.com,http://print.example.com
```

> Use comma-separated, complete origins, including the protocol. Leave out paths and trailing slashes.

## Why the Forwarded Host Fallback Is Safe Here

A client can set `X-Forwarded-Host`. The fallback is restricted to requests without `Sec-Fetch-Site`, which preserves the browser protections:

- Browsers send `Sec-Fetch-Site` and mark cross-origin requests as `cross-site` or `same-site`. The Go standard library rejects those requests before they reach this fallback. Tests in `internal/middleware/security_test.go` preserve this boundary.
- A script that controls request headers can already omit both `Sec-Fetch-Site` and `Origin`, which Go's origin protection deliberately permits for non-browser clients. The double-submit cookie enforced by `ValidateCSRF` continues to protect state-changing endpoints. See `internal/middleware/csrf.go`.

This fallback corrects false rejections for legitimate proxied requests.

## The Cookie `Secure` Attribute

`COOKIE_SECURE` controls cookies as follows:

| Value | Behavior |
| --- | --- |
| Unset or `auto` | Evaluates each request. Uses `r.TLS` for direct HTTPS and the first `X-Forwarded-Proto` value when TLS terminates at the proxy |
| `true` | Always adds `Secure` |
| `false` | Always omits `Secure` |

`auto` supports plain HTTP on a private LAN and marks HTTPS cookies as secure. Forward `X-Forwarded-Proto $scheme` so TLS termination works as expected.

If a proxy reports `X-Forwarded-Proto: https` while the browser is using HTTP, the browser drops the `Secure` cookie. The login endpoint then returns HTTP 200 and the UI immediately returns to the login page. For a deliberate HTTP deployment, set `COOKIE_SECURE=false`.

## Caddy

Caddy's `reverse_proxy` sets `Host`, `X-Forwarded-Host`, `X-Forwarded-Proto`, and `X-Forwarded-For` by default.

```caddyfile
print.example.com {
	reverse_proxy localhost:1180
	request_body {
		max_size 200MB
	}
}
```

## Traefik

Traefik forwards the `X-Forwarded-*` headers and preserves the original `Host` by default. If the router uses `stripPrefix` for cups-web, consult the URL subpath guidance below.

## Serving Under a URL Subpath

Use a dedicated hostname, subdomain, or port. The application currently expects to run at the root of a hostname. `frontend/vite.config.js` leaves `base` at its default, so the built UI references absolute paths such as `/assets/...`. Mounting the app under `https://example.com/cups/` causes missing static assets and a blank page.

## Login Rate Limits and Client IPs

`clientIP()` in `cmd/server/login_limiter.go` uses the first `X-Forwarded-For` value as the IP in the `IP|username` rate-limit key. Five failed attempts lock that key for 15 minutes.

The application trusts the forwarded header without a trusted-proxy allowlist. Behind a correctly configured proxy, the rate limiter uses the real client IP. Configure the proxy to supply `X-Forwarded-For` as in the Nginx example.

Publish the proxy's port and block direct public access to cups-web. Otherwise untrusted clients can control the IP used in the rate-limit key. The default Compose file uses host networking and has no port mapping. Set `LISTEN_ADDR=127.0.0.1:1180` to bind the web service to loopback. Use a firewall or the `Listen` and `<Location>` directives in `cupsd.conf` to restrict the CUPS listener on port 631. [Issue #107](https://github.com/hanxi/cups-web/issues/107)

## Related Documentation

- [Authentication and security](../../AGENTS.en.md#-authentication-and-security) covers the authentication chain and cookie conventions.
- [Docker build and networking](docker-build.md) explains the Compose networking choices.
