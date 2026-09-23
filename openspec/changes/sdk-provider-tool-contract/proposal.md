## Why

WP13 (#107) needs a consistent provider-tool domain and independently usable clients before the Gateway enables new request or response families. This first PR extracts the Apache prerequisites from #237 and preserves existing Gateway capability rejection.

## What Changes

- **BREAKING**: normalize preliminary and unary dynamic markers to booleans, preserving streaming input-start dynamic presence and distinct text/UI inference.
- Validate direct tool variants before native I/O and project nil Go provider args as an empty object.
- Extend the independent Grafana client to consume provider calls/results and reviewed continuation metadata.
- **BREAKING**: preserve native Anthropic MCP token/enabled presence with pointer fields; derive unary SDK timeout defaults from caller deadlines without overriding explicit timeouts.
- Migrate existing Gateway boolean consumers and published dependency pins without enabling provider tools or MCP on the service.

## Capabilities

### New Capabilities

- `sdk-provider-tools`: direct tool validation and the boundary between SDK readiness and Gateway activation.
- `anthropic-unary-context-deadline`: bounded unary requests preserve the original token budget and explicit timeout precedence.

### Modified Capabilities

- `v4-tool-type-split`: reject mixed tool variants and normalize nil Go args.
- `v4-tool-result-alignment`: boolean normalization and presence-sensitive input-start inference.
- `grafana-gateway-client`: bounded provider-call/result decoding and reviewed metadata.
- `mcp-server-tools`: native MCP option omission versus explicit empty/false.

## Impact

Touches the provider domain, orchestration, native providers, Grafana client, observability and cross-language tests. Gateway changes only migrate compiled consumers and immutable Apache pins. The reference follows the merged baseline upgrade: `08ae5ad05bc12496dd1ffcf64e34419e0831300d` (ai 7.0.107, provider 4.0.17, Gateway 4.0.87, Anthropic 4.0.58). This change does not upgrade that baseline independently.

Follow-up changes `gateway-provider-tool-runtime` and `gateway-anthropic-mcp` own service activation. Each change remains unarchived for review; this PR does not close #107.
