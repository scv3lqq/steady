# Deploy kit

Runs Steady on a Linux server as a hardened systemd service under its own system user. Steady is one
static binary with the frontend embedded and one SQLite file, so the host needs no Docker, no database
server and no Node. The server can also ship `main` by itself ([Auto-deploy](#auto-deploy)).

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
# STEADY_ALLOWED_HOSTS=habits.example.com
```

On the server, `curl -fsS http://127.0.0.1:8770/healthz` answers `{"status":"ok"}` while the database
is reachable.

## Reaching the app

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

## Settings and updates

- The binary: copy a new build to `DIR/bin/steady`, then `systemctl restart steady`, no sudo. The unit
  installs the staged binary to `/usr/local/bin` on every start. `install.sh` writes
  `/etc/polkit-1/rules.d/50-steady.rules`, which lets the deploy user (the one who ran it through sudo)
  start, stop and restart `steady.service` and nothing else. It needs polkit 0.106 or newer
  (`polkitd`); without it `install.sh` says so and a restart needs `sudo`.
- Settings: `sudoedit /etc/steady/env`, then `sudo systemctl restart steady`. The variables are
  `STEADY_ADDR`, `STEADY_DB`, `STEADY_LOG_LEVEL` and `STEADY_ALLOWED_HOSTS`. After a change of the port
  in `STEADY_ADDR`, rerun `sudo DIR/install.sh DIR`: it copies the port into the auto-deploy unit and
  checks `/healthz` there.
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
   ([Rollback](#rollback)).

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

## What protects what

- The service binds to `127.0.0.1` and has no authentication. The Host allowlist (loopback names on any
  port, the listen address and `STEADY_ALLOWED_HOSTS`) stops DNS rebinding and
  `http.CrossOriginProtection` guards the mutating routes. A reverse proxy must bring its own
  authentication.
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
