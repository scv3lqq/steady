# Private multi-user accounts

## Goal

Extend the existing single Go binary and SQLite architecture with private accounts,
operator-only account management, durable opaque sessions and complete data isolation.
The authorized extension replaces the single-user/no-auth and demo-seed requirements.
No live database, live service or deployment is part of this work.

Base: `34a5c2d`. Branch: `feat/private-accounts`.

## Constraints and workflow

Read `AGENTS.md`, its alias `CLAUDE.md`, and `docs/spec.md`. There are no project
`.claude/rules` or `.codex` rules in this checkout. English artifacts, no literal
U+2014, signed Conventional Commits <=72 characters. Preserve the pure-Go SQLite,
goose/sqlc, strict dependency-free TypeScript, optimistic queue, focus preservation,
Host allowlist, CrossOriginProtection and health-check contracts.

The feature is sliced into exclusive compiling module boundaries instead of one
source file per task: ownership changes necessarily span port/adapter/test
signatures, and a green gate cannot exist between those signature edits. Test-only
RED phases precede all production writers in the implementation wave. Each test
owner saves the completed failing command before receiving the GREEN phase.
Independent reviews bind to commits or recorded SHA256 manifests. Full gates run
serially after the integrated wave, before every commit, and by the final validator.
Generated output is staged explicitly before the freshness gate (which compares
working tree with index). No missing or failed gate constitutes PASS.

Baseline `make check`: exit 0, all Go packages and 12 Node date tests pass.
The optional shellcheck check reports not installed; bash syntax checks pass.
Baseline evidence is stored outside the repository. New authentication acceptance
tests do not exist on base; their RED evidence will be saved before implementation.

## Design decisions

### Accounts and legacy ownership

- Usernames are case-sensitive lowercase ASCII `[a-z][a-z0-9_-]{2,31}`.
  Reject whitespace and uppercase, do not normalize silently.
- Passwords are valid UTF-8, 12 through 128 bytes inclusive; no composition rules,
  normalization or truncation. Only the hidden interactive CLI creates/resets them.
- New users have zero habits, `week` view and `#4FA872` accent. Disable demo seeding.
- Migration 2 adds users, sessions and singleton bootstrap ledger. Add nullable
  `habits.user_id` referencing users; NULL denotes inaccessible legacy ownership.
  Keep habit IDs, names, positions, deleted_at, days and all legacy settings intact.
  Rename the old settings table to `legacy_settings`; new settings use composite
  `(user_id,key)` primary key. Preserve unknown legacy setting keys too.
- Legacy soft-deleted rows receive a `legacy_preserved` flag in migration 2.
  Purge excludes unowned and preserved rows. Restore clears preservation; a later
  explicit delete follows the existing 24-hour retention. This preserves all
  existing deleted data through upgrade, bootstrap and the next startup.
- `account bootstrap` is an explicit transaction: create owner, assign ALL NULL
  habits (active/deleted), copy ALL legacy settings, clear the legacy staging rows,
  insert completion ledger. A failure rolls back everything. A repeat with the
  same username is a no-op (false), never resetting password or reassigning data;
  another username or an existing account with that name fails safely. Ordinary
  account creation, browser visits and login never claim legacy data.
- Keep UUID habit IDs globally unique; insertion position is per owner including
  deleted rows. All reads and ID mutations include owner predicates in SQL.
  Cross-user mutations return the same 404 as a missing habit.
- Migration downgrade is intentionally rejected without destructive reverse SQL;
  restore a protected backup with its matching binary instead.

### Passwords, sessions and abuse bounds

Use maintained `golang.org/x/crypto/argon2.IDKey`, Argon2id v19, 65536 KiB memory,
3 iterations, 4 lanes, 16 random salt bytes and a 32-byte key. Store PHC strings;
verify parsed fixed supported parameters and compare in constant time. These are
RFC 9106 section 4's second recommended parameters, verified against the official
Go package docs and RFC on 2026-11-03:
https://pkg.go.dev/golang.org/x/crypto/argon2 and
https://www.rfc-editor.org/rfc/rfc9106.html#section-4.
Use `golang.org/x/term.ReadPassword` for terminal-only hidden entry and confirmation.
Resolve maintained dependency versions through the Go module proxy and commit sums.

- Sessions: 32 random bytes encoded raw URL-safe base64, only SHA-256 hex digests
  persisted; absolute expiry exactly 30 days after login, no sliding renewal.
