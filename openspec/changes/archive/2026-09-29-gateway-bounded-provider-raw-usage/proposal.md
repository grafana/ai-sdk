## Why

Gateway currently drops `provider.Usage.Raw` from unary success and streaming finish, so authenticated callers lose provider-native usage detail available in direct SDK results. In #201's two-client matrix, both the Go client and registered TypeScript client fail the unchanged `anthropic/recorded/usage-compaction-advisor` and `usage-fallback` usage goldens solely on per-step `usage.raw`, while normalized token counts match. On this branch the ProviderWire V4 testserver registers fake models only; recorded-fixture replay must be wired before those failures can serve as executable local acceptance evidence.

## What Changes

- Return an optional, valid, bounded provider JSON object under `usage.raw` in supported unary and streaming finish responses; preserve absent versus present without filtering provider-native keys. Do not add new top-level response fields.
- Fail closed on malformed, non-object, explicit null, oversized, invalid UTF-8, or unpaired escaped-surrogate supplied raw usage before Go normalization in server and Go client unary/stream paths; preserve normalized-count validation and safe pre-commit unary / post-commit stream error behavior.
- Preserve and validate optional `usage.raw` in the Go client for both methods while keeping unrelated private metadata filtered; prove the public registered TypeScript client consumes the same results.
- Update strict response schemas and focused raw HTTP, client, privacy/telemetry and both-client tests; establish deterministic streaming replay of both unchanged recorded Anthropic inputs through the real handler for both clients (or require #201's harness first), then run the recorded streaming matrix and direct conformance without modifying inputs or usage goldens. Cover unary separately with deterministic synthetic/focused tests; recorded streaming inputs do not establish unary response behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: Optional caller-visible raw usage, bounds and unary error/privacy behavior.
- `providerwire-v4-streaming-runtime`: Optional raw usage on finish, bounds and safe terminal error behavior.
- `grafana-gateway-client`: Decode and preserve optional bounded object raw usage for unary and finish, rejecting invalid supplied values.

## Impact

Affects `ai-gateway/providerwire/v4/{response.go,stream.go,schema/unary_success.json,schema/stream_event.json}`, `providers/grafana/{model.go,stream.go}`, their unit/HTTP tests, and the registered two-client Gateway matrix under `ai-gateway/test/providerwire-v4/`. `provider.Usage.Raw` already exists; request protocol, UI chunks, direct provider snapshots, and gateway-owned accounting/telemetry are unchanged. Reference is the registered `@ai-sdk/provider@4.0.17` and `@ai-sdk/gateway@4.0.88` in `test/conformance/upstream.yaml`, not an inference about Vercel's private server.
