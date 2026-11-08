# Show when each habit was created and started

## Goal

Every habit shows one dates line in all three views: when it was created and when it was started.

- **Created**: server clock at creation, stored as Unix seconds, shown as a local calendar date. Habits that predate the column have an unknown creation time and show no `Created` part.
- **Started**: the earliest marked day, derived from existing marks. It moves when the earliest mark changes and can precede `Created`, because past days can be marked. No marks: `Not started`.

## Base

Commit `f11bb47`. `make check` passed on it (78 web tests, all Go packages, lint, generated-code freshness).

The rollback path was tested on a spike: the base binary starts on a database already migrated to version 3, `/healthz` answers 200, and login, state, create, mark, rename, delete and restore all work. This depends on the column being nullable.

## Acceptance criteria

Each is a command run from the repo root. AC2 to AC7 fail on the base, AC1 passes on it.

1. `make check` exits 0. It requires generated output to be committed, so run it on a clean tree.
2. `grep -F 'ALTER TABLE habits ADD COLUMN created_at INTEGER;' internal/repository/sqlite/migrations/00003_habit_created_at.sql` prints the line.
3. `go test -race -count=1 -v -run 'TestStateCreatedAt|TestLegacyHabitsHaveUnknownCreatedAt|TestMigrationDowngradeRejectsWithoutChangingData' ./internal/repository/sqlite/` exits 0 and its output contains `--- PASS: TestStateCreatedAt` and `--- PASS: TestLegacyHabitsHaveUnknownCreatedAt`.
4. `go test -race -count=1 -v -run 'TestNewHabitDTOCreatedAt' ./internal/handler/httpapi/` exits 0 and its output contains `--- PASS: TestNewHabitDTOCreatedAt`.
5. `(cd web && npm run build && npm test)` exits 0 and the output contains `habitDatesText`.
6. Each of these prints one line: `grep -F 'Not started' docs/spec.md`, `grep -F 'createdAt: number | null' docs/spec.md`, `grep -F '`createdAt` (Unix seconds' README.md`, `grep -F 'creation time (unknown for habits that predate it)' AGENTS.md`.
7. The rollback script below prints `rollback ok` and exits 0.
8. UI walkthrough in headless Chrome at 320 px and 600 px wide, against the real server with a scratch database and account: the dates line appears in Week, Month and Year; marking an earlier day and clearing the earliest mark update `Started` without a reload; a habit with no marks shows `Not started`; `document.documentElement.scrollWidth` does not exceed the viewport width; typing in a focused name input during a poll keeps focus and caret; no console errors.

```bash
#!/usr/bin/env bash
set -euo pipefail
tmp=$(mktemp -d)
repo=$(pwd)
trap 'git -C "$repo" worktree remove --force "$tmp/src" >/dev/null 2>&1 || true; rm -rf "$tmp"' EXIT
CGO_ENABLED=0 go build -o "$tmp/new" ./cmd/steady
git worktree add --detach "$tmp/src" f11bb47 >/dev/null 2>&1
(cd "$tmp/src" && CGO_ENABLED=0 go build -o "$tmp/base" ./cmd/steady)
export STEADY_DB="$tmp/steady.db" STEADY_ADDR=127.0.0.1:18770 STEADY_COOKIE_SECURE=false
health() { for _ in $(seq 1 30); do curl -fsS "http://$STEADY_ADDR/healthz" >/dev/null 2>&1 && return 0; sleep 0.2; done; return 1; }
"$tmp/new" >/dev/null 2>&1 & pid=$!
health; kill "$pid"; wait "$pid" || true
"$tmp/base" >/dev/null 2>&1 & pid=$!
health; kill "$pid"; wait "$pid" || true
echo 'rollback ok'
```

## Gates

Full Gates, run by the orchestrator after a wave's files are committed, because `check-generated` fails on uncommitted `web/dist` or `sqlcdb` output: `make check`.

## Shared definitions

