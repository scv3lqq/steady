#!/usr/bin/env bash
# Installs and starts the steady service from a deploy dir: system user, /etc/steady/env, the hardened
# unit, a polkit rule that lets the deploy user restart the service without sudo, then enable --now.
# Idempotent: rerun it to move to a new binary, to pick up a changed port in the env file, or to turn
# auto-deploy on or off. An existing env file is kept as it is.
#
# Usage: sudo /abs/path/install.sh [DEPLOY_DIR]
#   DEPLOY_DIR: argument, else STEADY_DEPLOY_DIR, else <home of SUDO_USER>/steady-deploy.
#   It holds this kit and bin/steady (linux/amd64).
# Through `sudo env NAME=value ... install.sh`: AUTO_DEPLOY=on installs steady-deploy.timer
# (autodeploy.sh, it needs DEPLOY_DIR/allowed_signers, git, curl, jq and go), AUTO_DEPLOY=off removes
# it, unset keeps it as it is.
set -euo pipefail

fail() {
  echo "install.sh: $*" >&2
  exit 1
}
[[ $EUID -eq 0 ]] || fail "run as root: sudo $0"
umask 022

deploy_dir="${1:-${STEADY_DEPLOY_DIR:-}}"
if [[ -z $deploy_dir ]]; then
  [[ -n ${SUDO_USER:-} && $SUDO_USER != root ]] || fail "pass the deploy dir: sudo $0 /home/<user>/steady-deploy"
  user_home="$(getent passwd "$SUDO_USER" | cut -d: -f6)"
  [[ -n $user_home ]] || fail "no home directory for $SUDO_USER"
  deploy_dir="$user_home/steady-deploy"
fi
deploy_dir="$(realpath -e -- "$deploy_dir")" || fail "deploy dir not found: $deploy_dir"
# The path lands verbatim in the unit file.
[[ $deploy_dir =~ ^/[A-Za-z0-9._/-]+$ ]] || fail "deploy dir path may hold only letters, digits and . _ - /: $deploy_dir"
deploy_user="$(stat -c %U "$deploy_dir")"

svc=steady
home=/var/lib/steady
bin_src="$deploy_dir/bin/steady"
bin_dst=/usr/local/bin/steady
unit_src="$deploy_dir/systemd/steady.service"
unit_dst=/etc/systemd/system/steady.service
env_dir=/etc/steady
env_file="$env_dir/env"
polkit_rule=/etc/polkit-1/rules.d/50-steady.rules
auto_deploy="${AUTO_DEPLOY:-}"
deploy_svc=steady-deploy
deploy_script="$deploy_dir/autodeploy.sh"
deploy_signers="$deploy_dir/allowed_signers"
deploy_unit_dst="/etc/systemd/system/$deploy_svc.service"
deploy_timer_dst="/etc/systemd/system/$deploy_svc.timer"

for f in "$bin_src" "$unit_src"; do
  [[ -f $f ]] || fail "missing $f"
done
[[ -z $auto_deploy || $auto_deploy == on || $auto_deploy == off ]] || fail "AUTO_DEPLOY must be on or off: $auto_deploy"
if [[ -z $auto_deploy && -f $deploy_unit_dst ]]; then
  auto_deploy=keep
fi

# Prints the value of NAME in the env file, nothing when the line is absent.
env_value() {
  local line
  while IFS= read -r line || [[ -n $line ]]; do
    if [[ $line == "$1="* ]]; then
      printf '%s' "${line#*=}"
      return
    fi
  done <"$env_file"
}

# Root reads a file in a directory the deploy user owns: refuse links.
check_upload() {
  local owner links
  [[ -f $1 && ! -L $1 ]] || fail "$1 must be a regular file"
  read -r owner links < <(stat -c '%U %h' "$1")
  [[ $owner == "$deploy_user" || $owner == root ]] || fail "$1 is owned by $owner"
  [[ $links -eq 1 ]] || fail "$1 has more than one hard link"
}

