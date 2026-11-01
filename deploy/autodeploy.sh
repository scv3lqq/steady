#!/usr/bin/env bash
# Pull-based deploy of main, run by steady-deploy.timer as the deploy user. Each run fetches the public
# repo over HTTPS (no credentials anywhere) and ships the head of main only when its commit is signed
# by a key in DIR/allowed_signers, descends from the deployed commit, its CI checks passed and the kit
# in DIR matches the commit's deploy/. A build that matches the running binary is recorded without a
# restart. A binary that gives no answer on /healthz after the restart is rolled back. A changed kit
# (install.sh, autodeploy.sh, the units) still goes through the full server setup.
#
# Usage: DIR/autodeploy.sh DIR
#   The unit sets STEADY_PORT. AUTO_DEPLOY_REPO (a GitHub HTTPS URL) and AUTO_DEPLOY_BRANCH
#   point it at a fork; AUTO_DEPLOY_API overrides the GitHub API URL of the repo.
set -euo pipefail

fail() {
  echo "autodeploy.sh: $*" >&2
  exit 1
}

dir="$(realpath -e -- "${1:?usage: autodeploy.sh DEPLOY_DIR}")"
repo="${AUTO_DEPLOY_REPO:-https://github.com/scv3lqq/steady.git}"
branch="${AUTO_DEPLOY_BRANCH:-main}"
port="${STEADY_PORT:-8770}"
# The names of the jobs in .github/workflows/ci.yml.
checks=(test secrets)
svc=steady
src="$dir/src"
signers="$dir/allowed_signers"
state="$dir/autodeploy-state"
staged="$dir/bin/steady"
next="$dir/bin/steady.next"
prev="$dir/bin/steady.prev"
installed=/usr/local/bin/steady
api="http://127.0.0.1:$port"

if [[ -n ${AUTO_DEPLOY_API:-} ]]; then
  gh_api="$AUTO_DEPLOY_API"
else
  slug="${repo#https://github.com/}"
  slug="${slug%.git}"
  [[ $slug =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail "not a GitHub HTTPS URL, set AUTO_DEPLOY_API: $repo"
  gh_api="https://api.github.com/repos/$slug"
fi
[[ -s $signers ]] || fail "missing $signers: the keys whose signed commits may deploy"
[[ $port =~ ^[0-9]+$ ]] || fail "bad STEADY_PORT: $port"

state_get() {
  [[ -f $state ]] || return 0
  sed -n "s/^$1=//p" "$state" | head -n 1
}

state_set() {
  local tmp
  tmp="$(mktemp "$dir/.autodeploy-state.XXXXXX")"
  { [[ ! -f $state ]] || grep -v "^$1=" "$state" || true; } >"$tmp"
  printf '%s=%s\n' "$1" "$2" >>"$tmp"
  mv -f "$tmp" "$state"
}

# A deployed head makes older holds and rejections history: only deployed= stays.
mark_deployed() {
  local tmp
  tmp="$(mktemp "$dir/.autodeploy-state.XXXXXX")"
  printf 'deployed=%s\n' "$head" >"$tmp"
  mv -f "$tmp" "$state"
}

# The head waits for something that may change (CI, the kit, the build): said once per commit and
# reason, not every five minutes. Later runs try again.
hold() {
  local note="$head $1"
  if [[ $(state_get held) != "$note" ]]; then
    echo "holding ${head:0:12}: $1"
    [[ -z ${2:-} ]] || printf '%s\n' "$2"
    state_set held "$note"
  fi
  exit 0
}

# The head can never deploy: later runs skip it until main moves on.
reject() {
  echo "rejected ${head:0:12}: $1" >&2
  state_set rejected "$head"
  exit 1
}

answers() {
  local _
  for _ in $(seq 1 30); do
    if curl -fsS -m 2 "$api/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

# Only the deploy user's settings could steer git and go here, and neither should.
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0
export GOENV=off GOTOOLCHAIN=auto GOFLAGS=-mod=readonly GOPATH="$dir/.go" GOCACHE="$dir/.cache/go-build"

if [[ ! -d $src/.git ]]; then
  rm -rf "$src"
  git clone --quiet --no-checkout --single-branch --branch "$branch" -- "$repo" "$src"
fi
git -C "$src" remote set-url origin "$repo"
git -C "$src" fetch --quiet --prune origin "+refs/heads/$branch:refs/remotes/origin/$branch"
head="$(git -C "$src" rev-parse --verify "refs/remotes/origin/$branch^{commit}")"
[[ $head != "$(state_get deployed)" && $head != "$(state_get rejected)" ]] || exit 0

# The signature covers the tree the build uses, so the keys in DIR decide what runs, not GitHub.
if ! out="$(git -C "$src" -c gpg.ssh.allowedSignersFile="$signers" verify-commit "$head" 2>&1)"; then
  printf '%s\n' "$out" >&2
  reject "not signed by a key in $signers"
fi
# An older signed commit is signed too: a force push must not roll the server back unasked.
deployed="$(state_get deployed)"
if [[ -n $deployed ]] && ! git -C "$src" merge-base --is-ancestor "$deployed" "$head" 2>/dev/null; then
  hold "not a descendant of the deployed ${deployed:0:12} (history rewritten?): to ship it anyway, delete the deployed= line in $state"
fi

runs="$(curl -fsS -m 20 -H 'Accept: application/vnd.github+json' "$gh_api/commits/$head/check-runs?per_page=100" 2>/dev/null)" ||
  hold "no answer from the GitHub API"
for name in "${checks[@]}"; do
  # A rerun adds a check run with the same name: the newest one counts.
  result="$(jq -r --arg n "$name" '[.check_runs[] | select(.name == $n)] | if length == 0 then "missing"
    else max_by(.id) | if .status != "completed" then "pending" else .conclusion end end' <<<"$runs" 2>/dev/null)" ||
    hold "no check runs in the GitHub API answer"
  case $result in
  success) ;;
  missing | pending) hold "CI check $name is $result" ;;
  # Not a rejection: a rerun of the check may still pass.
  *) hold "CI check $name: $result" ;;
  esac