- Logout deletes the current session. Reset replaces hash and deletes every user
  session in one transaction. Session insertion compares the password hash read
  before verification against the current stored hash in the INSERT predicate;
  a concurrent reset cannot create an old-password session after revocation.
- Reject malformed tokens before hashing/querying; expired tokens are unauthorized.
  Delete expired sessions on insertion to bound stale storage.
- Login uses one generic 401 `invalid credentials` for unknown users/wrong passwords;
  unknown valid usernames perform a dummy password verification with equal work.
  Authentication infrastructure errors are generic 500 `internal error`.
- In-memory login limiter: at most 5 attempts per RemoteAddr IP per fixed 15-minute
  window; at most 30 attempts globally per fixed minute; max 1024 IP entries,
  expired entries pruned, full map fails closed; at most 2 concurrent password
  verifications, excess fails immediately with 429 `too many login attempts`.
  Count successful and failed attempts, do not clear on success. Ignore forwarded
  IP headers; document shared proxy quota and process-restart reset.
- No credentials, hash, cookie/token or raw auth errors in logs. Authentication
  handler errors use fixed safe logs. Keep access logging free of request bodies.

### HTTP and deployment

- `POST /api/login` JSON `{username,password}` -> 204 plus session cookie.
- `GET /api/session` -> 200 JSON `{username,binding}` or 401 `authentication required`.
  binding is lowercase SHA-256 hex of the UTF-8 string `steady-request:` plus the
  raw session token. It is a request-context marker, never a reusable session token.
  Every private API request must carry `X-Steady-Session` matching the current
  validated cookie binding (constant-time compare); absence/mismatch is 401.
  This prevents a queued request in another tab from using a replacement cookie.
- `POST /api/logout` -> 204, idempotent without a session; revoke and expire cookie.
- All existing habit/settings routes require server-validated session identity.
  Pass that user ID explicitly to application and repository methods. Never accept
  a client owner ID, including through settings or unknown JSON fields.
- Cookie name `steady_session`, Path `/`, HttpOnly, SameSite Strict, expiry and
  MaxAge matching 30 days. No Domain. Expiration cookie uses matching attributes.
- `STEADY_COOKIE_SECURE` boolean defaults true, parsed with strict true/false only.
  HTTPS reverse proxy deployment requires true, irrespective of backend HTTP;
  never infer it from untrusted forwarding headers. Explicit false supports local
  loopback HTTP or SSH tunnel only. Keep listener loopback even with auth.
- Host validation, CrossOriginProtection and no-store cover login/logout/session
  and protected APIs. Preserve existing body limits, JSON strictness and security
  headers. `/healthz` and static assets remain public and contain no private data.
- CLI: `steady account bootstrap <username>`, `steady account create <username>`,
  `steady account reset-password <username>`, using STEADY_DB and no HTTP listener.
  Reject unknown/extra arguments before password prompting, never accept password
  flags, environment variables or piped input. Prompt `Password: ` and
  `Confirm password: ` without echo. Confirmation mismatch is a safe error.
  Return fixed success text without secrets. Tests inject a password-reader port.
- Upgrade instructions stop the deploy timer/service, back up SQLite consistently
  with protected file permissions and retain matching old binary, use the new
  binary to bootstrap, start and verify two-user behavior before reenabling timer.
  Never run the old unauthenticated binary against a migrated database. Rollback
  is offline backup+matching-binary restoration, including safe stopped WAL/SHM
  cleanup. Do not execute any documented live operation during this task.

### Frontend

Keep `State` and all tracker behavior. Add separate auth status
`checking | anonymous | authenticated`, busy flag and generic display error.
Store exposes `auth`, `login(username,password): Promise<void>` and
`logout(): Promise<void>`. Initial load checks `/api/session` then private state.
Private requests capture the session binding alongside their generation and send
`X-Steady-Session`. Use native BroadcastChannel `steady-auth` to announce auth
transitions without credentials or identity data; other tabs immediately invalidate
private state, pending work and DOM and show login. A tab may sign in again.
Binding validation remains the authoritative cross-tab protection when delivery
is delayed. Test mismatched/missing binding and a two-tab queued mutation.
Auth operations are serialized; disable submission while busy so Set-Cookie
responses cannot reorder. Do not store password in application state.

