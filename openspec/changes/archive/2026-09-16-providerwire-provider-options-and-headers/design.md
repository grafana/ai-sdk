## Context

ProviderWire V4 refused every request carrying provider options or body-carried call headers. The request schema already modelled both fields, the TypeScript schema cases already accepted and rejected the right shapes, and committed goldens already pinned the target behavior. The gap was the Go mapper, and then what a caller could do once options reached a provider.

Providers merge caller option fields into what they send. `providers/openai-compatible` spreads unknown call-level fields into the request body and message or part metadata over the entry it builds. `providers/anthropic` decodes options with `encoding/json`, which matches field names case-insensitively. A caller who is not the account owner could therefore restate, through a provider namespace, a decision the runtime had already made: the catalog model, the prompt, tools, structured output or the stream transport.

### Registered baseline recheck

Source review used the registered versions in `test/conformance/upstream.yaml`.

| Evidence | Finding |
| --- | --- |
| `test/conformance/upstream.yaml` | Registered versions include `ai@7.0.65`, `@ai-sdk/gateway@4.0.52` and `@ai-sdk/openai-compatible@3.0.30`. |
| `@ai-sdk/gateway@4.0.52` language model | The client retains call-level `headers` and provider options in the body. |
| `ai@7.0.65` `generateText` | Adds a `user-agent` body header and sends `toolChoice: {"type":"auto"}` with no tools, captured from the client. |
| `@ai-sdk/openai-compatible@3.0.30` `getArgs` | Spreads unknown provider-option fields after `model` through `seed`, before `reasoning_effort`, `verbosity`, `messages`, `tools` and `tool_choice`. |
| `@ai-sdk/openai-compatible@3.0.30` `convertToOpenAICompatibleChatMessages` | Spreads `openaiCompatible` message and part metadata last, over `role`, `content`, `type` and `text`. |
| `providers/anthropic` | Reads the `anthropic` namespace only, through `AnthropicOptions`, `AnthropicSystemMessageOptions` and helpers for cache control, citations, document metadata and reasoning signatures. |

The Vercel Gateway has no equivalent of a caller who is not the account owner, so the refusals and the policy below are Grafana host behavior with no upstream counterpart.

## Goals / Non-Goals

### Goals

- Deliver call, message and text-part provider options and body-carried call headers to the selected backend.
- Keep the host's own decisions, and its credentials, out of a caller's reach.
- Forward only the options the selected backend reads.
- Fail safe for a provider type nobody has classified.

### Non-Goals

- Restricting openai-compatible fields, which is open on #115.
- Tool, file, reasoning and custom-content options, which belong to the packages that introduce those parts.
- Call headers for the direct Anthropic backend, which `providers/anthropic` does not read.

## Decisions

### Reject the reserved namespace rather than remove it

Work package 21 says to reserve and remove host-owned namespaces. This change rejects instead. A caller who received a 200 would have no way to learn their option was dropped, and `providers/openai-compatible` would otherwise splice a namespace that slipped through into the backend body. Configuration refuses an openai-compatible `providerName` that the provider resolves to `grafana`, including `grafana.chat` and padded spellings.

### Protect credential-bearing headers only

Providers apply call headers after their own authorization header, so a caller `Authorization` would replace the backend credential, and on Bedrock would enter the SigV4 canonical request. The refused set covers credential names, and a test drives the inbound Cloud edge with a valid stack assertion to assert every name it refuses is protected in the body. Protocol header names stay accepted and are not forwarded, because the HTTP contract retains a case-variant `AI-Language-Model-Id` in the body. Two body headers differing only in case are an invalid request, because either value could reach the backend.

### Refuse fields that name a decision this runtime made

A provider-option field named `model`, `fallbacks`, `messages`, `prompt`, `tools`, `toolChoice`, `functions`, `function_call`, `mcpServers`, `container`, `responseFormat`, `stream`, `streamOptions`, `role`, `tool_calls`, `tool_call_id`, `type`, `image_url`, `input_audio` or `file` is refused before resolution. Each would redirect the resolved call, run tools or models the catalog never granted on the host's credentials, or restore a capability the wire refuses. Names are compared after folding case and removing `_` and `-`, because an exact compare let `MCPServers` reach `mcp_servers`. Refusals stay before resolution, because they do not depend on the backend and the caller should learn of them.

### Forward only what the selected backend reads

Resolution is an in-process catalog lookup and bills nothing, so a second, backend-specific step runs after it. `catalog.ProviderOptionPolicy` lists a backend's namespaces and, optionally, its fields per namespace. The runtime drops other namespaces at call, message and part level without failing the request, because an option for another backend is one every provider ignores. Where fields are listed, other fields are removed; a namespace with nothing removed keeps its bytes.

### Restrict Anthropic fields, not openai-compatible fields

`providers/anthropic` ignores fields it does not read, so forwarding only those loses nothing it acts on, and a field it gains later starts off until someone classifies it. A test fails when its typed option structs gain an unclassified field. `providers/openai-compatible` forwards unknown fields on purpose, which is how vLLM or ollama parameters reach a backend. Its policy forwards the namespaces the provider reads for the configured name, checked against the provider itself, without restricting fields.

### Zero-value policy forwards nothing

A provider type added without a policy receives no caller options. The catalog code that constructs a provider is where its policy is set, so the omission is visible in review.

### Preserve a present-but-empty namespace

`{"providerOptions":{"ns":{}}}` maps to one namespace holding an empty object, while an absent or empty map maps to nothing. Go's `omitempty` cannot distinguish the two on the outbound path, so the inbound rule is explicit.

### Fixed documents, no caller echo

Namespace names, field names, header names and values are caller-controlled, so the three new error documents are static bytes like every other document in this runtime.

## Risks / Trade-offs

- **[openai-compatible fields stay unrestricted]** → The refused field set covers every field a registered provider is known to act on as a decision. Whether to restrict endpoint-specific fields, by default or through configuration, is raised on #115.
- **[A new Anthropic option is dropped until classified]** → The drift test fails for typed fields. Fields read by ad-hoc helpers are listed by hand in `service/provider_options.go` and have no automatic check.
- **[The openai-compatible policy mirrors unexported provider helpers]** → A test sends a marker through each candidate namespace to the real provider and requires the policy to match what it reads.
- **[A caller sending a refused name for an unrelated reason receives a 400]** → The refusal is explicit and fixed, and the names are listed in the unary runtime specification.
- **[#177 and #179 touch the same files]** → Whichever lands second rebases. #179 adds a provider type, which must set a policy or receive no caller options.

## Migration Plan

1. Merge with the command's catalog setting a policy for `anthropic` and `openai-compatible`.
2. No configuration changes are required, except renaming an openai-compatible provider whose `providerName` resolves to `grafana`, which now fails at startup.
3. Roll back by deploying the prior image; no persisted state or protocol version changes.

## Open Questions

- Should the Gateway restrict openai-compatible provider-option fields, either to typed fields by default with a per-provider allowlist in configuration, or not at all? Raised on #115.
