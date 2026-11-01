# Plan: Steady habit tracker (feat/steady-app)

## Goal

Build Steady exactly per `docs/spec.md` (UI) and `AGENTS.md` (architecture, conventions): one Go binary
(stdlib `http.ServeMux`, zerolog, `modernc.org/sqlite`, goose, sqlc) serving an embedded, dependency-free
TypeScript frontend, plus CI (`ci.yml`), a release workflow (`release.yml`), a pull-based deploy kit
(`deploy/`), README with screenshots. Base commit: `98d26e3`.

## Acceptance criteria

Each is a command with an observable result. Run from the repo root.

| # | Command | Pass when |
|---|---|---|
| A1 | `git add -A && make check` (clean tree after commit) | exit 0; output shows golangci-lint `0 issues`, `go test -race ./...` ok, `npm test` pass, `npm run build` no errors, freshness diff empty, check-deploy and check-text ok |
| A2 | `node "$SCRATCH/e2e/e2e.mjs"` (T22 writes it; `$SCRATCH` = the session scratchpad dir) | exit 0 and last line `E2E PASS`; zero console errors reported; prints `scrollWidth<=320 ok` for week, month, year |
| A3 | `go test -count=1 -race -v -run 'TestRoutes\|TestHostAllowlist\|TestCrossOrigin\|TestSecurityHeaders\|TestStatic' ./internal/handler/httpapi/` | exit 0; output contains `--- PASS: TestRoutes`, `--- PASS: TestHostAllowlist`, `--- PASS: TestCrossOrigin`, `--- PASS: TestSecurityHeaders`, `--- PASS: TestStatic` |
| A4 | `go test -count=1 -race -v -run 'TestSoftDeleteRestoreKeepsPosition\|TestPurge\|TestSeedOnce' ./internal/repository/sqlite/` | exit 0; the three `--- PASS:` lines present |
| A5 | `python3 scripts/check_text.py` | exit 0 |
| A6 | `git ls-files web/dist` | lists exactly `web/dist/dates.js`, `web/dist/main.js`, `web/dist/render.js`, `web/dist/store.js` |
| A7 | `ls assets/screenshots/week.png assets/screenshots/month.png assets/screenshots/year.png assets/screenshots/week-320.png && grep -F 'assets/screenshots/week-320.png' README.md` | exit 0 |
| A8 | `grep -F 'STEADY_ALLOWED_HOSTS' AGENTS.md README.md internal/config/config.go` | a hit in each file |
| A9 | `grep -hE '^\s*(-\s*)?uses:' .github/workflows/*.yml \| grep -vcE '@[0-9a-f]{40} # v[0-9]'` | prints `0` |
| A10 | `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/ci.yml .github/workflows/release.yml` | exit 0, no output |
| A11 | `grep -F "'steady/v*'" .github/workflows/release.yml && grep -F 'attest' .github/workflows/release.yml` | both hit |
| A12 | after push of `main`: `gh run list --branch main --workflow ci.yml --limit 1 --json conclusion,headSha -q '.[0]'` | `conclusion` is `success` and `headSha` equals `git rev-parse main` |

In A3, A4 and A9 the `\|` is a Markdown table escape: run it as a plain `|`.

Base result (recorded on `98d26e3` before any task): A1 fails (no Makefile), A2 to A11 fail (files absent),
A12 n/a. No tracked file on the base contains a literal em dash (checked with the S2 snippet), so A5 is a
whole-repo check with base count 0.

## Gates

The full gate is `make check` (T3 defines it). Generated-code freshness uses `git diff` against the index,
so the orchestrator stages the wave's files (`git add <paths>`) before running it.

Wave gates (the orchestrator runs these after each wave; the full gate cannot pass before the tests exist):

- after wave 1: `go build ./... && go vet ./... && golangci-lint run ./... && (cd web && npm ci && npm run build) && make check-text && make check-deploy`
- after wave 2: wave-1 gate plus `go mod tidy` (orchestrator commits go.mod/go.sum if changed), `go test -race ./...`, `(cd web && npm test)`
- after wave 3 and later: `git add -A && make check`

Implementers run only their task gates, never `make check` or the full `go test ./...`.

## Constraints (apply to every task)

- `AGENTS.md` and `CLAUDE.md` load into every agent; follow them (zerolog, `fmt.Errorf("op: %w", err)`,
  table-driven tests, no global mutable state, TS strict, `textContent` only, English only).
- No literal em dash (U+2014) in any file, this plan included. The UI builds it with
  `String.fromCharCode(0x2014)`. Curly quotes U+2019, U+201C, U+201D in UI strings are fine.
- Touch only the files your task lists. Do not commit (the orchestrator commits). Do not run `go mod tidy`
  or `go get` except in T1.
- Never bind or document binding to a public interface; no personal hosts, IPs, domains or home paths.

## Shared definitions

### S1. Versions and pins

- Go module `github.com/scv3lqq/steady`, `go 1.27.3` (no toolchain line).
- `github.com/rs/zerolog v1.35.1`, `modernc.org/sqlite v1.61.1`, `github.com/pressly/goose/v3 v3.29.0`.
- sqlc `v1.31.1`, always run as `CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`
  (the generated header carries the version, so local and CI must match).
- golangci-lint `v2.15.0` (local binary is 2.15.0).
- npm `typescript` exactly `6.0.3` (not 7.x: 7.x ships per-platform native binaries as optional
  dependencies, fragile in a lockfile made on macOS and installed on Linux). Node in CI: `26` (current LTS).
- gitleaks `v8.30.1` (matches `.pre-commit-config.yaml`).
- GitHub Actions (full SHA, version comment):

| action | pin |
|---|---|
| actions/checkout | `3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1` |
| actions/setup-go | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0` |
| actions/setup-node | `949feb2413d6458794dcd2491c4babbbce0c15c1 # v7.1.0` |
| golangci/golangci-lint-action | `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9.3.0` |
| actions/upload-artifact | `cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2` |
| actions/download-artifact | `9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2` |
| actions/attest | `1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2` |

### S2. No-em-dash snippet (task gates)

```bash
python3 -c 'import sys; bad=[p for p in sys.argv[1:] if chr(0x2014) in open(p, encoding="utf-8").read()]; print("em dash in:", bad) if bad else None; sys.exit(1 if bad else 0)' FILE...
```

### S3. Domain API (`internal/domain/domain.go`, package `domain`, stdlib imports only)

```go
const (
	MaxNameRunes  = 60
	DefaultName   = "Untitled"
	DayLayout     = "2006-01-02"
	DefaultAccent = "#4FA872"
)

type View string

const (
	ViewWeek    View = "week"
	ViewMonth   View = "month"
	ViewYear    View = "year"
	DefaultView      = ViewWeek
)

// ValidationError is bad client input; handlers map it to 400 with its text.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

const (
	ErrInvalidID     ValidationError = "invalid habit id: want a lowercase UUID"
	ErrNameRequired  ValidationError = "name is required"
	ErrNameTooLong   ValidationError = "name is longer than 60 characters"
	ErrInvalidDay    ValidationError = "invalid day: want YYYY-MM-DD"
	ErrInvalidView   ValidationError = "invalid view: want week, month or year"
	ErrInvalidAccent ValidationError = "invalid accent: want one of the palette colors"
	ErrEmptySettings ValidationError = "settings: nothing to update"
)

// StateError is a conflict with stored state; handlers map ErrNotFound to 404, ErrExists to 409.
type StateError string

func (e StateError) Error() string { return string(e) }

const (
	ErrNotFound StateError = "habit not found"
	ErrExists   StateError = "habit already exists"
)

type Habit struct {
	ID   string
	Name string
	Done []string // day keys, ascending; never nil in State
}

type State struct {
	Habits []Habit // non-deleted, position order; never nil
	View   View
	Accent string
}

type SettingsUpdate struct {
	View   *View
	Accent *string
}

func Palette() []string   // fresh slice each call, in this order:
// "#4FA872", "#8FBF4A", "#3FB8B0", "#5B8CEB", "#B07FE0", "#E07BB0", "#D96A6A", "#E08A45", "#E3C24F", "#ECE7DC"
func SeedNames() []string // fresh slice: "Drink 2 L of water", "Walk 30 minutes", "Read 10 pages"
func ValidateID(id string) error
func NormalizeName(name string) (string, error)
func ValidateDay(day string) error
func ParseView(s string) (View, error)
func ValidateAccent(accent string) error
func NewSettingsUpdate(view, accent *string) (SettingsUpdate, error)
```

Behavior:
- `ValidateID`: nil only when `len(id) == 36`, bytes 8, 13, 18, 23 are `-`, every other byte is in `0-9a-f`
  (lowercase only, any version nibble). Else `ErrInvalidID`.
- `NormalizeName`: `strings.TrimSpace`; empty result returns `DefaultName, nil`; more than `MaxNameRunes`
  runes (`utf8.RuneCountInString`) returns `"", ErrNameTooLong`; else the trimmed name.
