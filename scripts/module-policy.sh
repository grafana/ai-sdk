#!/usr/bin/env bash
# Repository module policy; invoke through mise tasks or the modes below.
# Inventory: published modules are tracked go.mod roots outside examples/ and
# test/; declared module paths must match their repository locations.
# Pins: declared and selected internal versions must resolve to commits on an
# independently fetched canonical grafana/ai-sdk main, not merely download.
# Standalone: published modules have no replacements and must download, verify,
# build, and test with a fresh public-proxy cache, readonly manifests, and no
# workspace substitutions. One module may be selected by root or module path.
# Boundary: the AGPL Gateway stays under ai-gateway/; Apache SDK modules must
# not reference it; root go.work and the SDK/Grafana graphs exclude it.
# Workspace: go.gateway.work explicitly selects local candidate source; the
# root workspace does not. Isolation: SDK and Grafana build/test without Gateway
# source present. These checks do not replace copied-code or dependency license
# review. Standalone builds gate artifacts, not source PRs.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
module_prefix=github.com/grafana/ai-sdk
gateway_module=$module_prefix/ai-gateway
canonical_url=https://github.com/grafana/ai-sdk.git
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
    path=$(GOWORK=off go mod edit -json "$file" | jq -er '.Module.Path')
    expected=$module_prefix
    [[ "$root" == . ]] || expected+=/$root
    [[ "$path" == "$expected" ]] || fail "$file declares $path, expected $expected"
    if [[ "${1:-published}" == all || ( "$root" != examples/* && "$root" != test/* ) ]]; then
      printf '%s\t%s\n' "$root" "$path"
    fi
  done <<<"$files"
  [[ "$has_root" == true ]] || fail 'missing tracked root go.mod'
}

standalone() (
  local list root path manifest replacements cache selection=${1:-}
  list=$(modules)
  if [[ -n "$selection" ]]; then
    list=$(awk -F '\t' -v wanted="$selection" '$1 == wanted || $2 == wanted' <<<"$list")
    [[ -n "$list" ]] || fail "unknown or local-only published module: $selection"
  fi
  export GOPROXY=https://proxy.golang.org GOPRIVATE='' GONOPROXY='' GONOSUMDB='' GOWORK=off GOFLAGS="$readonly_flags"
  cache=$(mktemp -d)
  trap 'chmod -R u+w "$cache" 2>/dev/null || true; rm -rf "$cache"' EXIT
  export GOMODCACHE=$cache
  while IFS=$'\t' read -r root path; do
    [[ -n "$root" ]] || continue
    echo "==> $root ($path)"
    (
      cd "$root"
      manifest=$(go mod edit -json)
      replacements=$(jq '(.Replace // []) | length' <<<"$manifest")
      [[ "$replacements" -eq 0 ]] || fail "$path contains replace directives"
      go mod download
      go mod verify >/dev/null
      go build ./...
      go test ./...
    )
  done <<<"$list"
)

pin_commit() {
  local path=$1 version=$2 root=$3 metadata subdir=$3
  [[ "$subdir" == . ]] && subdir=
  metadata=$(go mod download -json "$path@$version")
  jq -er --arg path "$path" --arg version "$version" --arg subdir "$subdir" --arg url "${canonical_url%.git}" '
    if .Error == null and .Path == $path and .Version == $version and
       .Origin.VCS == "git" and (.Origin.URL | sub("\\.git$"; "")) == $url and
       (.Origin.Subdir // "") == $subdir and (.Origin.Hash | test("^[0-9a-f]{40}$"))
    then .Origin.Hash else error("unverifiable origin for " + $path + "@" + $version) end
  ' <<<"$metadata"
}

verify_history() (
  local remote=$1 records=$2 dir anchor consumer path version hash
  unset GIT_CONFIG_PARAMETERS GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_TEMPLATE_DIR
  export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_COUNT=0
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' EXIT
  git init --bare -q "$dir"
  git -C "$dir" fetch -q --no-tags "$remote" refs/heads/main || fail "cannot fetch canonical main from $remote"
  anchor=$(git -C "$dir" rev-parse FETCH_HEAD)
  while IFS=$'\t' read -r consumer path version hash; do
    [[ -n "$consumer" ]] || continue
    [[ "$hash" =~ ^[0-9a-f]{40}$ ]] || fail "$consumer requires $path@$version: invalid commit $hash"
    if ! git -C "$dir" cat-file -e "$hash^{commit}" 2>/dev/null; then
      git -C "$dir" fetch -q --no-tags "$remote" "$hash" || fail "$consumer requires $path@$version: cannot fetch $hash from canonical repository"
    fi
    git -C "$dir" merge-base --is-ancestor "$hash" "$anchor" || fail "$consumer requires $path@$version ($hash): not an ancestor of canonical main"
  done <<<"$records"
)

merged_pins() {
  local list root consumer manifest graph declared selected versions path version selected_root hash replacements records=
  list=$(modules)
  export GOPROXY=https://proxy.golang.org GOPRIVATE='' GONOPROXY='' GONOSUMDB='' GOWORK=off GOFLAGS="$readonly_flags"
  while IFS=$'\t' read -r root consumer; do
    [[ -n "$root" ]] || continue
    manifest=$(cd "$root" && go mod edit -json)
    replacements=$(jq '(.Replace // []) | length' <<<"$manifest")
    [[ "$replacements" -eq 0 ]] || fail "$consumer contains replace directives"
    graph=$(cd "$root" && go list -m -json all)
    declared=$(jq -r '(.Require // [])[] | [.Path, .Version] | @tsv' <<<"$manifest")
    selected=$(jq -sr '.[] | select(.Main != true) | if .Replace then error("selected replacement for " + .Path) else [.Path, .Version] | @tsv end' <<<"$graph")
    versions=$(printf '%s\n%s\n' "$declared" "$selected" | sort -u)
    while IFS=$'\t' read -r path version; do
      [[ "$path" == "$module_prefix" || "$path" == "$module_prefix/"* ]] || continue
      selected_root=$(awk -F '\t' -v path="$path" '$2 == path {print $1}' <<<"$list")
      [[ -n "$selected_root" ]] || fail "$consumer references unknown internal module $path"
      hash=$(pin_commit "$path" "$version" "$selected_root") || fail "$consumer requires $path@$version: cannot resolve canonical origin"
      records+="$consumer"$'\t'"$path"$'\t'"$version"$'\t'"$hash"$'\n'
    done <<<"$versions"
  done <<<"$list"
  verify_history "$canonical_url" "$records"
  while IFS=$'\t' read -r consumer path version hash; do
    [[ -n "$consumer" ]] || continue
    printf '%s: %s@%s %s\n' "$consumer" "$path" "$version" "$hash"
  done <<<"$records"
}

boundary() {
  local path marker module workspace paths list root manifest references targets target candidate resolved graph status imports replacements
  for license in 'LICENSE:Apache License' 'ai-gateway/LICENSE:GNU AFFERO GENERAL PUBLIC LICENSE'; do
    path=${license%%:*}; marker=${license#*:}
    if [[ ! -s "$path" ]] || ! grep -Fq "$marker" "$path"; then
      fail "$path is missing the expected license text"
    fi
  done
  module=$(GOWORK=off go mod edit -json ai-gateway/go.mod | jq -er '.Module.Path')
  [[ "$module" == "$gateway_module" ]] || fail "ai-gateway/go.mod declares $module"
  [[ ! -e gateway/catalog && ! -e gateway/providerwire ]] || fail 'Gateway implementation must live below ai-gateway'
  workspace=$(go work edit -json go.work)
  replacements=$(jq '(.Replace // []) | length' <<<"$workspace")
  [[ "$replacements" -eq 0 ]] || fail 'root go.work must not contain replace directives'
  paths=$(jq -r '.Use[]?.DiskPath' <<<"$workspace")
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    module=$(GOWORK=off go mod edit -json "$path/go.mod" | jq -er '.Module.Path')
    [[ "$module" != "$gateway_module" && "$module" != "$gateway_module/"* ]] || fail 'Gateway module in root go.work'
  done <<<"$paths"
  list=$(modules all)
  while IFS=$'\t' read -r root module; do
    [[ -n "$root" ]] || continue
    [[ "$module" != "$gateway_module" && "$module" != "$gateway_module/"* ]] || continue
    manifest=$(GOWORK=off go mod edit -json "$root/go.mod")
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
    graph=$(cd "$root" && GOWORK=off GOFLAGS="$readonly_flags" go list -m all)
    while read -r module _; do
      [[ "$module" != "$gateway_module" && "$module" != "$gateway_module/"* ]] || fail "$root module graph contains Gateway"
    done <<<"$graph"
  done
  echo 'AI Gateway module and license boundary: OK'
}

workspace() {
  local file=$repo_root/go.gateway.work paths actual expected root module selected directory
  paths=$(go work edit -json "$file" | jq -r '.Use[].DiskPath')
  actual=
  while IFS= read -r root; do
    module=$(GOWORK=off go mod edit -json "$root/go.mod" | jq -er '.Module.Path')
    actual+="$module"$'\n'
    selected=$(cd ai-gateway && GOWORK="$file" GOFLAGS="$readonly_flags" go list -m -json "$module")
    directory=$(cd "$root" && pwd -P)
    [[ $(jq -r '.Main' <<<"$selected") == true && $(jq -r '.Dir' <<<"$selected") == "$directory" ]] || fail "Gateway workspace did not select local $module"
  done <<<"$paths"
  expected=$(modules | cut -f2 | sort)
  [[ $(printf '%s' "$actual" | sort) == "$expected" ]] || fail 'Gateway workspace must contain exactly the published modules'
  [[ $(go work edit -json go.work | jq -r '[.Use[]?.DiskPath] | index("./ai-gateway")') == null ]] || fail 'root go.work includes Gateway'
}

isolation() (
  local dir root module selected directory workspace_file
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' EXIT
  rsync -a --exclude='/.git' --exclude='/ai-gateway' --exclude='**/node_modules' "$repo_root/" "$dir/"
  [[ ! -e "$dir/ai-gateway" ]] || fail 'isolated copy contains Gateway source'
  workspace_file=$dir/go.work
  for root in . providers/grafana; do
    echo "==> $root without Gateway source"
    module=$module_prefix
    [[ "$root" == . ]] || module+=/$root
    selected=$(cd "$dir/$root" && GOWORK="$workspace_file" GOFLAGS="$readonly_flags" go list -m -json "$module")
    directory=$(cd "$dir/$root" && pwd -P)
    [[ $(jq -r '.Main' <<<"$selected") == true && $(jq -r '.Dir' <<<"$selected") == "$directory" ]] || fail "isolated copy did not select candidate $module"
    (cd "$dir/$root" && GOWORK="$workspace_file" GOFLAGS="$readonly_flags" go build ./... && GOWORK="$workspace_file" GOFLAGS="$readonly_flags" go test ./...)
  done
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  case "${1:-}" in
    modules) [[ $# == 1 ]] || fail 'usage: module-policy.sh modules'; modules ;;
    all-modules) [[ $# == 1 ]] || fail 'usage: module-policy.sh all-modules'; modules all ;;
    standalone) [[ $# -le 2 ]] || fail 'usage: module-policy.sh standalone [module-root-or-path]'; standalone "${2:-}" ;;
    pins) [[ $# == 1 ]] || fail 'usage: module-policy.sh pins'; merged_pins ;;
    boundary) [[ $# == 1 ]] || fail 'usage: module-policy.sh boundary'; boundary ;;
    workspace) [[ $# == 1 ]] || fail 'usage: module-policy.sh workspace'; workspace ;;
    isolation) [[ $# == 1 ]] || fail 'usage: module-policy.sh isolation'; isolation ;;
    *) fail 'usage: module-policy.sh {modules|all-modules|standalone|pins|boundary|workspace|isolation}' ;;
  esac
fi
