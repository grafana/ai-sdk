## Why

The SDK prerequisite in #238 can represent provider tools, but the Gateway still rejects them. This second WP13 slice enables bounded provider-tool transport independently of remote MCP configuration and its routing policy.

## What Changes

- Accept strict provider definitions and assistant provider-call/result history without changing function-tool validation.
- Encode ordered unary/SSE calls, results and execution markers with reviewed non-MCP metadata.
- Correlate deferred results with unresolved provider-owned history; permit previews only before a final result.
- Prove both-client behavior through the real handler and authenticated native Anthropic code-execution transport.
- Keep all nonempty root provider options, MCP metadata, effectful fallback and later output families rejected.

## Capabilities

### New Capabilities

- `gateway-provider-tools`: bounded direct-route definitions, results, lifecycle, ordinary continuation metadata, privacy and evidence.

### Modified Capabilities

- `gateway-unary-function-tools`: extend private unary output and history with provider calls/results.
- `gateway-streaming-function-tools`: extend the existing bounded stream lifecycle with provider calls/results and previews.

## Impact

Depends on `sdk-provider-tool-contract`; uses its already-published Apache pins with `GOWORK=off`. Changes AGPL request/output mapping, test-only schemas, deterministic transport tests, docs and parity coverage. Registered upstream versions and authentic provider fixture inputs remain unchanged. The subsequent `gateway-anthropic-mcp` change owns remote MCP support. All changes remain unarchived for review.
