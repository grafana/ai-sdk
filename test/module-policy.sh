#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
source "$repo_root/scripts/module-policy.sh"
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

assert_fails() {
  local expected=$1
  shift
  if ("$@") >"$fixture/failure.log" 2>&1; then
    fail "unexpected success: $*"
  fi
  grep -Fq "$expected" "$fixture/failure.log" || fail "missing '$expected' in: $(<"$fixture/failure.log")"
}

canonical=$fixture/canonical
git init -q -b main "$canonical"
git -C "$canonical" config user.email test@example.com
git -C "$canonical" config user.name Test
git -C "$canonical" commit --allow-empty -qm merged
merged=$(git -C "$canonical" rev-parse HEAD)
git -C "$canonical" tag v0.1.0
git -C "$canonical" checkout -q -b feature
git -C "$canonical" commit --allow-empty -qm 'branch only'
unmerged=$(git -C "$canonical" rev-parse HEAD)
git -C "$canonical" tag v0.2.0
git -C "$canonical" checkout -q main
git -C "$canonical" checkout -q -b synthetic
git -C "$canonical" merge --no-ff -qm 'synthetic PR merge' feature
synthetic=$(git -C "$canonical" rev-parse HEAD)
git -C "$canonical" checkout -q main

record() { printf '%s\t%s\t%s\t%s\n' "$module_prefix/ai-gateway" "$module_prefix" "$2" "$1"; }
verify_history "$canonical" "$(record "$merged" v0.1.0)"
verify_history "$canonical" "$(record "$merged" "v0.0.0-20260101000000-${merged:0:12}")"
assert_fails 'not an ancestor' verify_history "$canonical" "$(record "$unmerged" v0.2.0)"
assert_fails 'not an ancestor' verify_history "$canonical" "$(record "$synthetic" "v0.0.0-20260101000000-${synthetic:0:12}")"
assert_fails 'cannot fetch' verify_history "$canonical" "$(record "$(printf 'f%.0s' {1..40})" v0.1.0)"

fork=$fixture/fork
git clone -q --depth=1 "file://$canonical" "$fork"
git -C "$fork" fetch -q origin feature
git -C "$fork" config user.email fork@example.com
git -C "$fork" config user.name Fork
git -C "$fork" merge --no-ff -qm 'fork merge' FETCH_HEAD
assert_fails 'not an ancestor' verify_history "$canonical" "$(record "$unmerged" v0.2.0)"

version=v0.1.0
hash=$merged
go() {
  if [[ "$1 $2 $3" == 'mod download -json' ]]; then
    printf '{"Path":"%s","Version":"%s","Origin":{"VCS":"git","URL":"%s","Subdir":"%s","Hash":"%s"}}\n' \
      "${fixture_path:-$module_prefix/providers/openai}" "${fixture_version:-$version}" "${fixture_url:-${canonical_url%.git}}" \
      "${fixture_subdir:-providers/openai}" "${fixture_hash:-$hash}"
  else
    command go "$@"
  fi
}
[[ $(pin_commit "$module_prefix/providers/openai" "$version" providers/openai) == "$hash" ]] || fail 'valid pin origin rejected'
fixture_url=https://github.com/other/fork.git
assert_fails 'unverifiable origin' pin_commit "$module_prefix/providers/openai" "$version" providers/openai
unset fixture_url
fixture_subdir=providers/bedrock
assert_fails 'unverifiable origin' pin_commit "$module_prefix/providers/openai" "$version" providers/openai
unset fixture_subdir
fixture_hash=abcd
assert_fails 'unverifiable origin' pin_commit "$module_prefix/providers/openai" "$version" providers/openai
unset fixture_hash
fixture_path=$module_prefix/providers/unknown
assert_fails 'unverifiable origin' pin_commit "$module_prefix/providers/openai" "$version" providers/openai
unset fixture_path
fixture_version=v0.2.0
assert_fails 'unverifiable origin' pin_commit "$module_prefix/providers/openai" "$version" providers/openai
unset fixture_version
unset -f go
go() {
  if [[ "$1 $2 $3" == 'mod download -json' ]]; then
    printf '{"Error":"revision not found"}\n'
  else
    command go "$@"
  fi
}
assert_fails 'unverifiable origin' pin_commit "$module_prefix/providers/openai" "$version" providers/openai
unset -f go

module_repo=$fixture/modules
for root in . ai-gateway providers/openai examples/demo test/conformance; do
  path=$module_prefix
  [[ "$root" == . ]] || path+=/$root
  mkdir -p "$module_repo/$root"
  printf 'module %s\n\ngo 1.26.3\n' "$path" >"$module_repo/$root/go.mod"
