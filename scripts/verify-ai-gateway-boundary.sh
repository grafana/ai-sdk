#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

gateway_module="github.com/grafana/ai-sdk/ai-gateway"
gateway_root=$(realpath ai-gateway)
readonly_flags="${GOFLAGS:+$GOFLAGS }-mod=readonly"

verify_hash() {
  local path=$1
  local expected=$2
  local actual
  actual=$(sha256sum "$path" | cut -d' ' -f1)
  if [[ "$actual" != "$expected" ]]; then
    echo "$path does not match its approved license text" >&2
    exit 1
  fi
}

verify_hash LICENSE efb4d91baa7cd9559d3452f79deb2b3d9dc819c83d7058a1f90ac70484a1a23e
verify_hash ai-gateway/LICENSE 0d96a4ff68ad6d4b6f1f30f713b18d5184912ba8dd389f86aa7710db079abcb0

actual_module=$(GOWORK=off go mod edit -json ai-gateway/go.mod | jq -er '.Module.Path')
if [[ "$actual_module" != "$gateway_module" ]]; then
  echo "ai-gateway/go.mod declares $actual_module, expected $gateway_module" >&2
  exit 1
fi

workspace_json=$(go work edit -json)
while IFS= read -r workspace_path; do
  workspace_root=$(realpath -m "$workspace_path")
  if [[ "$workspace_root" == "$gateway_root" || "$workspace_root" == "$gateway_root/"* ]]; then
    echo "ai-gateway and its nested modules must not be registered in go.work" >&2
    exit 1
  fi

  workspace_go_mod="$workspace_path/go.mod"
  if [[ ! -f "$workspace_go_mod" ]]; then
    echo "go.work entry $workspace_path has no go.mod" >&2
    exit 1
  fi
  workspace_module=$(GOWORK=off go mod edit -json "$workspace_go_mod" | jq -er '.Module.Path')
  if [[ "$workspace_module" == "$gateway_module" || "$workspace_module" == "$gateway_module/"* ]]; then
    echo "ai-gateway and its nested modules must not be registered in go.work" >&2
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

while IFS= read -r replacement_path; do
  case "$replacement_path" in
    /*) replacement_root=$replacement_path ;;
    ./*|../*) replacement_root=$replacement_path ;;
    *) continue ;;
  esac
  resolved_replacement=$(realpath -m "$replacement_root")
  if [[ "$resolved_replacement" == "$gateway_root" || "$resolved_replacement" == "$gateway_root/"* ]]; then
    echo "go.work must not replace a module with AI Gateway source" >&2
    exit 1
  fi
done < <(jq -r '(.Replace // [])[]?.New.Path // empty' <<<"$workspace_json")

while IFS= read -r go_mod; do
  relative_path=${go_mod#./}
  if [[ "$relative_path" == ai-gateway/* ]]; then
    continue
  fi

  module_json=$(GOWORK=off go mod edit -json "$go_mod")
  if jq -e --arg module "$gateway_module" '
    [(.Require // [])[]?.Path, (.Replace // [])[]?.Old.Path, (.Replace // [])[]?.New.Path]
    | map(select(type == "string"))
    | any(. == $module or startswith($module + "/"))
  ' <<<"$module_json" >/dev/null; then
    echo "$relative_path requires or replaces the AI Gateway module" >&2
    exit 1
  fi

  while IFS= read -r replacement_path; do
    case "$replacement_path" in
      /*) replacement_root=$replacement_path ;;
      ./*|../*) replacement_root="$(dirname "$go_mod")/$replacement_path" ;;
      *) continue ;;
    esac
    resolved_replacement=$(realpath -m "$replacement_root")
    if [[ "$resolved_replacement" == "$gateway_root" || "$resolved_replacement" == "$gateway_root/"* ]]; then
      echo "$relative_path replaces a module with AI Gateway source" >&2
      exit 1
    fi
  done < <(jq -r '(.Replace // [])[]?.New.Path // empty' <<<"$module_json")
done < <(find . -name go.mod -not -path './.git/*' -not -path '*/node_modules/*' | sort)

if grep -r -n -F \
  --include='*.go' \
  --exclude-dir=.git \
  --exclude-dir=ai-gateway \
  --exclude-dir=node_modules \
  --exclude-dir=vendor \
  "$gateway_module" .; then
  echo "Go source outside ai-gateway imports the AI Gateway module" >&2
  exit 1
fi

if GOWORK=off GOFLAGS="$readonly_flags" go list -m all | grep -Eq "^${gateway_module}([ /]|$)"; then
  echo "the root module graph contains the AI Gateway module" >&2
  exit 1
fi

GOWORK=off GOFLAGS="$readonly_flags" go build ./...
GOWORK=off GOFLAGS="$readonly_flags" go test ./...

echo "AI Gateway module and license boundary: OK"
