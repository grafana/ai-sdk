#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
fixture=$(mktemp -d)
trap 'if [[ -n ${candidate:-} ]]; then mise trust --untrust "$candidate/mise.toml" >/dev/null 2>&1 || true; fi; rm -rf "$fixture"' EXIT

MODULE=ai-gateway mise run verify-published-module >"$fixture/baseline.log" 2>&1 || {
  echo 'The existing standalone Gateway baseline must pass before the coordinated-source proof' >&2
  tail -30 "$fixture/baseline.log" >&2
  exit 1
}

candidate=$fixture/candidate
mkdir -p "$candidate"
rsync -a --exclude='/.git' --exclude='**/node_modules' "$repo_root/" "$candidate/"

printf 'package provider\nfunc CandidateSourceMarker() string { return "candidate" }\n' >"$candidate/provider/candidate_source_marker.go"
printf 'package provider\nimport "testing"\nfunc TestCandidateSourceMarker(t *testing.T) { if CandidateSourceMarker() != "candidate" { t.Fatal("provider candidate was not executed") } }\n' >"$candidate/provider/candidate_source_marker_test.go"
printf 'package aisdk\nimport "%s/provider"\nfunc CandidateSourceMarker() string { return provider.CandidateSourceMarker() }\n' 'github.com/grafana/ai-sdk' >"$candidate/candidate_source_marker.go"
printf 'package aisdk\nimport "testing"\nfunc TestCandidateSourceMarker(t *testing.T) { if CandidateSourceMarker() != "candidate" { t.Fatal("root candidate was not executed") } }\n' >"$candidate/candidate_source_marker_test.go"
printf 'package v4\nimport (sdk "%s"; "%s/provider")\nfunc candidateSourceMarker() string { return sdk.CandidateSourceMarker() + "/" + provider.CandidateSourceMarker() }\n' 'github.com/grafana/ai-sdk' 'github.com/grafana/ai-sdk' >"$candidate/ai-gateway/providerwire/v4/candidate_source_marker.go"
printf 'package v4\nimport "testing"\nfunc TestCandidateSourceMarker(t *testing.T) { if candidateSourceMarker() != "candidate/candidate" { t.Fatal("Gateway candidate was not executed") } }\n' >"$candidate/ai-gateway/providerwire/v4/candidate_source_marker_test.go"

gofmt -w "$candidate"/provider/candidate_source_marker*.go "$candidate"/candidate_source_marker*.go "$candidate"/ai-gateway/providerwire/v4/candidate_source_marker*.go
(cd "$candidate" && git init -q && git add -A)
(cd "$candidate" && mise trust >/dev/null && mise deps >/dev/null)

for task in fmt-check build test-short vet lint test-providerwire-v4 test-integration test-conformance verify-sdk-gateway-isolation verify-ai-gateway-boundary verify-merged-pins; do
  echo "==> controlled candidate: $task"
  (cd "$candidate" && mise run "$task") >"$fixture/$task.log" 2>&1 || {
    tail -50 "$fixture/$task.log" >&2
    exit 1
  }
done

cp "$candidate/candidate_source_marker_test.go" "$fixture/valid-root-test.go"
printf 'package aisdk\nimport "testing"\nfunc TestCandidateSourceMarker(t *testing.T) { if CandidateSourceMarker() != "wrong" { t.Fatal("root candidate was not executed") } }\n' >"$candidate/candidate_source_marker_test.go"
if (cd "$candidate" && go test -short ./...) >"$fixture/source-regression.log" 2>&1; then
  echo 'The required root test path accepted a real source regression' >&2
  exit 1
fi
grep -Fq 'root candidate was not executed' "$fixture/source-regression.log" || {
  tail -50 "$fixture/source-regression.log" >&2
  exit 1
}
cp "$fixture/valid-root-test.go" "$candidate/candidate_source_marker_test.go"

if (cd "$candidate" && MODULE=ai-gateway mise run verify-published-module) >"$fixture/standalone.log" 2>&1; then
  echo 'The coordinated candidate unexpectedly passed standalone Gateway validation' >&2
  exit 1
fi
grep -Eq 'undefined: (aisdk|provider)\.CandidateSourceMarker' "$fixture/standalone.log" || {
  echo 'Standalone failure must be caused by the missing published candidate API' >&2
  tail -50 "$fixture/standalone.log" >&2
  exit 1
}

echo 'Controlled root + provider + Gateway candidate passes source checks; older merged pins fail standalone at the candidate API.'
