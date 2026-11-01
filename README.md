# Steady

Steady is a minimal single-page habit tracker. Mark each day of a habit as done on a week, month or year view, follow the streak and today's progress, and pick one of ten accent colors. Deleting a habit shows a toast with Undo. One Go binary serves the embedded TypeScript frontend, which has no runtime dependencies, and keeps all data in one SQLite file. It is a single-user app with no authentication, see [Security](#security).

| Week | Month | Year |
| --- | --- | --- |
| ![Steady week view with the three demo habits, 600 px wide](assets/screenshots/week.png) | ![Steady month view with the three demo habits, 600 px wide](assets/screenshots/month.png) | ![Steady year view with the three demo habits, 600 px wide](assets/screenshots/year.png) |

The same page at 320 px wide, the narrowest width it reflows to:

![Steady week view with the three demo habits, 320 px wide](assets/screenshots/week-320.png)

## Quick start

You need Go (the version in `go.mod` or newer).

```sh
go install github.com/scv3lqq/steady/cmd/steady@latest
steady
```

`go install` puts the binary in `$GOBIN`, or in `$(go env GOPATH)/bin` when `GOBIN` is not set. Open <http://127.0.0.1:8770>. The first run creates `data/steady.db` in the current directory and fills it with three demo habits.

From a clone, `make run` builds the frontend and starts the server with the default configuration. That also needs Node.js and npm.

## Configuration

Steady reads four environment variables. An unset or empty variable uses its default. An invalid `STEADY_ADDR` or `STEADY_LOG_LEVEL` stops the server at startup with a message that names the variable.

