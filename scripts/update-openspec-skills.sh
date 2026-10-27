#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

if [[ "${OPENSPEC_SKIP_UPGRADE:-0}" != 1 ]]; then
  mise upgrade --local --yes 'npm:@fission-ai/openspec'
fi

run_openspec() {
  mise exec 'npm:@fission-ai/openspec' -- openspec "$@"
}

tmp_dir=$(mktemp -d)
staging_dir=""
backup_dir=""
lock_dir=".agents/.openspec-update.lock"
lock_acquired=0
cleanup() {
  local status=$1
  trap - INT TERM

  if [[ -n "$backup_dir" && -d "$backup_dir" && ! -d .agents/skills ]]; then
    if ! mv "$backup_dir" .agents/skills; then
      echo "failed to restore .agents/skills; recovery files remain in $backup_dir" >&2
      rm -rf "$tmp_dir"
      return "$status"
    fi
  fi

  if [[ -d .agents/skills ]]; then
    if [[ -n "$backup_dir" ]]; then
      rm -rf "$backup_dir"
    fi
    if [[ -n "$staging_dir" ]]; then
      rm -rf "$staging_dir"
    fi
    if ((lock_acquired)); then
      rm -rf "$lock_dir"
    fi
  elif [[ -n "$staging_dir" ]]; then
    echo "missing .agents/skills; recovery files remain in $staging_dir" >&2
  fi

  rm -rf "$tmp_dir"
  return "$status"
}
trap 'cleanup $?' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if ! mkdir "$lock_dir"; then
  echo "another OpenSpec skill update is already running: $lock_dir" >&2
  exit 1
fi
lock_acquired=1
printf '%s\n' "$$" > "$lock_dir/pid"

project_dir="$tmp_dir/project"
config_dir="$tmp_dir/config"
home_dir="$tmp_dir/home"
mkdir -p "$project_dir/openspec" "$project_dir/.pi/skills" "$config_dir/openspec" "$home_dir"
cp openspec/config.yaml "$project_dir/openspec/config.yaml"

shopt -s nullglob
current_skills=(.agents/skills/openspec-*)
if ((${#current_skills[@]} == 0)); then
  echo "no centralized OpenSpec skills found in .agents/skills" >&2
  exit 1
fi
cp -R "${current_skills[@]}" "$project_dir/.pi/skills/"

cat > "$config_dir/openspec/config.json" <<'JSON'
{
  "profile": "custom",
  "delivery": "skills",
  "workflows": ["propose", "explore", "apply", "sync", "archive", "verify"]
}
JSON

(
  cd "$project_dir"
  mise exec 'npm:@fission-ai/openspec' -- env \
    HOME="$home_dir" \
    CODEX_HOME="$home_dir/.codex" \
    XDG_CONFIG_HOME="$config_dir" \
    XDG_DATA_HOME="$home_dir/.local/share" \
    OPENSPEC_TELEMETRY=0 \
    openspec update --force
)

version=$(run_openspec --version)
python3 - "$project_dir/.pi/skills" <<'PY'
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])

def workflow_reference(match):
    name, arguments = match.groups()
    reference = f"the `{name}` workflow"
    if arguments:
        reference += f" with `{arguments}`"
    return reference

for path in sorted(root.glob("openspec-*/SKILL.md")):
    content = path.read_text()
    content = re.sub(r"`/(openspec-[a-z-]+)(?: ([^`]+))?`", workflow_reference, content)
    content = content.replace(
        "User: /openspec-explore add-auth-system",
        "User: Explore the OpenSpec change add-auth-system",
    )
    path.write_text(content)
PY

expected_skills=(
  openspec-apply-change
  openspec-archive-change
  openspec-explore
  openspec-propose
  openspec-sync-specs
  openspec-verify-change
)
generated_skills=("$project_dir"/.pi/skills/openspec-*)
if ((${#generated_skills[@]} != ${#expected_skills[@]})); then
  echo "expected ${#expected_skills[@]} generated OpenSpec skills, found ${#generated_skills[@]}" >&2
  exit 1
fi
for skill in "${expected_skills[@]}"; do
  generated_skill="$project_dir/.pi/skills/$skill/SKILL.md"
  if [[ ! -f "$generated_skill" ]]; then
    echo "expected generated skill $skill/SKILL.md" >&2
    exit 1
  fi
  if ! grep -Fq "generatedBy: \"$version\"" "$generated_skill"; then
    echo "generated skill $skill does not identify OpenSpec $version" >&2
    exit 1
  fi
done
if grep -R -n -E '/(opsx|openspec-)' "$project_dir/.pi/skills"; then
  echo "generated OpenSpec skills contain tool-specific command references" >&2
  exit 1
fi

claude_skills=(.claude/skills/openspec-*)
if ((${#claude_skills[@]} != ${#expected_skills[@]})); then
  echo "expected ${#expected_skills[@]} Claude skill adapters, found ${#claude_skills[@]}" >&2
  exit 1
fi
for skill in "${expected_skills[@]}"; do
  adapter=".claude/skills/$skill"
  expected_target="../../.agents/skills/$skill"
  if [[ ! -L "$adapter" || "$(readlink "$adapter")" != "$expected_target" ]]; then
    echo "Claude skill adapter $adapter must point to $expected_target" >&2
    exit 1
  fi
done

run_openspec validate --all --strict

staging_dir=$(mktemp -d .agents/.skills-update.XXXXXX)
new_skills_dir="$staging_dir/skills"
cp -R .agents/skills "$new_skills_dir"
rm -rf "$new_skills_dir"/openspec-*
cp -R "${generated_skills[@]}" "$new_skills_dir/"

staged_skills=("$new_skills_dir"/openspec-*)
if ((${#staged_skills[@]} != ${#expected_skills[@]})); then
  echo "staged OpenSpec skill set is incomplete" >&2
  exit 1
fi
for skill in "${expected_skills[@]}"; do
  if [[ ! -f "$new_skills_dir/$skill/SKILL.md" ]]; then
    echo "staged skill $skill/SKILL.md is missing" >&2
    exit 1
  fi
done

backup_dir="$staging_dir.backup"
mv .agents/skills "$backup_dir"
mv "$new_skills_dir" .agents/skills

printf 'Centralized OpenSpec skills updated with OpenSpec %s.\n' "$version"