- **Column**: `habits.created_at INTEGER`, nullable, Unix seconds. `NULL` means unknown.
- **Domain field**: `CreatedAt time.Time` on `domain.Habit`, zero value when unknown.
- **Port**: `AddHabit(ctx context.Context, userID, id, name string, createdAt time.Time) error`.
- **API field**: `createdAt` on every habit object in JSON, a Unix-seconds integer or `null`, emitted after `done`.
- **Frontend field**: `createdAt: number | null` on both `Habit` and `ApiHabit`.
- **Dates function**: `export function habitDatesText(createdAt: number | null, doneKeys: readonly string[]): string` in `web/src/dates.ts`.
- **Dates line format**: `Created Nov 8, 2026 · Started Nov 10, 2026`, parts joined by space, U+00B7, space. Part one is omitted when `createdAt` is `null`. Part two is `Started <date>`, or `Not started` with no marks. A date is `en-US` with `{ month: 'short', day: 'numeric', year: 'numeric' }`.
- **CSS class**: `habit-dates`.

## Tasks

Waves: W1 = T1, T2. W2 = T3. W3 = T4, T5, T6. W4 = T7. W5 = T8. Frontend tasks are never concurrent, since `tsc` writes all of `web/dist/`.

### T1: documentation

Files: `docs/spec.md`, `README.md`, `AGENTS.md`. No dependencies.

`docs/spec.md`, locating each anchor by its text:

1. After the Week line starting `     - Right side (84px): the streak`, insert:
   `     - Full-width dates line (11px, faint), the last item of the row. See **Habit dates** under Data and logic.`
2. After the Month line starting `   - Each habit is a block with a top border`, insert:
   `   - Directly below the top line, the same dates line as the Week view.`
3. After the Year line `   - Same block header, with the count `214 days`.`, insert:
   `   - Directly below the header, the same dates line as the Week view.`
4. Replace the `type Habit = ...` line with:
   `type Habit = { id: string; name: string; createdAt: number | null; done: Record<string, true> }; // createdAt: Unix seconds, null when unknown; keys: local "YYYY-MM-DD"`
5. Immediately before the heading `## Accessibility`, insert this paragraph followed by a blank line:
   `**Habit dates.** One line reads `Created Nov 8, 2026 · Started Nov 10, 2026`. `Created` is the server's creation time in the browser's local time zone; it is omitted when `createdAt` is `null`, which marks a habit older than this field. `Started` is the earliest marked day: it moves when that mark changes and can precede `Created`, because past days can be marked. With no marks the second part reads `Not started`.`
6. After the Acceptance line starting `- Add, rename, delete + undo`, insert:
   `- Each habit shows its dates line in all three views. Marking an earlier day or clearing the earliest mark updates `Started` at once, and the line causes no horizontal scroll at 320px wide.`

`README.md`: in the `GET /api/state` row replace `Returns the current user's habits with their done days, plus their saved` with `Returns the current user's habits with their done days and `createdAt` (Unix seconds, `null` when unknown), plus their saved`.

`AGENTS.md`: replace `habits retain day marks, position and soft-delete state.` with `habits retain day marks, position, creation time (unknown for habits that predate it) and soft-delete state.`

Task gates: AC6 greps, `make check-text`. Done when all pass and nothing else in these files changed.

### T2: schema, queries, domain field

Files: `internal/repository/sqlite/migrations/00003_habit_created_at.sql` (new), `internal/repository/sqlite/queries.sql`, `internal/repository/sqlite/sqlcdb/*` (generated by `make generate` only), `internal/domain/domain.go`. No dependencies.

Migration, in the goose annotation style of `00002_accounts.sql`:

- Up: `ALTER TABLE habits ADD COLUMN created_at INTEGER;`. It stays nullable: the previous binary must keep inserting without this column during a rollback.
- Down: `SELECT * FROM downgrade_requires_backup_and_matching_binary;`, the same deliberate rejection as `00002`.

