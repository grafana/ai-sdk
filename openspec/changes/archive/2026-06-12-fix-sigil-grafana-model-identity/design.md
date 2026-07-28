## Context

The Grafana provider is a gateway-style transport provider. Its `LanguageModel.Provider()` returns `grafana`, and model calls are forwarded over provider-wire to a hosted endpoint that invokes the real backend model. This matches the upstream AI SDK gateway pattern, where the client-facing provider identifies the gateway rather than the routed vendor.

Sigil recording starts a generation before the provider call and currently seeds `GenerationStart.Model` from `p.Model.Provider()` and `p.Model.ModelID()`. For direct providers this is enough. For Grafana-hosted calls recorded client-side, that seed records `grafana/<model>` as the canonical model, even when the hosted endpoint returns response metadata that identifies the real backend provider and model. Sigil cost attribution needs the backend provider/model, but operators still need to know the call was routed through Grafana.

Existing provider types already carry response identity:

- Generate: `provider.GenerateResult.Response.Provider` and `Response.ModelID`.
- Stream: `provider.StreamPart{Type: PartResponseMeta, Provider, ModelID}`.

The fix should remain provider-agnostic. `middleware/sigil` must not import `providers/grafana` or other provider modules.

## Goals / Non-Goals

**Goals:**

- Record the real backend provider/model in Sigil generation rows when response metadata provides it.
- Preserve the transport provider/model as metadata so consumers can query calls routed through Grafana.
- Keep Grafana provider identity unchanged as `grafana`.
- Apply consistently to generate and stream recording.
- Avoid new dependencies and provider-specific imports in `middleware/sigil`.

**Non-Goals:**

- Do not change `provider.LanguageModel` or require a new interface method.
- Do not change the Grafana provider-wire request shape.
- Do not require Grafana-specific code in Sigil middleware.
- Do not solve live span start attributes when backend identity is only known after response metadata arrives.

## Decisions

### Prefer response metadata for final generation model identity

The Sigil mapper will populate `sigilsdk.Generation.Model` from successful response metadata when both provider and model are available. The generation recorder already falls back from the final generation to the start seed only when final model fields are empty, so setting the final model is enough to fix the exported generation row.

Alternative considered: change Grafana `model.Provider()` to return the backend provider. This was rejected because Grafana is a transport/gateway provider and existing registry behavior, tests, and upstream gateway semantics expect the client-facing provider to identify the transport.

Alternative considered: add a new `LanguageModel` metadata interface. This was rejected for this fix because the response metadata path already exists and avoids a wider public API change.

### Add transport identity metadata when final identity differs

Recording will preserve the wrapper model identity in metadata when response metadata changes the canonical generation model. Suggested metadata keys:

- `ai_sdk.transport.provider`: wrapper `LanguageModel.Provider()` value, e.g. `grafana`.
- `ai_sdk.transport.model`: wrapper `LanguageModel.ModelID()` value.

The metadata is generic rather than Grafana-specific. This lets Sigil users filter for gateway-routed calls without making `grafana` the canonical provider used for cost lookup.

Alternative considered: use a boolean such as `provided_via_grafana`. This was rejected as too provider-specific and less useful for future gateway-style providers.

### Thread wrapper model identity through mapping inputs

`MapGenerateResult` currently receives only call options, result, and context info. It should receive or derive the start model identity so it can add transport metadata only when final response identity differs. `StreamRecorder` already receives the `GenerationStart`, so it can use the seed identity for the same comparison after observing response metadata.

The public API surface in the existing spec lists `MapGenerateResult(params, result, ctxInfo)`. If implementation changes this signature, update the spec and call sites together. To minimize disruption, an internal helper can accept the start identity while preserving the exported function as a compatibility wrapper.

### Capture stream response metadata in `StreamRecorder`

`StreamRecorder.Observe` will store `Provider`, `ModelID`, and response ID from `PartResponseMeta`. `Generation()` will use those values to populate `Generation.Model` and `ResponseID` when available.

This mirrors `MapGenerateResult`, which can read `GenerateResult.Response` after the unary call completes.

## Risks / Trade-offs

- Span attributes may still start with the wrapper provider when backend identity is only known from response metadata -> The final exported generation row will be correct; a future enhancement can add an optional pre-call model identity resolver if span-start identity becomes required.
- Some hosted endpoints may omit response provider or model metadata -> The mapper will keep the existing seed identity and no transport override will be recorded.
- Metadata key naming may need alignment with Sigil conventions -> Use stable, generic `ai_sdk.transport.*` keys and add tests around them.
- Existing tests may assert exact model values for generated Sigil JSON -> Update or add focused tests to distinguish seed identity from response identity.

## Migration Plan

- Implement behind existing recording behavior with no configuration required.
- Existing direct-provider recordings remain unchanged because their response identity either matches the seed or is absent.
- Grafana-routed recordings gain corrected canonical model identity and extra transport metadata.
- Rollback is a code revert; no persisted data migration is required.

## Open Questions

- Should the transport metadata keys use a Sigil-owned prefix instead of `ai_sdk.transport.*`?
- Do downstream Sigil cost pipelines use only the final generation row, or do they also rely on live span attributes emitted at generation start?
