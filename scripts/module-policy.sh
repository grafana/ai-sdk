#!/usr/bin/env bash
# Repository module policy; invoke through mise tasks or the modes below.
# Inventory: published modules are tracked go.mod roots outside examples/ and
# test/; declared module paths must match their repository locations.
# Requirements: every tracked go.mod replaces each first-party module in its
# build graph with that module's directory and requires it at its version in
# .release-please-manifest.json, or its registered initial version. Sync writes
# both; consumers ignore replacements, so published requirements stay real.
# Validate: one or all modules download, verify, build and test with readonly
# manifests and their local first-party replacements.
# Boundary: the AGPL Gateway stays under ai-gateway/; Apache SDK modules must
# not reference it and the SDK/Grafana graphs exclude it. Isolation: SDK and
# Grafana build/test without Gateway source present. These checks do not
# replace copied-code or dependency license review.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
module_prefix=github.com/grafana/ai-sdk
gateway_module=$module_prefix/ai-gateway
manifest_file=.release-please-manifest.json
config_file=release-please-config.json
readonly_flags="${GOFLAGS:+$GOFLAGS }-mod=readonly"

fail() { echo "$*" >&2; exit 1; }

modules() {
  local file root path expected files has_root=false
  files=$(git ls-files 'go.mod' '*/go.mod')
  while IFS= read -r file; do
    [[ -f "$file" ]] || fail "tracked module $file is missing"
    root=${file%/go.mod}
    if [[ "$file" == go.mod ]]; then
      root=.
      has_root=true
    fi
    path=$(go mod edit -json "$file" | jq -er '.Module.Path')
    expected=$module_prefix
    [[ "$root" == . ]] || expected+=/$root
    [[ "$path" == "$expected" ]] || fail "$file declares $path, expected $expected"
    if [[ "${1:-published}" == all || ( "$root" != examples/* && "$root" != test/* ) ]]; then
      printf '%s\t%s\n' "$root" "$path"
    fi
  done <<<"$files"
  [[ "$has_root" == true ]] || fail 'missing tracked root go.mod'
}

validate() (
  local list root path selection=${1:-}
  list=$(modules all)
  if [[ -n "$selection" ]]; then
    list=$(awk -F '\t' -v wanted="$selection" '$1 == wanted || $2 == wanted' <<<"$list")
    [[ -n "$list" ]] || fail "unknown module: $selection"
  fi
  export GOFLAGS="$readonly_flags"
  while IFS=$'\t' read -r root path; do
    [[ -n "$root" ]] || continue
    echo "==> $root ($path)"
    (
      cd "$root"
      go mod download
      go mod verify >/dev/null
      go build ./...
      go test ./...
    )
  done <<<"$list"
)

module_root() {
  if [[ "$1" == "$module_prefix" ]]; then echo .; else echo "${1#"$module_prefix"/}"; fi
}

relative_path() {
  local from=$1 to=$2 up=./
  [[ "$from" == . ]] || up=$(sed -E 's#[^/]+#..#g' <<<"$from")/
  if [[ "$to" == . ]]; then echo "${up%/}"; else echo "$up$to"; fi
}

expected_version() {
  local root=$1 version
  version=$(jq -r --arg root "$root" '.[$root] // empty' "$manifest_file")
  [[ -n "$version" ]] || version=$(jq -r --arg root "$root" '.packages[$root]["initial-version"] // .["initial-version"] // empty' "$config_file")
  [[ -n "$version" ]] || fail "$root has no manifest or initial version"
  echo "v$version"
}

published_root() {
  local root
  root=$(module_root "$1")
  awk -F '\t' -v root="$root" '$1 == root { found = 1 } END { exit !found }' <<<"$2" || fail "unregistered first-party module: $1"
  echo "$root"
}

first_party_requirements() {
  go mod edit -json "$1/go.mod" | jq -r --arg prefix "$module_prefix" '
    (.Require // [])[] | select(.Path == $prefix or (.Path | startswith($prefix + "/"))) | .Path'
}

closure() {
  local published=$1 pending seen= path root
  pending=$(first_party_requirements "$2")
  while [[ -n "$pending" ]]; do
    path=${pending%%$'\n'*}
    if [[ "$pending" == *$'\n'* ]]; then pending=${pending#*$'\n'}; else pending=; fi
    [[ -n "$path" ]] || continue
    grep -Fxq -- "$path" <<<"$seen" && continue
    seen+="$path"$'\n'
    root=$(published_root "$path" "$published")
    pending+="${pending:+$'\n'}$(first_party_requirements "$root")"
  done
  printf '%s' "$seen" | sort
}

sync_module() {
  local published=$1 root=$2 path args=()
  while IFS= read -r path; do
    [[ -n "$path" ]] && args+=("-dropreplace=$path")
  done < <(go mod edit -json "$root/go.mod" | jq -r --arg prefix "$module_prefix" '
    (.Replace // [])[] | select(.Old.Path == $prefix or (.Old.Path | startswith($prefix + "/"))) | .Old.Path')
  while IFS= read -r path; do
    [[ -n "$path" ]] && args+=("-replace=$path=$(relative_path "$root" "$(module_root "$path")")")
  done < <(closure "$published" "$root")
  while IFS= read -r path; do
    [[ -n "$path" ]] && args+=("-require=$path@$(expected_version "$(published_root "$path" "$published")")")
  done < <(first_party_requirements "$root")
  (( ${#args[@]} == 0 )) || (cd "$root" && go mod edit "${args[@]}")
}

sync() {
  local published all root _ pass
  published=$(modules)
  all=$(modules all)
  for pass in 1 2; do
    while IFS=$'\t' read -r root _; do
      sync_module "$published" "$root"
    done <<<"$all"
    while IFS=$'\t' read -r root _; do
      (cd "$root" && GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=mod" go mod tidy)
    done <<<"$all"
  done
}

requirements() {
  local published all root module manifest graph path version target expected failures=
  published=$(modules)
  all=$(modules all)
  export GOFLAGS="$readonly_flags"
  while IFS=$'\t' read -r root module; do
    manifest=$(go mod edit -json "$root/go.mod")
    while IFS=$'\t' read -r path version; do
      [[ -n "$path" ]] || continue
      expected=$(expected_version "$(published_root "$path" "$published")")
      [[ "$version" == "$expected" ]] || failures+="$module requires $path@$version, expected $expected"$'\n'
    done < <(jq -r --arg prefix "$module_prefix" '(.Require // [])[]
      | select(.Path == $prefix or (.Path | startswith($prefix + "/"))) | [.Path, .Version] | @tsv' <<<"$manifest")
    while IFS=$'\t' read -r path target version; do
      [[ -n "$path" ]] || continue
      if [[ "$path" != "$module_prefix" && "$path" != "$module_prefix/"* ]]; then
        failures+="$module replaces non-first-party module $path"$'\n'
        continue
      fi
      expected=$(relative_path "$root" "$(published_root "$path" "$published")")
      [[ -z "$version" && "$target" == "$expected" ]] || failures+="$module replaces $path${version:+@$version} with $target, expected $expected"$'\n'
    done < <(jq -r '(.Replace // [])[] | [.Old.Path, .New.Path, (.Old.Version // "")] | @tsv' <<<"$manifest")
    while IFS= read -r path; do
      [[ -n "$path" ]] || continue
      expected=$(relative_path "$root" "$(module_root "$path")")
      jq -e --arg path "$path" --arg target "$expected" 'any((.Replace // [])[]; .Old.Path == $path and .New.Path == $target)' <<<"$manifest" >/dev/null ||
        failures+="$module does not replace $path with $expected"$'\n'
    done < <(closure "$published" "$root")
    if ! graph=$(cd "$root" && go list -m -json all); then
      failures+="$module build graph cannot be resolved"$'\n'
      continue
    fi
    while IFS=$'\t' read -r path target; do
      [[ -n "$path" ]] || continue
      expected=$(relative_path "$root" "$(published_root "$path" "$published")")
      [[ "$target" == "$expected" ]] || failures+="$module does not replace $path with $expected"$'\n'
    done < <(jq -r --arg prefix "$module_prefix" 'select(.Main != true)
      | select(.Path == $prefix or (.Path | startswith($prefix + "/"))) | [.Path, (.Replace.Path // "")] | @tsv' <<<"$graph")
  done <<<"$all"
  [[ -z "$failures" ]] || fail "${failures}run 'mise run tidy' to fix"
  echo 'First-party requirements and replacements: OK'
}

boundary() {
  local path marker module list root manifest references targets target candidate resolved graph status imports
  for license in 'LICENSE:Apache License' 'ai-gateway/LICENSE:GNU AFFERO GENERAL PUBLIC LICENSE'; do
    path=${license%%:*}; marker=${license#*:}
    if [[ ! -s "$path" ]] || ! grep -Fq "$marker" "$path"; then
      fail "$path is missing the expected license text"
    fi
  done
  module=$(go mod edit -json ai-gateway/go.mod | jq -er '.Module.Path')
  [[ "$module" == "$gateway_module" ]] || fail "ai-gateway/go.mod declares $module"
  [[ ! -e gateway/catalog && ! -e gateway/providerwire ]] || fail 'Gateway implementation must live below ai-gateway'
  list=$(modules all)
  while IFS=$'\t' read -r root module; do
    [[ -n "$root" ]] || continue
    [[ "$module" != "$gateway_module" && "$module" != "$gateway_module/"* ]] || continue
    manifest=$(go mod edit -json "$root/go.mod")
    references=$(jq -r --arg gateway "$gateway_module" '
      [(.Require // [])[]?.Path, (.Replace // [])[]?.Old.Path, (.Replace // [])[]?.New.Path]
      | map(select(type == "string" and (. == $gateway or startswith($gateway + "/")))) | .[]
    ' <<<"$manifest")
    [[ -z "$references" ]] || fail "$module requires or replaces Gateway: $references"
    targets=$(jq -r '(.Replace // [])[]?.New.Path // empty' <<<"$manifest")
    while IFS= read -r target; do
      case "$target" in
        /*) candidate=$target ;;
        ./*|../*) candidate="$root/$target" ;;
        *) continue ;;
      esac
      if [[ -d "$candidate" ]]; then
        resolved=$(cd "$candidate" && pwd -P)
        [[ "$resolved" != "$repo_root/ai-gateway" && "$resolved" != "$repo_root/ai-gateway/"* ]] || fail "$module replaces a module with Gateway source"
      fi
    done <<<"$targets"
  done <<<"$list"
  if imports=$(git grep -nF "$gateway_module" -- '*.go' ':(exclude)ai-gateway/**'); then
    fail "Go source outside ai-gateway references Gateway: $imports"
  else
    status=$?
    (( status == 1 )) || exit "$status"
  fi
  for root in . providers/grafana; do
    graph=$(cd "$root" && GOFLAGS="$readonly_flags" go list -m all)
    while read -r module _; do
      [[ "$module" != "$gateway_module" && "$module" != "$gateway_module/"* ]] || fail "$root module graph contains Gateway"
    done <<<"$graph"
  done
  echo 'AI Gateway module and license boundary: OK'
}

isolation() (
  local dir root module selected directory
  dir=$(mktemp -d)
  dir=$(cd "$dir" && pwd -P)
  trap 'rm -rf "$dir"' EXIT
  rsync -a --exclude='/.git' --exclude='/ai-gateway' --exclude='**/node_modules' "$repo_root/" "$dir/"
  [[ ! -e "$dir/ai-gateway" ]] || fail 'isolated copy contains Gateway source'
  export GOFLAGS="$readonly_flags"
  for root in . providers/grafana; do
    echo "==> $root without Gateway source"
    module=$module_prefix
    [[ "$root" == . ]] || module+=/$root
    selected=$(cd "$dir/$root" && go list -m -json "$module")
    directory=$(cd "$dir/$root" && pwd -P)
    [[ $(jq -r '.Main' <<<"$selected") == true && $(jq -r '.Dir' <<<"$selected") == "$directory" ]] || fail "isolated copy did not select candidate $module"
    (cd "$dir/$root" && go build ./... && SDK_GATEWAY_ISOLATION=1 go test ./...)
  done
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  case "${1:-}" in
    modules) [[ $# == 1 ]] || fail 'usage: module-policy.sh modules'; modules ;;
    all-modules) [[ $# == 1 ]] || fail 'usage: module-policy.sh all-modules'; modules all ;;
    validate) [[ $# -le 2 ]] || fail 'usage: module-policy.sh validate [module-root-or-path]'; validate "${2:-}" ;;
    requirements) [[ $# == 1 ]] || fail 'usage: module-policy.sh requirements'; requirements ;;
    sync) [[ $# == 1 ]] || fail 'usage: module-policy.sh sync'; sync ;;
    boundary) [[ $# == 1 ]] || fail 'usage: module-policy.sh boundary'; boundary ;;
    isolation) [[ $# == 1 ]] || fail 'usage: module-policy.sh isolation'; isolation ;;
    *) fail 'usage: module-policy.sh {modules|all-modules|validate|requirements|sync|boundary|isolation}' ;;
  esac
fi
