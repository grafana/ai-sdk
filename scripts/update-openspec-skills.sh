#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

tool_list=.agents/skills/.openspec-target

if [[ "${OPENSPEC_SKIP_UPGRADE:-0}" != 1 ]]; then
  mise upgrade --local --yes 'npm:@fission-ai/openspec'
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$tmp_dir/config/openspec" "$tmp_dir/home"
cat > "$tmp_dir/config/openspec/config.json" <<'JSON'
{
  "profile": "custom",
  "delivery": "skills",
  "workflows": ["propose", "explore", "apply", "sync", "archive", "verify"]
}
JSON

run_openspec() {
  mise exec 'npm:@fission-ai/openspec' -- env \
    HOME="$tmp_dir/home" \
    CODEX_HOME="$tmp_dir/home/.codex" \
    XDG_CONFIG_HOME="$tmp_dir/config" \
    XDG_DATA_HOME="$tmp_dir/home/.local/share" \
    OPENSPEC_TELEMETRY=0 \
    openspec "$@"
}

if [[ ! -f "$tool_list" ]]; then
  echo "missing $tool_list; set up the project with openspec init --tools claude,codex,pi first" >&2
  exit 1
fi

run_openspec validate --all --strict
run_openspec update --force

printf 'OpenSpec skills updated with OpenSpec %s.\n' "$(run_openspec --version)"
