## Why

ProviderWire V4 rejects every request that carries provider options or body-carried call headers. The registered client and the Go Gateway client both send these fields, so callers that set either one receive a 400, and the Go `ToolLoopAgent` sets a `User-Agent` call header of its own, so every agent call against a Gateway model fails. Work package 21 (#115) makes both fields reach the selected backend without letting a caller reach past the decisions and credentials the host owns.

## What Changes

- Map call-level, message-level and text-part provider options to opaque provider options, preserving nested JSON byte for byte.
- Map body-carried call headers to provider call headers, preserving each key's original case, and refuse two names differing only in case.
- Reject the reserved `grafana` namespace, and refuse an openai-compatible `providerName` that resolves to it at startup.
- Refuse credential-bearing body headers.
- Refuse provider-option fields that name a decision the runtime already made, in any spelling a provider reads, including a `content` field that carries image or file parts underneath the field checks.
- After resolution, forward only the namespaces the selected backend reads, and for Anthropic only the fields it reads, through a `catalog.ProviderOptionPolicy` the catalog carries per model.
- Retire the `provider-options` and `body-headers` unsupported-capability documents and add three fixed policy documents.

## Capabilities

### Modified Capabilities

- `providerwire-v4-unary-runtime`: provider options and body headers move from refused families to mapped values, with a reserved namespace, protected headers, protected fields and selected-backend forwarding.

## Impact

- `ai-gateway/catalog`: new exported `ProviderOptionPolicy` on `ResolvedModel`, `StaticEntry` and `RegistryRoute`. Its zero value forwards no provider options, so a library caller building a catalog must set one to forward any.
- `ai-gateway/providerwire/v4`: mapping, refusals and post-resolution filtering; three new error documents in `schema/error.json`.
- `ai-gateway/cmd/grafana-ai-gateway`: per-provider policies for `anthropic`, `openai` and `openai-compatible` in `internal/service`, a shared policy only where fallback candidates agree, and the reserved `providerName` check in `internal/config`.
- Deviates from #115 in two places, recorded in `design.md`: the reserved namespace is rejected rather than removed, and openai-compatible fields are forwarded unrestricted pending a decision on #115.
- `providers/anthropic` ignores `CallOptions.Headers`, so call headers do not reach that backend; recorded in `test/conformance/PARITY.md`.
