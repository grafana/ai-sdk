## Why

Provider-defined tools and provider-executed results need a consistent contract across direct model calls, streaming orchestration, and the Go Gateway client. Today, tool variants can carry incompatible fields, optional markers lose meaningful presence, and the client cannot consume the corresponding results safely. These SDK boundaries must work independently of whether the Gateway service accepts the new capability.

## What Changes

- **BREAKING**: Validate function and provider tool variants before native I/O; treat omitted Go provider args as an empty object without relaxing HTTP validation.
- Preserve unary and streaming marker presence while correcting streaming input-start inference. Preserve the distinct text-stream and UI classifications of known tools.
- Extend the bounded Go Gateway client to decode provider calls, results, and opaque continuation metadata, including deferred results without a repeated call.
- **BREAKING**: Distinguish omitted Anthropic MCP token/enabled options from explicitly empty or false values. Respect caller deadlines for large-budget unary Anthropic requests without changing the token budget or overriding explicit timeouts.
- Leave Gateway service activation out of this slice. Retain unsupported provider-tool request capabilities and actual native Anthropic MCP consumption checks, without blanket rejection of foreign options or opaque metadata.

## Capabilities

### New Capabilities

- `sdk-provider-tools`: direct tool validation, bounded opaque client metadata, and the service-activation boundary.
- `anthropic-unary-context-deadline`: context-derived default request timeouts for unary calls with large token budgets.

### Modified Capabilities

- `v4-tool-type-split`: validate exclusive tool variants and normalize nil Go provider args.
- `v4-tool-result-alignment`: normalize result markers and preserve input-start dynamic presence across text and UI streams.
- `grafana-gateway-client`: consume bounded provider calls/results and opaque metadata.
- `mcp-server-tools`: preserve optional native MCP configuration presence.

## Impact

Affects provider types, native request conversion, stream/UI projection, the standalone Go Gateway client, and their tests. Existing provider-domain marker field types remain unchanged; explicit Anthropic MCP token/enabled values use pointers. The SDK decoding extension does not itself activate Gateway execution. Service checks remain scoped to unsupported request capabilities and actual native consumption. The registered upstream reference is `ee3169b3c4880e2abe4d0d7c781243bb81822ec4` (ai 7.0.116, provider 4.0.18, Gateway 4.0.94, Anthropic 4.0.65). Native option forwarding, reusable fallback and opaque metadata transport are inherited prerequisites, not replaced by this SDK change.