# Before any change: everything autodeploy.sh needs on the host.
if [[ $auto_deploy == on || $auto_deploy == keep ]]; then
  for f in "$deploy_script" "$deploy_dir/systemd/$deploy_svc.service" "$deploy_dir/systemd/$deploy_svc.timer"; do
    [[ -f $f ]] || fail "missing $f"
  done
  check_upload "$deploy_signers"
  [[ -s $deploy_signers ]] || fail "$deploy_signers is empty: it lists the keys whose signed commits may deploy"
  for c in git curl jq go ssh-keygen; do
    command -v "$c" >/dev/null 2>&1 || fail "auto-deploy needs $c on the host"
  done
  # The name lands verbatim in the unit file.
  [[ $deploy_user =~ ^[a-z_][a-z0-9_-]*$ ]] || fail "the user name $deploy_user does not fit a unit file safely"
fi

echo "== user $svc"
if ! id -u "$svc" >/dev/null 2>&1; then
  useradd --system --user-group --home-dir "$home" --no-create-home \
    --shell /usr/sbin/nologin --comment "steady service" "$svc"
fi
install -d -o "$svc" -g "$svc" -m 0700 "$home"

echo "== binary $bin_dst"
install -o root -g root -m 0755 "$bin_src" "$bin_dst.new"
mv -f "$bin_dst.new" "$bin_dst"

echo "== env $env_file"
install -d -o root -g "$svc" -m 0750 "$env_dir"
if [[ ! -f $env_file ]]; then
  tmp="$(mktemp "$env_dir/.env.XXXXXX")"
  chown root:"$svc" "$tmp"
  chmod 0640 "$tmp"
  cat >"$tmp" <<EOF
# steady service environment (systemd EnvironmentFile). Edit: sudoedit $env_file
STEADY_ADDR=127.0.0.1:8770
STEADY_DB=$home/steady.db
STEADY_LOG_LEVEL=info
# Extra Host values for a reverse proxy, comma-separated:
# STEADY_ALLOWED_HOSTS=habits.example.com
EOF
  mv -f "$tmp" "$env_file"
fi
# The service falls back to its default address when the line is absent.
addr="$(env_value STEADY_ADDR)"
addr="${addr:-127.0.0.1:8770}"
port="${addr##*:}"
[[ $port =~ ^[0-9]+$ ]] || fail "STEADY_ADDR in $env_file has no numeric port: $addr"
api="http://127.0.0.1:$port"

echo "== unit $unit_dst"
unit="$(<"$unit_src")"
tmp="$(mktemp /etc/systemd/system/.steady.XXXXXX)"
printf '%s\n' "${unit//@DEPLOY_DIR@/"$deploy_dir"}" >"$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "$unit_dst"
systemctl daemon-reload
systemd-analyze verify "$unit_dst" || echo "install.sh: systemd-analyze reported the issues above" >&2

echo "== polkit rule $polkit_rule"
# Lets the deploy user restart the service after a binary swap without sudo. Nothing else: one
# user, one unit, three verbs.
manager="${SUDO_USER:-}"
[[ -n $manager && $manager != root ]] || manager="$deploy_user"
pk_version="$(pkaction --version 2>/dev/null | awk '{print $NF}' || true)"
pk_minor="${pk_version#0.}"
pk_minor="${pk_minor%%.*}"
if [[ $manager == root ]]; then
  polkit_state="skipped: no deploy user (run install.sh through sudo as that user)"
elif [[ ! $manager =~ ^[a-z_][a-z0-9_-]*$ ]]; then
  polkit_state="skipped: the user name $manager does not fit a polkit rule safely"
elif [[ -z $pk_version ]]; then
  polkit_state="skipped: polkit is not installed, updates need sudo systemctl restart $svc"
elif [[ $pk_version == 0.* && (! $pk_minor =~ ^[0-9]+$ || $pk_minor -lt 106) ]]; then
  polkit_state="skipped: polkit $pk_version reads no JavaScript rules, updates need sudo systemctl restart $svc"
