## 1. Establish transport regression evidence

- [ ] 1.1 Recheck the registered upstream implementations/tests and classify this work as native provider implementation plus core result propagation; confirm no baseline upgrade is needed.
- [ ] 1.2 Add failing deterministic HTTP tests for Anthropic ordinary-header precedence and beta union in both modes, and returned outbound JSON/response headers/full unary JSON for both adapters. Preserve existing OpenAI header coverage.
- [ ] 1.3 Prove the Anthropic public SDK raw-response path matches the current generated streaming request, feature beta translation, initial error wrapping, and decoder ownership; stop and revise the design if it cannot preserve the existing client/auth contract.

## 2. Preserve headers and exchange metadata

- [ ] 2.1 Forward Anthropic per-call ordinary headers and compose the normalized configured/call/feature beta union without mutating reusable configuration.
- [ ] 2.2 Add invocation-local request capture and response hooks in each adapter; populate existing unary/stream result fields and full Anthropic unary response JSON. Cover body-changing options and non-JSON metadata omission.
- [ ] 2.3 Attach HTTP headers to existing provider response-metadata events without duplicating events; add core StreamText, GenerateText, and Agent step/final-result tests, including the actual Anthropic Agent marker.
- [ ] 2.4 Add retry/concurrent-call tests and fake Vertex/preconfigured OpenAI/Mantle transport tests proving outbound-body accuracy, replay/signing preservation, and capture isolation.

## 3. Expose opt-in raw events

- [ ] 3.1 Add failing raw-output tests for omitted/false/true selection, normal and ignored events, Anthropic pings, framing-only input, valid JSON typed failures, invalid JSON, JSON null, and post-preflight error envelopes.
- [ ] 3.2 Extend OpenAI stream items with raw evidence through pump/preflight/consumption, preserving recoverable malformed handling, accepted-stream grace, and buffered raw-before-normalized ordering.
- [ ] 3.3 Move Anthropic streaming to the proven SDK raw-response/frame boundary and existing union/adapter conversion; preserve initial error classification and lifecycle while exposing frame evidence before typed handling.
- [ ] 3.4 Assert normalized output is invariant under raw selection, initial failures still return no stream, and post-preflight raw precedes errors; classify any upstream decoding/recovery mismatch without broadening scope.
- [ ] 3.5 Add cancellation/stalled-consumer and custom-decoder tests proving bounded queues, termination, and exactly-once framing/resource ownership.

## 4. Validate privacy, parity, and delivery

- [ ] 4.1 Run Gateway privacy/raw-rejection regressions with enriched native metadata; verify no request/response transport data is added to public projection or automatic logs.
- [ ] 4.2 Run affected provider/core tests, race tests for concurrent/cancellation paths, format/vet/lint, and documented public-module resolution checks, including Mantle consumer coverage.
- [ ] 4.3 Assess fixture impact and run `mise run parity-check`; retain authentic fixture provenance. If frontend wire behavior changes, add a deterministic schema-parsed scenario and run `mise run test-integration`.
- [ ] 4.4 Record the outbound-body observability adaptation and newly proven coverage in the appropriate parity documentation; correct directly affected stale header documentation without claiming full schema parity.
- [ ] 4.5 Complete the requested review/fix loop for the entire implementation branch diff (maximum three rounds), rerun final validation, and make focused conventional commits; do not open a PR unless requested.
