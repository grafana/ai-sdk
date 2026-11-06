# Design

## Context

See proposal.md for motivation. This is a parity-sensitive behavior addition, not a quick fix. The registered baseline is `@ai-sdk/openai-compatible@3.0.71` and `@ai-sdk/provider-utils@5.0.62` at `b7f6255acaae7724adc0fd37aac3dae3f3466578`. Its chat converter resolves references before media dispatch; `resolveProviderReference` accepts empty string IDs and rejects missing entries. The model passes `config.provider.split('.')[0].trim()`; metadata camel-casing does not apply to references.

The Go converter currently receives no provider name and routes references into inline/URL conversion. `provider.ValidateFileInputs` already rejects malformed references and conflicting arms. `provider-file-data-selection` owns the selected-arm contract. `PARITY.md` classifies this under mixed provider-adapter implementation coverage.

## Goals / Non-Goals

**Goals:** Thread the existing raw provider-options name into user-file conversion, project a presence-aware file ID and keep request metadata identical to the emitted body.

**Non-Goals:** No new public API, shared resolver, metadata namespace migration, tool-result capability, frontend scenario, or fixture recorder. See proposal.md for the user-visible boundaries.

## Decisions

1. Pass the namespace explicitly through prompt and user-content conversion into the file converter. Reuse `providerOptionsName`, which already matches the pinned model. Using response metadata's camel-cased key or probing aliases would resolve a different uploaded file.
2. After existing validation, handle the reference arm before MIME normalization or inline/URL conversion. Decode the already-validated reference as `map[string]string` and distinguish missing keys from empty values. Return a contextual Go error for absence; the repository forbids custom error types, so the upstream exception class is represented by ordinary error propagation before HTTP.
3. Add `FileID *string` and omit absent inline `file_data`. A pointer preserves the supported empty ID. Existing inline PDF payloads always contain a non-empty data-URL prefix, including empty bytes, so `omitempty` does not change their wire output. Keep existing outer metadata overlays after conversion, matching upstream spread ordering.
4. Use synthetic RoundTripper request snapshots for both methods, asserting exact emitted JSON, request metadata, mixed ordering, untouched caller input and pre-I/O failures. No matching recorded/imported provider input exists for this feature; invented recordings would misstate provenance. Register that evidence limit in upstream.yaml. No frontend wire behavior changes.

## Risks / Trade-offs

- Compatible servers differ in file-ID support → document endpoint-specific acceptance; passing synthetic tests establishes adapter projection only.
- Metadata may intentionally replace generated fields → retain upstream's existing overlay semantics and test the collision.
- Future #362 shares file conversion → keep the helper reusable while leaving tool-result stringification unchanged.

## Migration Plan

No caller migration or dependency change. Existing inline and URL requests retain their representation. Reverting the scoped provider change restores previous rejection behavior.