done
(cd "$module_repo" && git init -q && git add .)
(
  cd "$module_repo"
  [[ $(modules | wc -l) -eq 3 ]] || fail 'published module selection included local-only modules'
  [[ $(modules all | wc -l) -eq 5 ]] || fail 'tracked module inventory is incomplete'
  (
    printf 'module %s\n\ngo 1.26.3\nrequire %s v0.1.0\n' "$gateway_module" "$module_prefix" >ai-gateway/go.mod
    graph_extra=
    go() {
      if [[ "$1 $2 $3" == 'mod download -json' ]]; then
        local path=${4%@*} version=${4##*@} subdir=${4%@*}
        subdir=${subdir#"$module_prefix"/}
        [[ "$path" == "$module_prefix" ]] && subdir=
        printf '{"Path":"%s","Version":"%s","Origin":{"VCS":"git","URL":"%s","Subdir":"%s","Hash":"%s"}}\n' \
          "$path" "$version" "${canonical_url%.git}" "$subdir" "$merged"
      elif [[ "$1 $2 $3" == 'list -m -json' ]]; then
        local current
        current=$(command go mod edit -json | jq -r '.Module.Path')
        printf '{"Path":"%s","Main":true}\n' "$current"
        if [[ "$current" == "$gateway_module" ]]; then
          printf '{"Path":"%s","Version":"v0.2.0"}\n' "$module_prefix"
          printf '{"Path":"%s","Version":"v0.3.0"}\n' "$module_prefix/providers/openai"
          [[ -z "$graph_extra" ]] || printf '{"Path":"%s","Version":"v0.1.0"}\n' "$graph_extra"
        fi
      else
        command go "$@"
      fi
    }
    verify_history() { [[ "$1" == "$canonical_url" ]] && printf '%s' "$2" >"$fixture/records"; }
    merged_pins >/dev/null
    grep -Fq "$gateway_module"$'\t'"$module_prefix"$'\t'v0.1.0 "$fixture/records" || fail 'direct requirement missing'
    grep -Fq "$gateway_module"$'\t'"$module_prefix"$'\t'v0.2.0 "$fixture/records" || fail 'selected requirement missing'
    grep -Fq "$gateway_module"$'\t'"$module_prefix/providers/openai"$'\t'v0.3.0 "$fixture/records" || fail 'transitive selection missing'
    graph_extra=$module_prefix/providers/unknown
    assert_fails 'unknown internal module' merged_pins
  )
  printf 'module example.com/wrong\n' >providers/openai/go.mod
  assert_fails 'declares example.com/wrong' modules
)
assert_fails 'unknown or local-only' standalone test/conformance
assert_fails 'unknown or local-only' standalone nonexistent

boundary_repo=$fixture/boundary
for root in . ai-gateway providers/grafana; do
  path=$module_prefix
  [[ "$root" == . ]] || path+=/$root
  mkdir -p "$boundary_repo/$root"
  printf 'module %s\n\ngo 1.26.3\n' "$path" >"$boundary_repo/$root/go.mod"
done
cp "$repo_root/LICENSE" "$boundary_repo/LICENSE"
cp "$repo_root/ai-gateway/LICENSE" "$boundary_repo/ai-gateway/LICENSE"
printf 'go 1.26.3\nuse (\n .\n ./providers/grafana\n)\n' >"$boundary_repo/go.work"
printf 'package sdk\n' >"$boundary_repo/consumer.go"
(
  cd "$boundary_repo"
  git init -q
  git add .
  repo_root=$PWD
  boundary >/dev/null
  printf 'package sdk\nimport _ "%s"\n' "$gateway_module" >consumer.go
  assert_fails 'outside ai-gateway references Gateway' boundary
  printf 'package sdk\n' >consumer.go
  printf 'require %s v0.1.0\n' "$gateway_module" >>providers/grafana/go.mod
  assert_fails 'requires or replaces Gateway' boundary
)

bash "$repo_root/scripts/module-policy.sh" workspace
bash "$repo_root/scripts/module-policy.sh" boundary

candidate=$fixture/candidate.go
overlay=$fixture/overlay.json
printf 'package provider\nvar _ = candidateSourceOnlyMarker\n' >"$candidate"
jq -n --arg source "$repo_root/provider/doc.go" --arg candidate "$candidate" '{Replace:{($source):$candidate}}' >"$overlay"
for root in ai-gateway/cmd/grafana-ai-gateway ai-gateway/test/providerwire-v4/testserver; do
  (cd "$repo_root/$root" && GOWORK=off GOFLAGS=-mod=readonly command go build -overlay "$overlay" -o "$fixture/gateway" .)
  assert_fails candidateSourceOnlyMarker bash -c 'cd "$1" && GOWORK="$2" GOFLAGS=-mod=readonly go build -overlay "$3" -o "$4" .' \
    bash "$repo_root/$root" "$repo_root/go.gateway.work" "$overlay" "$fixture/gateway"
done

echo 'module policy fixtures: OK'
