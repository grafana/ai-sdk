#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
isolated=$(mktemp -d)
trap 'rm -rf "$isolated"' EXIT
rsync -a --exclude='/.git' --exclude='/ai-gateway' --exclude='**/node_modules' "$repo_root/" "$isolated/"
if [[ -e "$isolated/ai-gateway" ]]; then
  echo "isolated SDK copy contains AI Gateway source" >&2
  exit 1
fi

readonly_flags="${GOFLAGS:+$GOFLAGS }-mod=readonly"
for module in . providers/grafana; do
  echo "==> $module without AI Gateway source"
  (
    cd "$isolated/$module"
    GOWORK=off GOFLAGS="$readonly_flags" go build ./...
    GOWORK=off GOFLAGS="$readonly_flags" go test ./...
  )
done
