#!/usr/bin/env bash
set -euo pipefail

modules=$(bash scripts/module-policy.sh all-modules)
export GOWORK=off
while IFS=$'\t' read -r dir module; do
  echo "==> $module"
  (
    cd "$dir"
    go list -mod=readonly -deps -test -tags=conformance,mage,arrow_json_stdlib ./... >/dev/null
    go list -mod=readonly -m all >/dev/null
  )
done <<< "$modules"
for arch in amd64 arm64; do
  echo "==> AI Gateway plugin linux/$arch"
  (
    cd ai-gateway
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go list -mod=readonly -deps -tags=arrow_json_stdlib ./pkg >/dev/null
  )
done
git diff --exit-code -- go.mod go.sum '**/go.mod' '**/go.sum'
