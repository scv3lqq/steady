# Steady: design spec

Build **Steady**, a minimal single-page habit tracker. The design below is final: follow it exactly and don't add features, sections or decoration that aren't described.

## Deviations from the original prompt
- Storage: habits, the chosen view and the accent live in SQLite behind the Go server's JSON API (see AGENTS.md), not in localStorage; "survive a reload" means against the same server.
- Serving: the Go binary serves `web/index.html`, `web/styles.css` and `web/dist/` (embedded); `npm run build` still compiles the TypeScript; `make run` replaces "any static server".
- Frontend files live under `web/`; `src/store.ts` holds state plus API sync instead of localStorage access.
- The two em dashes the UI shows (zero streak, aria-label separator) are built in code with `String.fromCharCode(0x2014)`; this document names them instead of printing them.

## Stack (strict)
- **No runtime dependencies** and no frameworks: no React, Vue, Tailwind or UI kits.
- TypeScript (`strict: true`), compiled by `tsc` to ES modules (`target: ES2022`, `module: ES2022`). The only devDependency is `typescript`.
- Files: `index.html`, `styles.css`, `src/main.ts`, `src/store.ts` (state + API sync), `src/dates.ts` (date helpers), `src/render.ts` (DOM). Output goes to `dist/`.
- `package.json` scripts: `build` (`tsc`), `dev` (`tsc --watch`), and `test` (Node's built-in test runner over compiled modules). The Go server serves the app and JSON API.
- Rendering: plain DOM with one `render()` function that redraws the list from state. Use **event delegation** on the root (`data-action`, `data-id`, `data-date`). Don't use `innerHTML` with user text; use `textContent`.
- CSS: custom properties for tokens, flex/grid with `gap`, no preprocessors.

## Visual tokens
- Font: **Onest** 400/500/600 (Google Fonts), fallback `system-ui, sans-serif`. Use `font-variant-numeric: tabular-nums` on all numbers.
- Dark theme only:
  - `--bg #161513`, `--surface #1F1D1A`, `--line #2C2A26`, `--hover #26241F`
  - `--ink #ECE7DC` (main text), `--muted #9C968A`, `--faint #847E72`
  - `--empty #3A3732` (empty dot), `--cell-border #55514A` (empty week square)
  - `--accent` is chosen by the user, default `#4FA872`
- Accent palette (10 colors): `#4FA872 #8FBF4A #3FB8B0 #5B8CEB #B07FE0 #E07BB0 #D96A6A #E08A45 #E3C24F #ECE7DC`.
- Checkmark on a filled square uses `--bg` (dark on a light accent).
- Radii: week square 9px, buttons 8px, segmented control 11px, toast 12px. Use no shadows except on the toast.

## Layout
A centered column, `max-width: 600px`, padding `56px 20px 96px`, vertical gap 36px between blocks. It must reflow down to 320px.

1. **Header**
   - Row: on the left the wordmark **"Steady"** (15px/600); on the right the date `Sunday, November 1` (14px, muted, `nowrap`), a 32×32 button holding a 14px accent-colored circle, and a quiet **Log out** button. The accent button toggles the palette. Allow the account control to wrap onto a separate line at narrow widths without horizontal scroll.
   - Palette (when open): right-aligned row of 32×32 buttons, each with a 22px circle. The selected one gets a ring: `box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px <color>`. Use `role="radiogroup"`. Choosing a color persists it; the palette stays open until toggled.
   - H1 at 40px/500, letter-spacing −0.03em: `"X of N today"`, or `"All done today"` when X = N, or `"Let’s start"` when there are no habits.
   - Progress bar: 4px tall, track `--line`, fill `--accent`, width = done today / total, 0.3s width transition.

2. **Toolbar row** (space-between)
   - Segmented control on the left: **Week / Month / Year**, inside a `--surface` container with a 1px `--line` border and 3px padding. Each tab is 34px tall; the active tab has `--empty` background and `--ink` text, inactive tabs are muted. Use `role="tablist"`. Persist the chosen view.
   - A quiet text button on the right, `+ Habit` (14px, muted, transparent; hover background `--surface`, text `--ink`). It must **not** look like a primary CTA.
   - Clicking it shows an inline row under the toolbar: an underlined borderless input (17px, autofocus, placeholder `Name it, press Enter`, maxlength 60) plus a hint `Esc to cancel` (12px, faint). Enter adds the habit to the end and closes the row. Esc closes the row, and so does blur while the input is empty.

3. **List: Week view**
   - Header row: an empty flexible spacer (`flex: 1 1 140px`), then 7 columns 34px wide with weekday + date (`Mo`/`27`, 11px). Today's column is ink/600, the others faint. Then an 84px spacer.
   - Each habit row is a wrapping flex row with a 1px `--line` bottom border and 14px vertical padding:
     - Inline-editable name input (17px, transparent, 1px ink underline on focus). Saving happens on input; an empty name on blur becomes `Untitled`.
     - 7 squares, 34×34, 6px gap. Done = accent fill + accent border + check icon. Today, not done = `--surface` fill + 1.5px ink border. Past, not done = transparent + `--cell-border`. Clicking toggles the day.
     - Right side (84px): the streak (`5 days`, ink when > 0, an em dash (U+2014), faint, when 0), then a × delete button (32×32, faint; hover `--hover` / ink).

4. **List: Month view (last 30 days)**
   - Each habit is a block with a top border and 16px/18px padding. Top line: name input, the count `18 of 30` (13px, muted), then ×.
   - Below it, a grid of `repeat(30, minmax(0,1fr))` with a 4px gap holding round dots (`aspect-ratio: 1`). Done = accent, otherwise `--empty`; today gets an inset 1.5px ink ring. Clicking toggles the day.
   - Caption row (11px, faint, space-between): the start date `Oct 3` and `today`.

5. **List: Year view (GitHub-style, 53 weeks)**
   - Same block header, with the count `214 days`.
   - Grid: `grid-template-columns: repeat(53, minmax(0,1fr)); grid-template-rows: repeat(7, auto); grid-auto-flow: column; gap: 3px`. Columns are weeks and rows run Monday→Sunday. The last column is the current week; cells after today are `visibility: hidden` and disabled. Squares are radius 2px; today gets an inset 1px ink ring. Clicking toggles the day.
   - Caption: `November 2025` … `today`.

6. **Empty state**: `Nothing here yet` (17px) plus `Tap “+ Habit” above. Starting small is fine.` (14px, muted).

7. **Footer**: `Tap a square or dot to mark a day. Missed one? You can still mark it later.` (13px, faint).

8. **Undo toast**
   - Deleting a habit happens immediately, with **no confirm dialog**. A toast is fixed at bottom center, 28px from the bottom, with `--ink` background, `--bg` text and `role="status"`.
   - It reads `“<name>” deleted` with an **Undo** button (`#D9D3C7`, hover `#C9C2B5`). Undo restores the habit at its original index.
   - The toast auto-hides after 5 seconds; a new delete resets the timer.

## Data and logic
```ts
type Habit = { id: string; name: string; done: Record<string, true> }; // keys: local "YYYY-MM-DD"
```
- Persistence: see Deviations (SQLite via the JSON API).
- Dates use the **local timezone**: build keys from `getFullYear/getMonth/getDate`, never `toISOString`.
- **Streak**: start at today if today is done, otherwise at yesterday, then count consecutive done days backwards.
- Format dates with `Intl.DateTimeFormat('en-US', …)`. Pluralize as `1 day` / `N days`.
- New accounts start with no habits, view `week` and accent `#4FA872`. Do not seed demo habits or history. Historical screenshots contain made-up demonstration data.
- Re-render on every change. Don't rebuild the input that has focus: patch in place, or skip the name input while it's focused, so the caret doesn't jump while the user types. Defer a remote view change until the focused habit name loses focus.
- Poll the authenticated account state every 3 seconds while visible (20 periodic reads per minute per visible tab, plus immediate wake checks), so remote changes appear within 5 seconds with a responsive server. Pause hidden tabs. Check immediately on visibility restoration, focus, online, pageshow and resume; coalesce concurrent triggers and bound reads to 4 seconds. Keep optimistic writes, pending names, input drafts and Undo. Reject snapshots started during writes or before a newer local edit, even after the write queue drains; within one account, the last successfully saved day operation wins.

## Accessibility
- Every interactive element is a real `<button>` or `<input>`.
- Day buttons have `aria-pressed` and an `aria-label` like `Read 10 pages, October 30`, a space, an em dash (U+2014), a space, `done`.
- Show `:focus-visible` rings: 2px ink outline with a 2px offset.
- Keep hit targets at least 32px; the year/month dots are the exception.
- Under `prefers-reduced-motion`, disable transitions.

## Private accounts

- One instance serves private username/password accounts. Each account owns its habits, day marks, ordering and settings. After login, retain every tracker interaction and visual token above.
- Show a minimal login screen in the centered column using the existing dark palette, typography, underlined inputs and quiet buttons: wordmark, heading **Log in**, visible **Username** and **Password** labels, and a **Log in** submit button. Inputs use `autocomplete="username"` and `autocomplete="current-password"`; the password input is `type="password"`. Announce errors through an alert/live region. Keyboard submission and visible focus rings work at 320px.
- While checking the session, show a neutral loading state without tracker data. A network failure, timeout, malformed response, temporary server error or proxy rejection keeps session checking retryable, never requires a fresh login by itself. An already authenticated page retains its session and resumes automatically when the server returns. Only Steady's JSON 401 authentication-required response confirms session loss; do not confuse a reverse proxy's login challenge with application logout. Failed login shows a generic invalid-credentials error; throttling has a clear retry message. Disable duplicate authentication submissions. Do not store the password in application state or browser storage; clear the field after submission.
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
- Frontend tests prove private state clearing, abandoned queues and late responses cannot contaminate a later login, including two-tab cookie changes, stale bindings and broadcast authentication transitions. Cover independent clients, conflicting marks, delayed snapshots after queued writes finish, pending names, hidden tabs, wake/reconnect, timer coalescing and transient outages. Real HTTP tests close and reopen the server and the same SQLite file, replay the identical cookie/binding without renewal, and verify logout/reset/absolute expiry using session discovery without a binding header. Independently verify a complete two-user workflow and run `make check`, including generated-code freshness.
