# Cross-device sync and session recovery

## Goal
Visible clients converge to server state within five seconds under a responsive server. Temporary deployment/network failures preserve authentication and recover automatically. Base: f552fb0. Branch: feat/sync-session-recovery.

## Evidence and constraints
- Baseline make check passed (private evidence: base-check.log); shellcheck unavailable, Makefile explicitly skips it.
- SQLite persists session hashes and absolute expiry; HTTP binding derives deterministically from the opaque cookie. No backend auth redesign is needed.
- store.ts checkSession incorrectly transitions to anonymous on network errors and non-401 responses. main.ts has no remote refresh lifecycle.
- Follow AGENTS.md, spec and deploy README; CLAUDE.md aliases AGENTS.md and adds plans directory and headless checks at 320 and 600px. No .claude/rules directory exists.
- Preserve unrelated untracked TODO.md. No passwords/credentials are read. No production proxy configuration changes.

## Acceptance criteria and Gates
- make check and make build exit 0 on final staged/generated tree. Generated freshness compares against index: stage generated output before make check, then commit only after passing.
- cd web && npm run build && npm test: existing guards and new deterministic tests cover two clients, user isolation, delayed snapshot after write drain, pending names, concurrent mark ordering, transient session discovery, proxy 401 versus app 401, old generation, lifecycle triggers, hidden tabs, timeout and timer deduplication.
- go test -race ./internal/handler/httpapi: actual httptest HTTP clients preserve identical cookie and binding after server AND SQLite close/reopen, fixed expiry/no renewal, logout/reset/expiry revoke, two independent sessions converge and different account cannot read/write foreign habits.
- Headless Chrome checks 320 and 600px: no overflow, login/load, remote marks on/off, focus/caret, add draft and Undo intact, offline/reconnect, logout/reload and peer expiry. Use synthetic isolated accounts only.
- Signed commits, merge main, push main, resulting CI test/secrets pass, remove non-main local/origin branches per project rule.
- Baseline npm tests: 42 pass; baseline HTTP suites pass. New behavioral regressions must fail before production edits (save exited RED logs); persistence/security guards may pass baseline.

## Shared definitions
- New web/src/sync.ts exports startSync(load: () => Promise<void>, page: Document, host: Window): () => void. Single 3000ms interval only while visible, load immediately at start, visibility restoration, focus, online, pageshow and document resume; coalesce concurrent wake events into one in-flight check. Return idempotent stop that clears timer and unregisters listeners. On hidden or stopped state no new checks. Initial call replaces main.ts direct store.load(). Every trigger checks visibility; repeated events never duplicate intervals. Use host timer methods for testability.
- Store.refresh(): Promise<void> calls load only for checking/authenticated and when no auth operation is pending; anonymous login screen does not continually rediscover a cookie. Store.load retains explicit discovery semantics. One active periodic refresh promise per session; timeout each discovery/state GET with AbortSignal.timeout(4000) combined with session abort via AbortSignal.any. No timeout on queued writes. Native timeout is supported by target modern browsers and Node 26.
- A monotonic local revision increments before every local mutation (including pending rename, flush and queue enqueue). Each state GET captures revision at dispatch AND requires queue idle at dispatch and commit, same revision at commit, same generation and latest read ordering. Discard stale snapshots; polling or queue-drain recovery retrieves another. Never recursively retry indefinitely on edits. Overlay pending names on accepted snapshots. Keep latest server-successful mark as authoritative once queue drains; no replay of already successful writes.
- Confirm app authentication loss only for HTTP 401 JSON {error: "authentication required"} (session/state/write/flush), not HTML/proxy 401. Other failures are transient. checking remains checking with error "Unable to check your session. Try again."; authenticated state/binding survive transient reads. Invalid discovery response remains checking with same error. Only confirmed loss broadcasts/clears private data. Successful login discovery must use checking status during transient failures. No password retention or session lifetime change.
- Keep render.ts focus patching, UI Undo/add content and queue ownership. Regular refresh must not trigger auth transition. If remote view changes would rebuild a focused name input, defer view replacement while a tracker name input is focused (render only), then render on blur; preserve local pending name by Store overlay.

## Tasks
### T1: Frontend regression tests (test-first)
Ownership: web/test/store.test.mjs, web/test/sync.test.mjs. No production edits. Extend actual cookie/binding-aware doubles, do not use mocked 204 as revocation proof. Cases: 503/throw initial discovery stays checking then recovers, transient authenticated read, non-JSON proxy401 retains auth; app JSON401 clears; stale GET whose write completes before body arrives must retain local mark; GET started with outstanding write cannot commit old data; pending rename survives; overlapping GET ordering; generation isolation. Lifecycle fake Document/Window and clock cover exactly one interval, initial/immediate wake refresh, hidden pause, all wake signals, in-flight coalescing, stop. Two independent cookie clients sharing simulated per-account server state cover mark/unmark, last successful save wins and account isolation. Update response helper default 401 body to real app error; malformed binding test expects checking. Scoped gate: cd web && npm run build && npm test. RED expected until T3/T4, retain existing passing guards. Named mutations: remove revision check, clear auth on 503, remove visibility check, duplicate interval; reviewer probes these in isolated scratch.

