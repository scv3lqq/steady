# Private account documentation edits: deploy/README.md

Base: `34a5c2d`. Apply and check with the canonical prose-edit helper; change only this file.

**E1.** (lines 3-5)

````md E1-old
Runs Steady on a Linux server as a hardened systemd service under its own system user. Steady is one
static binary with the frontend embedded and one SQLite file, so the host needs no Docker, no database
server and no Node. The server can also ship `main` by itself ([Auto-deploy](#auto-deploy)).
````

````md E1-new
Runs Steady on a Linux server as a hardened systemd service under its own system user. Steady is one
static binary with the frontend embedded and one SQLite file, so the host needs no Docker, no database
server and no Node. Private username/password accounts are created through the server CLI. The first
upgrade from the unauthenticated version needs the [stopped-service procedure](#upgrade-to-private-accounts)
below before [auto-deploy](#auto-deploy) is enabled.
````

**E2.** (lines 49-52)

````md E2-old
STEADY_DB=/var/lib/steady/steady.db
STEADY_LOG_LEVEL=info
# STEADY_ALLOWED_HOSTS=habits.example.com
```
````

````md E2-new
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
````

**E3.** (lines 59-69)

````md E3-old
The service listens on `127.0.0.1:8770` only and has no authentication: it is a single-user app. Never
bind it to a public interface.

- SSH tunnel: `ssh -N -L 8770:127.0.0.1:8770 <user>@<server>`, then open <http://127.0.0.1:8770/> on
  your machine.
- Reverse proxy: a proxy that adds authentication and forwards to `127.0.0.1:8770`. Its public host goes
  into `STEADY_ALLOWED_HOSTS` (comma-separated, for example `STEADY_ALLOWED_HOSTS=habits.example.com`
  in `/etc/steady/env`, then `sudo systemctl restart steady`), and the proxy should forward the
  original `Host` header. Any other `Host` gets `403 host not allowed`: that is the guard against DNS
  rebinding. Loopback names (`localhost`, `127.0.0.1`, `::1`) pass on any port, so a tunnel on another
  local port works.
````

````md E3-new
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
- The Host allowlist accepts loopback names (`localhost`, `127.0.0.1`, `::1`) on any port, the listen
  address and configured public hosts. Other values receive `403 host not allowed`. Cross-origin
  protection also covers login and logout. `/healthz` needs no login.
````

**E4.** (lines 78-81)

````md E4-old
- Settings: `sudoedit /etc/steady/env`, then `sudo systemctl restart steady`. The variables are
  `STEADY_ADDR`, `STEADY_DB`, `STEADY_LOG_LEVEL` and `STEADY_ALLOWED_HOSTS`. After a change of the port
  in `STEADY_ADDR`, rerun `sudo DIR/install.sh DIR`: it copies the port into the auto-deploy unit and
  checks `/healthz` there.
````

````md E4-new
- Settings: `sudoedit /etc/steady/env`, then `sudo systemctl restart steady`. The variables are
  `STEADY_ADDR`, `STEADY_DB`, `STEADY_LOG_LEVEL`, `STEADY_ALLOWED_HOSTS` and `STEADY_COOKIE_SECURE`.
  After a change of the port in `STEADY_ADDR`, rerun `sudo DIR/install.sh DIR`: it copies the port into
  the auto-deploy unit and checks `/healthz` there.
````

**E5.** (lines 106-108)

````md E5-old
6. stages the binary, restarts the service through the polkit rule and calls `/healthz` once a second
   for up to 30 s. No answer: the previous binary goes back and the commit is skipped
   ([Rollback](#rollback)).
````

````md E5-new
6. stages the binary, restarts the service through the polkit rule and calls `/healthz` once a second
   for up to 30 s. No answer: the previous binary goes back and the commit is skipped
   ([Rollback](#rollback)). This mechanism must not perform the first private-account schema upgrade;
   disable the timer first and use the [manual upgrade procedure](#upgrade-to-private-accounts).
````

**E6.** (lines 130-147)

````md E6-old
## Rollback

- Automatic: when the new binary gives no answer on `/healthz` within 30 s of the restart,
  `autodeploy.sh` puts the previous binary back, restarts the service and rejects the commit. Later
  runs skip it until `main` moves on. If the previous binary does not answer either, the log says so:
  `sudo journalctl -u steady`.
- By hand: the binary the last restart replaced is `DIR/bin/steady.prev`. As the deploy user, run
  `mv DIR/bin/steady.prev DIR/bin/steady && systemctl restart steady` (the polkit rule allows the
  restart without sudo). Auto-deploy leaves the head it has already deployed alone, so the old binary
  stays until `main` moves on; then the next signed commit that passes CI ships again. To stop that,
  push a signed revert to `main`, or turn auto-deploy off with `sudo env AUTO_DEPLOY=off DIR/install.sh DIR`.

## Backups

The data is one SQLite file in WAL mode, `/var/lib/steady/steady.db`, in a directory only the `steady`
user can enter. Back it up with `sudo sqlite3 /var/lib/steady/steady.db ".backup <file>"`; it is safe
while the service runs. Never copy the live file without its `-wal`: the newest writes sit there until
a checkpoint.
````

````md E6-new
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
````

**E7.** (lines 151-154)

````md E7-old
- The service binds to `127.0.0.1` and has no authentication. The Host allowlist (loopback names on any
  port, the listen address and `STEADY_ALLOWED_HOSTS`) stops DNS rebinding and
  `http.CrossOriginProtection` guards the mutating routes. A reverse proxy must bring its own
  authentication.
````

````md E7-new
- The service binds to `127.0.0.1` and requires private account sessions for all habit/settings
  operations. Repository queries enforce user ownership for reads, day marks, delete and Undo.
  Legacy data remains inaccessible until explicit CLI bootstrap. The Host allowlist and
  `http.CrossOriginProtection` cover mutating routes including login/logout. HTTPS reverse proxies
  require Secure cookies; local HTTP needs the explicit documented exception. Passwords and session
  tokens are never logged.
````
