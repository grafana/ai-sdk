#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

if (( $# > 1 )); then
  echo "usage: $0 [published-module-root-or-path]" >&2
  exit 1
fi

selection=()
if (( $# == 1 )); then
  selection=("$1")
fi

export GOPROXY=https://proxy.golang.org
export GOPRIVATE=
export GONOPROXY=
export GONOSUMDB=
export GOWORK=off
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=readonly"

module_list=$(go run ./cmd/modulecheck modules "${selection[@]}")
module_cache=$(mktemp -d)
trap 'chmod -R u+w "$module_cache" 2>/dev/null || true; rm -rf "$module_cache"' EXIT
export GOMODCACHE="$module_cache"

while IFS=$'\t' read -r root module; do
  [[ -n "$root" && -n "$module" ]] || continue
  echo "==> $root ($module)"
  (
    cd "$root"
    if [[ $(go mod edit -json | jq '(.Replace // []) | length') -ne 0 ]]; then
      echo "$module contains replace directives" >&2
      exit 1
    fi
    go mod download
    go mod verify >/dev/null
    go build ./...
    go test ./...
  )
done <<<"$module_list"
