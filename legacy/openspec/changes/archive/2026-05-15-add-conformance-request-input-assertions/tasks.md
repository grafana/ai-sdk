## 1. Request Snapshot Model

- [x] 1.1 Add a normalized request snapshot shape for conformance fixtures with `method`, `path`, `headers`, and decoded JSON `body`.
- [x] 1.2 Add provider-specific header allowlist and redaction helpers for TypeScript tooling, starting with Anthropic behavior-affecting headers.
- [x] 1.3 Add matching header normalization and redaction helpers for the Go conformance runner.
- [x] 1.4 Add stable JSONL writing for request snapshots so generated fixture diffs are deterministic.

## 2. TypeScript Capture

- [x] 2.1 Update `test/conformance/tools/generate.mts` replay server to capture each upstream provider request before serving fixture responses.
- [x] 2.2 Update `generate.mts` to write `expected-requests.jsonl` with one normalized snapshot per provider request.
- [x] 2.3 Update `test/conformance/tools/record.mts` recording proxy to capture upstream provider requests and write redacted `expected-requests.jsonl`.
- [x] 2.4 Keep existing `input*.chunks.txt`, `expected.jsonl`, and `expected-object.json` generation behavior unchanged except for the new request snapshot output.

## 3. Go Runner Comparison

- [x] 3.1 Extend `test/conformance/runner.go` replay server to capture actual Go provider request snapshots in request order.
- [x] 3.2 Add `expected-requests.jsonl` loading to the Go conformance runner.
- [x] 3.3 Compare request counts, method, path, normalized headers, and decoded JSON bodies after the stream finishes.
- [x] 3.4 Ensure JSON object field order is ignored while array ordering and multi-step request ordering remain strict.
- [x] 3.5 Add focused tests for request comparison behavior: object order ignored, array order enforced, header casing normalized, volatile headers ignored, and secrets redacted.

## 4. Provider Wiring

- [x] 4.1 Wire direct Anthropic conformance tests to compare captured replay-server requests with upstream snapshots.
- [x] 4.2 Wire Grafana provider-wire conformance tests so the downstream Anthropic replay requests are compared against upstream snapshots.
- [x] 4.3 Keep Grafana provider-wire method, route, auth, streaming, content-type, accept, and body-decode validations at the provider-wire boundary.

## 5. Fixtures And Docs

- [x] 5.1 Regenerate request snapshots for existing conformance fixtures using the TypeScript generator.
- [x] 5.2 Review generated request snapshot diffs for secrets, volatile headers, and unexpected default differences.
- [x] 5.3 Align concrete request mismatches or document narrow follow-up issues instead of adding broad default normalization.
- [x] 5.4 Update `test/conformance/README.md` with the `expected-requests.jsonl` format and request assertion strategy.

## 6. Verification

- [x] 6.1 Run TypeScript tooling checks for the conformance tools.
- [x] 6.2 Run Go tests for the conformance module with the `conformance` build tag.
- [x] 6.3 Run `make test-conformance` from the repository root.
- [x] 6.4 Run `make fmt` and targeted Go tests affected by request snapshot helpers.
