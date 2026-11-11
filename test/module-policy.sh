#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
source "$repo_root/scripts/module-policy.sh"
fixture=$(mktemp -d)
fixture=$(cd "$fixture" && pwd -P)
trap 'rm -rf "$fixture"' EXIT

assert_fails() {
  local expected=$1
  shift
  if ("$@") >"$fixture/failure.log" 2>&1; then
    fail "unexpected success: $*"
  fi
  grep -Fq "$expected" "$fixture/failure.log" || fail "missing '$expected' in: $(<"$fixture/failure.log")"
}

requirements_repo=$fixture/requirements
write_module() {
  local root=$1 package=$2 imports=$3 path=$module_prefix
  [[ "$root" == . ]] || path+=/$root
  mkdir -p "$requirements_repo/$root"
  printf 'module %s\n\ngo 1.27.3\n' "$path" >"$requirements_repo/$root/go.mod"
  printf 'package %s\n' "$package" >"$requirements_repo/$root/$package.go"
  [[ -z "$imports" ]] || printf 'import _ "%s"\n' $imports >>"$requirements_repo/$root/$package.go"
}
write_module . aisdk ''
write_module providers/openai openai "$module_prefix"
write_module providers/bedrock bedrock "$module_prefix/providers/openai"
write_module examples/demo demo "$module_prefix/providers/bedrock"
printf 'require %s v0.0.1\n' "$module_prefix" >>"$requirements_repo/providers/openai/go.mod"
printf 'require %s v0.0.0\n' "$module_prefix/providers/openai" >>"$requirements_repo/providers/bedrock/go.mod"
printf 'require %s v0.0.0-20260101000000-0123456789ab\n' "$module_prefix/providers/bedrock" >>"$requirements_repo/examples/demo/go.mod"
printf '{".": "0.2.0", "providers/openai": "0.1.0"}\n' >"$requirements_repo/.release-please-manifest.json"
printf '{"initial-version": "0.0.1", "packages": {".": {}, "providers/openai": {}, "providers/bedrock": {"initial-version": "0.1.0-alpha.1"}}}\n' >"$requirements_repo/release-please-config.json"
(
  cd "$requirements_repo"
  git init -q
  git add .
  export GOPROXY=off GOFLAGS=-modcacherw
  assert_fails 'requires github.com/grafana/ai-sdk@v0.0.1, expected v0.2.0' requirements
  sync >/dev/null
  requirements >/dev/null
  bedrock=$(go mod edit -json providers/bedrock/go.mod)
  [[ $(jq -r --arg path "$module_prefix" '.Replace[] | select(.Old.Path == $path) | .New.Path' <<<"$bedrock") == ../.. ]] || fail 'transitive root replacement missing'
  [[ $(jq -r --arg path "$module_prefix/providers/openai" '.Require[] | select(.Path == $path) | .Version' <<<"$bedrock") == v0.1.0 ]] || fail 'requirement not set to the manifest version'
  demo=$(go mod edit -json examples/demo/go.mod)
  [[ $(jq -r --arg path "$module_prefix/providers/bedrock" '.Require[] | select(.Path == $path) | .Version' <<<"$demo") == v0.1.0-alpha.1 ]] || fail 'never-released requirement not set to its initial version'
  ! grep -q "$module_prefix" providers/bedrock/go.sum 2>/dev/null || fail 'first-party checksums remain'
  cp providers/bedrock/go.mod "$fixture/bedrock.mod"
  (cd providers/bedrock && go mod edit -require="$module_prefix/providers/openai@v0.0.0-20260101000000-0123456789ab")
  assert_fails 'providers/openai@v0.0.0-20260101000000-0123456789ab, expected v0.1.0' requirements
  cp "$fixture/bedrock.mod" providers/bedrock/go.mod
  (cd providers/bedrock && go mod edit -dropreplace="$module_prefix")
  assert_fails "does not replace $module_prefix with ../.." requirements
  cp "$fixture/bedrock.mod" providers/bedrock/go.mod
  (cd providers/bedrock && go mod edit -replace="$module_prefix/providers/openai=../openai")
  assert_fails 'expected ../../providers/openai' requirements
  cp "$fixture/bedrock.mod" providers/bedrock/go.mod
  (cd providers/bedrock && go mod edit -replace=example.com/other=../other)
  assert_fails 'replaces non-first-party module example.com/other' requirements
  cp "$fixture/bedrock.mod" providers/bedrock/go.mod
  (cd providers/bedrock && go mod edit -require="$module_prefix/providers/unknown@v0.1.0")
  assert_fails "unregistered first-party module: $module_prefix/providers/unknown" requirements
  cp "$fixture/bedrock.mod" providers/bedrock/go.mod
  requirements >/dev/null
)

module_repo=$fixture/modules
for root in . ai-gateway providers/openai examples/demo test/conformance; do
  path=$module_prefix
  [[ "$root" == . ]] || path+=/$root
  mkdir -p "$module_repo/$root"
  printf 'module %s\n\ngo 1.27.3\n' "$path" >"$module_repo/$root/go.mod"
