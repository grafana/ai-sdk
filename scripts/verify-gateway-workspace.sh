#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
workspace="$repo_root/go.gateway.work"
cd "$repo_root"

if go work edit -json go.work | jq -e '.Use[]? | select(.DiskPath == "./ai-gateway")' >/dev/null; then
  echo "root go.work must exclude AI Gateway" >&2
  exit 1
fi

workspace_modules=$(go work edit -json "$workspace" | jq -r '.Use[].DiskPath' | while IFS= read -r root; do
  module=$(GOWORK=off go mod edit -json "$root/go.mod" | jq -r '.Module.Path')
  printf '%s\t%s\n' "$root" "$module"
done)
expected_modules=$(go run ./cmd/modulecheck modules | cut -f2 | sort)
actual_modules=$(cut -f2 <<<"$workspace_modules" | sort)
if [[ "$actual_modules" != "$expected_modules" ]]; then
  echo "Gateway workspace must contain exactly the published SDK and Gateway modules" >&2
  exit 1
fi

while IFS=$'\t' read -r root module; do
  [[ -n "$root" && -n "$module" ]] || continue
  selected=$(cd ai-gateway && GOWORK="$workspace" GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=readonly" go list -m -json "$module")
  expected=$(cd "$root" && pwd -P)
  if [[ $(jq -r '.Main' <<<"$selected") != true || $(jq -r '.Dir' <<<"$selected") != "$expected" ]]; then
    echo "Gateway workspace did not select local $module ($expected)" >&2
    exit 1
  fi
done <<<"$workspace_modules"
