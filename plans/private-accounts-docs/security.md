# Private account documentation edits: SECURITY.md

Base: `34a5c2d`. Apply and check with the canonical prose-edit helper; change only this file.

**E1.** (lines 9-11)

````md E1-old
- Steady is a single-user app with no authentication. Anyone who can reach the port can read and change all data, so the default listen address is `127.0.0.1` and it should stay on a loopback address.
- The server checks the `Host` header against an allowlist (DNS rebinding) and rejects cross-origin browser requests that change data.
- Every response carries a Content-Security-Policy and other hardening headers.
````

````md E1-new
- Steady has private username/password accounts and no public registration. Account creation, legacy ownership bootstrap and password resets are terminal-only server CLI operations. No remote account management or recovery flow is exposed.
- Passwords use maintained Argon2id with the RFC 9106 second recommended profile (64 MiB, 3 iterations, 4 lanes, 16-byte random salt, 32-byte key). Passwords and confirmation are hidden interactive input; credentials and session tokens must never be printed or logged.
- Opaque sessions contain 32 random bytes; only SHA-256 hashes are persisted. They expire 30 days after login, without renewal. Logout revokes the current session; password reset atomically revokes all sessions, including protection against a concurrent old-password login.
- Every habit/settings read and mutation derives identity from a validated session and scopes ownership in repository queries. Private API requests also require an `X-Steady-Session` binding matching the validated cookie in constant time. `/api/session` returns `{username,binding}`; the binding is SHA-256 hex of UTF-8 `steady-request:` plus the raw token, a request-context marker with no standalone authentication power. Missing or stale bindings receive 401, preventing queued requests from using another account cookie after a tab switches accounts. Native `BroadcastChannel('steady-auth')` clears other tabs' state/DOM/pending work on auth transitions without sharing identity or credentials. Legacy data is inaccessible before explicit transactional owner bootstrap; retries never reassign it. New accounts are empty.
- The session cookie is `HttpOnly`, `SameSite=Strict`, `Path=/`, with Secure enabled by default. Keep the backend on loopback behind an HTTPS reverse proxy. Explicit `STEADY_COOKIE_SECURE=false` is only for local HTTP or an SSH tunnel; forwarded headers cannot weaken it.
- Login has bounded per-IP/global rate limiting and concurrent hashing. Unknown usernames and incorrect passwords receive generic invalid-credentials errors. Only the transport peer IP is trusted, never forwarded client-IP headers.
- The server checks the `Host` header against an allowlist (DNS rebinding) and rejects cross-origin browser requests that change data, including login/logout. `/healthz` remains public for deployment checks.
- Every response carries a Content-Security-Policy and other hardening headers; auth/private data are `no-store`.
- Protect the SQLite database and backups: neither is encrypted and both contain authentication material. The private-account schema upgrade requires a stopped service and explicit bootstrap. Never restore an unauthenticated binary onto the migrated database; recover the matching protected pre-upgrade backup and binary offline. See `deploy/README.md`.
````
