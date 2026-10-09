# Verification

## Branch and dependency

Implemented on `jeff/self-hosted-gateway-auth`, now based on open, ready-for-review #369 at `ea5bfa03bf062ebf344d63d5f769b52d6750d85a` (`nrbrd/byok`, base `nrbrd/failure-visibility`). Restacked from `323b6524f1c02cf2f4cf0a91b61e409a3741c88f`; the earlier `55a80621` base is also superseded. The single test conflict preserves both the dependency's ordinary account-decoding assertions and JWT/static-key policy isolation. Upstream native diagnostic echoes, field-based logging policy and listener defaults remain intact (private 8082, Cloud 8080, operational 8081).

#367 is merged; #368 remains open at `d17f449d5671c347594c90def582c9707c9eb5b8`. #248 remains separate open work (now the bounded OpenAI Chat Completions adapter), not a prerequisite. The dependency has archived `activate-gateway-auth-and-request-byok` with external deployment task 4.1 explicitly unchecked. Its nine synced specs and archive are inherited unchanged; this feature does not establish Cloud deployment readiness.

The exact registered upstream source `eb77f09e3c06c28e860d92e0de941b143c2eecec` was checked out and its Gateway 4.0.103 source/tests inspected. Explicit `createGateway({apiKey})` still sends Bearer credentials. No upstream pins, generated versions or changelogs were changed.

## Passing local checks

The original implementation passed the following checks on dependency base `323b6524`; subsequent restack validation is recorded separately below:

- `mise run check` (format, vet, lint, docs, candidate SDK/provider tests including Grafana client tests, and boundary).
- `mise run build`, `mise run test-ai-gateway`, `mise run test-integration`.
- `mise run test-ai-gateway-source-integration`: all 78 command tests passed, including both clients, YAML JWT, static discovery/unary/streaming, rejection, restart and secret checks; rerun after the final schema fix.
- `mise run parity-check`: complete baseline, provider shape/coverage, conformance, TypeScript typecheck and differential suites.
- `mise run lint-docs`, `test-ci-workflow`, `verify-gateway-workspace`, `verify-ai-gateway-boundary`, `verify-merged-pins`, `release-check`.
- `bash test/module-policy.sh` and `bash scripts/module-policy.sh isolation` (the commands behind the corresponding mise tasks).
- Candidate-workspace `go test -race ./cmd/grafana-ai-gateway/internal/...`; config/process race tests rerun after final schema validation.
- Strict OpenSpec validation and final diff whitespace/format checks.

Test-only macOS adjustments: `GODEBUG=x509sslcertoverrideplatform=1` lets the pinned Go toolchain honor fixture `SSL_CERT_FILE`; `TMPDIR=/private/tmp` avoids `/var` versus `/private/var` fixture path mismatches. Module-policy/isolation also used pinned Go 1.27.1 and GNU coreutils ahead of shims/BSD mktemp in PATH. Fixture commits used `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=commit.gpgsign GIT_CONFIG_VALUE_0=false` to avoid 1Password signing availability; no repository signing configuration was changed. Parity used `AI_SDK_UPSTREAM_ROOT=/tmp/self-hosted-auth-upstream` at the exact registered commit.

The original unchanged dependency reproduced the TLS failures (65/77 pass without the override, 77/77 with it) and module-policy path/tool selection failures. No production trust or validation was weakened to accommodate test fakes. An initial final-base integration invocation omitted the TLS override and failed; its corrected full rerun passed.

## Independent security review

Fresh read-only review covered configuration, snapshots, exact header admission, constant-time full digest scans, local identity, policy isolation, export exception, attribution privacy and tests. Two supported P2 findings were fixed:

- YAML merge/alias indirection bypassed presence checks. Reject both constructs before typed decoding; null/merge/alias regressions pass.
- YAML scalar coercion accepted boolean environment references or numeric identity names. Auth-only AST scalar-tag validation now requires strings, with rejection before secrets/binding and positive quoted-string regressions. Root mapping keys also require string tags, rejecting binary-tag encoded auth keys before secret lookup; config/process race regressions pass. Server mappings and keys are checked, and explicitly configured Cloud enabled must carry an actual YAML boolean tag; quoted yes/off and null are rejected.

The independent reviewer rechecked fixes and the restacked dependency composition. No provider recordings were fabricated; deterministic native responses are focused integration fixtures, not recorded-provider evidence.

## Remaining acceptance gap

`mise run test-ai-gateway-image-source` reported one explicit skip: Linux Docker is required. The existing source-image test now includes production static-key discovery, unauthorized requests and Cloud-port closure inside the container network, using native production endpoints and disabled export. Neither that production image bootstrap nor existing image source/license packaging assertions were executed locally. Linux Docker CI acceptance remains outstanding. These local checks do not establish external CI or deployment acceptance.

## Temporary-buffer cleanup follow-up

Both static-key hashing paths now validate before copying into a bounded 4096-byte owned buffer and explicitly clear populated bytes after hashing. Environment values and HTTP headers remain unchanged; this makes no compiler/runtime/hash-internal memory-erasure claim. Focused tests verify zeroed buffer contents, exact digests, invalid/oversized keys, real environment preservation and both header forms. Candidate auth/process/service race tests, source integration (78 command tests), repository lint and docs checks passed after the follow-up, using the previously documented test-only TLS override. Upstream client wire behavior and keyEnv provisioning remain unchanged.

Pinned Go 1.27.1 darwin/arm64 test-binary disassembly shows `digestAndClear` calling SHA-256 and its deferred cleanup calling `runtime.memclrNoHeapPointers`; this confirms emitted cleanup in that local build, not a cross-toolchain secure-erasure guarantee.

## Dependency and cleanup reviews before draft publication

Fresh read-only review of #369 at `ea5bfa03` found no concrete code findings in auth/account boundaries, listener lifecycle, BYOK construction and independent observation policies. This was source review against the dependency diff and exact pinned upstream, not new execution of its standalone branch or deployment validation. Its current GitHub checks pass; image jobs are conditionally skipped.

The independent auth reviewer also reviewed the application-owned buffer cleanup and reran focused auth/process tests, finding no actionable issues. Environment-backed provisioning and HTTP headers are preserved; only temporary owned copies are cleared. No GitHub review comments were posted.

## Final restack validation

On dependency `ea5bfa03`, `mise run check`, `mise run build`, `mise run parity-check`, full candidate Gateway internal race tests and `mise run test-ai-gateway-source-integration` pass (47 runtime integration tests and 79 command tests, zero skips). Workflow/release checks, Gateway workspace/boundary, merged pins, module-policy and SDK-without-Gateway isolation checks pass. Strict OpenSpec validation passes all 99 specs, with no active changes.

The first frontend integration run passed 139/140 tests; `useChat delivers updated metadata and transient data without retaining it` observed the updated message but its React history probe had not recorded the updated metadata. A separate full rerun passed all 140/140 without source changes. This non-reproducing test failure is recorded rather than silently treated as first-run success. The macOS/toolchain accommodations listed above remain test-only.

The restacked image-source check again explicitly skipped its one test because this host is not Linux with Docker. Production container bootstrap and packaging acceptance remain unexecuted here.