`queries.sql`:

- `ListActiveHabits`: select list becomes `id, name, created_at`.
- `InsertHabit`: column list becomes `(id, name, user_id, position, created_at)`, values gain `sqlc.arg(created_at)` after the position subquery. `ON CONFLICT (id) DO NOTHING` stays, so a retry keeps the original time.

`domain.go`, `Habit` struct: add after `Name`: `CreatedAt time.Time // zero when unknown: the habit predates creation tracking`. Import `time` if absent.

All additions are compatible: keyed literals still compile and the store passes a NULL until T3.

Task gates: `make generate`, `go build ./...`, `go vet ./internal/repository/sqlite/ ./internal/domain/`, `go test -race ./internal/repository/sqlite/ ./internal/app/ ./internal/domain/`, `golangci-lint run ./internal/repository/sqlite/... ./internal/domain/...`. Done when they pass.

### T3: record creation time

Files: `internal/usecase/service.go`, `internal/usecase/service_test.go`, `internal/repository/sqlite/store.go`, `internal/repository/sqlite/store_test.go`, and any other `_test.go` under `internal/` that stops compiling only because of the new parameter. Depends on T2. These change together because the port, its adapter and its fake must agree to compile.

`service.go`:

- Port line becomes the **Port** definition.
- In `Service.AddHabit`, after name normalization, `createdAt := s.now()`; pass it to `s.repo.AddHabit`; return `domain.Habit{ID: id, Name: norm, CreatedAt: createdAt, Done: []string{}}`. The public signature of `Service.AddHabit` does not change.

`store.go`:

- `Store.AddHabit` takes the **Port** parameter and passes `CreatedAt: sql.NullInt64{Int64: createdAt.Unix(), Valid: true}` in `InsertHabitParams`.
- `State` builds `domain.Habit{ID: h.ID, Name: h.Name, CreatedAt: createdAtTime(h.CreatedAt), Done: done}`.
- Add `func createdAtTime(v sql.NullInt64) time.Time`: zero `time.Time{}` when `!v.Valid`, otherwise `time.Unix(v.Int64, 0).UTC()`.

`service_test.go`:

- `fakeRepo.AddHabit` takes the new parameter and stores it in a new field `addedAt time.Time`. Existing recorded call strings do not change.
- In `TestAddHabit`, the success case expects `domain.Habit{ID: testID, Name: "Read", CreatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), Done: []string{}}`, and asserts `fakeRepo.addedAt` equals the same literal.
- Mutations: `Service.AddHabit` passes `time.Time{}` to the repository; `Service.AddHabit` returns a habit without `CreatedAt`. Command: `go vet ./internal/usecase/ && go test ./internal/usecase/`.

`store_test.go`: define `var testCreatedAt = time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)`, and pass it from the `mustAdd` helper and direct `AddHabit` callers. Signature changes only: other test files get the same literal or their own local value.

Task gates: `go build ./...`, `go vet ./internal/usecase/ ./internal/repository/sqlite/`, `go test -race ./internal/usecase/ ./internal/repository/sqlite/ ./internal/app/`, `golangci-lint run ./internal/usecase/... ./internal/repository/sqlite/...`.

### T4: persistence tests

Files: `internal/repository/sqlite/store_test.go`, `internal/repository/sqlite/accounts_test.go`. Depends on T3.

`store_test.go`, new table-driven `TestStateCreatedAt`, comparing with `time.Time.Equal`:

| Case | Steps | Expected |
| --- | --- | --- |
| recorded | add with `testCreatedAt` | `State` habit `CreatedAt` equals `testCreatedAt` |
| survives undo | add, delete, restore | equals `testCreatedAt` |
| retry keeps original | add, add same id with `testCreatedAt.Add(time.Hour)` | second add is `domain.ErrExists`, `CreatedAt` equals `testCreatedAt` |

`accounts_test.go`:

