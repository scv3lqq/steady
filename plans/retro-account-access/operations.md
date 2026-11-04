# Exact operations documentation edits

Base: `f3f47e6`. Target: `deploy/README.md`.

**E1.** (lines 69-70)

````md E1-old
On the server, `curl -fsS http://127.0.0.1:8770/healthz` answers `{"status":"ok"}` while the database
is reachable.
````

````md E1-new
On the server, `curl -fsS http://127.0.0.1:8770/healthz` answers `{"status":"ok"}` while the database
is reachable.

## Account access through additional proxy authentication

Account creation and password reset must cover the normal public entry path, including any existing
proxy login. Use this procedure for the initial owner as well as later accounts:

1. Inspect the active proxy configuration for additional login gates. For an already-authorized
   server credential, consult the configured password manager before requesting manual or GUI entry;
   do not inspect unrelated secrets.
2. Prepare a protected credential handoff before generating a password, then use the server-side
   Steady CLI with hidden prompts. Keep passwords out of arguments, environment variables and logs.
3. Provision the intended identity at a proxy login only when its credential store is restricted to
   Steady. Preserve other entries and back up the store before an atomic update. A store shared with
   another application needs explicit authorization for the broader access change. Never disable
   existing proxy protection or silently rotate another identity's password.
4. Use matching usernames and one password at both gates when existing policy permits; otherwise
   record which protected credential reference belongs to each gate. Do not reuse server-admin
   credentials as application credentials.
5. Verify a fresh client through the public HTTPS endpoint: pass every proxy gate, establish the
   intended Steady identity, and load that user's state (empty/default after `create`, preserved
   legacy habits/settings after `bootstrap`). Before logout or password reset, keep the valid Steady
   cookie only in verifier memory and confirm `GET /api/session` returns that identity with 200.
   Afterwards, replay that same cookie to `GET /api/session` with currently valid proxy credentials
   and require Steady's JSON 401 response. This route needs no binding header, so neither a proxy
   rejection nor an omitted `X-Steady-Session` can masquerade as revocation. Never log or persist the
   cookie. Report access ready only after these checks and the protected handoff are complete.

````
