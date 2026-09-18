# WP11 final review corrections

Reviewed PR #186 at `4df4ed41ba07cd498fc1747d7671b9c63b0ac293`, against Nara's
WP11 contract, the registered upstream baseline, and the WP9 dependency.
The initial read-only verdict found two concrete hosted-check blockers:

1. The superseded `wireText` decoder remained unused in the Go Gateway client,
   failing golangci-lint. Removed only the obsolete type and method.
2. `test/conformance/go.mod` still named root prerequisite `1d07c18` while its
   local Anthropic replacement required `07aacebe97a2`. Updated that requirement
   to `v0.1.0-alpha.1.0.20260916154023-07aacebe97a2`. Read-only `go mod tidy -diff`
   is empty afterward; no additional tidy changes were required.

The coordinator authorized these corrections and one signed local fix commit.
No push, hosted review submission, PR-body change, or deployment is part of this
worker's execution.

## Verified evidence

- Final Gateway pins: root `07aacebe97a2`, Anthropic `e9128cc3b35a`; both Apache
  provider modules consume the same immutable root. Hosted module resolution
  passed at the reviewed head.
- `GOWORK=off GOFLAGS=-mod=readonly go test -race -timeout=60s
  ./providerwire/v4 ./cmd/grafana-ai-gateway/internal/service`: pass.
- Anthropic `TestFunctionTools_NativeSelectedValues` with `-race`, immutable root,
  and a 45-second timeout: pass, including selected `{}` input schema and `[]`
  tool-result content, absent-value behavior, strict false, examples, and choice.
- Go Gateway client complete race suite with a 45-second timeout: pass before
  and after removing the obsolete decoder.
- Go Gateway client golangci-lint 2.12.2: zero issues after correction.
- Complete tagged conformance suite with `GOWORK=off`, readonly module mode,
  and a 90-second timeout: pass across harness and all provider packages.
- Strict WP11 OpenSpec validation, formatting, and `git diff --check`: pass.

Checks used installed Go 1.26.6 and the task build cache. An initial Anthropic
test command matched no tests; the exact verified test name above was then run
and passed. The initial lint executable path was corrected before its passing
run. These setup errors are not claimed as verification.

## Remaining scope and PR wording

The WP11 task record now distinguishes the completed immutable pins/native
selected-empty proof from the still-open composed fallback-tool integration
evidence. WP9's guard exists and denies effects before candidate invocation;
task 3.5 remains open for the composed HTTP/tool-continuation evidence. Production
activation remains WP10. The active OpenSpec merge gate is expected while these
draft tasks remain open; do not archive merely to hide incomplete acceptance.

Suggested replacement for the stale draft-boundaries paragraph in PR #186:

> This is a stacked review draft on WP9. Immutable Apache prerequisites are
> published and pinned, and deterministic native request tests prove selected
> empty schema and content preservation. Composed fallback-tool integration
> evidence and the remaining unchecked OpenSpec acceptance items stay open.
> Production activation belongs to WP10. Keep the PR based on WP9 until the
> prerequisite stack lands.

The two diagnosed CI failures are locally resolved. Hosted CI must be rerun after
the coordinator publishes the signed fix; this report does not claim green
hosted checks for an unpublished commit.
