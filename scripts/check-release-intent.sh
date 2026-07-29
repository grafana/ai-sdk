#!/usr/bin/env bash
set -euo pipefail

: "${BASE_SHA:?BASE_SHA is required}"
: "${HEAD_SHA:?HEAD_SHA is required}"

has_public_change=0
has_release_fragment=0

while IFS= read -r path; do
  case "$path" in
    .changes/*.md)
      has_release_fragment=1
      ;;
    *_test.go | internal/* | test/* | examples/* | scripts/*)
      ;;
    *.go)
      has_public_change=1
      ;;
  esac
done < <(git diff --name-only "$BASE_SHA" "$HEAD_SHA")

if ((has_public_change == 0)); then
  echo "No public non-test Go changes; no release decision is required."
  exit 0
fi

if ((has_release_fragment == 1)); then
  echo "Release fragment found."
  exit 0
fi

if [[ "${HAS_RELEASE_NONE:-false}" != "true" ]]; then
  echo "Public Go changes require a .changes/*.md fragment or the release:none label." >&2
  exit 1
fi

rationale=$(
  printf '%s\n' "${PR_BODY:-}" |
    sed -n 's/^Release-none rationale:[[:space:]]*//p' |
    head -n 1
)

if [[ -z "${rationale//[[:space:]]/}" ]]; then
  echo "The release:none label requires text after 'Release-none rationale:' in the PR body." >&2
  exit 1
fi

echo "release:none accepted: $rationale"
