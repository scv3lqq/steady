# Exact edits for docs/spec.md

**E1.** (lines 79-79)

````md E1-old
- Re-render on every change. Don't rebuild the input that has focus: patch in place, or skip the name input while it's focused, so the caret doesn't jump while the user types.
````

````md E1-new
- Re-render on every change. Don't rebuild the input that has focus: patch in place, or skip the name input while it's focused, so the caret doesn't jump while the user types. Defer a remote view change until the focused habit name loses focus.
- Poll the authenticated account state every 3 seconds while visible (at most 20 reads per minute per visible tab), so remote changes appear within 5 seconds with a responsive server. Pause hidden tabs. Check immediately on visibility restoration, focus, online, pageshow and resume; coalesce concurrent triggers and bound reads to 4 seconds. Keep optimistic writes, pending names, input drafts and Undo. Reject snapshots started during writes or before a newer local edit, even after the write queue drains; within one account, the last successfully saved day operation wins.
````

**E2.** (lines 92-92)

````md E2-old
- While checking the session, show a neutral loading state without tracker data. Failed login shows a generic invalid-credentials error; throttling has a clear retry message. Disable duplicate authentication submissions. Do not store the password in application state or browser storage; clear the field after submission.
````

````md E2-new
- While checking the session, show a neutral loading state without tracker data. A network failure, timeout, malformed response, temporary server error or proxy rejection keeps session checking retryable, never requires a fresh login by itself. An already authenticated page retains its session and resumes automatically when the server returns. Only Steady's JSON 401 authentication-required response confirms session loss; do not confuse a reverse proxy's login challenge with application logout. Failed login shows a generic invalid-credentials error; throttling has a clear retry message. Disable duplicate authentication submissions. Do not store the password in application state or browser storage; clear the field after submission.
````

**E3.** (lines 109-109)

````md E3-old
- Frontend tests prove private state clearing, abandoned queues and late responses cannot contaminate a later login, including two-tab cookie changes, stale bindings and broadcast authentication transitions. Independently verify a complete two-user workflow and run `make check`, including generated-code freshness.
````

````md E3-new
- Frontend tests prove private state clearing, abandoned queues and late responses cannot contaminate a later login, including two-tab cookie changes, stale bindings and broadcast authentication transitions. Cover independent clients, conflicting marks, delayed snapshots after queued writes finish, pending names, hidden tabs, wake/reconnect, timer coalescing and transient outages. Real HTTP tests close and reopen the server and the same SQLite file, replay the identical cookie/binding without renewal, and verify logout/reset/absolute expiry using session discovery without a binding header. Independently verify a complete two-user workflow and run `make check`, including generated-code freshness.
````