Each auth transition increments a generation and aborts current data fetches.
Clear habits/settings, debounce timers, pending names, queued counters and stale
flags. Jobs capture generation and check before dispatch, after response and JSON
parsing; old jobs never run with a later user's cookie and never update later
counters. Session 401 clears state and goes anonymous. Keep network-error recovery
after current queue drain. `flushPending` uses current generation/signal only.
New login must wait for old data requests to settle/abort before issuing auth.

Minimal real form with labels `Username` and `Password`, autocomplete username /
current-password, required fields, button `Log in`, accessible alert for errors.
Clear password field after submit, retain focused form inputs while typing. Show
`Checking session...` initially; tracker is initially hidden. Logout is a quiet
32px minimum button `Log out`, layout wraps without overflow at 320px. On auth
loss clear toast, its timer, add input and UI palette/adding state, remove private
DOM text and rows. Reuse existing colors, typography, focus rings and spacing.

## Shared definitions (cross-task contracts)

All account contracts below are defined in T1-owned `internal/domain/accounts.go`
and `internal/usecase/account_port.go` before source writers depend on them.
`domain.Account` has `ID`, `Username`, `PasswordHash` string fields, never directly
JSON-encoded. ID is a cryptographically random 16-byte lowercase hex string.
Domain errors: `ErrInvalidUsername`, `ErrInvalidPassword` (ValidationError),
`ErrUsernameExists`, `ErrAccountNotFound`, `ErrBootstrapComplete`,
`ErrInvalidCredentials`, `ErrUnauthenticated` (StateError).
Validation: `ValidateUsername(string) error`, `ValidatePassword([]byte) error`.

`usecase.AccountRepository`:

```go
CreateAccount(context.Context, domain.Account) error
BootstrapOwner(context.Context, domain.Account) (bool, error)
AccountByUsername(context.Context, string) (domain.Account, error)
ResetPassword(context.Context, string, string) error
InsertSession(context.Context, string, string, string, time.Time) error
SessionUser(context.Context, string, time.Time) (domain.Account, error)
RevokeSession(context.Context, string) error
```

InsertSession arguments are token hash, user ID, expected password hash, expiry.
ResetPassword arguments are username, new PHC hash. Account lookup/reset missing
returns ErrAccountNotFound; duplicate username ErrUsernameExists; invalid/expired
session ErrUnauthenticated; stale password on insert ErrInvalidCredentials.
Bootstrap false,nil is the same-username completed retry.

`usecase.SessionLifetime` is `30 * 24 * time.Hour`, declared in the port file;
repository expiry cleanup derives current time as insert expiry minus this duration.

`usecase.PasswordHasher`: `Hash([]byte) (string,error)` and
`Verify(string,[]byte) bool`. `internal/password.Hasher` implements it.
`usecase.NewAccounts(AccountRepository,PasswordHasher,func()time.Time)
(*Accounts,error)` creates a dummy password hash once; Accounts methods:

```go
Bootstrap(context.Context, string, []byte) (bool, error)
Create(context.Context, string, []byte) error
ResetPassword(context.Context, string, []byte) error
Login(context.Context, string, []byte) (string, time.Time, error)
Current(context.Context, string) (domain.Account, error)
Logout(context.Context, string) error
```

`usecase.Service` and `Repository` retain names and return values; prepend explicit
`userID string` after context for State and every habit/settings operation.
Purge/PurgeDeleted remain global maintenance, SQL excludes unowned/preserved rows.
Ping unchanged. No optional/variadic owner or anonymous fallback.
`httpapi.New` retains existing signature; Options gains `Accounts *usecase.Accounts`
and `CookieSecure bool`. Missing Accounts is constructor error, never bypass auth.
Config gains `CookieSecure bool` loaded from STEADY_COOKIE_SECURE.
T2 updates HTTP regression helpers to create/login accounts and pass IDs to Service.

## Tasks

### T1: persistence and ownership boundary

Ownership: `internal/domain/accounts.go`, its new tests;
`internal/usecase/account_port.go`; `internal/usecase/service.go` and tests;
all `internal/repository/sqlite/` including migration 2, SQL, generated output,
legacy fixture tests and replacement/removal of the unused seed path.
Existing anchors: port service.go:20, store.Open:44, SetDay:212,
UpdateSettings:241, queries.sql:1, migrations/00001_init.sql:2, seed.go:19.
Do not rewrite migration 1. All runtime database access goes through sqlc queries;
fixture SQL may create migration-1 databases and inject transaction failures.
Keep prior repository/usecase regressions adapted to explicit owners.

