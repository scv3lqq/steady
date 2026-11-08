# Deploy kit

Runs Steady on a Linux server as a hardened systemd service under its own system user. Steady is one
static binary with the frontend embedded and one SQLite file, so the host needs no Docker, no database
server and no Node. Private username/password accounts are created through the server CLI. The first
upgrade from the unauthenticated version needs the [stopped-service procedure](#upgrade-to-private-accounts)
below before [auto-deploy](#auto-deploy) is enabled.

> The kit assumes the repository is public: the host fetches over anonymous HTTPS and reads check runs from the unauthenticated GitHub API. While the repository is private, autodeploy cannot fetch or see CI.

| Path | What |
| --- | --- |
| `install.sh` | root: service user, `/etc/steady/env`, the unit, a polkit rule for restarts, start |
| `autodeploy.sh` | deploy user, from the timer: ships the signed head of `main` ([Auto-deploy](#auto-deploy)) |
| `systemd/steady.service` | unit template, `@DEPLOY_DIR@` is filled in by `install.sh` |
| `systemd/steady-deploy.{service,timer}` | auto-deploy units, installed by `install.sh` with `AUTO_DEPLOY=on` |

## Requirements

- On the server: Linux x86_64 with systemd, `curl` and `sudo`, and a user to deploy as (the deploy user)
  that you can reach over SSH. Optional: polkit 0.106 or newer, for restarts without sudo, and
  `sqlite3`, for [backups](#backups).
- For [auto-deploy](#auto-deploy) also: `git`, `jq`, `ssh-keygen` (the OpenSSH client), polkit 0.106 or
  newer (required there) and a Go toolchain, 1.21 or newer, installed outside `/home` (the deploy unit
  hides the other home directories; Go downloads the toolchain version `go.mod` asks for). Outbound
  HTTPS to GitHub and to the Go module proxy.
- On your machine: Go and `make` to build the binary, `ssh` and `scp`.

## Install

Below, `DIR` stands for the deploy dir on the server, `/home/<user>/steady-deploy` by default. Always
write `DIR` as an absolute path: a local shell expands `~` before ssh sends the command.

1. Build the binary: `make build-linux`.
2. Copy the kit and the binary:
   `ssh <user>@<server> 'mkdir -p DIR/bin && chmod 700 DIR'`,
   `scp -r deploy/systemd deploy/install.sh deploy/autodeploy.sh <user>@<server>:DIR/`,
   `scp bin/steady-linux-amd64 <user>@<server>:DIR/bin/steady`.
3. **Sudo password:** `ssh -t <user>@<server> sudo DIR/install.sh DIR`. The script creates the system
   user `steady` (home `/var/lib/steady`, no login shell), installs the binary to
   `/usr/local/bin/steady`, writes `/etc/steady/env` (root:steady 0640) if it is missing, installs the
   unit and a polkit rule (see [Settings and updates](#settings-and-updates)), leaves
   `DIR/install-state` (no secrets: whether the rule and auto-deploy are in place), enables the service
   and waits for `/healthz`. It ends with `service: active, listening on http://127.0.0.1:8770` and the
   `/healthz` answer.

The first run writes this env file. Later runs keep it, so edit it with `sudoedit /etc/steady/env`:

```sh
STEADY_ADDR=127.0.0.1:8770
STEADY_DB=/var/lib/steady/steady.db
STEADY_LOG_LEVEL=info
STEADY_COOKIE_SECURE=true
# STEADY_ALLOWED_HOSTS=habits.example.com
```

Before the first browser login, create the owner's account in an interactive server terminal:

```sh
sudo -u steady env STEADY_DB=/var/lib/steady/steady.db /usr/local/bin/steady account bootstrap owner
```

The command asks for a password and confirmation with echo disabled. Usernames match
`[a-z][a-z0-9_-]{2,31}`; passwords are valid UTF-8, 12 to 128 bytes. Never pass passwords in arguments,
environment variables or piped input. Use `account create <username>` for additional empty accounts and
`account reset-password <username>` to change a password and revoke all sessions. Use the same database
path and filesystem user for every account command.

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


## Reaching the app

The service listens on `127.0.0.1:8770` and authenticates private accounts. Keep the backend on loopback.

- SSH tunnel or local HTTP: set `STEADY_COOKIE_SECURE=false` explicitly in `/etc/steady/env`, restart,
  run `ssh -N -L 8770:127.0.0.1:8770 <user>@<server>`, then open <http://127.0.0.1:8770/>.
  The cookie travels over local HTTP in this mode; never use it for a public HTTP endpoint.
- HTTPS reverse proxy: terminate TLS at the proxy and retain the default `STEADY_COOKIE_SECURE=true`.
  The public host goes into `STEADY_ALLOWED_HOSTS` (for example `habits.example.com`); forward the
  original `Host` header and restart after env changes. Do not trust forwarded headers to choose cookie
  security or the login limiter's client IP. The limiter uses only the immediate transport peer, so
  users behind one proxy share its per-IP allowance. The proxy may apply additional edge limits.
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

- The Host allowlist accepts loopback names (`localhost`, `127.0.0.1`, `::1`) on any port, the listen
  address and configured public hosts. Other values receive `403 host not allowed`. Cross-origin
  protection also covers login and logout. `/healthz` needs no login.

## Settings and updates

- The binary: copy a new build to `DIR/bin/steady`, then `systemctl restart steady`, no sudo. The unit
  installs the staged binary to `/usr/local/bin` on every start. `install.sh` writes
  `/etc/polkit-1/rules.d/50-steady.rules`, which lets the deploy user (the one who ran it through sudo)
  start, stop and restart `steady.service` and nothing else. It needs polkit 0.106 or newer
  (`polkitd`); without it `install.sh` says so and a restart needs `sudo`.
- Settings: `sudoedit /etc/steady/env`, then `sudo systemctl restart steady`. The variables are
  `STEADY_ADDR`, `STEADY_DB`, `STEADY_LOG_LEVEL`, `STEADY_ALLOWED_HOSTS` and `STEADY_COOKIE_SECURE`.
  After a change of the port in `STEADY_ADDR`, rerun `sudo DIR/install.sh DIR`: it copies the port into
  the auto-deploy unit and checks `/healthz` there.
- Logs: `sudo journalctl -u steady` (sudo unless the deploy user is in `adm` or `systemd-journal`).
- A change in `deploy/` (the scripts, the units) reaches the server by copying the kit again (step 2 of
  [Install](#install), without the binary) and rerunning `sudo DIR/install.sh DIR`.

## Auto-deploy

Optional: the server pulls `main` and ships it, so a push needs no step on your machine. Nothing
reaches into the server and GitHub holds no credential of it: every 5 minutes `steady-deploy.timer`
runs `DIR/autodeploy.sh` as the deploy user, which

1. fetches `main` over HTTPS from the public repo (no token) into `DIR/src`; an unchanged head ends the
   run;
2. checks the head commit's SSH signature against `DIR/allowed_signers`: a commit no key there signed
   is rejected and never deploys. A head that does not descend from the deployed commit (a force push)
   waits until you delete the `deployed=` line in `DIR/autodeploy-state`;
3. waits for the CI checks `test` and `secrets` to pass on that commit (the unauthenticated GitHub API,
   no token);
4. holds while `deploy/` in the commit, `README.md` aside, differs from the kit in `DIR`: those changes
   need root, so they still go through the full setup;
5. builds the binary with the flags of `make build-linux`
   (`CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' ./cmd/steady`). `web/dist` is
   committed, so the Go toolchain is all the build needs. The same bytes as the running binary are
   recorded without a restart, so commits that leave the Go code and `web/dist` alone (docs, CI) restart
   nothing;
6. stages the binary, restarts the service through the polkit rule and calls `/healthz` once a second
   for up to 30 s. No answer: the previous binary goes back and the commit is skipped
   ([Rollback](#rollback)). This mechanism must not perform the first private-account schema upgrade;
   disable the timer first and use the [manual upgrade procedure](#upgrade-to-private-accounts).

Turn it on:

- On the server: the tools and the polkit rule from [Requirements](#requirements). Your commits on
  `main` are SSH-signed.
- `DIR/allowed_signers`, one line per key whose commits may deploy, in the format of
  `ssh-keygen -Y verify`: a name, `namespaces="git"` and the public key
  (`you@example.com namespaces="git" ssh-ed25519 AAAA...`). Copy it over with
  `scp allowed_signers <user>@<server>:DIR/`. It must be a regular file owned by the deploy user or
  root.
- `sudo env AUTO_DEPLOY=on DIR/install.sh DIR`. `AUTO_DEPLOY=off` removes the units; a run without it
  keeps them.

Follow it: `sudo journalctl -u steady-deploy` (sudo unless the deploy user is in `adm` or
`systemd-journal`), which says once per commit why it waits. `DIR/autodeploy-state` holds the last
deployed, held and rejected commit. A fork sets `AUTO_DEPLOY_REPO` (and `AUTO_DEPLOY_BRANCH`) as
`Environment=` lines in `sudo systemctl edit steady-deploy.service`.

Commits that GitHub signs, such as a merge in the web UI, carry GitHub's key, not yours: they wait for
your next signed commit on top.

## Upgrade to private accounts

Do this manually before the first account-enabled binary is started. The old unauthenticated binary
must never run against the migrated database. Health checks alone do not prove authentication or
schema compatibility, so keep auto-deploy disabled throughout this upgrade. These commands assume
the default database path; substitute the configured path consistently if it differs.

1. Build and verify the new binary off the server (`make check`, then `make build-linux`). Keep it
   separate from the currently staged/running binary until the backup is complete.
2. Disable and stop auto-deploy if installed, and stop the app:

   ```sh
   sudo systemctl disable --now steady-deploy.timer
   sudo systemctl stop steady-deploy.service steady.service
   ```

   If the auto-deploy units are absent, stop only `steady.service`. Confirm the service and any deploy
   job are stopped before replacing files or creating the backup.
3. Save a consistent database backup and matching old binary in a protected directory:

   ```sh
   sudo install -d -m 0700 /var/backups/steady
   sudo sqlite3 /var/lib/steady/steady.db ".backup /var/backups/steady/pre-accounts.db"
   sudo chmod 0600 /var/backups/steady/pre-accounts.db
   sudo install -m 0700 /usr/local/bin/steady /var/backups/steady/pre-accounts
   ```

   Choose a fresh backup location if those names already exist; do not overwrite your only recovery
   copy. Check the copy with `sudo sqlite3 /var/backups/steady/pre-accounts.db 'PRAGMA integrity_check;'`
   and require `ok`. Retain the old source revision/configuration alongside the matching binary.
4. Copy the new binary to `DIR/bin/steady` as in Install. With the service still stopped, install
   a root-owned executable copy outside the deploy user's private directory, then bootstrap from an
   interactive terminal as the service user:

   ```sh
   sudo install -o root -g root -m 0755 DIR/bin/steady /usr/local/bin/steady
   sudo -u steady env STEADY_DB=/var/lib/steady/steady.db /usr/local/bin/steady account bootstrap owner
   ```

   The deploy directory is mode 0700 and the service user cannot traverse it. Use the installed
   `/usr/local/bin/steady` path for every operator account command; retain the protected matching old
   binary saved in step 3 for recovery.

   Opening the database runs goose migrations. Bootstrap transactionally creates the owner, assigns
   every legacy habit, mark, position, setting and soft-deleted row, and records the owner ID in a
   singleton completion marker. Until this succeeds, legacy data is inaccessible over HTTP. A retry
   with the exact same username is a no-op with no password or data changes; another username is
   rejected. Migration marks existing soft-deleted rows `legacy_preserved`; purge excludes both
   unowned and preserved rows before and after bootstrap and on startup. Restore clears that flag,
   and a later explicit delete follows ordinary 24-hour retention. Never use a browser visitor or
   ordinary account creation to claim legacy data.
5. Configure `STEADY_COOKIE_SECURE=true` for HTTPS, or explicit `false` only for local HTTP/tunnel,
   and the allowed proxy host. Start `steady.service`; `/healthz` must still return
   `{"status":"ok"}`. Log in as the owner and verify existing habits, marks, ordering and settings.
   Create a second account with the terminal-only `account create <username>` command, verify that
   it starts empty with week view and accent `#4FA872`, and independently check that both accounts
   remain separate after logout/login. A password reset must invalidate prior sessions.
6. Only after these checks, re-enable the timer if it was previously enabled:
   `sudo systemctl enable --now steady-deploy.timer`. Ensure the previously staged rollback binary
   is also account-compatible before allowing binary-only rollback. Preserve the protected
   pre-upgrade backup and matching binary for offline recovery.

The ownership migration must not destructively downgrade. Do not run goose down or swap an old
unauthenticated binary onto this database. If upgrade recovery is required, stop auto-deploy and the
service again, preserve the failed database consistently for investigation, then restore the protected
pre-upgrade backup and its matching binary together while offline. After confirming every database
process is stopped, remove only this database's stale `-wal` and `-shm` sidecars before installing the
backup at the configured path; restore its `steady` ownership and restricted permissions. Restore the
matching binary to both `DIR/bin/steady` and `/usr/local/bin/steady` before starting, because the unit
copies the staged binary on startup. Keep external access disabled: the restored old version has no
authentication. Do not re-enable auto-deploy until the failed upgrade has been resolved.

## Rollback

Binary-only rollback is permitted only between versions compatible with the existing account schema
and authentication rules. It never substitutes for the offline backup recovery above.

- Automatic: after a compatible deployment fails `/healthz` for 30 seconds, `autodeploy.sh` reinstalls
  the previous staged binary and rejects the commit. Do not enable this across the first account
  migration or keep an unauthenticated binary as its rollback candidate.
- By hand: with the timer stopped, stage a verified compatible binary in `DIR/bin/steady`, then
  restart `steady.service` and verify authentication as well as `/healthz` before enabling the timer.

## Backups

The data is one SQLite file in WAL mode, `/var/lib/steady/steady.db`, in a directory only the `steady`
user can enter. With a protected destination, `sudo sqlite3 /var/lib/steady/steady.db ".backup <file>"`
creates a consistent copy while the service runs. `VACUUM INTO` is another consistent-copy option.
Never copy a live database without its `-wal`: the newest writes sit there until a checkpoint.
Backups include private habits, password hashes and session hashes. Use restricted permissions and
protect the destination directory, including copies made during upgrades.

## What protects what

- The service binds to `127.0.0.1` and requires private account sessions for all habit/settings
  operations. Repository queries enforce user ownership for reads, day marks, delete and Undo.
  Legacy data remains inaccessible until explicit CLI bootstrap. The Host allowlist and
  `http.CrossOriginProtection` cover mutating routes including login/logout. HTTPS reverse proxies
  require Secure cookies; local HTTP needs the explicit documented exception. Passwords and session
  tokens are never logged.
- The polkit rule grants one user three verbs on one unit: no other unit, no enable, disable or edit.
  The deploy user already owns `DIR`, whose binary the unit runs, and has sudo.
- Auto-deploy trusts the keys in `DIR/allowed_signers`, not GitHub: the signature covers the tree it
  builds, so neither a takeover of the GitHub account nor of the repo ships code without your key, and
  a force push back to an older signed commit waits for you. The CI check only keeps red builds out.
  Its unit sees no other path in `/home` than `DIR`, writes only there, has no Docker socket (a deploy
  user in the docker group would be root through it) and can't gain privileges, so no sudo.
- The service runs as `steady` under a hardened unit: `ProtectSystem=strict`, `ProtectHome`, no
  capabilities, private `/tmp` and devices, no memory that is writable and executable at once. Its only
  writable path is `/var/lib/steady`.
- What this does not protect: the database is not encrypted. Root on the host and the `steady` user can
  read it, and so can anyone who holds a copy of the disk or a backup.
