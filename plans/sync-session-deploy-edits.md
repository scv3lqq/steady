# Exact edits for deploy/README.md

**E1.** (lines 111-113)

````md E1-old
- Cookies are `steady_session`, `HttpOnly`, `SameSite=Strict`, `Path=/`, no Domain, and Secure unless
  explicitly disabled. Sessions persist across restarts and expire 30 days after login without sliding
  renewal; logout revokes the current session and a password reset revokes all of that user's sessions.
````

````md E1-new
- Cookies are `steady_session`, `HttpOnly`, `SameSite=Strict`, `Path=/`, no Domain, and Secure unless
  explicitly disabled. Sessions persist across restarts and expire 30 days after login without sliding
  renewal; logout revokes the current session and a password reset revokes all of that user's sessions.
- Visible pages poll every 3 seconds; hidden pages pause. Returning to the page, reconnecting or waking
  a device checks immediately. Temporary deployment downtime retries automatically with the same
  cookie and binding; it must not be treated as logout. No password is retained by the frontend.

### Diagnosing repeated login

The confirmed application failure was session discovery turning a network failure or HTTP 503 into
a login screen despite a valid stored session. The client now stays in retryable session checking.
Real HTTP regression tests close/reopen the server and SQLite and verify the identical cookie and
binding without changing the 30-day expiry. This is not evidence about an external live proxy.

If login is still requested, distinguish the gate before changing configuration:

1. Inspect the response status and type: Steady returns JSON 401 with `authentication required` for
   an expired/revoked/absent session; HTML 401 or a `WWW-Authenticate` challenge may belong to a proxy.
   Preserve additional proxy authentication until its purpose and any change are agreed.
2. Verify the browser sends `steady_session` to the same host, path and HTTPS scheme. Keep Secure,
   HttpOnly and SameSite settings intact; never paste a cookie, binding or password into logs.
3. Check the active service uses the same absolute `STEADY_DB` before and after restart. The kit uses
   `/var/lib/steady/steady.db`; local default `data/steady.db` depends on the working directory.
4. Compare `/api/session` before/after restart with the existing browser cookie, without logging it.
   A valid session must return the same identity/binding without a new Set-Cookie header. Check
   expiry, explicit logout and password resets before concluding that deployment removed sessions.

````
