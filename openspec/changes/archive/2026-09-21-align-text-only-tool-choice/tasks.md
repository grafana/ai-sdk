## 1. Core defaulting and regressions

- [x] 1.1 Compare choice preparation against registered upstream `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` and identify the core, provider-call, and Gateway compatibility boundaries without upgrading the baseline.
- [x] 1.2 Add provider-call tests for omitted choice with absent, empty, nonempty, and filtered-out tools; preserve explicit choices and per-step precedence/reset. Cover GenerateText and both Agent entry points.
- [x] 1.3 Default nil effective choice to auto independently of tools in `streamtext.go`, without mutating configuration or explicit choices.

## 2. Fallback admission and boundary regressions

- [x] 2.1 Allow only absent or pure automatic choice with no tools in `fallbackTextRequest`, preserving original options and all other effect guards.
- [x] 2.2 Test both fallback entry points, pre-commit failover and restart-at-primary with auto preserved, and zero physical invocations for non-auto choices, invalid auto, tools/history, or unsupported controls.
- [x] 2.3 Add HTTP regression coverage in `runtime_test.go` for the unchanged mapper: both modes, choice presence/preservation, absent/empty tools, correct invocation counts, and schema/unsupported-family rejection without resolution or SSE commitment.

## 3. Actual high-level client evidence

- [x] 3.1 Add a narrow actual Go StreamText probe with explicit local workspace/core/client verification; preserve independent ordinary client and Gateway builds.
- [x] 3.2 Add actual Go/TypeScript text-only stream calls through the existing authenticated edge/real-command composition. Capture unmodified inbound auto, one backend invocation, expected text, and credential privacy using the existing pinned TypeScript dependency.
- [x] 3.3 Extend the existing low-level fallback command matrix to run omitted and automatic choice in both modes while retaining effect rejection and direct function-tool round trips.

## 4. Validation and documentation

- [x] 4.1 Run root and isolated Gateway tests, focused core/Gateway race tests, command integration, direct conformance and parity checks, build, vet, and lint. Preserve provider input provenance and existing fixture expectations.
- [x] 4.2 Update parity coverage and only the spec requirements affected by this PR: core defaulting, fallback admission, the unary-tool fallback prohibition, and high-level authenticated streaming evidence. Document the custom-provider impact and fallback-specific rollout risk.
- [x] 4.3 Validate OpenSpec and whitespace; confirm no mapper, production dependency, lockfile, client serializer, authentication, or UI/SSE protocol changes.

## Validation evidence

- Passed: `mise run test`, `mise run build`, `mise run test-ai-gateway-command` (28 tests), `mise run parity-check` (including pinned ProviderWire contracts and direct conformance), `mise run vet`, `mise run lint` (zero issues), and focused core/Gateway fallback/runtime race tests.
- Strict OpenSpec validation and `git diff --check` pass. Regression tests use the local core where needed; isolated Gateway checks retain published dependency selection.
- No provider recordings, fixture expectations, production dependency pins, or UI/SSE formats changed. No client request is rewritten.

## Deferred validation

The owner deferred #201's paired pre-fix reproduction, unchanged `anthropic/upstream/text-generation` replay through both Gateway executors, and broader matrix rerun to that harness's integration. These checks remain unexecuted follow-up, not shipping gates or passing evidence. Do not build a replacement harness or change requests, provider inputs, or goldens to force matrix results.
