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
mkdir -p "$project_dir/openspec" "$project_dir/.pi/skills" "$config_dir/openspec"
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
  XDG_CONFIG_HOME="$config_dir" run_openspec update --force
)

version=$(run_openspec --version)
python3 - "$project_dir/.pi/skills" <<'PY'
from pathlib import Path
import sys

root = Path(sys.argv[1])
replacements = {
    "openspec-apply-change/SKILL.md": [
        (
            'Always announce: "Using change: <name>" and how to override (e.g., `/opsx-apply <other>`).',
            'Always announce: "Using change: <name>" and explain that the user can name a different change to override it.',
        ),
    ],
    "openspec-explore/SKILL.md": [
        (
            "User: /opsx-explore add-auth-system",
            "User: Explore the OpenSpec change add-auth-system",
        ),
    ],
    "openspec-propose/SKILL.md": [
        (
            "When ready to implement, run /opsx-apply",
            "When ready to implement, invoke the OpenSpec apply-change workflow or ask me to implement.",
        ),
        (
            '- Prompt: "Run `/opsx-apply` or ask me to implement to start working on the tasks."',
            '- Prompt: "Invoke the OpenSpec apply-change workflow or ask me to implement to start working on the tasks."',
        ),
    ],
}
for relative_path, path_replacements in replacements.items():
    path = root / relative_path
    content = path.read_text()
    for old, new in path_replacements:
        count = content.count(old)
        if count != 1:
            raise SystemExit(
                f"expected generated text exactly once in {relative_path}, found {count}: {old}"
            )
        content = content.replace(old, new, 1)
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
if grep -R -n -F '/opsx' "$project_dir/.pi/skills"; then
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
