# Private account documentation edits: docs/spec.md

Base: `34a5c2d`. Apply and check with the canonical prose-edit helper; change only this file.

**E1.** (lines 14-15)

````md E1-old
- Files: `index.html`, `styles.css`, `src/main.ts`, `src/store.ts` (state + localStorage), `src/dates.ts` (date helpers), `src/render.ts` (DOM). Output goes to `dist/`.
- `package.json` scripts: `build` (`tsc`), `dev` (`tsc --watch`), and serve the folder with any static server.
````

````md E1-new
- Files: `index.html`, `styles.css`, `src/main.ts`, `src/store.ts` (state + API sync), `src/dates.ts` (date helpers), `src/render.ts` (DOM). Output goes to `dist/`.
- `package.json` scripts: `build` (`tsc`), `dev` (`tsc --watch`), and `test` (Node's built-in test runner over compiled modules). The Go server serves the app and JSON API.
````

**E2.** (lines 34-34)

````md E2-old
   - Row: on the left the wordmark **"Steady"** (15px/600); on the right the date `Sunday, November 1` (14px, muted, `nowrap`) and a 32×32 button holding a 14px accent-colored circle. The button toggles the palette.
````

````md E2-new
   - Row: on the left the wordmark **"Steady"** (15px/600); on the right the date `Sunday, November 1` (14px, muted, `nowrap`), a 32×32 button holding a 14px accent-colored circle, and a quiet **Log out** button. The accent button toggles the palette. Allow the account control to wrap onto a separate line at narrow widths without horizontal scroll.
````

**E3.** (lines 78-78)

````md E3-old
- First-run seed: `Drink 2 L of water`, `Walk 30 minutes`, `Read 10 pages`, with some random history over the last year so the Month and Year views aren't empty.
````

````md E3-new
- New accounts start with no habits, view `week` and accent `#4FA872`. Do not seed demo habits or history. Historical screenshots contain made-up demonstration data.
````

**E4.** (lines 88-91)

````md E4-old
## Acceptance
- `npm run build` passes with zero TS errors, and the app opens from `index.html` with no console errors.
- Add, rename, delete + undo, toggling a day in all three views, switching views and changing the accent all work and survive a reload.
- There is no horizontal scroll at 320px wide.
````

````md E4-new
## Private accounts

- One instance serves private username/password accounts. Each account owns its habits, day marks, ordering and settings. After login, retain every tracker interaction and visual token above.
- Show a minimal login screen in the centered column using the existing dark palette, typography, underlined inputs and quiet buttons: wordmark, heading **Log in**, visible **Username** and **Password** labels, and a **Log in** submit button. Inputs use `autocomplete="username"` and `autocomplete="current-password"`; the password input is `type="password"`. Announce errors through an alert/live region. Keyboard submission and visible focus rings work at 320px.
- While checking the session, show a neutral loading state without tracker data. Failed login shows a generic invalid-credentials error; throttling has a clear retry message. Disable duplicate authentication submissions. Do not store the password in application state or browser storage; clear the field after submission.
- **Log out** revokes the current server session. Expiry or logout clears habits, settings, pending names, queued work, add-input content, palette and Undo toast. Use a session generation and cancellation to prevent a late response or queued mutation from reaching a later login. Each queued job captures its `X-Steady-Session` binding; native `BroadcastChannel('steady-auth')` broadcasts only an authentication-transition notification, with no identity or credentials, to clear other tabs' private state, DOM and pending work. Keep optimistic tracker updates and focused-name-input handling within the same session.
- There is no public signup, remote account creation, invitation system, admin UI, email integration, OAuth or password recovery flow. The operator uses only `steady account bootstrap <username>`, `steady account create <username>` and `steady account reset-password <username>` on the server. Password and confirmation are read from a hidden interactive terminal prompt, never arguments, environment variables, piped input, logs or terminal output.
- Usernames match case-sensitive lowercase ASCII `[a-z][a-z0-9_-]{2,31}`. Passwords are valid UTF-8, 12 to 128 bytes, with no truncation or extra composition rules. Duplicate account creation fails.
- Explicit bootstrap creates the owner's account, assigns all legacy habits, marks, ordering, settings and soft-deleted rows, and records the owner ID in a singleton completion marker, in one transaction. Legacy data remains inaccessible over HTTP before bootstrap and is never assigned to the first visitor. An exact same-username retry is a no-op without password/data changes; a different username is rejected. Migration marks existing soft-deleted rows `legacy_preserved`; purge excludes both unowned and preserved rows before and after bootstrap and across startup. Restore clears preservation; a later explicit delete follows ordinary 24-hour retention.
- Store passwords with maintained [`golang.org/x/crypto/argon2`](https://pkg.go.dev/golang.org/x/crypto/argon2) Argon2id, using the [RFC 9106 second recommended profile](https://www.rfc-editor.org/rfc/rfc9106.html#section-4): 65536 KiB, 3 iterations, 4 lanes, random 16-byte salt and 32-byte key in PHC format.
- Sessions use 32 cryptographically random opaque bytes; persist only SHA-256 token hashes. Expire after exactly 30 days from login without sliding renewal and persist across restarts. Logout revokes the current session. Password reset changes the password and revokes every session atomically; a concurrent login checked against the old hash must not issue a usable new session.
- Cookie: `steady_session`, `Path=/`, `HttpOnly`, `SameSite=Strict`, no Domain. `STEADY_COOKIE_SECURE=true` by default for HTTPS deployment; only explicit `false` permits local loopback HTTP or an SSH tunnel. Never infer Secure from forwarded headers.
- Preserve the Host allowlist and `http.CrossOriginProtection` for login/logout and every mutation. Require a validated session for every habit/settings operation and derive ownership on the server. All repository reads and mutations scope by user, including direct ID requests, day marks, delete and Undo. `GET /api/session` validates the cookie and returns `{username,binding}`. Binding is SHA-256 hex of UTF-8 `steady-request:` concatenated with the raw opaque token, a request-context marker that cannot authenticate without the cookie. Every private API request requires `X-Steady-Session` matching that validated cookie binding in constant time; absent or stale values return 401 even when a different account cookie is now active. Session discovery does not require this header. Another user's ID returns the same 404 as an unknown habit. Keep auth/private responses `no-store`; leave `/healthz` public.
- Login bounds: 5 attempts per `RemoteAddr` IP per 15 minutes, 30 globally per minute, 2 concurrent hashes, at most 1024 IP entries. Prune expired entries and fail closed at capacity; ignore forwarded IP headers. Use generic invalid-credentials responses and bounded 429 rejection.
- Retain goose migrations and sqlc with committed generated output, the single Go binary and SQLite architecture. Protect backups, disable auto-deploy and stop the service before upgrading, run explicit bootstrap, verify two accounts, then re-enable auto-deploy. Never roll an unauthenticated old binary onto a migrated database; restore the matching protected backup/binary offline instead. See `deploy/README.md` for exact steps.

## Acceptance
- `npm run build` passes with zero TS errors, and the app served by the Go binary has no console errors.
- Add, rename, delete + undo, toggling a day in all three views, switching views and changing the accent all work and survive a reload for the authenticated user.
- Login, logout, errors and tracker controls have no horizontal scroll at 320px wide.
- Automated tests cover legacy migration/bootstrap retries, CLI create/duplicates/reset, login success/failure, restart persistence, expiry, revocation, rate limits, unauthenticated access, cross-origin rejection and two-user isolation for every read/mutation route, including direct foreign habit IDs.
- Frontend tests prove private state clearing, abandoned queues and late responses cannot contaminate a later login, including two-tab cookie changes, stale bindings and broadcast authentication transitions. Independently verify a complete two-user workflow and run `make check`, including generated-code freshness.
````
