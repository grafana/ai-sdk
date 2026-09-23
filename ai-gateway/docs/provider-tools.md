# Provider tools on direct routes

The strict ProviderWire V4 Gateway accepts registered function and provider
(tool ID, name, object args) definitions in unary and streaming requests to
direct public models. It does not execute application tools or store a session.
Provider-executed calls and results are forwarded in order; an absent or false
`providerExecuted` call remains client-owned. Streaming tool-input IDs and
preliminary results have bounded lifecycles. A provider-executed call can finish
without a result and receive that result in a later request when the unresolved
call is included in assistant history. An unfinished preliminary series fails
safely; an ordinary call without a result may finish normally. Provider-specific
image previews emitted before their call are deferred with generated media
(WP16), not silently accepted as correlated WP13 results.

## Anthropic-hosted MCP

An authenticated caller of a **direct Anthropic route** may supply the
registered `providerOptions.anthropic.mcpServers` array. This is the only
nonempty root provider-option family enabled before general provider options
(WP21). The provider receives the caller-supplied server name, URL, optional
authorization token and tool configuration in the native Anthropic request.
Calls to non-Anthropic and fallback routes reject MCP configuration before a
physical request. A nonempty MCP server list is effectful even without tool
definitions and cannot be retried across fallback candidates.

The strict adapter requires distinct nonempty server names and HTTPS URLs
without embedded user credentials or fragments. Server definitions and nested
settings are bounded, and unsupported option keys fail before native I/O.
Native Anthropic `toolConfiguration.enabled` preserves absent versus explicit
false, and `authorizationToken` preserves absent versus explicit empty. The
Gateway authenticates requests before decoding bodies; it does **not** connect
to the MCP URL itself. Anthropic performs the remote connection, so operators
must review destinations, authorization scope and Anthropic's remote egress
controls before deployment. These checks are Grafana host policy, not a claim
about Vercel's unpublished hosted Gateway policy.

The Gateway's total model-duration limit supplies a context deadline for
Anthropic unary requests. The Go Anthropic SDK uses that deadline as its
per-attempt timeout when the configured model has a large default output
budget; explicit native timeout options still take precedence. The Gateway
does not rewrite `maxOutputTokens`. Direct Go callers without a deadline or
explicit timeout retain the SDK's own non-streaming guard.

A validated MCP tool call/result can carry only the configured server's name
and the registered `mcp-tool-use` type in public `anthropic` tool metadata.
That name is caller-selected, not a backend model or provider instance ID.
Neither the URL nor authorization token enters normalized output, public errors,
logs, metrics or metadata-only Agent Observability. The selected native
Anthropic request necessarily contains them, and a Go client's caller-owned
`Request.Body` contains the serialized request. Do not log either request
body. Other supported tool metadata is projected from reviewed Anthropic or
OpenAI/Azure correlation fields; arbitrary provider metadata is discarded.

## Remaining boundaries

Provider tools that also emit sources, generated files, reasoning, custom
content, or approvals still encounter those families' explicit safe failure
until their owning work packages land. Nested tool-result output/content
provider options and unrestricted root options remain deferred. Native
provider conversions for unsupported tool IDs retain their documented warning
behavior. Gateway tests cover deterministic fake-provider native requests and
client interoperability; they do not claim a live MCP network recording or a
verified deployed Anthropic egress policy. Production rollout must separately
validate secrets, notices, source offer, backend network policy and capability
smoke with its approved configuration.

See [Grafana Go client](../../docs/providers/grafana-gateway.md), [Anthropic
provider](../../docs/providers/anthropic.md) and the [pinned parity baseline](../../test/conformance/upstream.yaml).
