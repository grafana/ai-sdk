# Verification

## Branch and dependency

Implemented on `jeff/self-hosted-gateway-auth`, based on draft #369 at `323b6524f1c02cf2f4cf0a91b61e409a3741c88f` (`nrbrd/byok`, base `nrbrd/failure-visibility`). Restacked from `55a80621b3c27d966e1bb8b86cb6b67a65cad844` after the dependency moved; earlier-base results were superseded by the checks below. Import and documentation conflicts preserved upstream attribution tests and new listener defaults (private 8082, Cloud 8080, operational 8081).

#367 remained open at `f45a83be727999cceb2524dfc331f8c0f5a98c90`; #368 remained open at `fca4aeb33a4cc9a6787972a90aaaaca20dbf9c0a`. #248 remains a separate draft startup/router overlap, not a prerequisite. The inherited active `activate-gateway-auth-and-request-byok` change is unchanged. Its deployment gate and eventual spec composition remain owned by the dependency; archiving this standalone capability does not satisfy that gate or establish Cloud deployment readiness.

The exact registered upstream source `eb77f09e3c06c28e860d92e0de941b143c2eecec` was checked out and its Gateway 4.0.103 source/tests inspected. Explicit `createGateway({apiKey})` still sends Bearer credentials. No upstream pins, generated versions or changelogs were changed.

## Passing local checks

On the final dependency base, broad checks completed before the final scalar-tag fix; affected config/process race tests, full source integration, lint and docs checks were rerun afterward:

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

`mise run test-ai-gateway-image-source` reported one explicit skip: Linux Docker is required. The existing source-image test now includes production static-key discovery, unauthorized requests and Cloud-port closure inside the container network, using native production endpoints and disabled export. Neither that production image bootstrap nor existing image source/license packaging assertions were executed locally. Linux Docker CI acceptance remains outstanding. No external CI, deployment, merge, push, release or comments were performed.

## Temporary-buffer cleanup follow-up

Both static-key hashing paths now validate before copying into a bounded 4096-byte owned buffer and explicitly clear populated bytes after hashing. Environment values and HTTP headers remain unchanged; this makes no compiler/runtime/hash-internal memory-erasure claim. Focused tests verify zeroed buffer contents, exact digests, invalid/oversized keys, real environment preservation and both header forms. Candidate auth/process/service race tests, source integration (78 command tests), repository lint and docs checks passed after the follow-up, using the previously documented test-only TLS override. Upstream client wire behavior and keyEnv provisioning remain unchanged.

Pinned Go 1.27.1 darwin/arm64 test-binary disassembly shows `digestAndClear` calling SHA-256 and its deferred cleanup calling `runtime.memclrNoHeapPointers`; this confirms emitted cleanup in that local build, not a cross-toolchain secure-erasure guarantee.
