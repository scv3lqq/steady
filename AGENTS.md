# AGENTS.md

Steady is a minimal single-page habit tracker: one Go binary that serves an embedded TypeScript frontend and stores its data in SQLite. The visual and behavioral spec is `docs/spec.md`. Follow it exactly and add nothing it does not describe.

## Project and architecture

- `cmd/steady`: entry point. Reads config, wires `internal/app`, serves HTTP, shuts down gracefully, and exposes terminal-only `account bootstrap`, `account create` and `account reset-password` commands.
- `internal/config`: env config. `STEADY_ADDR` (default `127.0.0.1:8770`), `STEADY_DB` (default `data/steady.db`), `STEADY_LOG_LEVEL` (default `info`), `STEADY_ALLOWED_HOSTS` (comma-separated extra Host values for a reverse proxy, default empty), `STEADY_COOKIE_SECURE` (default `true`, explicit `false` only for local HTTP).
- `internal/domain`: habits, day keys (`YYYY-MM-DD`), settings (view, accent palette) and their validation. No infrastructure imports.
- `internal/usecase`: application services. Defines the repository port it needs.
- `internal/repository/sqlite`: SQLite adapter. Embedded goose migrations, sqlc-generated queries (`sqlcdb/`, from `queries.sql` and `sqlc.yaml`), account ownership, transactional legacy bootstrap and persistent hashed sessions. No first-run demo seed.
- `internal/handler/httpapi`: JSON API and static file serving on the stdlib `http.ServeMux`.
- `internal/app`: wiring.
- `web/`: frontend (`index.html`, `styles.css`, `src/{main,store,dates,render}.ts`), compiled by `tsc` into `web/dist/`, embedded with `go:embed`.
- `deploy/`: server kit (systemd unit, pull-based autodeploy timer, install script).

Data model: users own habits and settings; habits retain day marks, position and soft-delete state. Sessions persist only token hashes and expiry. Day keys are built by the browser in its local timezone; the server only validates and stores them. Deleting a habit soft-deletes it, so Undo restores it at its original position. Ordinary owned soft-deleted rows are purged 24 h after deletion (on startup and on each delete). Migration marks existing soft-deleted rows `legacy_preserved`; purge excludes both unowned and preserved rows, including after bootstrap and restart. Restore clears preservation, so a later explicit delete uses ordinary 24-hour retention. New accounts have empty habits and default settings. Every repository read and mutation enforces session-derived user ownership, including day marks, delete and Undo.

Generated code is committed: `web/dist/` (so `go build` and `go install` need only the Go toolchain) and the sqlc output. CI fails if either is stale.

## Development commands

- `make check`: everything CI runs (lint, tests, generated-code freshness, deploy script checks, text checks).
- `make test`: `go test -race ./...` plus `npm test` in `web/` (Node's built-in test runner over compiled frontend modules).
- `make check-text`: fails on a literal em dash in any tracked or new file (`scripts/check_text.py`).
- `make lint`: golangci-lint v2.
- `make web`: install and build the frontend in `web/`.
- `make generate`: sqlc (pinned version via `go run`, `CGO_ENABLED=0`).
- `make run`: build the frontend and run the server against `data/steady.db`.
- `make build` / `make build-linux`: static binaries (`CGO_ENABLED=0`).

## Conventions

- Go: zerolog for logs; wrap errors as `fmt.Errorf("op: %w", err)`; table-driven tests; no global mutable state; pure-Go SQLite driver (`modernc.org/sqlite`), no CGO.
- Database access goes through sqlc-generated code and goose migrations only.
- Frontend: TypeScript `strict`, no runtime dependencies, `typescript` is the only devDependency. One `render()` redraws from state; event delegation via `data-action`, `data-id`, `data-date`; user text goes through `textContent`, never `innerHTML`. Never rebuild the name input that has focus.
- The UI updates optimistically and syncs each change to the API. On an API error it reloads state from the server once the queued requests have finished. Authentication boundaries clear private state, pending names, timers and Undo, and guard queued requests and late responses by session generation. Every queued private request captures `X-Steady-Session` from `/api/session` (`{username,binding}`); server validation requires both cookie and matching binding. The binding is a request-context marker, not a reusable session. Native `BroadcastChannel('steady-auth')` clears other tabs' private state/DOM/pending work on auth transitions and carries no identity or credentials. Never retain a password in state or browser storage.
- Source files contain no literal em dash (U+2014). Where the UI needs one, build it with `String.fromCharCode(0x2014)`.
- Commits: Conventional Commits, imperative mood, subject at most 72 characters, signed. Trunk-based: short-lived branches off `main`.
- Public-ready: English only; no personal hosts, IPs, domains or home paths anywhere in the repo.

## Setup, deployment and data safety

- The server binds to `127.0.0.1` and authenticates private accounts. Use an HTTPS reverse proxy with `STEADY_COOKIE_SECURE=true` (the default), or an SSH tunnel/local loopback HTTP with explicit `false`; never infer cookie security from forwarded headers.
- All habit/settings operations require validated opaque sessions and server-derived ownership. Mutating API routes, including login/logout, retain `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`). `/healthz` remains public.
- Account management is server-side CLI only with hidden terminal password/confirmation prompts. No password arguments, environment variables, piped input or logging. No public signup, remote account creation, invitations, admin UI, email, OAuth or recovery flow. Follow `docs/spec.md` for password hashing, session lifetime, username rules and bounded login rate limits.
- The database is one SQLite file in WAL mode. Back it up with `sqlite3 <db> ".backup <file>"` or `VACUUM INTO`; never copy the live file without its `-wal`. Backups contain authentication material and require protected storage.
- Deployment is pull-based (`deploy/README.md`): a systemd timer fetches `main`, verifies the head commit signature and green CI, builds, restarts and rolls back if `/healthz` fails. The first account upgrade requires auto-deploy disabled, the service stopped, a protected consistent backup with matching old binary and explicit owner bootstrap. Never run an old unauthenticated binary against a migrated database or destructively downgrade ownership; recover offline using its matching backup and binary.
