## Why

Authorized developers cannot currently inspect configured route candidates through either normalized Gateway client without an inference call. Issue [#324](https://github.com/grafana/ai-sdk/issues/324), work package 37 of the revised Gateway plan, and the [approved #321 decision](../archive/2026-10-02-define-gateway-developer-evidence-contract/decision.md) establish a concrete discovery/access contract that can now be delivered independently of runtime diagnostics.

## What Changes

- Extend existing authenticated `/config` rows with `gateway: {canonicalModelId, aliases, candidates: [{providerInstance, provider, modelId}]}` from explicit route configuration, preserving candidate order and existing normalized row/specification behavior.
- Preserve those facts through optional `ModelInfo.Gateway *ConfiguredRoute` and typed candidate values on the existing Go `Provider.ListModels` API.
- Supply the approved documented, typechecked `fetchConfiguredModels({baseURL, headers, fetch, signal, maxBytes})` TS consumer helper against the same route. Stock pinned `getAvailableModels()` still strips the extension; no new endpoint or published TS package is introduced.
- Enforce row, alias, candidate and UTF-8 string policy in config/server validation; independently bound complete documents and validate structure/consistency in both access paths without duplicating numeric policy ceilings. Use standard JSON string decoding and reject malformed or duplicate catalogs without partial results.
- Keep listing at the same authenticated account/visibility boundary as resolution, exposing authorized provider/model identifiers but never provider credentials, secret references or unrelated account state.
- Update obsolete discovery-concealment requirements, actual access examples and evidence boundaries without changing operator telemetry capture or runtime response/error behavior.

## Capabilities

### New Capabilities

- `gateway-configured-discovery`: Authorized bounded configured-route projection, TS companion access, and independent server/client/command proof.

### Modified Capabilities

- `gateway-model-catalog`: Explicit configured candidate metadata, defensive copying and consistent listing/resolution visibility without inferred provider inventories.
- `grafana-gateway-client`: Optional typed configured-route retention through existing discovery; replace discovery-only backend concealment with authorized configuration access.
- `gateway-ordered-text-fallback`: Permit configured topology in discovery while leaving execution, runtime evidence and operator capture requirements unchanged.
- `gateway-provider-configuration`: Permit explicitly configured provider-instance/provider/model facts in discovery while retaining credential exclusions and existing telemetry/error rules.

## Impact

- AGPL Gateway: `ai-gateway/catalog`, command configuration/catalog construction, discovery handler/schema, registered ProviderWire workspace, command tests and Gateway docs.
- Apache SDK: independent `providers/grafana` discovery types/decoder/tests and client guidance under `docs/`; no Gateway imports or shared server validator.
- Reference: `@ai-sdk/gateway` 4.0.94, `ai` 7.0.116, provider 4.0.18 and provider-utils 5.0.49 at upstream commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`, matching `test/conformance/upstream.yaml`. Gateway runtime/client and host-composition coverage are currently mixed.
- Sole feature prerequisite: approved #321 discovery decision, already present in this authorized parent stack at `7b0b35cf` (PR #327, `nrbrd/gw-contract`). Implement/review on this stack and validate cumulative source; a future child PR targets `nrbrd/gw-contract` while #327 is open, merges after the parent, and rebases/retargets main after parent merge. No merge-first implementation gate or stopped #303/#309 code, #280/#322/#323 delivery or unsupported provider activation is required.
- No inference selection changes, runtime attempts/errors/native diagnostics, routing controls (#316), BYOK/account construction (#317), credential database, new authorization engine or cost/health ranking. Current static command/Cloud fixtures are not deployed customer-account isolation proof.
