## Why

The authenticated Gateway command can only route public models to direct Anthropic. Work package #120 lets operators route public models to named OpenAI-compatible backends such as vLLM, Ollama and OpenAI itself, with the same privacy, bounded-transport and discovery guarantees as the Anthropic composition.

## What Changes

- Add an `openai-compatible` provider type to the Gateway command model configuration. Each named instance requires an API key environment reference and a base URL and accepts an optional provider name; the provider name is rejected for other provider types.
- Construct compatible models over the existing hardened model transport, so they share redirect rejection, the response-header timeout and the cumulative response byte bound with Anthropic. The existing `anthropic.response-*` limits govern both provider types; no flags are added.
- Request streaming usage from compatible backends so ProviderWire finish parts carry token counts.
- Keep backend identity private: discovery, public errors, logs and metrics never expose the configured provider name, provider label, base URL, API key or backend model ID.
- Name the valid provider types in the unsupported-type startup error.

## Capabilities

### New Capabilities

- `gateway-provider-configuration`: Gateway command provider instance configuration for OpenAI-compatible backends, covering startup validation, backend construction over the bounded model transport, streaming usage, cancellation and privacy of backend identity.

### Modified Capabilities

None.

## Impact

- `ai-gateway/cmd/grafana-ai-gateway/internal/config`: provider type validation and the optional `providerName` field.
- `ai-gateway/cmd/grafana-ai-gateway/internal/service`: catalog construction for the new type.
- `ai-gateway/go.mod`: requires the pinned `providers/openai-compatible` SDK module.
- `ai-gateway/test/providerwire-v4/gateway-command.test.ts`: real-process tests against a fake compatible backend.
- `test/conformance/PARITY.md`: the authenticated Gateway service-composition row records the compatible-provider evidence.
- Existing Anthropic configurations and flags keep their current behavior.
