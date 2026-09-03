#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

gateway_module="github.com/grafana/ai-sdk/ai-gateway"
gateway_root=$(cd ai-gateway && pwd -P)
readonly_flags="${GOFLAGS:+$GOFLAGS }-mod=readonly"

replacement_targets_gateway() {
  local base=$1
  local target=$2
  local candidate
  case "$target" in
    /*) candidate=$target ;;
    ./*|../*) candidate="$base/$target" ;;
    *) return 1 ;;
  esac

  local resolved
  resolved=$(cd "$candidate" 2>/dev/null && pwd -P) || return 1
  [[ "$resolved" == "$gateway_root" || "$resolved" == "$gateway_root/"* ]]
}

require_license() {
  local path=$1
  local marker=$2
  if [[ ! -s "$path" ]] || ! grep -Fq "$marker" "$path"; then
    echo "$path is missing the expected license text" >&2
    exit 1
  fi
}

require_license LICENSE "Apache License"
require_license ai-gateway/LICENSE "GNU AFFERO GENERAL PUBLIC LICENSE"

actual_module=$(GOWORK=off go mod edit -json ai-gateway/go.mod | jq -er '.Module.Path')
if [[ "$actual_module" != "$gateway_module" ]]; then
  echo "ai-gateway/go.mod declares $actual_module, expected $gateway_module" >&2
  exit 1
fi

workspace_json=$(go work edit -json)
while IFS= read -r workspace_path; do
  workspace_go_mod="$workspace_path/go.mod"
  if [[ ! -f "$workspace_go_mod" ]]; then
    echo "go.work entry $workspace_path has no go.mod" >&2
    exit 1
  fi
  workspace_module=$(GOWORK=off go mod edit -json "$workspace_go_mod" | jq -er '.Module.Path')
  if [[ "$workspace_module" == "$gateway_module" || "$workspace_module" == "$gateway_module/"* ]]; then
    echo "ai-gateway modules must not be registered in go.work" >&2
    exit 1
  fi
done < <(jq -r '.Use[]?.DiskPath' <<<"$workspace_json")

if jq -e --arg module "$gateway_module" '
  [(.Replace // [])[]?.Old.Path, (.Replace // [])[]?.New.Path]
  | map(select(type == "string"))
  | any(. == $module or startswith($module + "/"))
' <<<"$workspace_json" >/dev/null; then
  echo "go.work must not replace the AI Gateway module" >&2
  exit 1
fi
while IFS= read -r target; do
  if replacement_targets_gateway "$repo_root" "$target"; then
    echo "go.work must not replace a module with AI Gateway source" >&2
    exit 1
  fi
done < <(jq -r '(.Replace // [])[]?.New.Path // empty' <<<"$workspace_json")

while IFS= read -r go_mod; do
  if [[ "$go_mod" == ./ai-gateway/* ]]; then
    continue
  fi

  module_json=$(GOWORK=off go mod edit -json "$go_mod")
  if jq -e --arg module "$gateway_module" '
    [(.Require // [])[]?.Path, (.Replace // [])[]?.Old.Path, (.Replace // [])[]?.New.Path]
    | map(select(type == "string"))
    | any(. == $module or startswith($module + "/"))
  ' <<<"$module_json" >/dev/null; then
    echo "${go_mod#./} requires or replaces the AI Gateway module" >&2
    exit 1
  fi
  while IFS= read -r target; do
    if replacement_targets_gateway "$(dirname "$go_mod")" "$target"; then
      echo "${go_mod#./} replaces a module with AI Gateway source" >&2
      exit 1
    fi
  done < <(jq -r '(.Replace // [])[]?.New.Path // empty' <<<"$module_json")
done < <(find . -name go.mod ! -path './.git/*' ! -path '*/node_modules/*' | sort)

while IFS= read -r source; do
  if grep -nF "$gateway_module" "$source"; then
    echo "Go source outside ai-gateway imports the AI Gateway module" >&2
    exit 1
  fi
done < <(find . -type f -name '*.go' ! -path './.git/*' ! -path './ai-gateway/*' ! -path '*/node_modules/*' ! -path '*/vendor/*' | sort)

while read -r module _; do
  if [[ "$module" == "$gateway_module" || "$module" == "$gateway_module/"* ]]; then
    echo "the root module graph contains the AI Gateway module" >&2
    exit 1
  fi
done < <(GOWORK=off GOFLAGS="$readonly_flags" go list -m all)

GOWORK=off GOFLAGS="$readonly_flags" go build ./...
GOWORK=off GOFLAGS="$readonly_flags" go test ./...

echo "AI Gateway module and license boundary: OK"