RED cases: upgrade fixture with active/deleted habits, old deletion timestamp,
day marks and nondefault/unknown settings; prebootstrap invisible to any account;
bootstrap preserves exact stored values transactionally across restart; same retry
false and unchanged hash, different retry error; duplicate username; injected
bootstrap failure rolls back account and data; new account empty/default;
two-user per-owner state/order/settings and each direct ID mutation rejected;
direct sqlc MarkDay/UnmarkDay calls with a foreign owner have zero effect
even without the Store precheck (assert affected DB rows); owned-day predicates; reset revokes only target user and stale
password session insertion fails. Validation boundaries username/password.
Named mutations: remove user predicate from RenameHabit or UnmarkDay; remove
reset session revocation; omit legacy settings assignment. Isolation/upgrade tests
must catch each. Reviewer runs mutations on isolated scratch copy.

RED command `go test ./internal/domain ./internal/usecase ./internal/repository/sqlite`.
Save completed failure evidence before GREEN. GREEN scoped gates:
`make generate`, `go test -race ./internal/domain ./internal/usecase ./internal/repository/sqlite`,
`golangci-lint run ./internal/domain/... ./internal/usecase/... ./internal/repository/sqlite/...`.
During simultaneous T2 writes only package-local failures may be reported as
seam-pending; final integrated gates must be entirely green.

### T2: password, accounts, CLI and HTTP boundary

Depends on T1 shared contracts; exclusive ownership: `internal/password/`,
`internal/usecase/accounts.go`, `internal/usecase/accounts_test.go`,
`internal/handler/httpapi/`, `internal/config/`, `internal/app/`, `cmd/steady/`,
`go.mod`, `go.sum`. Do not edit T1 files; coordinate exact seams with T1 owner.
Existing anchors: app.wire:48 (remove seed/purge-loss risk), handler.New:106,
route registration:133, fail:261, middleware.go:28/76, cmd run:33.
Keep existing regression coverage including headers, JSON, Host and cross-origin
checks with authenticated helpers. Use fixed safe auth errors. CLI reader is
injectable, production terminal only, config is validated before opening DB.

RED cases: correct/wrong/unknown login, malformed JSON and oversized credentials,
fixed hash parameters/salts/malformed PHC/wrong password; durable sessions after
store restart, exact 30-day expiry, malformed/expired tokens, logout, reset
revocation (including parallel verify/reset insertion); all unauthenticated routes
401 before mutation; two users every route including forged IDs and foreign Undo,
settings and days; healthz public; cookie flags both explicit HTTP and HTTPS modes;
Host/CrossOrigin rejection for login/logout without side effects; missing/stale
session binding rejected even after another account replaces the cookie; bounded limiter
per IP/global/concurrent/full table/expiry, forwarded header ignored;
CLI create/duplicate/reset/bootstrap/no-op/mismatch/unknown and password-argument
rejection/no echo reader contract; new app no seed and legacy unchanged at startup.
Named mutations: bypass authenticated middleware; skip logout revocation; remove
cookie Secure or CrossOriginProtection; disable generation-checked session insert.
Use injectable time and lightweight fake hasher in broad integration tests, plus
real Argon2 parameter/verification tests. Keep heavy hashes bounded in tests too.

RED command `go test ./internal/password ./internal/usecase ./internal/handler/httpapi ./internal/config ./internal/app ./cmd/steady`.
Save completed failure evidence before GREEN. GREEN scoped gates:
`go test -race ./internal/password ./internal/usecase ./internal/handler/httpapi ./internal/config ./internal/app ./cmd/steady`,
`golangci-lint run ./internal/password/... ./internal/usecase/... ./internal/handler/httpapi/... ./internal/config/... ./internal/app/... ./cmd/steady/...`.

### T3: authenticated frontend

Exclusive ownership: `web/src/store.ts`, `web/src/main.ts`, `web/src/render.ts`,
`web/index.html`, `web/styles.css`, `web/package.json`, `web/test/store.test.mjs`,
related new frontend tests, generated `web/dist/` (dates output unchanged).
No runtime dependencies or new dev dependencies. Keep date tests unchanged.
Anchors: store State:9, load:97, flushPending:155, enqueue:243, send:250;
render Ui:18/focused input:239; main draw:22/click:57/input:104/pagehide:146.
Implement the closed frontend design and shared HTTP contract above.