else
  [[ -d ${polkit_rule%/*} ]] || install -d -o root -g root -m 0755 "${polkit_rule%/*}"
  tmp="$(mktemp "${polkit_rule%/*}/.steady.XXXXXX")"
  cat >"$tmp" <<EOF
// Installed by steady's install.sh: $manager may start, stop and restart $svc.service
// without sudo, so a binary update needs no password. Nothing else is granted.
polkit.addRule(function (action, subject) {
  if (action.id === "org.freedesktop.systemd1.manage-units" &&
      action.lookup("unit") === "$svc.service" &&
      subject.user === "$manager") {
    var verb = action.lookup("verb");
    if (verb === "start" || verb === "stop" || verb === "restart") {
      return polkit.Result.YES;
    }
  }
});
EOF
  chmod 0644 "$tmp"
  if cmp -s "$tmp" "$polkit_rule"; then
    rm -f "$tmp"
    polkit_state="installed for $manager, unchanged"
  else
    mv -f "$tmp" "$polkit_rule"
    polkit_state="installed for $manager (polkit $pk_version)"
  fi
fi
echo "$polkit_state"

echo "== auto-deploy $deploy_svc.timer"
case $auto_deploy in
on | keep)
  # autodeploy.sh restarts the service as the deploy user, through the rule above.
  [[ $polkit_state == installed* && $manager == "$deploy_user" ]] ||
    fail "auto-deploy needs the polkit rule for $deploy_user (owner of $deploy_dir): $polkit_state"
  for f in service timer; do
    unit="$(<"$deploy_dir/systemd/$deploy_svc.$f")"
    unit="${unit//@DEPLOY_DIR@/"$deploy_dir"}"
    unit="${unit//@DEPLOY_USER@/"$deploy_user"}"
    tmp="$(mktemp /etc/systemd/system/.steady.XXXXXX)"
    printf '%s\n' "${unit//@PORT@/"$port"}" >"$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "/etc/systemd/system/$deploy_svc.$f"
  done
  systemctl daemon-reload
  systemd-analyze verify "$deploy_unit_dst" "$deploy_timer_dst" ||
    echo "install.sh: systemd-analyze reported the issues above" >&2
  [[ $auto_deploy == keep ]] || systemctl enable --now --quiet "$deploy_svc.timer"
  auto_deploy_state="timer $(systemctl is-enabled "$deploy_svc.timer" 2>/dev/null || true), deploys as $deploy_user"
  ;;
off)
  if [[ -f $deploy_timer_dst ]]; then
    systemctl disable --now --quiet "$deploy_svc.timer" || true
  fi
  rm -f "$deploy_unit_dst" "$deploy_timer_dst"
  systemctl daemon-reload
  auto_deploy_state="off"
  ;;
*) auto_deploy_state="off" ;;
esac
echo "$auto_deploy_state"

# Progress the deploy user can read (the env file is root:steady 0640). No secrets in it.
# mktemp + mv: a link planted in the deploy dir gets replaced, never written through.
tmp="$(mktemp "$deploy_dir/.install-state.XXXXXX")"
printf '# written by install.sh, no secrets\npolkit=%s\nautodeploy=%s\n' "$polkit_state" "$auto_deploy_state" >"$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "$deploy_dir/install-state"

echo "== start $svc"
systemctl enable --quiet "$svc"
if systemctl is-active --quiet "$svc"; then
  systemctl restart "$svc"
else
  systemctl start "$svc"
fi
health=""
for _ in $(seq 1 30); do
  if health="$(curl -fsS -m 2 "$api/healthz" 2>/dev/null)"; then
    break
  fi
  health=""
  sleep 1
done
[[ -n $health ]] || fail "$api/healthz gave no answer in 30s: journalctl -u $svc -n 50"

echo
echo "service: $(systemctl is-active "$svc"), listening on $api"
echo "healthz: ${health:0:200}"