done
(cd "$module_repo" && git init -q && git add .)
(
  cd "$module_repo"
  [[ $(modules | wc -l) -eq 3 ]] || fail 'published module selection included local-only modules'
  [[ $(modules all | wc -l) -eq 5 ]] || fail 'tracked module inventory is incomplete'
  printf 'module example.com/wrong\n' >providers/openai/go.mod
  assert_fails 'declares example.com/wrong' modules
  printf 'module %s\n\ngo 1.27.3\n' "$module_prefix/providers/openai" >providers/openai/go.mod
)
assert_fails 'unknown module' validate nonexistent
(
  cd "$module_repo"
  go() {
    case "$1 $2" in
      'mod edit') command go "$@" ;;
      'mod download'|'mod verify'|'build ./...'|'test ./...') printf '%s\t%s\n' "$PWD" "$1 $2" >>"$fixture/scoped-operations" ;;
      *) fail "unexpected Go command: $*" ;;
    esac
  }
  for selection in ai-gateway "$gateway_module"; do
    : >"$fixture/scoped-operations"
    validate "$selection" >/dev/null
    [[ $(wc -l <"$fixture/scoped-operations") -eq 4 ]] || fail "scoped selection did not run all checks for $selection"
    for operation in 'mod download' 'mod verify' 'build ./...' 'test ./...'; do
      grep -Fxq "$module_repo/ai-gateway"$'\t'"$operation" "$fixture/scoped-operations" || fail "missing scoped operation: $operation"
    done
  done
)

boundary_repo=$fixture/boundary
for root in . ai-gateway providers/grafana middleware/logger; do
  path=$module_prefix
  [[ "$root" == . ]] || path+=/$root
  mkdir -p "$boundary_repo/$root"
  printf 'module %s\n\ngo 1.27.3\n' "$path" >"$boundary_repo/$root/go.mod"
done
cp "$repo_root/LICENSE" "$boundary_repo/LICENSE"
cp "$repo_root/ai-gateway/LICENSE" "$boundary_repo/ai-gateway/LICENSE"
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
  printf 'module %s\n\ngo 1.27.3\nreplace %s => ../../ai-gateway\n' "$module_prefix/providers/grafana" "$module_prefix" >providers/grafana/go.mod
  assert_fails 'replaces a module with Gateway source' boundary
  printf 'module %s\n\ngo 1.27.3\n' "$module_prefix/providers/grafana" >providers/grafana/go.mod
  printf 'package logger\nimport _ "%s"\n' "$gateway_module" >middleware/logger/consumer.go
  git add middleware/logger/consumer.go
  assert_fails 'outside ai-gateway references Gateway' boundary
  rm middleware/logger/consumer.go
  git add -u middleware/logger/consumer.go
  printf 'require %s v0.1.0\n' "$gateway_module" >>middleware/logger/go.mod
  assert_fails 'requires or replaces Gateway' boundary
)

source_absent_repo=$fixture/source-absent
mkdir -p "$source_absent_repo/providers/grafana" "$source_absent_repo/ai-gateway"
printf 'module %s\n\ngo 1.27.3\n' "$module_prefix" >"$source_absent_repo/go.mod"
printf 'module %s/providers/grafana\n\ngo 1.27.3\nrequire %s v0.1.0\nreplace %s => ../..\n' "$module_prefix" "$module_prefix" "$module_prefix" >"$source_absent_repo/providers/grafana/go.mod"
printf 'package aisdk\nfunc CandidateOnly() string { return "candidate" }\n' >"$source_absent_repo/candidate.go"
printf 'package aisdk\nimport "testing"\nfunc TestCandidate(t *testing.T) { if CandidateOnly() != "candidate" { t.Fatal("candidate root not executed") } }\n' >"$source_absent_repo/candidate_test.go"
printf 'package grafana\nimport "%s"\nfunc Candidate() string { return aisdk.CandidateOnly() }\n' "$module_prefix" >"$source_absent_repo/providers/grafana/candidate.go"
printf 'package grafana\nimport "testing"\nfunc TestCandidate(t *testing.T) { if Candidate() != "candidate" { t.Fatal("candidate client not executed") } }\n' >"$source_absent_repo/providers/grafana/candidate_test.go"
(
  repo_root=$source_absent_repo
  isolation
  printf 'package aisdk\nfunc CandidateOnly() string { return "regressed" }\n' >"$source_absent_repo/candidate.go"
  assert_fails 'candidate root not executed' isolation
)

bash "$repo_root/scripts/module-policy.sh" boundary

candidate=$fixture/candidate.go
overlay=$fixture/overlay.json
printf 'package provider\nvar _ = candidateSourceOnlyMarker\n' >"$candidate"
jq -n --arg source "$repo_root/provider/doc.go" --arg candidate "$candidate" '{Replace:{($source):$candidate}}' >"$overlay"
for root in ai-gateway/pkg ai-gateway/test/providerwire-v4/testserver; do
  assert_fails candidateSourceOnlyMarker bash -c 'cd "$1" && GOFLAGS=-mod=readonly go build -overlay "$2" -o "$3" .' \
    bash "$repo_root/$root" "$overlay" "$fixture/gateway"
done
assert_fails candidateSourceOnlyMarker bash -c 'cd "$1" && GOFLAGS="-mod=readonly -overlay=$2" mage build:linux build:linuxARM64' \
  bash "$repo_root/ai-gateway" "$overlay"

echo 'module policy fixtures: OK'
