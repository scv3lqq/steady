# Exact instruction edits

Base: `f3f47e6`. Target: `AGENTS.md`.

**E1.** (lines 36-37)

````md E1-old
- Frontend: TypeScript `strict`, no runtime dependencies, `typescript` is the only devDependency. One `render()` redraws from state; event delegation via `data-action`, `data-id`, `data-date`; user text goes through `textContent`, never `innerHTML`. Never rebuild the name input that has focus.
- The UI updates optimistically and syncs each change to the API. On an API error it reloads state from the server once the queued requests have finished. Authentication boundaries clear private state, pending names, timers and Undo, and guard queued requests and late responses by session generation. Every queued private request captures `X-Steady-Session` from `/api/session` (`{username,binding}`); server validation requires both cookie and matching binding. The binding is a request-context marker, not a reusable session. Native `BroadcastChannel('steady-auth')` clears other tabs' private state/DOM/pending work on auth transitions and carries no identity or credentials. Never retain a password in state or browser storage.
````

````md E1-new
- Frontend: TypeScript `strict`, no runtime dependencies, `typescript` is the only devDependency. One `render()` redraws from state; event delegation via `data-action`, `data-id`, `data-date`; user text goes through `textContent`, never `innerHTML`. Never rebuild the name input that has focus.
- The UI updates optimistically and syncs each change to the API. On an API error it reloads state from the server once the queued requests have finished. Authentication boundaries clear private state, pending names, timers and Undo, and guard queued requests and late responses by session generation. Every queued private request captures `X-Steady-Session` from `/api/session` (`{username,binding}`); server validation requires both cookie and matching binding. The binding is a request-context marker, not a reusable session. Native `BroadcastChannel('steady-auth')` clears other tabs' private state/DOM/pending work on auth transitions and carries no identity or credentials. Never retain a password in state or browser storage.
- Authentication tests for changed flows must enforce the actual HTTP cookie, binding-header and status contract in their doubles. Confirm login, state loading, logout/reload and peer-tab expiry against the real handler; a mocked success response alone does not prove session revocation.
````

**E2.** (lines 45-46)

````md E2-old
- All habit/settings operations require validated opaque sessions and server-derived ownership. Mutating API routes, including login/logout, retain `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`). `/healthz` remains public.
- Account management is server-side CLI only with hidden terminal password/confirmation prompts. No password arguments, environment variables, piped input or logging. No public signup, remote account creation, invitations, admin UI, email, OAuth or recovery flow. Follow `docs/spec.md` for password hashing, session lifetime, username rules and bounded login rate limits.
````

````md E2-new
- All habit/settings operations require validated opaque sessions and server-derived ownership. Mutating API routes, including login/logout, retain `http.CrossOriginProtection` and a Host allowlist against DNS rebinding (loopback names on any port, the listen address, plus `STEADY_ALLOWED_HOSTS`). `/healthz` remains public.
- Account management is server-side CLI only with hidden terminal password/confirmation prompts. No password arguments, environment variables, piped input or logging. No public signup, remote account creation, invitations, admin UI, email, OAuth or recovery flow. Follow `docs/spec.md` for password hashing, session lifetime, username rules and bounded login rate limits.
- Account provisioning and password resets must cover every existing login gate on the normal public entry path, including the initial owner. A loopback API login is not proof of public access. Follow `deploy/README.md#account-access-through-additional-proxy-authentication` and report which protected credential reference belongs to each gate.
````
