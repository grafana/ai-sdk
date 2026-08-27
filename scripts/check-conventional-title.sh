#!/usr/bin/env bash
# Validate that a pull request title is a Conventional Commit. Pull requests
# are squash-merged with the title as the subject, and release-please derives
# versions and changelog entries from that subject, so an unparsable title
# silently drops the change from its release.
set -euo pipefail

: "${PR_TITLE:?PR_TITLE is required}"

types="feat|fix|perf|revert|deps|docs|chore|refactor|test|build|ci|style"
subject_pattern="^(${types})(\([^)]+\))?!?: .+"

if [[ "$PR_TITLE" =~ $subject_pattern ]]; then
  echo "Pull request title is a Conventional Commit."
  exit 0
fi

echo "::error::Not a Conventional Commit: ${PR_TITLE}"
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
