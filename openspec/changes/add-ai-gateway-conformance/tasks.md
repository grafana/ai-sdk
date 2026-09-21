## 1. Lock the inventory and reference contract

- [x] 1.1 Reconfirm the registered upstream gateway/core source and current replay flows; identify reusable execution/comparison helpers and record relevant pins without upgrading them.
- [x] 1.2 Add failing harness tests for complete two-client discovery, newly added provider directories, unique row identities, filtered-run scope, and exclusion of provider-independent UI cases.
- [x] 1.3 Implement shared gateway inventory/result types and reconciliation; reject missing/duplicate rows and empty default inventories without capability allowlists.

## 2. Separate execution from expectation generation

- [x] 2.1 Refactor Go scenario execution to support the gateway model and collect output, errors, backend requests, and optional artifacts without weakening existing direct assertions.
- [x] 2.2 Refactor TypeScript scenario execution for reuse outside the generator, preserving high-level defaults, tools, approvals, configured messages, output options, expected stream errors, and existing unary operation semantics.
- [x] 2.3 Share or reuse established normalization/comparison behavior for both client results; add positive and negative unit controls for UI, request, usage, object, and unary mismatches.
- [x] 2.4 Verify the refactor leaves direct fixture results unchanged and gateway execution never writes inputs or goldens.

## 3. Add container-backed replay orchestration

- [x] 3.1 Add production-image build/reuse selection and run metadata capture, retaining pinned gateway dependencies and the module/license boundary.
- [x] 3.2 Add isolated test networking, replay framing for existing provider operations, test-only authentication/credentials, and per-attempt public-model to backend-model configuration.
- [x] 3.3 Add provider-isolated startup and readiness handling; preserve actual unsupported-provider setup evidence in every affected row without blocking other providers or substituting protocols.
- [x] 3.4 Add bounded execution and cleanup for containers, processes, listeners, and temporary configuration; test startup failure, scenario timeout, cancellation, and fresh multi-step state for each client.

## 4. Execute both gateway clients

- [x] 4.1 Add the pinned `@ai-sdk/gateway` executor against the container API prefix, using the shared high-level streaming and existing unary scenario paths without request rewriting.
- [x] 4.2 Add the `providers/grafana` executor against the same image with independent replay/tool state and no gateway implementation import.
- [x] 4.3 Update only necessary test dependency pins/lockfile entries and baseline validation coverage for the new consumers, preserving the registered package versions.
- [x] 4.4 Run all discovered provider fixtures through both paths without fail-fast; retain current unsupported features and providers as real failures.

## 5. Report compatibility and infrastructure failures

- [x] 5.1 Produce machine-readable results, provider/client summaries, stage-specific errors, applicable assertion diffs, and bounded credential-redacted artifacts; include missing backend request evidence after early rejection.
- [x] 5.2 Add harness tests proving one failed row does not hide later rows, global failures mark unexecuted rows honestly, and incomplete reports or failed assertions produce nonzero exit status.
- [x] 5.3 Add `mise run test-conformance-gateway` with full-matrix defaults and explicit local image/scenario/client selection for reproduction.

## 6. Add advisory CI and contributor guidance

- [x] 6.1 Add a separate full-matrix CI job that builds the current image, publishes available reports after failure, and retains a failing conclusion without `continue-on-error` success conversion.
- [x] 6.2 Verify existing direct/parity checks and publication/deployment dependencies remain unchanged; document maintainer verification that repository rules leave the gateway check non-required.
- [x] 6.3 Update the conformance README and `PARITY.md` with architecture, local reproduction, report interpretation, current coverage limitations, advisory status, and later promotion to required enforcement.

## 7. Validate the harness without requiring gateway parity

- [x] 7.1 Run focused harness tests, TypeScript typechecks, applicable Go checks, `mise run verify-ai-gateway-boundary`, and `mise run parity-check`; investigate any direct-suite regression rather than rebaseline it.
- [x] 7.2 Run the full gateway task and retain its initial failure report; verify exactly two rows per discovered provider fixture, correct setup-versus-execution attribution, continued execution after failures, and resource cleanup.
- [x] 7.3 Verify fixture inputs and expectations remain unchanged, classify observed differences without suppressing them, and confirm no production gateway fixes were bundled merely to improve the score.
- [x] 7.4 Validate the OpenSpec change and review implementation evidence against these requirements; accept a complete, trustworthy red matrix as delivery rather than requiring all compatibility rows to pass.