- New `TestLegacyHabitsHaveUnknownCreatedAt`: use the `legacyDatabase` fixture and the bootstrap flow of the nearest existing legacy test, then assert the bootstrapped habit has `CreatedAt.IsZero()` true.
- Make `TestMigrationDowngradeRejectsWithoutChangingData` table-driven over target versions `2` and `3`: migrate to that version with `provider.UpTo`, call `provider.Down`, and keep every existing assertion in both subtests. The name stays, so AC3 still matches it. A `Down` runs only the latest migration, so without this the `00002` guard loses its coverage.

Mutations, one at a time:

- `Store.AddHabit`: `Valid: false`.
- `Store.State`: drop `CreatedAt`.
- `createdAtTime`: invert the `Valid` check.
- `00002_accounts.sql` Down: replace with `SELECT 1;` (subtest 2).
- `00003_habit_created_at.sql` Down: replace with `SELECT 1;` (subtest 3).

Command: `go vet ./internal/repository/sqlite/ && go test ./internal/repository/sqlite/`.

Task gates: `go vet ./internal/repository/sqlite/`, `go test -race ./internal/repository/sqlite/`, `golangci-lint run ./internal/repository/sqlite/...`.

### T5: API field

Files: `internal/handler/httpapi/handler.go`, `internal/handler/httpapi/handler_test.go`, `internal/handler/httpapi/dto_test.go` (new, `package httpapi`), and other `_test.go` files in that package that use `habitJSON`. Depends on T3.

`handler.go`:

- `habitDTO`: add after `Done`: `CreatedAt *int64 `json:"createdAt"``.
- `newHabitDTO`: when `h.CreatedAt.IsZero()` leave it nil; otherwise point it at `h.CreatedAt.Unix()`. `createReq` does not change: the client never supplies the time.

`handler_test.go`:

- Add `habitJSONWith(createdAt, id, name string, done ...string) string`, with `createdAt` a raw JSON literal, appending `,"createdAt":<literal>` after the `done` array.
- `habitJSON(id, name, done...)` delegates to it with `strconv.FormatInt(fixedNow().Unix(), 10)`.
- Rename responses pass `"null"`, because `Service.RenameHabit` returns a habit with no time. Where a test's clock is not `fixedNow`, pass that clock's value.

`dto_test.go`, table-driven `TestNewHabitDTOCreatedAt`, comparing `json.Marshal(newHabitDTO(h))` as a string:

| Case | Input | Expected |
| --- | --- | --- |
| unknown | `domain.Habit{ID: "a", Name: "Read", Done: []string{}}` | `{"id":"a","name":"Read","done":[],"createdAt":null}` |
| known | same with `CreatedAt: time.Unix(1793525400, 0)` | `{"id":"a","name":"Read","done":[],"createdAt":1793525400}` |

Mutations: `newHabitDTO` always leaves `CreatedAt` nil; `newHabitDTO` uses `UnixMilli()`. Command: `go vet ./internal/handler/httpapi/ && go test ./internal/handler/httpapi/`.

Task gates: `go vet ./internal/handler/httpapi/`, `go test -race ./internal/handler/httpapi/`, `golangci-lint run ./internal/handler/httpapi/...`.

### T6: date text

Files: `web/src/dates.ts`, `web/test/dates.test.mjs`, `web/dist/*` (built). No dependency on Go tasks.

`dates.ts`: add a formatter constant next to the existing `Intl.DateTimeFormat` constants with the **Dates line format** options, and the **Dates function**. The earliest key is the lexicographic minimum of `doneKeys` (do not rely on key order). Format it through `fromKey`, and format `createdAt` as `new Date(createdAt * 1000)`. Do not change existing functions.

`dates.test.mjs` (the suite runs in `America/New_York`), one test named with `habitDatesText`, using `·` (U+00B7):

