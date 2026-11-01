# AGENTS.md

Steady is a minimal single-page habit tracker: one Go binary that serves an embedded TypeScript frontend and stores its data in SQLite. The visual and behavioral spec is `docs/spec.md`. Follow it exactly and add nothing it does not describe.

## Project and architecture

- `cmd/steady`: entry point. Reads config, wires `internal/app`, serves HTTP, shuts down gracefully.
- `internal/config`: env config. `STEADY_ADDR` (default `127.0.0.1:8770`), `STEADY_DB` (default `data/steady.db`), `STEADY_LOG_LEVEL` (default `info`), `STEADY_ALLOWED_HOSTS` (comma-separated extra Host values for a reverse proxy, default empty).
- `internal/domain`: habits, day keys (`YYYY-MM-DD`), settings (view, accent palette) and their validation. No infrastructure imports.
- `internal/usecase`: application services. Defines the repository port it needs.
- `internal/repository/sqlite`: SQLite adapter. Embedded goose migrations, sqlc-generated queries (`sqlcdb/`, from `queries.sql` and `sqlc.yaml`), first-run seed.
- `internal/handler/httpapi`: JSON API and static file serving on the stdlib `http.ServeMux`.
- `internal/app`: wiring.
- `web/`: frontend (`index.html`, `styles.css`, `src/{main,store,dates,render}.ts`), compiled by `tsc` into `web/dist/`, embedded with `go:embed`.
- `deploy/`: server kit (systemd unit, pull-based autodeploy timer, install script).

Data model: `habits(id, name, position, deleted_at)`, `habit_days(habit_id, day)`, `settings(key, value)`. Day keys are built by the browser in its local timezone; the server only validates and stores them. Deleting a habit soft-deletes it, so Undo restores it at its original position. Soft-deleted rows are purged 24 h after deletion (on startup and on each delete).

Generated code is committed: `web/dist/` (so `go build` and `go install` need only the Go toolchain) and the sqlc output. CI fails if either is stale.

## Development commands

- `make check`: everything CI runs (lint, tests, generated-code freshness, deploy script checks, text checks).
- `make test`: `go test -race ./...` plus `npm test` in `web/` (Node's built-in test runner over the compiled date helpers).
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
- The UI updates optimistically and syncs each change to the API. On an API error it reloads state from the server once the queued requests have finished.
- Source files contain no literal em dash (U+2014). Where the UI needs one, build it with `String.fromCharCode(0x2014)`.
- Commits: Conventional Commits, imperative mood, subject at most 72 characters, signed. Trunk-based: short-lived branches off `main`.
- Public-ready: English only; no personal hosts, IPs, domains or home paths anywhere in the repo.

## Setup, deployment and data safety

- The server binds to `127.0.0.1` and has no authentication: it is a single-user app. Reach it through an SSH tunnel or a reverse proxy that adds auth. Never bind it to a public interface as is.
- Mutating API routes are protected by `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`).
- The database is one SQLite file in WAL mode. Back it up with `sqlite3 <db> ".backup <file>"` or `VACUUM INTO`; never copy the live file without its `-wal`.
- Deployment is pull-based (`deploy/README.md`): a systemd timer fetches `main`, verifies the head commit signature and green CI, builds, restarts and rolls back if `/healthz` fails.
