## Why

Provider-defined tools and provider-executed results need a consistent contract across direct model calls, streaming orchestration, and the Go Gateway client. Today, tool variants can carry incompatible fields, optional markers lose meaningful presence, and the client cannot consume the corresponding results safely. These SDK boundaries must work independently of whether the Gateway service accepts the new capability.

## What Changes

- **BREAKING**: Validate function and provider tool variants before native I/O; treat omitted Go provider args as an empty object without relaxing HTTP validation.
- **BREAKING**: Normalize unary dynamic and preliminary markers while retaining the presence of streaming input-start dynamic. Preserve the distinct text-stream and UI classifications of known tools.
- Extend the bounded Go Gateway client to decode provider calls, results, and reviewed continuation metadata, including deferred results without a repeated call.
- **BREAKING**: Distinguish omitted Anthropic MCP token/enabled options from explicitly empty or false values. Respect caller deadlines for large-budget unary Anthropic requests without changing the token budget or overriding explicit timeouts.
- Keep the Gateway service's existing rejection of provider tools and MCP options until it has corresponding validation and routing support.

## Capabilities

### New Capabilities

- `sdk-provider-tools`: direct tool validation, bounded client metadata projection, and the service-activation boundary.
- `anthropic-unary-context-deadline`: context-derived default request timeouts for unary calls with large token budgets.

### Modified Capabilities

- `v4-tool-type-split`: validate exclusive tool variants and normalize nil Go provider args.
- `v4-tool-result-alignment`: normalize result markers and preserve input-start dynamic presence across text and UI streams.
- `grafana-gateway-client`: consume bounded provider calls/results and reviewed metadata.
- `mcp-server-tools`: preserve optional native MCP configuration presence.

## Impact

Affects provider types, native request conversion, stream/UI projection, the standalone Go Gateway client, and their tests. Some Go field types change; existing consumers must migrate. The Gateway service continues to reject the newly decodable capabilities until service-side support is added. The registered upstream reference is `08ae5ad05bc12496dd1ffcf64e34419e0831300d` (ai 7.0.107, provider 4.0.17, Gateway 4.0.87, Anthropic 4.0.58).
