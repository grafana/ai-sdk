#!/usr/bin/env bash
# Repoint every published nested Go module at a released core version.
#
# Runs after the core tag exists so that go.mod and go.sum are updated from the
# module proxy in one step. Writes "changed=true" to GITHUB_OUTPUT when any
# module file was modified.
set -euo pipefail

core_version="${1:?usage: sync-core-requirement.sh <core-version>}"
case "$core_version" in
v*) ;;
*) core_version="v$core_version" ;;
esac

core_module="github.com/grafana/ai-sdk"
export GOWORK=off
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

wait_for_proxy() {
  local attempt
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    if go list -m "${core_module}@${core_version}" >/dev/null 2>&1; then
      return 0
    fi
    echo "waiting for ${core_module}@${core_version} on the module proxy (attempt ${attempt})"
    sleep 15
  done
  echo "${core_module}@${core_version} is not resolvable from the module proxy" >&2
  return 1
}

wait_for_proxy

changed=false
while IFS= read -r go_mod; do
  directory=$(dirname "$go_mod")
  if [ "$directory" = "." ]; then
    continue
  fi

  module_path=$(go mod edit -json "$go_mod" | jq -r '.Module.Path')
  case "$module_path" in
  github.com/grafana/ai-sdk/examples/* | github.com/grafana/ai-sdk/test/*)
    continue
    ;;
  esac

  current=$(go mod edit -json "$go_mod" | jq -r --arg module "$core_module" \
    '(.Require // []) | map(select(.Path == $module)) | (.[0].Version // "")')
  if [ -z "$current" ] || [ "$current" = "$core_version" ]; then
    continue
  fi

  echo "==> ${directory}: ${current} -> ${core_version}"
  (cd "$directory" && go get "${core_module}@${core_version}" && go mod tidy)
  changed=true
done < <(git ls-files 'go.mod' '*/go.mod' | sort)

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "changed=${changed}" >>"$GITHUB_OUTPUT"
fi
echo "changed=${changed}"
