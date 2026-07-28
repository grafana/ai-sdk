## Why

Conformance tests currently prove that Go SDK model output matches upstream TypeScript `UIMessageChunk` output for the same provider response fixtures, but they do not prove that both SDKs send equivalent provider request inputs to produce those responses. Request bodies and behavior-affecting headers carry model messages, tools, provider options, output settings, beta flags, and transport routing metadata, so request drift can remain invisible when fixture replay still yields matching output.

## What Changes

- Extend conformance fixture generation to capture upstream TypeScript request inputs alongside `expected.jsonl` output.
- Add request-input fixtures that store normalized request snapshots for each provider API call in a test case, including the JSON body and selected behavior-affecting headers.
- Extend the Go conformance replay server to capture actual Go provider requests and compare them against the expected upstream request snapshots.
- Compare JSON request bodies semantically after decoding, so object field ordering does not matter while missing, extra, or different fields still fail the test.
- Preserve exact ordering for semantically ordered values such as message arrays, tool arrays, content arrays, stop sequences, and multi-step request sequence order.
- Document the request assertion strategy and fixture format so future default-value or SDK-specific ordering problems can be handled intentionally as they appear.

## Capabilities

### New Capabilities

_None. This extends the existing conformance testing capability._

### Modified Capabilities

- `conformance-testing`: Add request input fixture capture and order-insensitive semantic request input assertions for conformance replay tests.

## Impact

- **Code**: `test/conformance/runner.go`, `test/conformance/tools/generate.mts`, `test/conformance/tools/record.mts`, shared TypeScript conformance helpers, and provider-specific conformance server wiring.
- **Fixtures**: Adds expected request input files next to existing `expected.jsonl` files under `test/conformance/<provider>/{upstream,recorded}/<case>/`.
- **Tests**: `make test-conformance` will validate both model output chunks and request inputs for fixtures that include request expectations.
- **Production APIs**: No changes to public SDK APIs or provider behavior.
