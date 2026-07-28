#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
fixture_dir="$test_dir/repo"
mkdir -p "$fixture_dir/scripts"
cp "$repo_root/scripts/update-openspec-skills.sh" "$fixture_dir/scripts/"
cp -R "$repo_root/.agents" "$repo_root/.claude" "$repo_root/openspec" "$fixture_dir/"
export MISE_CONFIG_FILE="$repo_root/mise.toml"
cd "$fixture_dir"

snapshot() {
  python3 - <<'PY'
from hashlib import sha256
from pathlib import Path

result = sha256()
for root_name in (".agents/skills", ".claude/skills"):
    root = Path(root_name)
    for path in sorted(root.rglob("*")):
        relative = path.as_posix().encode()
        if path.is_symlink():
            result.update(b"L\0" + relative + b"\0" + path.readlink().as_posix().encode() + b"\0")
        elif path.is_file():
            result.update(b"F\0" + relative + b"\0" + path.read_bytes() + b"\0")
print(result.hexdigest())
PY
}

assert_layout() {
  [[ ! -e .pi ]]
  [[ ! -e .agents/.openspec-update.lock ]]
  shopt -s nullglob
  local leftovers=(.agents/.skills-update.*)
  ((${#leftovers[@]} == 0))
}

run_failure_case() {
  local mode=$1
  local expected_status=$2
  local wrapper_dir
  wrapper_dir=$(mktemp -d)
  local before after status
  before=$(snapshot)

  cat > "$wrapper_dir/mv" <<'SH'
#!/usr/bin/env bash
count=0
if [[ -f "$MV_COUNT_FILE" ]]; then
  count=$(<"$MV_COUNT_FILE")
fi
count=$((count + 1))
printf '%s' "$count" > "$MV_COUNT_FILE"
if ((count == 2)); then
  if [[ "$MV_FAILURE_MODE" == term ]]; then
    kill -TERM "$PPID"
    sleep 0.2
    exit 143
  fi
  exit 99
fi
exec "$REAL_MV" "$@"
SH
  chmod +x "$wrapper_dir/mv"

  set +e
  PATH="$wrapper_dir:$PATH" \
    REAL_MV="$real_mv" \
    MV_COUNT_FILE="$wrapper_dir/count" \
    MV_FAILURE_MODE="$mode" \
    OPENSPEC_SKIP_UPGRADE=1 \
    bash scripts/update-openspec-skills.sh > "$wrapper_dir/output.log" 2>&1
  status=$?
  set -e

  if ((status != expected_status)); then
    cat "$wrapper_dir/output.log" >&2
    echo "expected $mode status $expected_status, got $status" >&2
    exit 1
  fi
  after=$(snapshot)
  if [[ "$before" != "$after" ]]; then
    echo "$mode changed the skill tree despite rollback" >&2
    exit 1
  fi
  assert_layout
  rm -rf "$wrapper_dir"
}

real_mv=$(command -v mv)

before=$(snapshot)
mkdir .agents/.openspec-update.lock
set +e
OPENSPEC_SKIP_UPGRADE=1 bash scripts/update-openspec-skills.sh > "$test_dir/lock.log" 2>&1
lock_status=$?
set -e
rm -rf .agents/.openspec-update.lock
if ((lock_status == 0)) || [[ "$before" != "$(snapshot)" ]]; then
  cat "$test_dir/lock.log" >&2
  echo "concurrent update lock was not enforced" >&2
  exit 1
fi

before=$(snapshot)
OPENSPEC_SKIP_UPGRADE=1 bash scripts/update-openspec-skills.sh
if [[ "$before" != "$(snapshot)" ]]; then
  echo "OpenSpec skill output is stale" >&2
  exit 1
fi
assert_layout

version=$(mise exec 'npm:@fission-ai/openspec' -- openspec --version)
for skill_file in .agents/skills/openspec-*/SKILL.md; do
  grep -Fq "generatedBy: \"$version\"" "$skill_file"
done

run_failure_case fail 99
run_failure_case term 143
printf 'OpenSpec skill updater tests passed.\n'