done

git -C "$src" checkout --quiet --force --detach "$head"
changed=()
while IFS= read -r f; do
  cmp -s "$src/$f" "$dir/${f#deploy/}" || changed+=("${f#deploy/}")
done < <(git -C "$src" ls-files -- deploy ':!deploy/README.md')
((${#changed[@]} == 0)) || hold "the kit changed (${changed[*]}): run the full server setup"

mkdir -p "$dir/bin"
# The flags of `make build-linux`, for this host: a commit that leaves the code alone builds the same
# bytes, so it needs no restart. web/dist is committed, so the Go toolchain is all the build needs.
if ! out="$(cd "$src" && CGO_ENABLED=0 \
  go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$next" ./cmd/steady 2>&1)"; then
  rm -f "$next"
  hold "the build failed" "$(tail -n 20 <<<"$out")"
fi
want="$(sha256sum "$next" | cut -d' ' -f1)"
have="$(sha256sum "$installed" 2>/dev/null | cut -d' ' -f1 || true)"
if [[ $want == "$have" ]]; then
  mv -f "$next" "$staged"
  mark_deployed
  echo "${head:0:12}: the binary is unchanged, no restart"
  exit 0
fi

[[ ! -f $installed ]] || cp -f "$installed" "$prev"
mv -f "$next" "$staged"
if ! out="$(systemctl --no-ask-password restart "$svc" 2>&1)"; then
  [[ ! -f $prev ]] || mv -f "$prev" "$staged"
  hold "the restart was refused (the polkit rule from install.sh?)" "$out"
fi
if answers; then
  mark_deployed
  echo "deployed ${head:0:12} ($(git -C "$src" log -1 --format=%s "$head"))"
  exit 0
fi
echo "${head:0:12} gave no answer on $api/healthz in 30s: rolling back" >&2
if [[ -f $prev ]]; then
  mv -f "$prev" "$staged"
  systemctl --no-ask-password restart "$svc" || true
  answers || echo "the previous binary gives no answer either: journalctl -u $svc" >&2
fi
reject "no answer after the restart"
