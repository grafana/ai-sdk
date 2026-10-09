## Why

The SDK prerequisite in #238 can represent provider tools, but the Gateway still rejects them. This second WP13 slice enables bounded provider-tool transport and the existing native providers' hosted-MCP request semantics.

## What Changes

- Accept strict provider definitions and assistant provider-call/result history without changing function-tool validation.
- Encode ordered unary/SSE calls, results and execution markers with bounded opaque metadata and generic tool correlation and native-only provider interpretation.
- Correlate deferred results with unresolved provider-owned history; permit previews only before a final result.
- Prove both-client behavior through the real handler and authenticated native Anthropic code-execution transport.
- Forward native Anthropic MCP configuration/history and OpenAI MCP tool definitions without a Gateway hosted-MCP capability gate. Preserve opaque foreign options and metadata, native authority guards and reusable fallback; document possible repeated billing and remote effects.

## Capabilities

### New Capabilities

- `gateway-provider-tools`: bounded definitions, results, lifecycle, opaque continuation metadata and independently controlled telemetry capture.

### Modified Capabilities

- `gateway-unary-function-tools`: extend private unary output and history with provider calls/results.
- `gateway-streaming-function-tools`: extend the existing bounded stream lifecycle with provider calls/results and previews.

## Impact

Depends on `sdk-provider-tool-contract`; uses its already-published Apache pins with `GOWORK=off`. Changes AGPL request/output mapping, test-only schemas, deterministic transport tests, docs and parity coverage. Registered upstream versions and authentic provider fixture inputs remain unchanged. The subsequent `gateway-anthropic-mcp` change retains separately scoped native configuration validation and continuation evidence. The completed runtime change is archived.
