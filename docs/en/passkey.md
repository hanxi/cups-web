# Passkey sign-in

[简体中文](../passkey.md) · English

## Prerequisites

- **HTTPS and a domain name**: WebAuthn needs a secure context and the RP ID cannot be an IP. `localhost` is the only exception.
- Sign in with a password first, then add a passkey on the Passkeys page.

## Environment variables

| Variable | Description |
| --- | --- |
| `PASSKEY_ENABLED` | Master switch, off by default. When off, no passkey routes are registered and the UI hides the entry points |
| `PASSKEY_RP_ID` | Domain such as `print.example.com`. Required when enabled |
| `PASSKEY_RP_ORIGINS` | Full origins, comma-separated. Falls back to `TRUSTED_ORIGINS` |
| `PASSKEY_RP_NAME` | Display name, default `CUPS Web` |
| `DISABLE_PASSWORD_LOGIN` | Disable password sign-in. **Takes effect only when at least one admin has a passkey**, otherwise it is ignored to avoid lockout |

Invalid configuration (missing RP ID, IP as RP ID, missing origin) stops startup with a message.

## Design notes

- Discoverable credentials: registration uses `ResidentKey: required` and sign-in uses `BeginDiscoverableLogin`. The browser lists accounts after one click, so no username is needed.
- After verification the server uses the same `auth.SetSession` and CSRF cookie path as password sign-in.
- Begin-to-finish challenges live in process memory: 5 minute TTL, single use, 2000 entry cap, linked by the `X-Passkey-Session` header.
- Login endpoints reuse the per-IP login rate limiter. Every failure returns the same `passkey authentication failed`.
- A clone warning (sign count regression) rejects the login. Guest cannot register or use passkeys.
- Registration and deletion reject the API key channel. Limit 10 per user.
- Register at least two passkeys and keep password sign-in as a fallback.
