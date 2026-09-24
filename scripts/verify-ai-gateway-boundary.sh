#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

gateway_module=github.com/grafana/ai-sdk/ai-gateway
gateway_root=$(cd ai-gateway && pwd -P)
readonly_flags="${GOFLAGS:+$GOFLAGS }-mod=readonly"

for license in 'LICENSE:Apache License' 'ai-gateway/LICENSE:GNU AFFERO GENERAL PUBLIC LICENSE'; do
  path=${license%%:*}
  marker=${license#*:}
  if [[ ! -s "$path" ]] || ! grep -Fq "$marker" "$path"; then
    echo "$path is missing the expected license text" >&2
    exit 1
  fi
done

actual_module=$(GOWORK=off go mod edit -json ai-gateway/go.mod | jq -er '.Module.Path')
if [[ "$actual_module" != "$gateway_module" ]]; then
  echo "ai-gateway/go.mod declares $actual_module, expected $gateway_module" >&2
  exit 1
fi
if [[ -e gateway/catalog || -e gateway/providerwire ]]; then
  echo "Gateway implementation must live below ai-gateway" >&2
  exit 1
fi

workspace_json=$(go work edit -json go.work)
if [[ $(jq '(.Replace // []) | length' <<<"$workspace_json") -ne 0 ]]; then
  echo "root go.work must not contain replace directives" >&2
  exit 1
fi
workspace_paths=$(jq -r '.Use[]?.DiskPath' <<<"$workspace_json")
while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  module=$(GOWORK=off go mod edit -json "$path/go.mod" | jq -er '.Module.Path')
  if [[ "$module" == "$gateway_module" || "$module" == "$gateway_module/"* ]]; then
    echo "Gateway modules must not be registered in root go.work" >&2
    exit 1
  fi
done <<<"$workspace_paths"

module_list=$(GOWORK=off go run ./cmd/modulecheck all-modules)
while IFS=$'\t' read -r root module; do
  [[ -n "$root" && -n "$module" ]] || continue
  if [[ "$module" == "$gateway_module" || "$module" == "$gateway_module/"* ]]; then
    continue
  fi
  manifest=$(GOWORK=off go mod edit -json "$root/go.mod")
  references=$(jq -r --arg gateway "$gateway_module" '
    [(.Require // [])[]?.Path, (.Replace // [])[]?.Old.Path, (.Replace // [])[]?.New.Path]
    | map(select(type == "string" and (. == $gateway or startswith($gateway + "/"))))
    | .[]
  ' <<<"$manifest")
  if [[ -n "$references" ]]; then
    echo "$module requires or replaces the Gateway module: $references" >&2
    exit 1
  fi
  targets=$(jq -r '(.Replace // [])[]?.New.Path // empty' <<<"$manifest")
  while IFS= read -r target; do
    case "$target" in
      /*) candidate=$target ;;
      ./*|../*) candidate="$root/$target" ;;
      *) continue ;;
    esac
    if [[ -d "$candidate" ]]; then
      resolved=$(cd "$candidate" && pwd -P)
      if [[ "$resolved" == "$gateway_root" || "$resolved" == "$gateway_root/"* ]]; then
        echo "$module replaces a module with Gateway source" >&2
        exit 1
      fi
    fi
  done <<<"$targets"
done <<<"$module_list"

if imports=$(git grep -nF "$gateway_module" -- '*.go' ':(exclude)ai-gateway/**'); then
  echo "Go source outside ai-gateway references the Gateway module: $imports" >&2
  exit 1
else
  status=$?
  (( status == 1 )) || exit "$status"
fi

for root in . providers/grafana; do
  graph=$(cd "$root" && GOWORK=off GOFLAGS="$readonly_flags" go list -m all)
  while read -r module _; do
    if [[ "$module" == "$gateway_module" || "$module" == "$gateway_module/"* ]]; then
      echo "$root module graph contains the Gateway module" >&2
      exit 1
    fi
  done <<<"$graph"
done

echo "AI Gateway module and license boundary: OK"