- `ValidateDay`: `time.Parse(DayLayout, day)` must succeed and `t.Format(DayLayout) == day`; else `ErrInvalidDay`.
- `ParseView`: exactly `week`, `month`, `year` (case-sensitive); else `"", ErrInvalidView`.
- `ValidateAccent`: exact, case-sensitive member of `Palette()`; else `ErrInvalidAccent`.
- `NewSettingsUpdate`: both nil returns `ErrEmptySettings`; a non-nil view goes through `ParseView`, a
  non-nil accent through `ValidateAccent` (view checked first); the result holds pointers to the validated
  values (copies, not the caller's pointers).

### S4. Repository port (`internal/usecase/service.go`) and SQLite adapter (`internal/repository/sqlite`)

```go
// in package usecase
type Repository interface {
	State(ctx context.Context) (domain.State, error)
	AddHabit(ctx context.Context, id, name string) error            // domain.ErrExists when the id is taken (also by a soft-deleted row)
	RenameHabit(ctx context.Context, id, name string) error         // domain.ErrNotFound when no active row
	DeleteHabit(ctx context.Context, id string, at time.Time) error // soft delete; domain.ErrNotFound when no active row
	RestoreHabit(ctx context.Context, id string) error              // domain.ErrNotFound when the row is gone (idempotent on an active row)
	PurgeDeleted(ctx context.Context, before time.Time) (int64, error) // hard-deletes rows with deleted_at < before.Unix()
	SetDay(ctx context.Context, id, day string, done bool) error    // domain.ErrNotFound when no active row
	UpdateSettings(ctx context.Context, u domain.SettingsUpdate) error
	Ping(ctx context.Context) error
}
```

```go
// in package sqlite (import path github.com/scv3lqq/steady/internal/repository/sqlite)
type Store struct { /* db *sql.DB; q *sqlcdb.Queries */ }

func Open(ctx context.Context, path string) (*Store, error)
func (s *Store) Close() error
func (s *Store) SeedOnce(ctx context.Context, now time.Time, rng *rand.Rand) (bool, error) // math/rand/v2
// plus every Repository method above, same signatures
```

Settings keys stored in `settings`: `view`, `accent`, `seeded` (unexported consts in package sqlite).

### S5. Use case API (`internal/usecase/service.go`, package `usecase`)

```go
const PurgeAfter = 24 * time.Hour

type Service struct { /* repo Repository; now func() time.Time; log zerolog.Logger */ }

func New(repo Repository, now func() time.Time, log zerolog.Logger) *Service
func (s *Service) State(ctx context.Context) (domain.State, error)
func (s *Service) AddHabit(ctx context.Context, id, name string) (domain.Habit, error)
func (s *Service) RenameHabit(ctx context.Context, id string, name *string) (domain.Habit, error)
func (s *Service) DeleteHabit(ctx context.Context, id string) error
func (s *Service) RestoreHabit(ctx context.Context, id string) error
func (s *Service) SetDay(ctx context.Context, id, day string, done bool) error
func (s *Service) UpdateSettings(ctx context.Context, view, accent *string) error
func (s *Service) Purge(ctx context.Context) (int64, error)
func (s *Service) Ping(ctx context.Context) error
```

### S6. HTTP API contract (handler T16, tests T18, frontend T2/T13, README T21)

All JSON responses: header `Content-Type: application/json`, body written with `json.NewEncoder(w).Encode`
(compact JSON plus `\n`). Tests compare `strings.TrimSpace(body)`. Error body: `{"error":"<text>"}`.

| Method and path | Request body | Success | Errors |
|---|---|---|---|
| `GET /api/state` | none | 200 `{"habits":[{"id":"...","name":"...","done":["2026-10-30"]}],"view":"week","accent":"#4FA872"}`; empty lists are `[]` | 500 |
| `POST /api/habits` | `{"id":"<uuid>","name":"..."}` (name optional, missing = "") | 201 `{"id":"...","name":"<normalized>","done":[]}` | 400 validation, 409 `habit already exists` |
| `PATCH /api/habits/{id}` | `{"name":"..."}` (required) | 200 `{"id":"...","name":"<normalized>"}` | 400 (`name is required` when the key is missing), 404 |
| `DELETE /api/habits/{id}` | none | 204 | 400, 404 |
| `POST /api/habits/{id}/restore` | none | 204 | 400, 404 |
| `PUT /api/habits/{id}/days/{day}` | none | 204 (idempotent) | 400, 404 |
| `DELETE /api/habits/{id}/days/{day}` | none | 204 (idempotent) | 400, 404 |
| `PUT /api/settings` | `{"view"?:"week\|month\|year","accent"?:"<palette color>"}` | 204 | 400 |
| `GET /healthz` | none | 200 `{"status":"ok"}` | 503 `{"error":"database unavailable"}` |
| `GET /` | | 200 `index.html`, `text/html; charset=utf-8` | |
| `GET /styles.css` | | 200, `text/css; charset=utf-8` | |
| `GET /dist/{file}` | | 200 for a file read at startup from `dist/*.js`, `text/javascript; charset=utf-8` | 404 otherwise (no directory listing) |

- Request bodies: `http.MaxBytesReader` 4096 bytes, `json.Decoder.DisallowUnknownFields()`, exactly one
  JSON value (a second `Decode` must return `io.EOF`). Any decode failure (malformed, unknown field, too
  large, trailing data) is 400 `invalid JSON body`.
- Error mapping (one helper): `errors.As(err, &domain.ValidationError)` gives 400 with that value's text
  (not the wrapped `err.Error()`); `errors.Is(err, domain.ErrNotFound)` 404 `habit not found`;
  `errors.Is(err, domain.ErrExists)` 409 `habit already exists`; anything else 500 `internal error`, logged
  with zerolog at error level (`method`, `path`, error).
- Middleware order, outermost first: security headers, Host allowlist, `http.CrossOriginProtection`, mux.
- Security headers on every response (rejections included):
  - `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`
  - `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store`
- Host allowlist: lowercase `r.Host`; its hostname is `net.SplitHostPort` host when that succeeds, else the
  whole value, with `[` `]` trimmed. Allowed when the hostname is `localhost`, `127.0.0.1` or `::1` (any port,
  so SSH tunnels on another local port work), or when it matches an `Options.AllowedHosts` entry
  (lowercased): an entry with a port must equal the full Host; an entry without a port matches the hostname
  on any port. Else 403 `{"error":"host not allowed"}`. Empty Host is rejected.
- Cross-origin: `cop := http.NewCrossOriginProtection()`, `cop.SetDenyHandler(...)` writing 403
  `{"error":"cross-origin request rejected"}`, `cop.Handler(mux)` wraps the whole mux (it only acts on
  non-safe methods, which are exactly the mutating routes).

### S7. Config API (`internal/config/config.go`, package `config`)

```go
type Config struct {
	Addr       string        // STEADY_ADDR, default "127.0.0.1:8770"
	DBPath     string        // STEADY_DB, default "data/steady.db"
	LogLevel   zerolog.Level // STEADY_LOG_LEVEL, default info
	ExtraHosts []string      // STEADY_ALLOWED_HOSTS, comma-separated, default empty
}

func Load(getenv func(string) string) (Config, error) // an empty value means "use the default"
func (c Config) AllowedHosts() []string               // []string{strings.ToLower(c.Addr)} followed by ExtraHosts
```

- Addr must split with `net.SplitHostPort` and its port must be an integer 1..65535; else error
  `config: STEADY_ADDR %q: want host:port`.
- LogLevel: `zerolog.ParseLevel(strings.ToLower(v))`; error, or the result `zerolog.NoLevel`, gives
  `config: STEADY_LOG_LEVEL %q: unknown level`.
- ExtraHosts: split on `,`, `strings.TrimSpace`, `strings.ToLower`, drop empty items; nil when none.

### S8. httpapi API (`internal/handler/httpapi`, package `httpapi`)

```go
type Options struct {
	AllowedHosts []string // see S6 Host allowlist
}

func New(svc *usecase.Service, static fs.FS, opts Options, log zerolog.Logger) (http.Handler, error)
```

`static` has `index.html`, `styles.css` and `dist/*.js` at its root. `New` reads `index.html`, `styles.css`
and every `fs.Glob(static, "dist/*.js")` match into memory; a missing `index.html` or `styles.css` returns
an error `httpapi: read <name>: <err>` wrapped with `%w`.

### S9. App API (`internal/app/app.go`, package `app`) and embed (`web/embed.go`, package `web`)

```go
type App struct { /* handler http.Handler; store *sqlite.Store */ }

func New(ctx context.Context, cfg config.Config, log zerolog.Logger) (*App, error)
func (a *App) Handler() http.Handler
func (a *App) Close() error
```

```go
// web/embed.go
// Package web holds the frontend, embedded into the binary.
package web

import "embed"

// FS has index.html, styles.css and the compiled dist/*.js at its root.
//
//go:embed index.html styles.css dist/*.js
var FS embed.FS
```

### S10. SQL (T1 transcribes byte for byte; spike-verified with sqlc v1.31.1)

```yaml sqlc.yaml
version: "2"
sql:
  - engine: sqlite
    schema: internal/repository/sqlite/migrations
    queries: internal/repository/sqlite/queries.sql
    gen:
      go:
        package: sqlcdb
        out: internal/repository/sqlite/sqlcdb
        emit_empty_slices: true
```

```sql 00001_init.sql
-- +goose Up
CREATE TABLE habits (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    deleted_at INTEGER
) STRICT;

CREATE TABLE habit_days (
    habit_id TEXT NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    day TEXT NOT NULL,
    PRIMARY KEY (habit_id, day)
) STRICT, WITHOUT ROWID;

CREATE TABLE settings (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
) STRICT;

-- +goose Down
DROP TABLE settings;
DROP TABLE habit_days;
DROP TABLE habits;
```

```sql queries.sql
-- name: ListActiveHabits :many
SELECT id, name FROM habits WHERE deleted_at IS NULL ORDER BY position, id;

-- name: ListActiveDays :many
SELECT d.habit_id, d.day FROM habit_days AS d JOIN habits AS h ON h.id = d.habit_id
WHERE h.deleted_at IS NULL ORDER BY d.habit_id, d.day;

-- name: InsertHabit :execrows
INSERT INTO habits (id, name, position)
VALUES (sqlc.arg(id), sqlc.arg(name), (SELECT COALESCE(MAX(position), -1) + 1 FROM habits))
ON CONFLICT (id) DO NOTHING;

-- name: RenameHabit :execrows
UPDATE habits SET name = sqlc.arg(name) WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteHabit :execrows
UPDATE habits SET deleted_at = CAST(sqlc.arg(deleted_at) AS INTEGER) WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: RestoreHabit :execrows
UPDATE habits SET deleted_at = NULL WHERE id = sqlc.arg(id);

-- name: PurgeDeletedBefore :execrows
DELETE FROM habits WHERE deleted_at IS NOT NULL AND deleted_at < CAST(sqlc.arg(cutoff) AS INTEGER);

-- name: CountActiveHabit :one
SELECT COUNT(*) FROM habits WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: MarkDay :exec
INSERT INTO habit_days (habit_id, day) VALUES (sqlc.arg(habit_id), sqlc.arg(day))
ON CONFLICT (habit_id, day) DO NOTHING;

-- name: UnmarkDay :exec
DELETE FROM habit_days WHERE habit_id = sqlc.arg(habit_id) AND day = sqlc.arg(day);

-- name: ListSettings :many
SELECT key, value FROM settings;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO UPDATE SET value = excluded.value;

-- name: InsertSettingIfAbsent :execrows
INSERT INTO settings (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO NOTHING;
```

Generated API (package `sqlcdb`, from the spike): `New(db DBTX) *Queries`, `(*Queries).WithTx(*sql.Tx) *Queries`;
`CountActiveHabit(ctx, id string) (int64, error)`; `InsertHabit(ctx, InsertHabitParams{ID, Name string}) (int64, error)`;
`InsertSettingIfAbsent(ctx, InsertSettingIfAbsentParams{Key, Value string}) (int64, error)`;
`ListActiveDays(ctx) ([]HabitDay{HabitID, Day string}, error)`; `ListActiveHabits(ctx) ([]ListActiveHabitsRow{ID, Name string}, error)`;
`ListSettings(ctx) ([]Setting{Key, Value string}, error)`; `MarkDay(ctx, MarkDayParams{HabitID, Day string}) error`;
`PurgeDeletedBefore(ctx, cutoff int64) (int64, error)`; `RenameHabit(ctx, RenameHabitParams{Name, ID string}) (int64, error)`;
`RestoreHabit(ctx, id string) (int64, error)`; `SoftDeleteHabit(ctx, SoftDeleteHabitParams{DeletedAt int64; ID string}) (int64, error)`;
`UnmarkDay(ctx, UnmarkDayParams{HabitID, Day string}) error`; `UpsertSetting(ctx, UpsertSettingParams{Key, Value string}) error`.

SQLite DSN: `path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate"`,
driver name `sqlite`, `db.SetMaxOpenConns(1)`. With one connection, code inside a transaction must use
`s.q.WithTx(tx)` only; a call on `s.q` or `s.db` while a tx is open deadlocks.

goose: `//go:embed migrations/*.sql` into an `embed.FS`, `fs.Sub(fsys, "migrations")`,
`goose.NewProvider(goose.DialectSQLite3, db, sub)`, `provider.Up(ctx)`. Never call `provider.Close()` (it
closes the `*sql.DB`). No goose globals. goose prints nothing without `WithVerbose`.

### S11. Frontend state API (`web/src/store.ts`)

```ts
export type View = 'week' | 'month' | 'year';
export interface Habit { id: string; name: string; done: Record<string, true>; }
export interface State { habits: Habit[]; view: View; accent: string; }
export interface Deleted { habit: Habit; index: number; }
export const PALETTE: readonly string[]; // the 10 colors in S3 order
export const DEFAULT_ACCENT = '#4FA872';
export const RENAME_DEBOUNCE_MS = 300;

export class Store {
  constructor(onChange: () => void);
  get state(): State;
  load(): Promise<void>;
  addHabit(name: string): void;
  renameHabit(id: string, name: string): void;
  commitRename(id: string): void;
  flushPending(): void;
  deleteHabit(id: string): Deleted | null;
  restoreHabit(d: Deleted): void;
  toggleDay(id: string, day: string): void;
  setView(view: View): void;
  setAccent(accent: string): void;
}
```

### S12. Date helpers API (`web/src/dates.ts`, pure, no DOM)

```ts
export const MONTH_DAYS = 30;
export const YEAR_WEEKS = 53;
export function startOfDay(d: Date): Date;               // new Date(y, m, day)
export function dayKey(d: Date): string;                  // local "YYYY-MM-DD" from getFullYear/getMonth/getDate, zero-padded
export function fromKey(key: string): Date;               // local midnight new Date(y, m - 1, d); input assumed valid
export function addDays(d: Date, n: number): Date;        // new Date(y, m, day + n): calendar arithmetic, DST-safe
export function lastNDays(today: Date, n: number): Date[]; // n local midnights, oldest first, last is startOfDay(today)
export function mondayOf(d: Date): Date;                  // startOfDay(d) minus (getDay() + 6) % 7 days
export function yearGrid(today: Date): Date[];            // 371 dates: start = addDays(mondayOf(today), -7 * 52), then start + i for i in 0..370
export function yearCaptionMonth(gridStart: Date): Date;  // gridStart.getDate() === 1 ? first of its month : first of the next month
export function streak(done: Readonly<Record<string, true>>, today: Date): number;
export function countDone(done: Readonly<Record<string, true>>, days: readonly Date[]): number;
export function pluralDays(n: number): string;            // n === 1 ? '1 day' : `${n} days`
export function formatHeaderDate(d: Date): string;        // en-US {weekday:'long', month:'long', day:'numeric'} -> "Sunday, November 1"
export function formatShortDate(d: Date): string;         // en-US {month:'short', day:'numeric'} -> "Oct 3"
export function formatLongDate(d: Date): string;          // en-US {month:'long', day:'numeric'} -> "October 30"
export function formatMonthYear(d: Date): string;         // en-US {month:'long', year:'numeric'} -> "November 2025"
export function weekdayShort(d: Date): string;            // en-US {weekday:'short'} first 2 chars -> "Mo"
```

- `streak`: `d = startOfDay(today)`; if `!done[dayKey(d)]` then `d = addDays(d, -1)`; count while
  `done[dayKey(d)]`, stepping `d = addDays(d, -1)`.
- Never use `toISOString` or millisecond arithmetic for days. `Intl.DateTimeFormat` instances may be
  module-level consts (immutable).
- Week view days = `lastNDays(now, 7)` (rolling, today rightmost: the spec defines only today and past
  states for week squares). Month = `lastNDays(now, MONTH_DAYS)`. Year = `yearGrid(now)`, column-major
  (index = week * 7 + weekday, Monday first), matching `grid-auto-flow: column` with 7 rows.
- Year caption month rule matches the spec example: today 2026-11-01 gives grid start 2025-10-27 and
  caption `November 2025` (first month that begins inside the grid).

### S13. DOM contract

`web/index.html`, verbatim (T6 transcribes it; the T6 gate diffs it against this block):

```html index.html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Steady</title>
  <link rel="icon" href="data:,">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="stylesheet" href="/styles.css">
  <script type="module" src="/dist/main.js"></script>
</head>
<body>
  <main class="app" id="app">
    <header class="header">
      <div class="topbar">
        <span class="wordmark">Steady</span>
        <div class="topbar-end">
          <span class="today" id="today"></span>
          <button type="button" class="accent-toggle" id="accent-toggle" data-action="toggle-palette" aria-label="Accent color" aria-expanded="false" aria-controls="palette"><span class="swatch"></span></button>
        </div>
      </div>
      <div class="palette" id="palette" role="radiogroup" aria-label="Accent color" hidden>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Green" data-action="accent" data-value="#4FA872"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Lime" data-action="accent" data-value="#8FBF4A"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Teal" data-action="accent" data-value="#3FB8B0"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Blue" data-action="accent" data-value="#5B8CEB"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Violet" data-action="accent" data-value="#B07FE0"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Pink" data-action="accent" data-value="#E07BB0"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Red" data-action="accent" data-value="#D96A6A"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Orange" data-action="accent" data-value="#E08A45"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Yellow" data-action="accent" data-value="#E3C24F"><span class="swatch"></span></button>
        <button type="button" class="palette-color" role="radio" aria-checked="false" aria-label="Ivory" data-action="accent" data-value="#ECE7DC"><span class="swatch"></span></button>
      </div>
      <h1 class="title" id="title"></h1>
      <div class="progress" aria-hidden="true"><div class="progress-fill" id="progress"></div></div>
    </header>
    <div class="controls">
      <div class="toolbar">
        <div class="tabs" role="tablist" aria-label="View">
          <button type="button" class="tab" role="tab" aria-selected="false" data-action="view" data-value="week">Week</button>
          <button type="button" class="tab" role="tab" aria-selected="false" data-action="view" data-value="month">Month</button>
          <button type="button" class="tab" role="tab" aria-selected="false" data-action="view" data-value="year">Year</button>
        </div>
        <button type="button" class="add-toggle" data-action="show-add">+ Habit</button>
      </div>
      <div class="add-row" id="add-row" hidden>
        <input class="add-input" id="add-input" type="text" maxlength="60" placeholder="Name it, press Enter" aria-label="New habit name" autocomplete="off" spellcheck="false">
        <span class="hint">Esc to cancel</span>
      </div>
    </div>
    <section class="list" id="list" aria-label="Habits"></section>
    <footer class="footer">Tap a square or dot to mark a day. Missed one? You can still mark it later.</footer>
    <div class="toast" id="toast" role="status" hidden>
      <span class="toast-text" id="toast-text"></span>
      <button type="button" class="undo" data-action="undo">Undo</button>
    </div>
  </main>
</body>
</html>
```

Elements `render.ts` creates inside `#list` (class names are the contract with `styles.css`):

- Empty state (no habits, any view): `div.empty` > `p.empty-title` (`Nothing here yet`) + `p.empty-text`
  (`Tap “+ Habit” above. Starting small is fine.`).
- Week header (view week, at least one habit), first child: `div.week-head[aria-hidden=true]` >
  `span.name-spacer`, `div.days` > 7 x `span.wd` (+ `.is-today` on today) > `span.wd-name` (`Mo`) +
  `span.wd-date` (`26`), then `span.side-spacer`.
- Week row: `div.habit.week-row[data-id=<id>][data-view=week]` > `input.name`, `div.days` > 7 x `button.sq`,
  `div.side` > `span.streak` (+ `.is-zero` when 0) + `button.del`.
- Month block: `div.habit.block.month-block[data-id][data-view=month]` > `div.block-head` > `input.name`,
  `span.count` (`18 of 30`), `button.del`; then `div.month-grid` > 30 x `button.dot`; then
  `div.caption` > `span` (start date `Oct 3`) + `span` (`today`).
- Year block: `div.habit.block.year-block[data-id][data-view=year]` > `div.block-head` (same, count
  `214 days`), `div.year-grid` > 371 x `button.cell`, `div.caption` > `span` (`November 2025`) + `span` (`today`).
- `input.name`: `type=text`, `maxlength=60`, `data-action=rename`, `data-id`, `aria-label="Habit name"`,
  `autocomplete=off`, `spellcheck=false`.
- `button.del`: `type=button`, `data-action=delete`, `data-id`, `aria-label="Delete <name>"`, text `×` (U+00D7).
- Day buttons (`button.sq`, `button.dot`, `button.cell`): `type=button`, `data-action=toggle`, `data-id`,
  `data-date=<YYYY-MM-DD>`, `aria-pressed="true|false"`, `aria-label="<name>, <formatLongDate> <EM> done"` or
  `... <EM> not done`, where `EM = String.fromCharCode(0x2014)`. State classes: `.is-done`, `.is-today`;
  year cells after today also get `.is-future` and `disabled`. A done `button.sq` holds one
  `svg.check[aria-hidden=true][viewBox="0 0 16 16"]` with a `path` `d="M3.5 8.5l3 3 6-7"`, `fill=none`,
  `stroke=currentColor`, `stroke-width=2`, `stroke-linecap=round`, `stroke-linejoin=round` (built with
  `createElementNS`).

### S14. UI text and logic literals (render.ts, main.ts, e2e)

- H1: no habits `Let’s start` (U+2019); all done today (x = n > 0) `All done today`; else `${x} of ${n} today`.
- Progress fill width: `n === 0 ? 0 : x / n * 100` percent, set with `el.style.width` (CSSOM, allowed by CSP).
- Accent: `document.documentElement.style.setProperty('--accent', state.accent)`. Never `setAttribute('style')`
  and no `style=` attributes (the CSP has no `'unsafe-inline'` for styles).
- Streak text: `pluralDays(n)` (class `.streak`), or `EM` with class `.is-zero` when 0.
- Month count `${countDone(done, monthDays)} of 30`; year count `pluralDays(countDone(done, gridDays up to and including today))`.
- Toast text: `“${habit.name}” deleted` (U+201C, U+201D), auto-hide after 5000 ms, a new delete restarts the timer.

### S15. Make targets (T3 defines; CI, README, AGENTS use these names)

`check`, `check-generated`, `check-deploy`, `check-text`, `lint`, `test`, `web`, `generate`, `run`, `build`, `build-linux`.

### S16. Screenshot files (T22 writes, T21 links)

`assets/screenshots/week.png`, `assets/screenshots/month.png`, `assets/screenshots/year.png` (600 px wide),
`assets/screenshots/week-320.png` (320 px wide), all from freshly seeded demo data.

## Tasks

Waves: W1 = T1..T6, W2 = T7..T12, W3 = T13..T17, W4 = T18..T19, W5 = T20..T22.

### T1. Go foundation (W1)

Files: `go.mod`, `go.sum`, `.golangci.yml`, `internal/domain/domain.go`, `sqlc.yaml`,
`internal/repository/sqlite/migrations/00001_init.sql`, `internal/repository/sqlite/queries.sql`,
`internal/repository/sqlite/sqlcdb/*.go` (generated).

- `go mod init github.com/scv3lqq/steady`; set `go 1.27.3`; `go get github.com/rs/zerolog@v1.35.1 modernc.org/sqlite@v1.61.1 github.com/pressly/goose/v3@v3.29.0` (they stay `// indirect` until later waves import them; do not tidy).
- `.golangci.yml`: copy the `.golangci.yml` of an existing sibling project, replace that project's module path
  with `github.com/scv3lqq/steady` everywhere, drop the `internal/testutil/` exclusion path (keep `_test\.go`).
- `domain.go`: S3 exactly, with a package doc comment and doc comments on exported symbols (revive).
- `sqlc.yaml`, `00001_init.sql`, `queries.sql`: the S10 blocks byte for byte. Then run
  `CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`.

Task gates:
```bash
for f in sqlc.yaml:'yaml sqlc.yaml' internal/repository/sqlite/migrations/00001_init.sql:'sql 00001_init.sql' internal/repository/sqlite/queries.sql:'sql queries.sql'; do
  file="${f%%:*}"; tag="${f#*:}"
  awk -v t="$tag" '$0=="```"t{f=1;next} f&&$0=="```"{exit} f' plans/steady-app.md | diff - "$file" || exit 1
done
shasum -a 256 internal/repository/sqlite/sqlcdb/*.go > "$TMPDIR/sqlc.sum"
CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
shasum -a 256 -c "$TMPDIR/sqlc.sum"
gofmt -l internal/domain
go build ./internal/... && go vet ./internal/domain/ ./internal/repository/sqlite/sqlcdb/
golangci-lint config verify && golangci-lint run ./internal/domain/... ./internal/repository/sqlite/sqlcdb/...
grep -E '^go 1\.27\.3$' go.mod
```
Plus S2 over `internal/domain/domain.go`. DoD: gates green; `gofmt -l` prints nothing.

### T2. Web foundation (W1)

Files: `web/package.json`, `web/package-lock.json`, `web/tsconfig.json`, `web/src/dates.ts`, `web/src/store.ts`,
`web/dist/dates.js`, `web/dist/store.js`.

- `package.json`: `{"name":"steady-web","private":true,"type":"module","scripts":{"build":"tsc","dev":"tsc --watch","test":"TZ=America/New_York node --test test/dates.test.mjs"},"devDependencies":{"typescript":"6.0.3"}}` (pretty-printed, 2-space indent). Create the lockfile with `npm install` in `web/` (exact version, no caret).
- `tsconfig.json`: `compilerOptions`: `target` `ES2022`, `module` `ES2022`, `moduleResolution` `bundler`,
  `lib` `["ES2022", "DOM"]` (add `"DOM.Iterable"` only if tsc 6 reports missing iterable DOM types),
  `strict` true, `noUncheckedIndexedAccess` true, `noImplicitReturns` true, `noFallthroughCasesInSwitch` true,
  `noUnusedLocals` true, `noUnusedParameters` true, `rootDir` `src`, `outDir` `dist`, `newLine` `lf`,
  `skipLibCheck` true; `include` `["src/**/*.ts"]`. Imports between modules use the `.js` extension
  (`import { dayKey } from './dates.js'`).
- `dates.ts`: S12 exactly.
- `store.ts`: S11 with this behavior:
  - Private fields: `#state` (initial `{habits: [], view: 'week', accent: DEFAULT_ACCENT}`),
    `#queue: Promise<void> = Promise.resolve()`, `#timers: Map<string, number>`, `#pending: Map<string, string>`.
  - `load()`: `GET /api/state`; on a non-2xx or a thrown fetch, resolve without changing state; on success
    convert each `done: string[]` to `Record<string, true>`, replace `#state`, call `onChange()`.
  - `#enqueue(method, path, body?)`: chain on `#queue`; send `fetch(path, {method, headers, body})` with
    `Content-Type: application/json` and `JSON.stringify(body)` only when a body is given; a non-2xx status or
    a thrown fetch calls `await this.load()` (reload and re-render). The chain never rejects. No console output.
  - Paths use `encodeURIComponent` for id and day: `/api/habits`, `/api/habits/${id}`, `/api/habits/${id}/restore`,
    `/api/habits/${id}/days/${day}`, `/api/settings`.
  - `addHabit(name)`: id `crypto.randomUUID()`; push `{id, name, done: {}}`; `onChange()`; enqueue `POST /api/habits {id, name}`.
  - `renameHabit(id, name)`: set the habit's local name to the raw value; store it in `#pending`; restart a
    `RENAME_DEBOUNCE_MS` timer whose callback sends the pending name with `PATCH /api/habits/{id} {name}` via
    the queue. Does not call `onChange()` (the input already shows the text).
  - `commitRename(id)` (blur): if a timer is pending, clear it and send now; then set the local name to
    `name.trim()` or `Untitled` when empty; `onChange()`.
  - `flushPending()` (pagehide): for every pending name clear the timer and send `PATCH` immediately with
    `fetch(..., {keepalive: true})`, bypassing the queue; clear both maps.
  - `deleteHabit(id)`: unknown id returns `null`; a pending rename for it is sent first (via the queue);
    normalize the local name like `commitRename`; splice it out; `onChange()`; enqueue `DELETE /api/habits/{id}`;
    return `{habit, index}`.
  - `restoreHabit(d)`: no-op when the id is already present; insert at `Math.min(d.index, habits.length)`;
    `onChange()`; enqueue `POST /api/habits/{id}/restore`.
  - `toggleDay(id, day)`: flip `done[day]` (delete the key when unmarking); `onChange()`; enqueue `PUT` or `DELETE /api/habits/{id}/days/{day}`.
  - `setView(view)` / `setAccent(accent)`: no-op when unchanged (and for an accent not in `PALETTE`);
    update; `onChange()`; enqueue `PUT /api/settings` with `{view}` or `{accent}`.

Task gates:
```bash
(cd web && npm ci && npm run build)
ls web/dist/dates.js web/dist/store.js
grep -F '"typescript": "6.0.3"' web/package.json
! grep -nE 'innerHTML|toISOString|localStorage' web/src/dates.ts web/src/store.ts
```
Plus S2 over the two `.ts` files. DoD: zero tsc errors.

### T3. Tooling: Makefile and text check (W1)

Files: `Makefile`, `scripts/check_text.py`.

Makefile (recipes as below; tabs):

```make
BIN := bin/steady
BIN_LINUX := bin/steady-linux-amd64
SQLC_VERSION := v1.31.1
GENERATED := web/dist internal/repository/sqlite/sqlcdb

.PHONY: check check-generated check-deploy check-text lint test web generate run build build-linux

check: check-generated lint test check-deploy check-text

check-generated: web generate
	go mod tidy -diff
	git diff --exit-code -- $(GENERATED)
	@untracked="$$(git ls-files --others --exclude-standard -- $(GENERATED))"; \
	if [ -n "$$untracked" ]; then echo "untracked generated files:"; echo "$$untracked"; exit 1; fi

web:
	cd web && npm ci --no-audit --no-fund && npm run build

generate:
	CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

lint:
	golangci-lint run ./...

test:
	go test -race ./...
	cd web && npm test

check-deploy:
	set -e; for script in deploy/*.sh; do bash -n "$$script"; done
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck deploy/*.sh; else echo "shellcheck not installed, skipped"; fi

check-text:
	python3 scripts/check_text.py

run: web
	go run ./cmd/steady

build:
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o $(BIN) ./cmd/steady

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags='-s -w' -o $(BIN_LINUX) ./cmd/steady
```

`test` keeps cgo enabled on purpose: `-race` needs cgo on Linux; binaries are built with `CGO_ENABLED=0`.

`scripts/check_text.py` (Python 3.11+, typed, ruff and mypy strict clean, docstring states why):
- Lists files with `git ls-files -z --cached --others --exclude-standard` (subprocess, `check=True`).
- Skips paths that no longer exist and binary files (a NUL byte in the content).
- Reports every `path:line` whose bytes contain `chr(0x2014).encode()` (never write the literal), prints
  `em dash (U+2014) found:` followed by the hits, exits 1; else prints nothing and exits 0.

Task gates:
```bash
make -n check >/dev/null
python3 scripts/check_text.py
uvx ruff check scripts/ && uvx ruff format --check scripts/ && uvx mypy --strict scripts/check_text.py
printf 'x %s y\n' "$(python3 -c 'print(chr(0x2014))')" > "$TMPDIR/probe.txt" && cp "$TMPDIR/probe.txt" ./probe-emdash.txt && ! python3 scripts/check_text.py; rc=$?; rm -f ./probe-emdash.txt; exit $rc
```
DoD: gates green; the probe file is removed.

### T4. CI and release workflows (W1)

Files: `.github/workflows/ci.yml`, `.github/workflows/release.yml`.

`ci.yml`:
- `name: ci`; `on`: `push: branches: [main]`, `pull_request:`, `workflow_dispatch:`; `permissions: contents: read`.
- Job `test` (`runs-on: ubuntu-latest`, no `name:` key, so the check run is named `test`): checkout
  (`persist-credentials: false`); setup-go (`go-version-file: go.mod`); setup-node (`node-version: 26`,
  `cache: npm`, `cache-dependency-path: web/package-lock.json`); golangci-lint-action
  (`version: v2.15.0`, `install-only: true`, which puts the binary on PATH); `run: make check`.
- Job `secrets` (check run `secrets`): checkout (`fetch-depth: 0`, `persist-credentials: false`); setup-go
  (`go-version-file: go.mod`); `run: go install github.com/zricethezav/gitleaks/v8@v8.30.1` then
  `run: "$(go env GOPATH)/bin/gitleaks" git --redact --verbose .` (full history on every event, unlike
  gitleaks-action, which scans only the pushed range on push).
- Pins from S1 only.

`release.yml`:
- `name: release`; `on: push: tags: ['steady/v*']`; top-level `permissions: {}`.
- Job `build` (`permissions: contents: read`): checkout (`persist-credentials: false`), setup-go
  (`go-version-file: go.mod`), a bash step that sets `version="${GITHUB_REF_NAME#steady/}"` and loops over
  `linux/amd64 linux/arm64 darwin/amd64 darwin/arm64`, building
  `CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false -ldflags='-s -w' -o "out/steady-${version}-${os}-${arch}" ./cmd/steady`,
  then `(cd out && sha256sum steady-* > SHA256SUMS)`; upload-artifact (`name: release`, `path: out/`, `if-no-files-found: error`).
- Job `release` (`needs: build`; `permissions: contents: write, id-token: write, attestations: write`):
  download-artifact (`name: release`, `path: out`); actions/attest (`subject-path: 'out/steady-*'`);
  `run: gh release create "$GITHUB_REF_NAME" out/* --repo "$GITHUB_REPOSITORY" --title "$GITHUB_REF_NAME" --generate-notes`
  with `env: GH_TOKEN: ${{ github.token }}`. Never interpolate `${{ }}` expressions inside `run:` scripts
  except through `env:`.

Task gates:
```bash
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/ci.yml .github/workflows/release.yml
test "$(grep -hE '^\s*(-\s*)?uses:' .github/workflows/*.yml | grep -vcE '@[0-9a-f]{40} # v[0-9]')" = 0
grep -F "'steady/v*'" .github/workflows/release.yml
```
Plus S2 over both files.

### T5. Deploy kit (W1)

Files: `deploy/autodeploy.sh`, `deploy/install.sh`, `deploy/README.md`, `deploy/systemd/steady.service`,
`deploy/systemd/steady-deploy.service`, `deploy/systemd/steady-deploy.timer`.

Adapt from the deploy kit of an existing sibling project (its autodeploy script, install script, README,
service unit, deploy unit and timer).
Never read any `.env` there.

- Keep the generic core: `autodeploy.sh DEPLOY_DIR`; `repo="${AUTO_DEPLOY_REPO:-https://github.com/scv3lqq/steady.git}"`,
  `branch="${AUTO_DEPLOY_BRANCH:-main}"`, `port="${STEADY_PORT:-8770}"`, `checks=(test secrets)`, `svc=steady`,
  staged/next/prev binaries named `steady`, `installed=/usr/local/bin/steady`, `api="http://127.0.0.1:$port"`;
  isolated git env; head commit signature check against `DEPLOY_DIR/allowed_signers`; force-push hold via
  `merge-base --is-ancestor`; CI wait through the unauthenticated check-runs API with `curl` + `jq`;
  kit-drift check; build `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$next" ./cmd/steady`
  (only the Go toolchain is needed because `web/dist` is committed); unchanged-sha256 shortcut; restart;
  health check `curl -fsS -m 2 "$api/healthz"` up to 30 times, 1 s apart; rollback to `.prev` and reject.
- Drop everything anki-specific: the pass-in-progress gate, `provider.sh`, `smoke.sh`, `test-provider.sh`,
  `deploy/anki/`, Claude/Codex installs, AnkiConnect key, secret handling, settings list, session report.
- `install.sh`: keep the deploy-user model and the polkit rule for the one unit; create system user `steady`
  (home `/var/lib/steady`, nologin); write `/etc/steady/env` (root:steady 0640) with
  `STEADY_ADDR=127.0.0.1:8770`, `STEADY_DB=/var/lib/steady/steady.db`, `STEADY_LOG_LEVEL=info` and a commented
  `# STEADY_ALLOWED_HOSTS=habits.example.com` line; fill `@DEPLOY_DIR@`, `@DEPLOY_USER@`, `@PORT@` in the units.
- `steady.service`: anki's hardening set plus `MemoryDenyWriteExecute=yes` (Go does not JIT);
  `User=steady`, `Group=steady`, `UMask=0077`, `StateDirectory=steady` (mode 0700), `EnvironmentFile=/etc/steady/env`,
  `ExecStartPre=-+/usr/bin/install -C -o root -g root -m 0755 @DEPLOY_DIR@/bin/steady /usr/local/bin/steady`,
  `ExecStart=/usr/local/bin/steady`, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`, no docker ordering.
- Timer: `OnActiveSec=1min`, `OnUnitInactiveSec=5min`, `RandomizedDelaySec=30s`.
- `deploy/README.md`: requirements (systemd Linux, git, curl, jq, Go toolchain), install, `allowed_signers`
  format, the autodeploy pass step by step, rollback, how to reach the app (SSH tunnel
  `ssh -N -L 8770:127.0.0.1:8770 <user>@<server>`, or a reverse proxy that adds auth and whose public host
  goes into `STEADY_ALLOWED_HOSTS`), backups (`sqlite3 /var/lib/steady/steady.db ".backup <file>"`, never copy
  the live file without its `-wal`), and a note: "The kit assumes the repository is public: the host fetches
  over anonymous HTTPS and reads check runs from the unauthenticated GitHub API. While the repository is
  private, autodeploy cannot fetch or see CI." Placeholders only (`<user>`, `<server>`, `example.com`).

Task gates:
```bash
for s in deploy/*.sh; do bash -n "$s" || exit 1; done
! grep -rniE 'anki|claude|codex|8767' deploy/
grep -F '/healthz' deploy/autodeploy.sh && grep -F 'allowed_signers' deploy/autodeploy.sh
grep -F 'MemoryDenyWriteExecute=yes' deploy/systemd/steady.service
grep -F 'OnUnitInactiveSec=5min' deploy/systemd/steady-deploy.timer
```
Plus S2 over every file in `deploy/`.

### T6. Static page: HTML and CSS (W1)

Files: `web/index.html`, `web/styles.css`.

- `index.html`: the S13 block byte for byte.
- `styles.css`: custom properties for tokens, flex/grid with `gap`, no shadows except the toast (and the
  spec's palette ring and inset rings, which are `box-shadow` rings). Required rules (values are the
  contract; add only what layout needs):
  - `:root`: `--bg #161513; --surface #1F1D1A; --line #2C2A26; --hover #26241F; --ink #ECE7DC; --muted #9C968A; --faint #847E72; --empty #3A3732; --cell-border #55514A; --accent #4FA872`.
  - `[hidden] { display: none !important; }` (the flex rules below would otherwise override `hidden`).
  - `*, *::before, *::after { box-sizing: border-box; }`; `html { background: var(--bg); color-scheme: dark; }`;
    `body { margin: 0; background: var(--bg); color: var(--ink); font-family: 'Onest', system-ui, sans-serif; font-weight: 400; font-variant-numeric: tabular-nums; }`;
    `button, input { font: inherit; color: inherit; }`; `button { cursor: pointer; border: 0; background: none; padding: 0; }`.
  - `button:focus-visible { outline: 2px solid var(--ink); outline-offset: 2px; }`; `.undo:focus-visible { outline-color: var(--bg); }`;
    inputs show focus with their 1px ink underline (`outline: none`).
  - `.app { max-width: 600px; margin: 0 auto; padding: 56px 20px 96px; display: flex; flex-direction: column; gap: 36px; }`
  - `.header { display: flex; flex-direction: column; gap: 20px; }`; `.topbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }`;
    `.wordmark { font-size: 15px; font-weight: 600; }`; `.topbar-end { display: flex; align-items: center; gap: 8px; }`;
    `.today { font-size: 14px; color: var(--muted); white-space: nowrap; }`.
  - `.accent-toggle, .palette-color { width: 32px; height: 32px; border-radius: 8px; display: grid; place-items: center; }`;
    `.accent-toggle .swatch { width: 14px; height: 14px; border-radius: 50%; background: var(--accent); }`;
    `.palette { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px; }`;
    `.palette-color .swatch { width: 22px; height: 22px; border-radius: 50%; background: var(--c); }`;
    `.palette-color[aria-checked="true"] .swatch { box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--c); }`;
    one rule per palette color: `.palette-color[data-value="#4FA872"] { --c: #4FA872; }` (all 10).
  - `.title { margin: 0; font-size: 40px; font-weight: 500; letter-spacing: -0.03em; line-height: 1.1; }`
  - `.progress { height: 4px; background: var(--line); overflow: hidden; }`; `.progress-fill { height: 100%; width: 0; background: var(--accent); transition: width 0.3s; }`.
  - `.controls { display: flex; flex-direction: column; gap: 12px; }`;
    `.toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 12px; }`.
  - `.tabs { display: flex; background: var(--surface); border: 1px solid var(--line); border-radius: 11px; padding: 3px; }`;
    `.tab { height: 34px; padding: 0 12px; border-radius: 8px; font-size: 14px; color: var(--muted); }`;
    `.tab[aria-selected="true"] { background: var(--empty); color: var(--ink); }`.
  - `.add-toggle { height: 34px; padding: 0 12px; border-radius: 8px; font-size: 14px; color: var(--muted); background: transparent; }`;
    `.add-toggle:hover { background: var(--surface); color: var(--ink); }`.
  - `.add-row { display: flex; align-items: baseline; gap: 12px; }`;
    `.add-input { flex: 1 1 auto; min-width: 0; font-size: 17px; background: transparent; border: 0; border-bottom: 1px solid var(--line); border-radius: 0; padding: 6px 0; outline: none; }`;
    `.add-input:focus { border-bottom-color: var(--ink); }`; `.add-input::placeholder { color: var(--faint); }`;
    `.hint { font-size: 12px; color: var(--faint); white-space: nowrap; }`.
  - `.list { display: flex; flex-direction: column; }`.
  - Week: `.week-head { display: flex; flex-wrap: wrap; align-items: center; column-gap: 16px; padding-bottom: 8px; }`;
    `.name-spacer { flex: 1 1 140px; }`; `.side-spacer { flex: 0 0 84px; }`; `.days { display: flex; gap: 6px; }`;
    `.wd { width: 34px; display: flex; flex-direction: column; align-items: center; font-size: 11px; line-height: 1.3; color: var(--faint); }`;
    `.wd.is-today { color: var(--ink); font-weight: 600; }`;
    `.week-row { display: flex; flex-wrap: wrap; align-items: center; column-gap: 16px; row-gap: 10px; padding: 14px 0; border-bottom: 1px solid var(--line); }`;
    `.name { flex: 1 1 140px; min-width: 0; font-size: 17px; background: transparent; border: 0; border-bottom: 1px solid transparent; border-radius: 0; padding: 4px 0; outline: none; }`;
    `.name:focus { border-bottom-color: var(--ink); }`;
    `.sq { width: 34px; height: 34px; border-radius: 9px; border: 1px solid var(--cell-border); background: transparent; display: grid; place-items: center; color: var(--bg); }`;
    `.sq.is-today { background: var(--surface); border: 1.5px solid var(--ink); }`;
    `.sq.is-done { background: var(--accent); border: 1px solid var(--accent); }` (after `.is-today`, so done wins);
    `.check { width: 16px; height: 16px; }`;
    `.side { flex: 0 0 84px; display: flex; align-items: center; justify-content: flex-end; gap: 4px; }`;
    `.streak { font-size: 13px; color: var(--ink); white-space: nowrap; }`; `.streak.is-zero { color: var(--faint); }`;
    `.del { flex: 0 0 32px; width: 32px; height: 32px; border-radius: 8px; color: var(--faint); font-size: 18px; display: grid; place-items: center; }`;
    `.del:hover { background: var(--hover); color: var(--ink); }`.
  - Blocks: `.block { border-top: 1px solid var(--line); padding: 16px 18px; display: flex; flex-direction: column; gap: 12px; }`
    (spec "16px/18px padding" read as CSS shorthand: 16px vertical, 18px horizontal);
    `.block-head { display: flex; align-items: center; gap: 12px; }`; `.count { font-size: 13px; color: var(--muted); white-space: nowrap; }`.
  - Month: `.month-grid { display: grid; grid-template-columns: repeat(30, minmax(0, 1fr)); gap: 4px; }`;
    `.dot { min-width: 0; aspect-ratio: 1; border-radius: 50%; background: var(--empty); }`; `.dot.is-done { background: var(--accent); }`;
    `.dot.is-today { box-shadow: inset 0 0 0 1.5px var(--ink); }`.
  - Year: `.year-grid { display: grid; grid-template-columns: repeat(53, minmax(0, 1fr)); grid-template-rows: repeat(7, auto); grid-auto-flow: column; gap: 3px; }`;
    `.cell { min-width: 0; aspect-ratio: 1; border-radius: 2px; background: var(--empty); }`; `.cell.is-done { background: var(--accent); }`;
    `.cell.is-today { box-shadow: inset 0 0 0 1px var(--ink); }`; `.cell.is-future { visibility: hidden; }`.
  - `.caption { display: flex; justify-content: space-between; font-size: 11px; color: var(--faint); }`.
  - `.empty { display: flex; flex-direction: column; gap: 6px; }`; `.empty-title { margin: 0; font-size: 17px; }`;
    `.empty-text { margin: 0; font-size: 14px; color: var(--muted); }`; `.footer { font-size: 13px; color: var(--faint); }`.
  - Toast: `.toast { position: fixed; left: 50%; bottom: 28px; transform: translateX(-50%); display: flex; align-items: center; gap: 12px; max-width: calc(100% - 32px); padding: 8px 8px 8px 16px; background: var(--ink); color: var(--bg); border-radius: 12px; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45); font-size: 14px; white-space: nowrap; }`;
    `.toast-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; }`;
    `.undo { flex: 0 0 auto; height: 32px; padding: 0 12px; border-radius: 8px; background: #D9D3C7; color: var(--bg); font-weight: 500; }`;
    `.undo:hover { background: #C9C2B5; }`.
  - `@media (prefers-reduced-motion: reduce) { *, *::before, *::after { transition: none !important; animation: none !important; } }`.

Task gates:
```bash
awk '$0=="```html index.html"{f=1;next} f&&$0=="```"{exit} f' plans/steady-app.md | diff - web/index.html
for c in 4FA872 8FBF4A 3FB8B0 5B8CEB B07FE0 E07BB0 D96A6A E08A45 E3C24F ECE7DC; do grep -qF "[data-value=\"#$c\"]" web/styles.css || { echo "missing $c"; exit 1; }; done
grep -F '[hidden]' web/styles.css && grep -F 'prefers-reduced-motion' web/styles.css
test "$(grep -c 'box-shadow' web/styles.css)" -le 4
```
Plus S2 over both files.

### T7. Domain tests (W2, after T1)

File: `internal/domain/domain_test.go` (package `domain`). Table-driven; literal expectations:

| Test | Cases (input -> want) | Mutation it catches |
|---|---|---|
| `TestValidateID` | `"0f8fad5b-d9cb-469f-a165-70867728950e"` -> nil; `"0F8FAD5B-D9CB-469F-A165-70867728950E"` -> ErrInvalidID; `""` -> ErrInvalidID; `"0f8fad5bd9cb469fa16570867728950e"` (32 chars) -> ErrInvalidID; `"0f8fad5b-d9cb-469f-a165-70867728950"` (35) -> ErrInvalidID; `"0f8fad5b-d9cb-469f-a165-70867728950g"` -> ErrInvalidID; `"0f8fad5b_d9cb-469f-a165-70867728950e"` -> ErrInvalidID | M1: `ValidateID` accepts `A-F` too (case-insensitive hex check) |
| `TestNormalizeName` | `"  Read  "` -> `"Read"`, nil; `""` -> `"Untitled"`; `"   "` -> `"Untitled"`; 60 x `"é"` -> same string, nil; 61 x `"é"` -> `""`, ErrNameTooLong; 60 x `"a"` -> nil | M2: compare `len(name)` (bytes) instead of rune count (60 x `é` then fails) |
| `TestValidateDay` | `"2026-11-01"` nil; `"2028-02-29"` nil; `"2026-02-29"` ErrInvalidDay; `"2026-13-01"` ErrInvalidDay; `"2026-1-1"` ErrInvalidDay; `"2026-11-01T00:00:00Z"` ErrInvalidDay; `""` ErrInvalidDay | M3: drop the parse error check (return nil whenever the round-trip is skipped) |
| `TestParseView` | `"week"`, `"month"`, `"year"` -> same View, nil; `"Week"`, `""`, `"decade"` -> `""`, ErrInvalidView | M4: `strings.EqualFold` matching |
| `TestValidateAccent` | each of the 10 palette colors -> nil; `"#4fa872"` -> ErrInvalidAccent; `"#000000"` -> ErrInvalidAccent; `""` -> ErrInvalidAccent | M5: case-insensitive compare |
| `TestNewSettingsUpdate` | (nil, nil) -> ErrEmptySettings; (`"month"`, nil) -> View month, Accent nil; (nil, `"#5B8CEB"`) -> Accent `#5B8CEB`, View nil; (`"decade"`, `"#5B8CEB"`) -> ErrInvalidView; (`"week"`, `"#000000"`) -> ErrInvalidAccent | M6: return nil error when both are nil |
| `TestPaletteIsCopy` | `p := Palette(); p[0] = "x"`; `Palette()[0] == "#4FA872"`, `len(Palette()) == 10` | M7: `Palette` returns a package-level slice |
| `TestErrorTexts` | `ErrNotFound.Error() == "habit not found"`, `ErrExists.Error() == "habit already exists"`, `ErrInvalidDay.Error() == "invalid day: want YYYY-MM-DD"` | M8: change a message |

Compare errors with `errors.Is`. Mutation command: `go vet ./internal/domain/ && go test ./internal/domain/`.
Task gates: `gofmt -l internal/domain`, `go vet ./internal/domain/`, `golangci-lint run ./internal/domain/...`, `go test -race ./internal/domain/`.

### T8. Config (W2, after T1)

File: `internal/config/config.go`. S7 exactly; package doc comment.
Task gates: `gofmt -l internal/config`, `go vet ./internal/config/`, `golangci-lint run ./internal/config/...`, `go build ./internal/config/`, S2.

### T9. SQLite store and seed (W2, after T1)

Files: `internal/repository/sqlite/store.go`, `internal/repository/sqlite/seed.go` (package `sqlite`).

- `store.go`: `//go:embed migrations/*.sql`; `Open`: `os.MkdirAll(filepath.Dir(path), 0o750)`, `sql.Open("sqlite", dsn)`
  with the S10 DSN, `SetMaxOpenConns(1)`, `PingContext`, goose Up (S10); on any error close the db and return
  `fmt.Errorf("open sqlite %s: %w", path, err)` / `fmt.Errorf("migrate: %w", err)`.
- `State`: `ListSettings` into a map; view = `domain.DefaultView` unless the stored value passes
  `domain.ParseView`; accent = `domain.DefaultAccent` unless it passes `domain.ValidateAccent`;
  `ListActiveHabits`; `ListActiveDays` grouped by habit id (already sorted by the query); every habit's
  `Done` is a non-nil slice; `Habits` non-nil.
- `AddHabit`: `InsertHabit`, 0 rows -> `domain.ErrExists`. `RenameHabit`: 0 rows -> `domain.ErrNotFound`.
  `DeleteHabit`: `SoftDeleteHabit{DeletedAt: at.Unix(), ID: id}`, 0 rows -> ErrNotFound. `RestoreHabit`: 0 rows
  -> ErrNotFound. `PurgeDeleted`: `PurgeDeletedBefore(before.Unix())`.
- `SetDay`: one tx (`BeginTx`, `defer tx.Rollback()`), `q := s.q.WithTx(tx)`; `CountActiveHabit == 0` ->
  ErrNotFound; `MarkDay` or `UnmarkDay`; commit.
- `UpdateSettings`: one tx; `UpsertSetting` for `view` when `u.View != nil` and for `accent` when `u.Accent != nil`.
- `Ping`: `s.db.PingContext`. `Close`: `s.db.Close`.
- `seed.go`: `SeedOnce(ctx, now, rng)`: one tx; `InsertSettingIfAbsent{"seeded", "1"}`; 0 rows -> commit, return
  false. Else for `i, name := range domain.SeedNames()` with chances `[]float64{0.8, 0.6, 0.45}` (local
  variable): `id := newID()`, `InsertHabit`, then for `d := 0; d < 365; d++` mark
  `now.AddDate(0, 0, -d).Format(domain.DayLayout)` when `rng.Float64() < chance`. Commit; return true.
  `newID() (string, error)`: 16 bytes from `crypto/rand.Read`, `b[6] = b[6]&0x0f | 0x40`, `b[8] = b[8]&0x3f | 0x80`,
  formatted lowercase `%x-%x-%x-%x-%x` over `b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]`.
- Wrap every error `fmt.Errorf("<op>: %w", err)` except returning the domain sentinels (wrap those too, e.g.
  `fmt.Errorf("rename habit %s: %w", id, domain.ErrNotFound)`, so `errors.Is` still works).

Task gates: `gofmt -l internal/repository/sqlite`, `go vet ./internal/repository/sqlite/...`,
`golangci-lint run ./internal/repository/sqlite/...`, `go build ./internal/repository/sqlite/...`, S2.

### T10. Use case service (W2, after T1)

File: `internal/usecase/service.go` (package `usecase`). S4 port plus S5 API:
- `AddHabit`: `domain.ValidateID`, `domain.NormalizeName`, `repo.AddHabit`; returns `Habit{ID, Name, Done: []string{}}`.
- `RenameHabit`: `ValidateID`; `name == nil` -> `domain.ErrNameRequired`; `NormalizeName(*name)`; `repo.RenameHabit`;
  returns `Habit{ID, Name}` (Done nil).
- `DeleteHabit`: `ValidateID`; `repo.DeleteHabit(ctx, id, s.now())`; on success call `s.Purge(ctx)`; a purge
  error is logged `s.log.Warn().Err(err).Msg("purge deleted habits")` and not returned.
- `RestoreHabit`: `ValidateID`; repo. `SetDay`: `ValidateID`, `ValidateDay`, repo.
- `UpdateSettings`: `domain.NewSettingsUpdate(view, accent)`; repo.
- `Purge`: `repo.PurgeDeleted(ctx, s.now().Add(-PurgeAfter))`. `State`, `Ping`: pass through.
- Validation happens before any repository call. Wrap with op names: `state`, `add habit`, `rename habit`,
  `delete habit`, `restore habit`, `set day`, `update settings`, `purge`, `ping`.

Task gates: `gofmt -l internal/usecase`, `go vet ./internal/usecase/`, `golangci-lint run ./internal/usecase/...`, S2.

### T11. Date helper tests (W2, after T2)

File: `web/test/dates.test.mjs`. `import { test } from 'node:test'; import assert from 'node:assert/strict';`
`import * as d from '../dist/dates.js';`. Runs under `TZ=America/New_York` (npm script). Cases (dates are
built with `new Date(y, mIndex, day[, h, min])`, local):

| Case | Expect | Mutation it catches |
|---|---|---|
| dayKey | `dayKey(new Date(2026, 0, 5)) === '2026-01-05'`; `dayKey(new Date(2026, 10, 1, 23, 59)) === '2026-11-01'` | N1: `dayKey` via `toISOString().slice(0, 10)` (gives 2026-11-02 at 23:59 in New York) |
| fromKey | `fromKey('2026-11-01')` has year 2026, month 10, date 1, hours 0 | N2: `new Date(key)` (UTC parse, 2026-10-31 20:00 local) |
| addDays across DST | `dayKey(addDays(new Date(2026, 10, 1), 1)) === '2026-11-02'`; `dayKey(addDays(new Date(2026, 10, 1), -1)) === '2026-10-31'`; `dayKey(addDays(new Date(2026, 2, 7), 1)) === '2026-03-08'`; `dayKey(addDays(new Date(2026, 2, 8), 1)) === '2026-03-09'` | N3: `new Date(d.getTime() + n * 86400000)` (Nov 1 + 24 h is still Nov 1 23:00) |
| addDays month/year ends | `'2027-01-01'` from (2026, 11, 31) + 1; `'2026-12-31'` from (2027, 0, 1) - 1; `'2028-02-29'` from (2028, 1, 28) + 1; `'2026-03-01'` from (2026, 1, 28) + 1 | none named (all four dates are in standard time, so N3 passes here); kept as a calendar check |
| lastNDays | `lastNDays(new Date(2026, 10, 1, 15, 30), 30)`: length 30, first `'2026-10-03'`, last `'2026-11-01'`, last has hours 0; with n = 7 first is `'2026-10-26'`; `lastNDays(new Date(2026, 2, 20), 30)` first `'2026-02-19'`, last `'2026-03-20'` (crosses the March DST start) | N4: off by one (`-n` instead of `-(n - 1)`) |
| mondayOf | Sunday (2026, 10, 1) -> `'2026-10-26'`; Monday (2026, 9, 26) -> `'2026-10-26'`; Wednesday (2026, 10, 4) -> `'2026-11-02'` | N5: `(getDay() + 7) % 7` (Sunday maps to itself) |
| yearGrid | `yearGrid(new Date(2026, 10, 1))`: length 371, first `'2025-10-27'`, index 370 `'2026-11-01'`; `yearGrid(new Date(2026, 10, 4))`: first `'2025-11-03'`, index 370 `'2026-11-08'`; every 7th element (index % 7 === 0) is a Monday (`getDay() === 1`) | N6: 52 columns (`-7 * 51`) |
| yearCaptionMonth | `formatMonthYear(yearCaptionMonth(fromKey('2025-10-27'))) === 'November 2025'`; `dayKey(yearCaptionMonth(fromKey('2025-12-01'))) === '2025-12-01'`; `dayKey(yearCaptionMonth(fromKey('2025-12-29'))) === '2026-01-01'` | N7: always the next month (breaks the 12-01 case) |
| streak | today (2026, 10, 1): `{'2026-10-30','2026-10-31','2026-11-01'}` -> 3; `{'2026-10-30','2026-10-31'}` -> 2; `{'2026-11-01'}` -> 1; `{}` -> 0; `{'2026-10-30'}` -> 0; `{'2026-10-29','2026-10-31','2026-11-01'}` -> 2; today (2026, 10, 2): `{'2026-10-31','2026-11-01','2026-11-02'}` -> 3; today (2026, 2, 9): `{'2026-03-07','2026-03-08','2026-03-09'}` -> 3 (crosses the March DST start) | N8: no fallback to yesterday (the 2-day case gives 0) |
| countDone | done `{'2026-10-30','2026-11-01','2025-01-01'}` over `lastNDays(new Date(2026, 10, 1), 30)` -> 2 | N9: count all keys of `done` |
| pluralDays | 0 -> `'0 days'`, 1 -> `'1 day'`, 2 -> `'2 days'`, 21 -> `'21 days'`, 214 -> `'214 days'` | N10: `n <= 1` singular (0 gives '0 day') |
| formatters | `formatHeaderDate(new Date(2026, 10, 1)) === 'Sunday, November 1'`; `formatShortDate(new Date(2026, 9, 3)) === 'Oct 3'`; `formatLongDate(new Date(2026, 9, 30)) === 'October 30'`; `formatMonthYear(new Date(2025, 10, 1)) === 'November 2025'`; `weekdayShort` of (2026, 9, 26) `'Mo'`, (2026, 9, 29) `'Th'`, (2026, 10, 1) `'Su'` | N11: `weekdayShort` slices 3 chars |

Plain JS (no build step). Mutation command: `(cd web && npm run build && npm test)` (mutate `web/src/dates.ts`).
Task gates: `(cd web && npm run build && npm test)`, S2 over the test file.

### T12. Renderer (W2, after T2)

Files: `web/src/render.ts`, `web/dist/render.js`.

```ts
import type { Deleted, State } from './store.js';
export interface Ui { paletteOpen: boolean; adding: boolean; toast: Deleted | null; }
export function render(root: HTMLElement, state: State, ui: Ui, now: Date): void;
```

One `render()` redraws everything from state; no module-level mutable state (keyed rows live in the DOM).
It queries the S13 skeleton inside `root` by id each call and:
- Sets `#today` text `formatHeaderDate(now)`; `--accent` (S14); palette: `#palette.hidden = !ui.paletteOpen`,
  `#accent-toggle[aria-expanded]`, each `.palette-color[aria-checked]` = its `data-value === state.accent`.
- `#title` text and `#progress` width (S14); tabs `aria-selected` = `data-value === state.view`.
- `#add-row.hidden = !ui.adding` (never touches `#add-input`'s value or focus).
- Toast: `#toast.hidden = ui.toast === null`; `#toast-text` text per S14.
- List (S13 structures), keyed so a focused input is never rebuilt or moved:
  1. Collect existing children of `#list` that have `data-id` and `data-view === state.view` into a map by id.
  2. Build the wanted child list: no habits -> a fresh `div.empty`; else (week view) a fresh `div.week-head`
     first, then per habit the existing row/block for its id or a newly created one, updated in place.
  3. Remove children not in the wanted list; then for each index `i` insert `wanted[i]` with
     `list.insertBefore(wanted[i], list.children[i] ?? null)` only when `list.children[i] !== wanted[i]`.
  4. Updating a row in place: set `input.name.value = habit.name` only when the input is not
     `document.activeElement` and the value differs; update `button.del` aria-label; update streak/count
     text and `.is-zero`; sync day buttons: if the container already holds the same number of buttons with the
     same `data-date` sequence, patch classes, `aria-pressed`, `aria-label`, `disabled` and the check icon in
     place (so a focused day button keeps focus); otherwise rebuild that container's buttons. Captions update text.
- Days: week `lastNDays(now, 7)`; month `lastNDays(now, MONTH_DAYS)`; year `yearGrid(now)` with
  `.is-future` + `disabled` for keys greater than `dayKey(now)` (string compare). `.is-today` on the today key.
  Week head shows `weekdayShort` and `getDate()` per day, `.is-today` on today.
- All text through `textContent`; never `innerHTML`, `insertAdjacentHTML` or `outerHTML`.

Task gates:
```bash
(cd web && npm run build)
! grep -nE 'innerHTML|insertAdjacentHTML|outerHTML|setAttribute\(.style|toISOString' web/src/render.ts
grep -F 'fromCharCode(0x2014)' web/src/render.ts
```
Plus S2 over `render.ts`.

### T13. Entry point (W3, after T12)

Files: `web/src/main.ts`, `web/dist/main.js`.

- One `start()` function (called once at module end) holding all state in closures: `store = new Store(draw)`,
  `ui: Ui = {paletteOpen: false, adding: false, toast: null}`, the toast timer handle, and
  `draw = () => render(root, store.state, ui, new Date())` where `root = document.getElementById('app')`.
- Delegated listeners on `root` only (plus `pagehide` on `window`):
  - `click` on `closest('[data-action]')`: `toggle-palette` flips `ui.paletteOpen`, draw; `accent` ->
    `store.setAccent(data-value)`; `view` -> `store.setView` when the value is `week|month|year`;
    `show-add` -> `ui.adding = true`, draw, focus `#add-input`; `toggle` -> `store.toggleDay(data-id, data-date)`;
    `delete` -> `d = store.deleteHabit(id)`; when non-null set `ui.toast = d`, clear and restart a 5000 ms
    timer that sets `ui.toast = null` and draws; draw; `undo` -> when `ui.toast`: `store.restoreHabit(ui.toast)`,
    `ui.toast = null`, clear the timer, draw.
  - `input` on `input.name` -> `store.renameHabit(id, value)`.
  - `focusout` on `input.name` -> `store.commitRename(id)`; on `#add-input` with an empty trimmed value ->
    close the row (`ui.adding = false`, draw).
  - `keydown` on `#add-input`: `Enter` -> trimmed value non-empty: `store.addHabit(value)`; in both cases
    clear the input, `ui.adding = false`, draw. `Escape` -> clear, `ui.adding = false`, draw.
  - `window` `pagehide` -> `store.flushPending()`.
- Start: `store.load()` (its `onChange` draws). No console output.

Task gates: `(cd web && npm run build)`, `! grep -nE 'innerHTML|localStorage|console\.' web/src/main.ts`, S2.

### T14. Config tests (W3, after T8)

File: `internal/config/config_test.go`. Table-driven over `Load` with a map-backed getenv:

| Case | env | want |
|---|---|---|
| defaults | none | Addr `127.0.0.1:8770`, DBPath `data/steady.db`, LogLevel `zerolog.InfoLevel`, ExtraHosts nil |
| overrides | `STEADY_ADDR=0.0.0.0:9000`, `STEADY_DB=/tmp/x.db`, `STEADY_LOG_LEVEL=DEBUG` | those values, LogLevel Debug |
| hosts | `STEADY_ALLOWED_HOSTS=" a.example , ,B.example:8443 "` | ExtraHosts `["a.example", "b.example:8443"]` |
| bad addr | `STEADY_ADDR=nope` | error text `config: STEADY_ADDR "nope": want host:port` |
| bad port | `STEADY_ADDR=127.0.0.1:99999` and `127.0.0.1:x` | error (same format) |
| bad level | `STEADY_LOG_LEVEL=loud` | error text `config: STEADY_LOG_LEVEL "loud": unknown level` |

Plus `TestAllowedHosts`: `Config{Addr: "127.0.0.1:8770", ExtraHosts: []string{"a.example"}}.AllowedHosts()` ->
`["127.0.0.1:8770", "a.example"]`.
Mutations: C1 drop the port range check (99999 accepted); C2 skip `strings.ToLower` on hosts; C3 drop
empty-item filtering. Mutation command: `go vet ./internal/config/ && go test ./internal/config/`.
Task gates: gofmt, vet, golangci-lint on the package, `go test -race ./internal/config/`.

### T15. Store tests (W3, after T9)

File: `internal/repository/sqlite/store_test.go` (package `sqlite`, internal test so raw SQL via `s.db` is
allowed for assertions). Helper `newStore(t)` opens `filepath.Join(t.TempDir(), "steady.db")` with
`t.Context()` and registers `Close` in `t.Cleanup`. IDs: `idA = "00000000-0000-4000-8000-00000000000a"`,
`idB ...0b`, `idC ...0c`, `idD ...0d`. `t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)`.

| Test | Steps and literal expectations | Mutation it catches |
|---|---|---|
| `TestOpenPragmas` | `PRAGMA journal_mode` = `wal`, `PRAGMA foreign_keys` = 1, `PRAGMA busy_timeout` = 5000 | S1: drop `_pragma=foreign_keys(1)` from the DSN |
| `TestAddHabitOrderAndConflict` | add A `Alpha`, B `Beta`; State names `[Alpha Beta]`, each `Done` non-nil with len 0; add A again -> `errors.Is(err, domain.ErrExists)` | S2: `AddHabit` ignores 0 affected rows |
| `TestSoftDeleteRestoreKeepsPosition` | add A, B, C; `SetDay(B, "2026-10-31", true)`; `DeleteHabit(B, t0)` -> names `[A C]`; add D -> `[A C D]`; `RestoreHabit(B)` -> `[A B C D]` and B's Done `["2026-10-31"]`; on a fresh store: add B `Beta`, C `Gamma`; delete C; add A `Alpha`; restore C -> names `[Beta Gamma Alpha]` | S3: `InsertHabit` computes `MAX(position)` over `WHERE deleted_at IS NULL` only (edit queries.sql, `make generate`), D collides with B |
| `TestNotFound` (table) | rename unknown; delete unknown; delete B twice (second); restore unknown; `SetDay` unknown; `SetDay` on soft-deleted B; rename soft-deleted B -> each `errors.Is(err, domain.ErrNotFound)` | S4: `SetDay` skips the `CountActiveHabit` check |
| `TestPurge` | add A, B; `SetDay(A, "2026-10-30", true)`; `DeleteHabit(A, t0)`; `PurgeDeleted(t0)` -> 0; `PurgeDeleted(t0.Add(time.Second))` -> 1; `RestoreHabit(A)` -> ErrNotFound; `SELECT COUNT(*) FROM habit_days WHERE habit_id = idA` -> 0; State names `[B]` | S5: `<` becomes `<=` in `PurgeDeletedBefore` (first purge returns 1) |
| `TestSetDay` | mark `2026-10-31` twice -> Done `["2026-10-31"]`; mark `2026-10-01` -> `["2026-10-01" "2026-10-31"]`; unmark `2026-10-31` -> `["2026-10-01"]`; unmark again -> nil error | S6: `SetDay` calls `MarkDay` for `done == false` |
| `TestSettings` | defaults `week`, `#4FA872`; update view `month` -> `month`, `#4FA872`; update accent `#5B8CEB` -> `month`, `#5B8CEB`; update view `year` -> `year`, `#5B8CEB`; raw `UPDATE settings SET value = 'decade' WHERE key = 'view'` -> State view `week` | S7: `UpdateSettings` writes the accent key with an empty value when `Accent == nil` |
| `TestSeedOnce` | `now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)`, `rng := rand.New(rand.NewPCG(1, 2))`; first call -> true; names equal `domain.SeedNames()`; each Done non-empty, every key `>= "2025-11-02"` and `<= "2026-11-01"`; second call -> false, still 3 habits; delete all three; third call -> false, 0 habits | S8: `SeedOnce` ignores the `InsertSettingIfAbsent` row count |

Mutation command: `go vet ./internal/repository/sqlite/... && go test ./internal/repository/sqlite/...`
(S3 also needs `make generate` before the test). Task gates: gofmt, vet, golangci-lint on the package,
`go test -race ./internal/repository/sqlite/...`.

### T16. HTTP adapter (W3, after T10)

Files: `internal/handler/httpapi/handler.go`, `internal/handler/httpapi/middleware.go` (package `httpapi`).

- `handler.go`: S8 `New`; routes exactly as S6 on `http.NewServeMux()` with method patterns (`GET /{$}`,
  `GET /styles.css`, `GET /dist/{file}`, `GET /api/state`, `POST /api/habits`, `PATCH /api/habits/{id}`,
  `DELETE /api/habits/{id}`, `POST /api/habits/{id}/restore`, `PUT /api/habits/{id}/days/{day}`,
  `DELETE /api/habits/{id}/days/{day}`, `PUT /api/settings`, `GET /healthz`); JSON DTOs with tags
  (`habitDTO{ID "id", Name "name", Done []string "done"}`, `stateDTO{Habits "habits", View "view", Accent "accent"}`,
  `nameDTO{ID "id", Name "name"}`, request structs `createReq{ID string "id"; Name string "name"}`,
  `renameReq{Name *string "name"}`, `settingsReq{View *string "view"; Accent *string "accent"}`); `Done` and
  `Habits` always encode as `[]`; decode helper and error helper per S6; static assets from the in-memory map.
- `middleware.go`: `secureHeaders(next)`, `allowHosts(extra []string, next)`, the cross-origin wrapper with
  its deny handler, `hostAllowed(host string, extra []string) bool` per S6. `New` returns
  `secureHeaders(allowHosts(opts.AllowedHosts, cop.Handler(mux)))`.

Task gates: gofmt, `go vet ./internal/handler/httpapi/`, `golangci-lint run ./internal/handler/httpapi/...`,
`go build ./internal/handler/httpapi/`, S2.

### T17. Use case tests (W3, after T10)

File: `internal/usecase/service_test.go`. A hand-written fake `Repository` records calls (method names,
args) and returns configured errors. `now` fixed at `time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)`.

| Case | Expect | Mutation |
|---|---|---|
| AddHabit bad id `"x"` | `errors.Is(err, domain.ErrInvalidID)`; no repo call | U1: call the repo before validating |
| AddHabit `"  Read "` | repo got name `Read`; returned `Habit{ID, "Read", []string{}}` with non-nil Done | U2: pass the raw name |
| RenameHabit nil name | `domain.ErrNameRequired`; no repo call | U3: treat nil as empty (`Untitled`) |
| RenameHabit `""` | repo got `Untitled` | |
| DeleteHabit ok | repo calls `DeleteHabit(at = now)` then `PurgeDeleted(before = now - 24h)` | U4: purge with `now` instead of `now - PurgeAfter` |
| DeleteHabit, purge fails | returns nil | U5: return the purge error |
| DeleteHabit, repo ErrNotFound | `errors.Is(err, domain.ErrNotFound)`; no purge call | U6: purge before deleting |
| SetDay bad day `2026-1-1` | `domain.ErrInvalidDay`; no repo call | |
| UpdateSettings (nil, nil) | `domain.ErrEmptySettings` | |
| UpdateSettings (`year`, nil) | repo got View `year`, Accent nil | |
| Purge | repo got `before = 2026-10-31T12:00:00Z` | U4 |

Mutation command: `go vet ./internal/usecase/ && go test ./internal/usecase/`. Task gates: gofmt, vet,
golangci-lint on the package, `go test -race ./internal/usecase/`.

### T18. HTTP tests (W4, after T16 and T9)

Files: `internal/handler/httpapi/handler_test.go`, `internal/handler/httpapi/middleware_test.go` (package `httpapi`).

Harness: real stack on a temp file: `sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "steady.db"))`,
`usecase.New(store, fixedNow, zerolog.Nop())`, `New(svc, static, Options{AllowedHosts: []string{"steady.example.com"}}, zerolog.Nop())`
with `static := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>t</title>")}, "styles.css": {Data: []byte("body{}")}, "dist/main.js": {Data: []byte("export {};")}}`.
Requests through `httptest.NewRequestWithContext` + `httptest.NewRecorder`, `req.Host = "127.0.0.1:8770"`
unless the case sets it, mutating requests with `Sec-Fetch-Site: same-origin`. IDs as in T15.
Bodies compared with `strings.TrimSpace`.

`TestRoutes` (table, sequential steps on one server, each step: method, path, body, want status, want body):
1. `GET /api/state` -> 200 `{"habits":[],"view":"week","accent":"#4FA872"}`
2. `POST /api/habits {"id":idA,"name":"  Read  "}` -> 201 `{"id":"<idA>","name":"Read","done":[]}`
3. `POST /api/habits {"id":idB,"name":""}` -> 201 name `Untitled`
4. `POST /api/habits {"id":"abc","name":"x"}` -> 400 `{"error":"invalid habit id: want a lowercase UUID"}`
5. `POST` with an uppercase UUID -> 400 (same body)
6. `POST {"id":idC,"name":<61 x é>}` -> 400 `{"error":"name is longer than 60 characters"}`; with 60 x `é` -> 201
7. `POST {"id":idA,"name":"again"}` -> 409 `{"error":"habit already exists"}`
8. `POST {"id":idD,"name":"x","extra":1}` -> 400 `{"error":"invalid JSON body"}`; `POST` body `{` -> same; body of 5000 bytes -> same; body `{"id":idD}{}` -> same
9. `PATCH /api/habits/<idA> {"name":"New"}` -> 200 `{"id":"<idA>","name":"New"}`; `PATCH .../<idD> {"name":"x"}` -> 404 `{"error":"habit not found"}`; `PATCH .../<idA> {}` -> 400 `{"error":"name is required"}`; `PATCH /api/habits/abc {"name":"x"}` -> 400 invalid id
10. `PUT /api/habits/<idA>/days/2026-10-31` -> 204; `GET /api/state` shows idA done `["2026-10-31"]`; `PUT .../days/2026-13-01` -> 400 `{"error":"invalid day: want YYYY-MM-DD"}`; `PUT .../days/2026-1-1` -> 400; `PUT /api/habits/<idD>/days/2026-10-31` -> 404; `DELETE .../<idA>/days/2026-10-31` -> 204
11. `DELETE /api/habits/<idB>` -> 204; again -> 404; `GET /api/state` habit ids `[idA idC]`; `POST /api/habits/<idB>/restore` -> 204; state ids `[idA idB idC]` (original index); `POST /api/habits/<idD>/restore` -> 404
12. `PUT /api/settings {"view":"month"}` -> 204 and state view `month`; `{"view":"decade"}` -> 400 `{"error":"invalid view: want week, month or year"}`; `{"accent":"#000000"}` -> 400 `{"error":"invalid accent: want one of the palette colors"}`; `{}` -> 400 `{"error":"settings: nothing to update"}`; `{"accent":"#5B8CEB"}` -> 204 and state accent `#5B8CEB`
13. `POST /api/state` -> 405
14. `GET /healthz` -> 200 `{"status":"ok"}`

`TestHealthzDown`: close the store, `GET /healthz` -> 503 `{"error":"database unavailable"}`.

`TestStatic` (table): `GET /` 200 `text/html; charset=utf-8` body equal to the MapFS index; `GET /styles.css`
200 `text/css; charset=utf-8`; `GET /dist/main.js` 200 `text/javascript; charset=utf-8`; `GET /dist/missing.js` 404;
`GET /dist/` 404; `GET /nope` 404.

`TestHostAllowlist` (table, `GET /api/state`): Host `evil.example` -> 403 `{"error":"host not allowed"}`;
`127.0.0.1.evil.example` -> 403; `""` -> 403; `localhost:9999` -> 200; `[::1]:8770` -> 200; `127.0.0.1` -> 200;
`steady.example.com` -> 200; `Steady.Example.COM` -> 200; `steady.example.com:8443` -> 200 (entry without port);
and with `Options{AllowedHosts: []string{"proxy.example:8443"}}`: `proxy.example:8443` -> 200, `proxy.example:9000` -> 403.

`TestCrossOrigin` (table): `POST /api/habits` valid body with `Sec-Fetch-Site: cross-site` -> 403
`{"error":"cross-origin request rejected"}` and a following `GET /api/state` shows no habit; `Origin: http://evil.example`
without `Sec-Fetch-Site` -> 403; `Sec-Fetch-Site: same-origin` -> 201; `GET /api/state` with `Sec-Fetch-Site: cross-site` -> 200.

`TestSecurityHeaders`: on a 200 (`GET /api/state`), a 403 (bad Host) and a static 200 (`GET /`): CSP equals the
S6 string exactly; `X-Content-Type-Options: nosniff`; `Referrer-Policy: no-referrer`; `Cache-Control: no-store`.

Mutations (mutation command `go vet ./internal/handler/httpapi/ && go test ./internal/handler/httpapi/`):
H1 `hostAllowed` returns true for any Host; H2 remove `cop.Handler` from the chain; H3 drop `DisallowUnknownFields`;
H4 map `domain.ErrNotFound` to 400; H5 `secureHeaders` skips CSP; H6 `GET /dist/{file}` serves files outside the map
(e.g. falls back to `http.FileServerFS`, listing `/dist/`); H7 an entry without a port no longer matches other ports.
Task gates: gofmt, vet, golangci-lint on the package, `go test -race ./internal/handler/httpapi/`.

### T19. Wiring: app, cmd, embed (W4, after T8, T9, T10, T16)

Files: `internal/app/app.go`, `cmd/steady/main.go`, `web/embed.go`.

- `web/embed.go`: the S9 block.
- `app.New`: `sqlite.Open(ctx, cfg.DBPath)`; `store.SeedOnce(ctx, time.Now(), rng)` with
  `rng := rand.New(rand.NewChaCha8(seed))`, `seed` from `crypto/rand` (32 bytes); log `info` `seeded demo habits`
  when it returns true; `svc := usecase.New(store, time.Now, log)`; `svc.Purge(ctx)` (log `info` `purged deleted habits`
  with `count` when > 0); `httpapi.New(svc, web.FS, httpapi.Options{AllowedHosts: cfg.AllowedHosts()}, log)`.
  Any error closes the store and returns wrapped. `var _ usecase.Repository = (*sqlite.Store)(nil)` lives here.
- `cmd/steady/main.go`: `main` calls `run()`; on error print to stderr and `os.Exit(1)`. `run`: `config.Load(os.Getenv)`;
  `zerolog.New(os.Stderr).Level(cfg.LogLevel).With().Timestamp().Logger()`; `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`;
  `app.New`; `net.Listen("tcp", cfg.Addr)`; `http.Server{Handler: a.Handler(), ReadHeaderTimeout: 5s, ReadTimeout: 10s, WriteTimeout: 10s, IdleTimeout: 60s}`;
  log `info` `listening` with `addr`; serve in a goroutine; on signal `Shutdown` with a 10 s timeout, log
  `shutting down`; close the app.

Task gates: gofmt, `go vet ./internal/app/ ./cmd/steady/ ./web/`, `golangci-lint run ./internal/app/... ./cmd/... ./web/...`,
`CGO_ENABLED=0 go build -o "$TMPDIR/steady" ./cmd/steady`, then a smoke run:
```bash
db="$TMPDIR/steady-smoke.db"; rm -f "$db"*
STEADY_DB="$db" STEADY_ADDR=127.0.0.1:18771 "$TMPDIR/steady" & pid=$!
sleep 2; curl -fsS http://127.0.0.1:18771/healthz; curl -fsS http://127.0.0.1:18771/api/state | head -c 200; echo
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18771/dist/main.js
kill -TERM $pid; wait $pid
```
Expect `{"status":"ok"}`, three seeded habits, `200`, exit 0 after SIGTERM.

### T20. App test (W5, after T19)

File: `internal/app/app_test.go`. `TestNewServesSeededApp`: `cfg := config.Config{Addr: "127.0.0.1:8770", DBPath: filepath.Join(t.TempDir(), "steady.db"), LogLevel: zerolog.Disabled}`;
`New`; via `httptest` with Host `127.0.0.1:8770`: `GET /healthz` 200; `GET /api/state` has 3 habits whose names equal
`domain.SeedNames()`; `GET /` 200 containing `<script type="module" src="/dist/main.js"></script>`;
`GET /dist/main.js`, `/dist/render.js`, `/dist/store.js`, `/dist/dates.js` 200; `Close`; `New` again on the same DB ->
still 3 habits. Mutation A1: `app.New` drops the `SeedOnce` call (0 habits). Mutation command:
`go vet ./internal/app/ && go test ./internal/app/`. Task gates: gofmt, vet, golangci-lint on the package, `go test -race ./internal/app/`.

### T21. Docs: README and AGENTS (W5, after T19)

Files: `README.md`, `AGENTS.md`.

- `README.md` (replace the stub): one-paragraph what it is; screenshots (S16; week, month, year at 600 px in a
  row or stacked, then the 320 px one) with alt text; Quick start (`go install github.com/scv3lqq/steady/cmd/steady@latest`
  then `steady`, open `http://127.0.0.1:8770`; from a clone `make run`); Configuration table (the four env vars
  with defaults); API table (S6 routes, one line each); Security (single-user, no auth, binds `127.0.0.1`,
  reach it through an SSH tunnel or a reverse proxy that adds auth and whose host goes into `STEADY_ALLOWED_HOSTS`,
  `http.CrossOriginProtection`, Host allowlist against DNS rebinding, CSP); Data (one SQLite file in WAL mode,
  backup with `sqlite3 <db> ".backup <file>"` or `VACUUM INTO`, deleted habits purged after 24 h); Deploy
  (pointer to `deploy/README.md`, release binaries from `steady/v*` tags with `SHA256SUMS` and provenance);
  Development (S15 targets, `make check` is what CI runs; generated code is committed). English, no
  personal hosts or paths, no em dash.
- `AGENTS.md` edits (quote-exact; keep everything else):
  - E1 old: ``- `internal/config`: env config. `STEADY_ADDR` (default `127.0.0.1:8770`), `STEADY_DB` (default `data/steady.db`), `STEADY_LOG_LEVEL` (default `info`).``
    new: ``- `internal/config`: env config. `STEADY_ADDR` (default `127.0.0.1:8770`), `STEADY_DB` (default `data/steady.db`), `STEADY_LOG_LEVEL` (default `info`), `STEADY_ALLOWED_HOSTS` (comma-separated extra Host values for a reverse proxy, default empty).``
  - E2 old: ``- `internal/repository/sqlite`: SQLite adapter. Embedded goose migrations, sqlc-generated queries, first-run seed.``
    new: ``- `internal/repository/sqlite`: SQLite adapter. Embedded goose migrations, sqlc-generated queries (`sqlcdb/`, from `queries.sql` and `sqlc.yaml`), first-run seed.``
  - E3 old: ``Soft-deleted rows are purged later.``
    new: ``Soft-deleted rows are purged 24 h after deletion (on startup and on each delete).``
  - E4 old: ``- `make generate`: sqlc.``
    new: ``- `make generate`: sqlc (pinned version via `go run`, `CGO_ENABLED=0`).``
  - E5 old: ``- `make test`: `go test -race ./...` plus `npm test` in `web/` (Node's built-in test runner over the compiled date helpers).``
    new: the same line followed by a new line ``- `make check-text`: fails on a literal em dash in any tracked or new file (`scripts/check_text.py`).``
  - E6 old: ``- Mutating API routes are protected by `http.CrossOriginProtection` and a Host allowlist against DNS rebinding.``
    new: ``- Mutating API routes are protected by `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`).``

Task gates:
```bash
grep -F 'STEADY_ALLOWED_HOSTS' AGENTS.md README.md
for f in week month year week-320; do grep -F "assets/screenshots/$f.png" README.md || exit 1; done
grep -F 'go install github.com/scv3lqq/steady/cmd/steady@latest' README.md
grep -F 'make check-text' AGENTS.md
```
Plus S2 over both files.

### T22. Browser acceptance run and screenshots (W5, after T13 and T19)

Files: `assets/screenshots/week.png`, `assets/screenshots/month.png`, `assets/screenshots/year.png`,
`assets/screenshots/week-320.png`. Script (not in the repo): `$SCRATCH/e2e/e2e.mjs` where `$SCRATCH` is
given in the dispatch prompt.

`e2e.mjs`: Node 26, no npm dependencies (built-in `WebSocket`, `fetch`, `child_process`). It:
1. Removes and recreates `$SCRATCH/e2e/run/`; starts the server with
   `STEADY_DB=$SCRATCH/e2e/run/steady.db STEADY_ADDR=127.0.0.1:18770 make run` (cwd the repo), waits for `/healthz`.
2. Launches `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome --headless=new --remote-debugging-port=<free> --user-data-dir=$SCRATCH/e2e/run/profile --no-first-run`,
   attaches to a page target over CDP, enables `Runtime`, `Log`, `Page`, `Network`; records console errors
   (`Runtime.consoleAPICalled` type `error`/`assert`, `Runtime.exceptionThrown`, `Log.entryAdded` level `error`).
3. Width via `Emulation.setDeviceMetricsOverride` (600 x 900 desktop, 320 x 700 mobile with `mobile: true`).
4. README shots first, from the freshly seeded data at 600 px: week, month, year (full page) to
   `assets/screenshots/{week,month,year}.png`; at 320 px week to `assets/screenshots/week-320.png`. Also six
   review shots (3 views x 2 widths) to `$SCRATCH/e2e/run/`.
5. Checks, each printed as `ok <name>` or `FAIL <name>: <detail>`, every persisted change re-checked after
   `Page.reload` (wait for `#list` to have children or `.empty`):
   - seeded: 3 habits named per S3 seed names in order; header date equals `Intl.DateTimeFormat('en-US', {weekday:'long', month:'long', day:'numeric'})` of today; H1 matches `^\d+ of 3 today$` or is `All done today`; footer text exact (S13).
   - add: click `+ Habit`, `#add-input` focused, type `Stretch` (CDP `Input.insertText`), Enter -> 4 habits, last `Stretch`; reload persists. Esc closes the row; blur with an empty input closes it.
   - rename: focus Stretch's `input.name`, select all, type `Stretch 5 min`, blur -> reload persists; clear it and blur -> shows `Untitled`, reload persists; rename back to `Stretch`.
   - focus kept: while typing in a name input, trigger a re-render (dispatch a click on the palette toggle via `element.click()` from JS) and assert `document.activeElement` is still that input and its value is intact.
   - delete + undo: delete the habit at index 1 -> toast visible with text `“Walk 30 minutes” deleted`; click Undo -> `Walk 30 minutes` back at index 1; reload -> still index 1.
   - toast timer: add `Tmp A`, `Tmp B`; delete `Tmp A`; wait 3 s; delete `Tmp B` (toast now names `Tmp B`); wait 3 s -> still visible; wait 2.5 s more -> hidden; reload -> neither exists.
   - week toggle: click today's `button.sq` of habit 0 -> `aria-pressed` flips and `.is-done` matches; reload persists; the streak text matches `^\d+ days?$` or is `String.fromCharCode(0x2014)` with `.is-zero`.
   - month: click tab Month -> `aria-selected=true`, 30 dots per block, caption first span equals `Intl` `{month:'short', day:'numeric'}` of today minus 29 days, second `today`; toggle yesterday's dot; reload -> view still month, dot persisted; count text matches `^\d+ of 30$`.
   - year: click Year -> 371 cells per block, cells after today have `visibility: hidden` and `disabled`, caption second span `today`; toggle the cell two days ago; reload persists.
   - accent: open the palette (`#palette` visible, `aria-expanded=true`), click Blue -> `getComputedStyle(documentElement).getPropertyValue('--accent').trim()` is `#5B8CEB`, that button `aria-checked=true`, a done square's background is `rgb(91, 140, 235)`; reload persists; palette stays open until toggled.
   - aria-label of a day button matches `^.+, [A-Z][a-z]+ \d{1,2} <EM> (done|not done)$`, where `<EM>` stands for `String.fromCharCode(0x2014)` (build the regex in code from it).
   - computed styles vs spec: body background `rgb(22, 21, 19)`; `.app` max-width 600px, padding `56px 20px 96px`, row gap 36px; `.wordmark` 15px/600; `.today` 14px, `white-space: nowrap`; `.accent-toggle` 32x32 with a 14px swatch; `.palette-color` 32x32 with a 22px swatch; `.title` 40px, weight 500, letter-spacing `-1.2px`; `.progress` height 4px; `.progress-fill` transition includes `width 0.3s`; `.tabs` border 1px, padding 3px, radius 11px; `.tab` height 34px; `.sq` 34x34, radius 9px; `.days` gap 6px; `.del` 32x32; `.month-grid` gap 4px with 30 columns; `.dot` radius 50%; `.year-grid` gap 3px, 53 columns; `.cell` radius 2px; `.caption` 11px; `.footer` 13px; font-family starts with `Onest`; `font-variant-numeric` `tabular-nums` on body.
   - toast styles: position fixed, bottom 28px, radius 12px, background `rgb(236, 231, 220)`, `role=status`; `.undo` background `rgb(217, 211, 199)`.
   - H1 variants and empty state, last: mark today for every habit -> `All done today`; delete every habit -> `Let’s start`, `.empty-title` `Nothing here yet`, `.empty-text` exact (S13), footer still present; reload persists.
   - 320 px: for week, month and year (re-add one habit first if empty), `document.documentElement.scrollWidth <= 320`, printed as `scrollWidth<=320 ok <view>`.
   - zero console errors over the whole run (print each error if any).
6. Prints `E2E PASS` and exits 0 when every check passed, else `E2E FAIL <n>` and exits 1. Always kills Chrome and
   the server (the server's process group) in a `finally`.

Read the six review shots (Read tool) and compare them with `docs/spec.md` (header row, H1, tabs, squares,
dots, year grid, footer, toast). Report each mismatch with the spec line. Do not edit source files: report
problems instead. Task gates: `node "$SCRATCH/e2e/e2e.mjs"` exits 0 with `E2E PASS`; the four PNGs exist and
are wider than 0 bytes.

## Seams

| Producer | Consumer | Contract |
|---|---|---|
| T1 domain | T7, T9, T10, T16, T18, T15, T17, T20 | S3 |
| T1 SQL + sqlcdb | T9, T15 | S10 |
| T10 port | T9 (implements), T19 (asserts), T17 (fake) | S4 |
| T10 service | T16, T18, T19 | S5 |
| T8 config | T19, T14, T20 | S7 |
| T16 handler | T18, T19, T22, T2/T13 (fetch paths, statuses) | S6, S8 |
| T19 app | T20, T22 | S9 |
| T2 store | T12, T13 | S11 |
| T2 dates | T11, T12 | S12 |
| T6 html/css | T12, T13, T22 | S13 class/id names |
| T12 render | T13 | `render(root, state, ui, now)`, `Ui` (T12 section) |
| T12/T13 text | T22 | S14 |
| T3 Makefile | T4 (`make check`), T21, T22 (`make run`) | S15 |
| T22 screenshots | T21 | S16 |
| T4 job ids | T5 (`checks=(test secrets)`) | job ids `test`, `secrets` without `name:` keys |

## Not doing

- No authentication, accounts, or multi-user support; no localStorage persistence.
- No features, sections, or decoration beyond `docs/spec.md` (no reordering, no keyboard shortcuts beyond Enter/Esc in the add row, no themes).
- No changes to repo visibility, no release tag, no server access, no deploy run.
- No dependabot config (not requested).
- No TypeScript lint tool besides `tsc --strict`; no bundler; TypeScript 7.x not used (S1).
- No `-race` with `CGO_ENABLED=0`; binaries only are cgo-free.

## Iteration 1: review fixes (non-blocking findings folded in)

These supersede the matching text above.

- T6 CSS values changed by review: `.topbar { gap: 6px; }`, `.name { padding: 5px 0; min-height: 32px; }`,
  `.side { min-width: 0; }` (a 10+ day streak no longer misaligns squares; the longest dates fit at 320 px; 32 px hit target).
- F1 (`web/src/store.ts`, `web/dist/store.js`): a failed request must not wipe later optimistic edits or an
  unsent rename. `#enqueue` counts queued requests; a failure sets `#stale = true` and reloads only when the
  queue has drained (`#queued === 0`), then clears `#stale`. `load()` re-applies `#pending` names to the
  matching habits before it replaces `#state`. Gates: `(cd web && npm run build)`, S2, and a scratch run with a
  fake `fetch`: toggle day A (500), toggle day B while A is in flight -> requests `PUT A, PUT B, GET state` in
  that order; a rename pending during a reload keeps the typed name locally.
- F2 (`web/index.html`, `web/src/main.ts`, `web/dist/main.js`): the Google Fonts stylesheet is added from
  `start()` with `document.createElement('link')` (`rel=stylesheet`, the same href:
  `https://fonts.googleapis.com/css2?family=Onest:wght@400;500;600&display=swap`) and `document.head.append`,
  before `store.load()`. Why: a parser-inserted stylesheet blocks the module script until the font host answers
  (measured 3.3 s here), and a blocked host would leave the page blank until the connection times out. The S13
  block above is updated: the two preconnect links stay, the stylesheet link is gone. The CSP is unchanged.
  Gates: the T6 awk diff of `index.html`, `(cd web && npm run build)`, the T13 forbidden-API grep, S2.
- T22 note: with the empty add row open, the first mouse click elsewhere only closes the row (blur hides it and
  the layout shifts), so close the row before clicking a day. Chrome at DPR 1 reports a 1.5px border as 1px, so
  check the today square's border color rather than its used width.
- T15 additions from review: `TestAddHabitOrderAndConflict` also renames A to `Renamed` and reads it back from `State`; `TestSeedOnce` compares names with the literal list `Drink 2 L of water`, `Walk 30 minutes`, `Read 10 pages` and asserts `domain.ValidateID(h.ID) == nil` for each seeded habit; new `TestOpenCreatesParentDir` opens `filepath.Join(t.TempDir(), "nested", "data", "steady.db")`. The S3 mutation is caught by the fresh-store scenario, not by D colliding with B.
- T20 additions from review: the restart step in `TestNewServesSeededApp` compares habit ids (random at seed time) with the first start's ids, proving the same database is reopened; new `TestHostAllowlistFromConfig` with `ExtraHosts: []string{"steady.test"}`: Host `steady.test` gives 200, `other.test` gives 403.
- T18 additions from review: a closed-store `GET /api/state` case expects 500 `{"error":"internal error"}`, with a `zerolog.New(&buf)` logger asserting level `error`, `method`, `path`, and that the response does not contain the error text; rows expecting a JSON body also assert `Content-Type: application/json`; `TestStatic` adds `dist/notes.txt` to the MapFS and expects `GET /dist/notes.txt` 404.
