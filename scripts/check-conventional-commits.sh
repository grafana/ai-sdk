#!/usr/bin/env bash
# Validate that every non-merge commit in a pull request is a Conventional
# Commit. Release versions and changelog entries are derived from these
# messages, so an unparsable subject silently drops a change from its release.
set -euo pipefail

: "${BASE_SHA:?BASE_SHA is required}"
: "${HEAD_SHA:?HEAD_SHA is required}"

types="feat|fix|perf|revert|deps|docs|chore|refactor|test|build|ci|style"
subject_pattern="^(${types})(\([^)]+\))?!?: .+"

failed=0
while IFS= read -r sha; do
  subject=$(git log -1 --format=%s "$sha")
  if [ "$(git rev-list --parents -n 1 "$sha" | wc -w)" -gt 2 ]; then
    continue
  fi
  if [[ ! "$subject" =~ $subject_pattern ]]; then
    echo "::error::Not a Conventional Commit: ${sha} ${subject}"
    failed=1
  fi
done < <(git rev-list "$BASE_SHA..$HEAD_SHA")

if ((failed == 1)); then
  cat >&2 <<'USAGE'

Use "<type>[optional scope][!]: <description>", for example:

  feat(providers/openai): add continuation support
  fix: reject incomplete tool results
  feat!: drop the deprecated stream reader

Release-relevant types: feat (minor), fix and perf (patch), and any type with
"!" or a "BREAKING CHANGE:" footer (major). Everything else is released as a
silent maintenance commit. See release/README.md.
USAGE
  exit 1
fi

echo "All commits are Conventional Commits."