| Variable | Default | Meaning |
| --- | --- | --- |
| `STEADY_ADDR` | `127.0.0.1:8770` | Listen address as `host:port`. Keep it on a loopback address, see [Security](#security). |
| `STEADY_DB` | `data/steady.db` | Path of the SQLite file. A missing directory is created. |
| `STEADY_LOG_LEVEL` | `info` | One of `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`, in any letter case. Logs are JSON lines on stderr. |
| `STEADY_ALLOWED_HOSTS` | empty | Comma-separated extra `Host` values to accept, for a reverse proxy, for example `habits.example.com`. An entry without a port matches that name on any port, an entry with a port must equal the whole `Host` header. |

## API

The frontend talks to this JSON API. IDs are lowercase UUIDs made by the client, days are `YYYY-MM-DD` keys built in the browser's local time zone.

| Route | What it does |
| --- | --- |
| `GET /api/state` | Returns all habits with their done days, plus the saved `view` and `accent`. |
| `POST /api/habits` | Creates a habit from `{"id": "<uuid>", "name": "..."}` and answers 201; a taken id is 409. |
| `PATCH /api/habits/{id}` | Renames a habit from `{"name": "..."}`; `name` is required. |
| `DELETE /api/habits/{id}` | Soft-deletes a habit, so that it can be restored (204). |
| `POST /api/habits/{id}/restore` | Restores a deleted habit at its original position (204). |
| `PUT /api/habits/{id}/days/{day}` | Marks a day done (204, repeating it changes nothing). |
| `DELETE /api/habits/{id}/days/{day}` | Clears a day (204, repeating it changes nothing). |
| `PUT /api/settings` | Saves `view` (`week`, `month` or `year`) and `accent` (one of the ten palette colors); both are optional, at least one is required (204). |
| `GET /healthz` | Answers `{"status":"ok"}`, or 503 when the database does not answer. |
| `GET /` | The page (`index.html`). |
| `GET /styles.css` | The stylesheet. |
| `GET /dist/{file}` | A compiled script; any other name is 404. |

API errors are JSON, `{"error":"<text>"}`: 400 for invalid input (for example a name longer than 60 characters), 404 `habit not found`, 409 `habit already exists`, 403 when the Host or cross-origin check rejects the request, 500 `internal error`. A request body is at most 4096 bytes, one JSON object, no unknown fields. A blank habit name becomes `Untitled`.

## Security

Steady is a single-user app and has no authentication: anyone who can reach the port can read and change all data. It listens on `127.0.0.1` by default. Keep `STEADY_ADDR` on a loopback address and do not bind it to a public interface.

To reach it from another machine:

- Open an SSH tunnel, `ssh -N -L 8770:127.0.0.1:8770 <user>@<server>`, then browse to <http://127.0.0.1:8770> on your machine.
- Or put a reverse proxy in front that adds its own authentication and forwards the original `Host` header, and list the proxy's host name in `STEADY_ALLOWED_HOSTS`.

What the server enforces itself:

- A Host allowlist against DNS rebinding. It accepts `localhost`, `127.0.0.1` and `::1` on any port (so a tunnel on another local port works), the listen address (`STEADY_ADDR`) and the `STEADY_ALLOWED_HOSTS` entries. Any other `Host` gets `403 host not allowed`.
- `http.CrossOriginProtection` from the standard library. It rejects cross-origin browser requests that use an unsafe method, which covers every route that changes data, with `403 cross-origin request rejected`.
- A Content-Security-Policy on every response. The page may load only its own script, stylesheet and images, plus the Onest font from Google Fonts (the browser asks Google for the font), may connect only to itself, and may not be framed. Responses also carry `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` and `Cache-Control: no-store`.

## Data

Everything lives in one SQLite file, `data/steady.db` by default (`STEADY_DB`), in WAL mode: while the server runs, `-wal` and `-shm` files sit next to it. Schema migrations are embedded in the binary and run at startup. A new database is seeded once with three demo habits and a year of random history, and deleting them does not bring them back.

Back it up with `sqlite3 <db> ".backup <file>"` or `sqlite3 <db> "VACUUM INTO '<file>'"`; either gives a consistent copy while the server runs. Never copy the live file without its `-wal` file: the newest writes are kept there until a checkpoint.

Deleting a habit hides it at once and keeps its row, so Undo can bring it back at its original position. A purge removes the habits deleted more than 24 hours ago for good; it runs on startup and on each delete.

## Deploy

[`deploy/README.md`](deploy/README.md) describes the server kit: a hardened systemd service under its own user, and an optional pull-based auto-deploy of `main` that checks the commit signature and CI and rolls back when `/healthz` does not answer.

Pushing a tag named `steady/vX.Y.Z` runs the release workflow. It builds static binaries (`CGO_ENABLED=0`) for linux and darwin on amd64 and arm64, and publishes a GitHub release with the files `steady-vX.Y.Z-<os>-<arch>` and a `SHA256SUMS` file. Each binary also gets a build provenance attestation. Attestations need a public repository on a personal plan: a tag pushed while the repository is private fails at the attestation step and publishes no release. To verify a downloaded binary:

```sh
gh attestation verify steady-vX.Y.Z-linux-amd64 --repo scv3lqq/steady
```

## Development

You need Go, Node.js with npm, golangci-lint v2 and python3. shellcheck is optional. `make check` is what CI runs; CI also scans the history for secrets with gitleaks.

| Target | What it does |
| --- | --- |
| `make check` | Runs `check-generated`, `lint`, `test`, `check-deploy` and `check-text`. |
| `make check-generated` | Rebuilds the frontend and the sqlc code, then fails if they differ from the index or if `go mod tidy` would change `go.mod` or `go.sum`. |
| `make check-deploy` | Syntax-checks `deploy/*.sh` with `bash -n` and runs shellcheck when it is installed. |
| `make check-text` | Fails on a literal em dash (U+2014) in any tracked or new file. |
| `make lint` | Runs golangci-lint. |
| `make test` | Runs `go test -race ./...` and `npm test` in `web/`. |
| `make web` | Installs the npm dependencies and compiles the frontend into `web/dist/`. |
| `make generate` | Runs sqlc, at a pinned version through `go run`. |
| `make run` | Runs `make web`, then the server with the default configuration. |
| `make build` | Builds a static binary, `bin/steady`. |
| `make build-linux` | Builds a static linux/amd64 binary, `bin/steady-linux-amd64`. |

Generated code is committed: `web/dist/`, so that `go build` and `go install` need only the Go toolchain, and the sqlc output in `internal/repository/sqlite/sqlcdb/`. After you change the TypeScript sources, `queries.sql` or a migration, run `make web generate` and commit the result; `make check` and CI fail when it is stale.

The UI spec is [`docs/spec.md`](docs/spec.md). Architecture and conventions are in [`AGENTS.md`](AGENTS.md).
