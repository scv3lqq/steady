# Private account documentation edits: AGENTS.md

Base: `34a5c2d`. Apply and check with the canonical prose-edit helper; change only this file.

**E1.** (lines 7-8)

````md E1-old
- `cmd/steady`: entry point. Reads config, wires `internal/app`, serves HTTP, shuts down gracefully.
- `internal/config`: env config. `STEADY_ADDR` (default `127.0.0.1:8770`), `STEADY_DB` (default `data/steady.db`), `STEADY_LOG_LEVEL` (default `info`), `STEADY_ALLOWED_HOSTS` (comma-separated extra Host values for a reverse proxy, default empty).
````

````md E1-new
- `cmd/steady`: entry point. Reads config, wires `internal/app`, serves HTTP, shuts down gracefully, and exposes terminal-only `account bootstrap`, `account create` and `account reset-password` commands.
- `internal/config`: env config. `STEADY_ADDR` (default `127.0.0.1:8770`), `STEADY_DB` (default `data/steady.db`), `STEADY_LOG_LEVEL` (default `info`), `STEADY_ALLOWED_HOSTS` (comma-separated extra Host values for a reverse proxy, default empty), `STEADY_COOKIE_SECURE` (default `true`, explicit `false` only for local HTTP).
````

**E2.** (lines 11-11)

````md E2-old
- `internal/repository/sqlite`: SQLite adapter. Embedded goose migrations, sqlc-generated queries (`sqlcdb/`, from `queries.sql` and `sqlc.yaml`), first-run seed.
````

````md E2-new
- `internal/repository/sqlite`: SQLite adapter. Embedded goose migrations, sqlc-generated queries (`sqlcdb/`, from `queries.sql` and `sqlc.yaml`), account ownership, transactional legacy bootstrap and persistent hashed sessions. No first-run demo seed.
````

**E3.** (lines 17-17)

````md E3-old
Data model: `habits(id, name, position, deleted_at)`, `habit_days(habit_id, day)`, `settings(key, value)`. Day keys are built by the browser in its local timezone; the server only validates and stores them. Deleting a habit soft-deletes it, so Undo restores it at its original position. Soft-deleted rows are purged 24 h after deletion (on startup and on each delete).
````

````md E3-new
Data model: users own habits and settings; habits retain day marks, position and soft-delete state. Sessions persist only token hashes and expiry. Day keys are built by the browser in its local timezone; the server only validates and stores them. Deleting a habit soft-deletes it, so Undo restores it at its original position. Ordinary owned soft-deleted rows are purged 24 h after deletion (on startup and on each delete). Migration marks existing soft-deleted rows `legacy_preserved`; purge excludes both unowned and preserved rows, including after bootstrap and restart. Restore clears preservation, so a later explicit delete uses ordinary 24-hour retention. New accounts have empty habits and default settings. Every repository read and mutation enforces session-derived user ownership, including day marks, delete and Undo.
````

**E4.** (lines 24-24)

````md E4-old
- `make test`: `go test -race ./...` plus `npm test` in `web/` (Node's built-in test runner over the compiled date helpers).
````

````md E4-new
- `make test`: `go test -race ./...` plus `npm test` in `web/` (Node's built-in test runner over compiled frontend modules).
````

**E5.** (lines 37-37)

````md E5-old
- The UI updates optimistically and syncs each change to the API. On an API error it reloads state from the server once the queued requests have finished.
````

````md E5-new
- The UI updates optimistically and syncs each change to the API. On an API error it reloads state from the server once the queued requests have finished. Authentication boundaries clear private state, pending names, timers and Undo, and guard queued requests and late responses by session generation. Every queued private request captures `X-Steady-Session` from `/api/session` (`{username,binding}`); server validation requires both cookie and matching binding. The binding is a request-context marker, not a reusable session. Native `BroadcastChannel('steady-auth')` clears other tabs' private state/DOM/pending work on auth transitions and carries no identity or credentials. Never retain a password in state or browser storage.
````

**E6.** (lines 44-47)

````md E6-old
- The server binds to `127.0.0.1` and has no authentication: it is a single-user app. Reach it through an SSH tunnel or a reverse proxy that adds auth. Never bind it to a public interface as is.
- Mutating API routes are protected by `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`).
- The database is one SQLite file in WAL mode. Back it up with `sqlite3 <db> ".backup <file>"` or `VACUUM INTO`; never copy the live file without its `-wal`.
- Deployment is pull-based (`deploy/README.md`): a systemd timer fetches `main`, verifies the head commit signature and green CI, builds, restarts and rolls back if `/healthz` fails.
````

````md E6-new
- The server binds to `127.0.0.1` and authenticates private accounts. Use an HTTPS reverse proxy with `STEADY_COOKIE_SECURE=true` (the default), or an SSH tunnel/local loopback HTTP with explicit `false`; never infer cookie security from forwarded headers.
- All habit/settings operations require validated opaque sessions and server-derived ownership. Mutating API routes, including login/logout, retain `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`). `/healthz` remains public.
- Account management is server-side CLI only with hidden terminal password/confirmation prompts. No password arguments, environment variables, piped input or logging. No public signup, remote account creation, invitations, admin UI, email, OAuth or recovery flow. Follow `docs/spec.md` for password hashing, session lifetime, username rules and bounded login rate limits.
- The database is one SQLite file in WAL mode. Back it up with `sqlite3 <db> ".backup <file>"` or `VACUUM INTO`; never copy the live file without its `-wal`. Backups contain authentication material and require protected storage.
- Deployment is pull-based (`deploy/README.md`): a systemd timer fetches `main`, verifies the head commit signature and green CI, builds, restarts and rolls back if `/healthz` fails. The first account upgrade requires auto-deploy disabled, the service stopped, a protected consistent backup with matching old binary and explicit owner bootstrap. Never run an old unauthenticated binary against a migrated database or destructively downgrade ownership; recover offline using its matching backup and binary.
````