RED Node built-in tests mock fetch/deferred responses: initial 401, successful
login then private state, generic failure, expiry during mutation, logout clears
state/settings, late GET/late JSON cannot overwrite next user, old queued mutation
is skipped before dispatch, old failures do not reload next user, debounce/pending
rename/pagehide cleared, optimistic failure reload waits for queue drain, serialized
auth requests and busy flags; BroadcastChannel invalidation and captured binding
prevent cross-tab cookie-switch contamination. Expected default week/#4FA872 and zero habits on
anonymous transition. Named mutations: drop generation guard before queued send;
drop guard after response/json; do not clear pending timer on logout. Tests must
catch these on isolated reviewer copies. Change npm test to include all *.test.mjs.

RED: `cd web && npm run build && node --test test/store.test.mjs` (command must
actually run tests against base built code). Save exit and log before GREEN.
Scoped gates: `cd web && npm run build && npm test`.
Headless UI workflow at 320 and 600 belongs to final independent validation.

### T4: specification and operations documentation

Exclusive ownership: `README.md`, `docs/spec.md`, `AGENTS.md`, `SECURITY.md`,
`deploy/README.md`, `.env.example`. Source documentation edits from separate
`plans/private-accounts-docs/*.md` old/new blocks generated from base by
`$PLAN_EDITS stub`. Apply/check each with `$PLAN_EDITS apply/check` and the base.
Do not hand-retype old text. Document every closed decision above, exact CLI,
expiry, username/password rules, Secure local HTTP exception, upgrade/backup and
rollback constraints. Remove contradictory single-user/no-auth/demo-seed claims;
historical screenshots remain explicitly demo examples. No live operations.
Task gates: all six exact prose checks and `make check-text`; final validator
cross-checks documentation against implementation and flags contradictions.

## Seams

| Producer | Consumer | Shared definition |
| --- | --- | --- |
| T1 domain/port | T2 auth/CLI | Account, validation/errors, AccountRepository, PasswordHasher |
| T1 service/store | T2 HTTP/app/regressions | explicit userID signatures, scoped purge |
| T2 HTTP | T3 store | three auth routes, 204/200/401/429 and cookie behavior |
| T3 store | T3 renderer/main | auth status/busy/error and login/logout methods |
| T1/T2/T3 | T4 docs | all closed design decisions and exact commands/config |
| T1 sqlc/T3 tsc | final Gates | regenerated output staged, freshness unchanged |

## Acceptance criteria and Gates

All commands run from repository root except explicit `cd web`:

1. `go test -race ./...`: old regressions plus bootstrap fixture, account CLI,
   sessions, rate-limit, HTTP protection and complete ownership tests pass.
2. `cd web && npm test`: date regressions and auth-transition/race tests pass.
3. `make check`: generated freshness, tidy, lint, all tests, deploy syntax and text
   checks exit zero; optional shellcheck absence is reported honestly.
4. `make build && make build-linux`: single static binaries build successfully.
5. All `$PLAN_EDITS check` commands pass byte-for-byte for documented edits.
6. Independent validator builds binary and uses an isolated temporary DB to prove
   CLI hidden prompts, two accounts, separate state/order/settings, all mutations,
   reload/restart persistence, logout, reset revocation, unauthorized and foreign
   requests. Test-only credentials generated in memory, never printed/committed.
7. Independent headless Chrome at 320/600: initial login, accessible labels and
   keyboard submit, generic error, two users/login/logout, all three views,
   add/rename/caret/delete/Undo/days/settings/reload, expiry clears private DOM,
   no horizontal overflow or page console errors. Save screenshots privately.
8. `git diff --check 34a5c2d` passes; reviewed hashes/commits exactly match final
   content; all task findings resolved or explicitly accepted by validator.
9. Signed Conventional Commits on feature branch; push that branch and inspect CI
   when available. Do not merge to main (deployment is pull-based on main).

Final verifier owns its serial `make check` run. Orchestrator archives precommit
commands, output, exit status, tree hashes and resulting commit associations in
private scratch. No raw test credentials or session tokens in evidence logs.

## Not doing

Public registration, remote account provisioning, invitations, admin UI, email,
OAuth, recovery flows, additional tracker features, new infrastructure or frontend
runtime dependencies, live database modification, deployment, main merge, force
push or history rewriting. No weakening signing, CI or permission boundaries.
