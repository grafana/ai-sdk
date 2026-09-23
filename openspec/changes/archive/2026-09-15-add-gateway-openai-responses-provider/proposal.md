## Why

The authenticated Gateway command can only route public models to direct Anthropic. Work package #119 lets operators configure named OpenAI Responses instances through `providers/openai`, with the same bounded transport, startup validation and backend privacy as the Anthropic composition.

## What Changes

- Add an `openai` provider type to the Gateway command model configuration. Each named instance requires `apiKeyEnv` and accepts an optional `baseURL` that defaults to `https://api.openai.com/v1`.
- Construct OpenAI Responses models from explicit client options over the shared hardened model client, so ambient `OPENAI_*` SDK environment configuration cannot redirect credentials or add request headers.
- State in the shared model limit flag help text that the limits govern all provider types.

## Capabilities

### New Capabilities

- `gateway-provider-configuration`: Gateway command provider instance configuration for OpenAI Responses backends, covering startup validation, explicit-option construction over the bounded model transport, and privacy of backend identity.

### Modified Capabilities

None.

## Impact

- `ai-gateway/cmd/grafana-ai-gateway/internal/config`: provider type validation and flag help text.
- `ai-gateway/cmd/grafana-ai-gateway/internal/service`: catalog construction for the new type.
- `ai-gateway/go.mod`: requires the pinned `providers/openai` SDK module and `openai-go/v3`.
- `ai-gateway/test/providerwire-v4/gateway-command.test.ts`: real-process tests against a fake Responses backend.
- `test/conformance/PARITY.md` and the Gateway README record the new provider type.
- Existing Anthropic configurations keep their behavior.