### T2: Real HTTP lifecycle and independent clients
Ownership: internal/handler/httpapi/session_lifecycle_test.go only. Guard tests with httptest server, real SQLite file close/reopen, synthetic hasher and clock, actual net/http clients/cookiejar. Same saved cookie must work with same binding on new handler/server on same SQLite. Check no Set-Cookie renewal, day 29 success and exact day30 expiry; logout and reset then replay unchanged cookie to /api/session yields app JSON401. Two distinct account sessions plus another account verify marks on/off and foreign isolation. Scoped gates: gofmt file; go test -race ./internal/handler/httpapi; golangci-lint run ./internal/handler/httpapi/.... These guards should pass baseline. No production edits.

### T3: Store implementation
Depends T1 exited RED. Ownership: web/src/store.ts and generated web/dist/store.js only. Implement Shared definitions Store changes; preserve existing methods and queue/auth boundary semantics. Scoped gate: cd web && npm run build && node --test test/store.test.mjs. No full suite. Named mutations from T1 apply to this source.

### T4: Sync lifecycle and main wiring
Depends T1 exited RED and T3 completed. Ownership: web/src/sync.ts, web/src/main.ts, web/src/render.ts, generated web/dist/sync.js, web/dist/main.js, web/dist/render.js only. Implement Shared definitions lifecycle and focus handling. Main imports startSync, passes () => store.refresh(), document, window; retained pagehide flush. Scheduler stop on pagehide? Do not permanently stop for bfcache; hide event clears scheduling when appropriate, pageshow resumes. Tests may refine fake lifecycle. Scoped gates: cd web && npm run build && node --test test/sync.test.mjs. No full suite.

### T5: Documentation
Ownership: docs/spec.md, deploy/README.md. Exact old/new prose blocks are in plans/sync-session-spec-edits.md and plans/sync-session-deploy-edits.md, produced using canonical plan_edits.py; implementer applies those blocks only. Document polling/load, lifecycle, stale response rule, temporary failure versus confirmed logout, fixed lifetime and diagnosis of DB/cookie/proxy. Do not imply production deployment was inspected. Scoped gate: plan_edits check using the two documentation subplans for both docs; make check-text.

## Seams
| Producer | Consumer | Shared definition |
| T1 | T3 | refresh, local revision, app JSON401 |
| T1 | T4 | startSync signature and lifecycle |
| T3 | T4 | Store.refresh and no auth transition on periodic reads |
| T2 | T5 | same cookie/binding/SQLite and fixed expiry |
| T3/T4 | T5 | polling and temporary error behavior |

## Not doing
SSE/WebSockets, schema migration, revisions API, password storage, sliding renewal, offline write replay, proxy protection changes, production credentials or production deployment access.

## Execution and evidence
T1/T2 first, wait for RED exit before source writers. T3 then T4 sequentially because tsc emits the entire compile unit. Scoped builds may deterministically regenerate unchanged sibling dist files; only changed owned outputs are staged. Review exact commits or file hashes independently; full gates serially only after completed waves, acceptance-validator runs final full gates. Up to five fix iterations, stop on same finding three times or unavailable mandatory gates. All logs stay in private scratch, never commit environment paths. Baseline evidence external to repo. Generated output staged for freshness before precommit gates, never skip hooks. Merge/push authorization overrides canonical feature branch defaults.

## Iteration 1: pagehide write ownership
Independent review reproduced a state snapshot overwriting a pending keepalive rename after pagehide/pageshow. This is within the original pending-name/stale-response requirements; fix before final acceptance.

### T6: Pagehide regression (test-first)
Ownership: web/test/store.test.mjs only. Add named cases holding keepalive PATCH while a GET snapshot is dispatched and completes before/after PATCH completion. Expected local name remains `Still typing after wake`; once server accepts the rename a later snapshot converges. A stale flush from an earlier generation must never decrement current-generation write tracking. Named mutation: remove flush accounting from read eligibility. Gate: cd web && npm run build && node --test test/store.test.mjs. Save exited RED before T7. Reviewer's reproduction is in private scratch probe-flush.

### T7: Track pagehide writes
Depends exited T6 RED. Ownership: web/src/store.ts and generated web/dist/store.js only. Add private flushing count initialized to zero and reset on auth transition. Increment synchronously when dispatching each keepalive rename; include it with queued writes at state GET dispatch and commit eligibility. Decrement in finally only when captured generation matches current generation. A GET dispatched while flushing stays ineligible even if flush finished before body arrives. Keep timeout-free keepalive write, session binding, session-loss handling, pending/auth cleanup. Later polling supplies authoritative state; no offline replay or new timers. Gate: cd web && npm run build && node --test test/store.test.mjs. Other code unchanged.

Seam T6/T7: flushing participates in snapshot eligibility, scoped by session generation. Re-review the changed hashes and rerun full Gates before commit. This early review fix is incorporated before the final validator instead of shipping a known failure to acceptance.