| `createdAt` | `doneKeys` | Expected |
| --- | --- | --- |
| `1794106800` | `['2026-11-10', '2026-11-12']` | `Created Nov 7, 2026 · Started Nov 10, 2026` |
| `1794106800` | `['2026-11-12', '2026-11-10']` | `Created Nov 7, 2026 · Started Nov 10, 2026` |
| `1794106800` | `[]` | `Created Nov 7, 2026 · Not started` |
| `null` | `['2025-11-03']` | `Started Nov 3, 2025` |
| `null` | `[]` | `Not started` |

Mutations: the new formatter gets `timeZone: 'UTC'`; the earliest key becomes `doneKeys[0]`; the `null` branch is removed; the empty-marks branch returns `''`. Command: `(cd web && npm run build && TZ=America/New_York node --test test/dates.test.mjs)`.

Task gates: `(cd web && npm run build && TZ=America/New_York node --test test/dates.test.mjs)`.

### T7: store field

Files: `web/src/store.ts`, `web/test/store.test.mjs`, `web/test/sync.test.mjs`, `web/dist/*`. Depends on T6.

`store.ts`:

- Add the **Frontend field** to `Habit` and `ApiHabit`.
- `toState` copies `createdAt: h.createdAt`.
- `addHabit` pushes `{ id, name, createdAt: Math.floor(Date.now() / 1000), done: {} }`. The POST body stays `{ id, name }`.

Tests: API habit fixtures gain `createdAt` so the doubles match the real contract: `1793525400` for `ALICE`, `null` for `BOB` and any other fixture habit. New cases, written in the style of the existing `addHabit` test:

| Case | Expected |
| --- | --- |
| state load of `ALICE` | habit `createdAt` is `1793525400` |
| state load of `BOB` | habit `createdAt` is `null` |
| `addHabit` | `createdAt` is an integer between the seconds read before and after the call |
| delete then undo | `createdAt` is unchanged |

Mutations: `toState` omits `createdAt`; `addHabit` sets `createdAt: null`. Command: `(cd web && npm run build && TZ=America/New_York node --test test/store.test.mjs)`.

Task gates: `(cd web && npm run build && npm test)`.

### T8: dates line

Files: `web/src/render.ts`, `web/styles.css`, `web/dist/*`. Depends on T7. No automated render tests exist: AC8 covers this.

`render.ts`:

- Import `habitDatesText`.
- In `createRow`, before the `if (view === 'week')` branch: `const dates = create('div', 'habit-dates');`.
- Week: `row.append(name, grid, side, dates);`. Month and year: `row.append(head, dates, grid, caption);`.
- In `updateRow`, before `if (frame.view === 'week')`: `setText(required(row, '.habit-dates'), habitDatesText(habit.createdAt, Object.keys(habit.done)));`.
- Do not touch the name-input update guard or how rows are reused.

`styles.css`, after the `.caption` rule:

```css
.habit-dates {
  flex: 0 0 100%;
  font-size: 11px;
  color: var(--faint);
}

[data-view='month'] .habit-dates,
[data-view='year'] .habit-dates {
  margin-bottom: 10px;
}
```

Task gates: `(cd web && npm run build && npm test)`, `make check-text`.

## Seams

| Producer | Consumer | Shared definition |
| --- | --- | --- |
| T2 | T3 | Column; sqlc `CreatedAt sql.NullInt64` |
| T2 | T5 | Domain field |
| T3 | T4 | Port, `testCreatedAt` |
| T3 | T5 | Domain field set by `Service.AddHabit` |
| T5 | T7 | API field |
| T6 | T8 | Dates function, Dates line format |
| T7 | T8 | Frontend field |
| T1 | T6, T8 | Dates line format |
| T8 | T1 | CSS class matches the 11px faint spec line |

## Not doing

- No user-editable start date, no backfill of `created_at` for existing habits, no estimate from marks.
- No new route, no change to the `POST /api/habits` request.
- No change to day-mark validation: a mark may still precede creation.
- No tooltip, popover or expandable row.
- No edits to the local `TODO.md`.
