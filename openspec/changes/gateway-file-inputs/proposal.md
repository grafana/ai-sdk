## Why

[Issue #109](https://github.com/grafana/ai-sdk/issues/109), work package 15 of the AI Gateway technical plan, requires unambiguous file inputs through both Gateway clients. The current strict mapper rejects files, public `DataContent` construction cannot select every empty arm, and string filenames collapse absence into explicit empty even where native providers distinguish them.

## What Changes

- Add public constructors and inspection for inline bytes/base64, URL, provider-reference, and text data, with explicit empty selection and conflict validation at construction, SDK decoding, and conversion boundaries.
- **BREAKING**: Make request-file and tool-result-file filenames presence-aware (`*string`), including ordinary UI `FilePart` inputs; migrate SDK, provider, client, middleware, tests, and examples together. Source-document and generated-output filename semantics do not change.
- Enable user/assistant `file` parts and file content in supported tool-result history for unary and streaming requests. Preserve message-level and file-part provider options without forwarding reserved host namespaces.
- Extend independent Go client projection, native provider conversion, direct-call validation, and privacy-safe logical observation.
- Prove selected arms, filename presence, invalid-input rejection, byte bounds, and native conversion against the registered upstream baseline and client-generated goldens.
- Keep generated media output (WP16), reasoning content/files (WP17), custom content, approvals/provider-executed behavior, top-level provider options/body headers, and new fallback capabilities outside this change.

## Capabilities

### New Capabilities

- `provider-file-data-selection`: Public selected-arm construction/inspection, validation, and provider conversion semantics for file inputs.
- `gateway-file-inputs`: Bounded strict file mapping, scoped provider options, privacy, and cross-client acceptance evidence.

### Modified Capabilities

- `provider-v4-content-model`: Presence-aware request-file filenames while retaining flat content structs and source normalization.
- `v4-tool-result-alignment`: Presence-aware tool-result-file filenames.
- `ui-message-conversion`: Preserve ordinary UI file filename presence through JSON and model-message conversion.
- `gateway-ordered-text-fallback`: Preserve text fallback eligibility for semantically empty message-option namespaces without enabling file fallback.
- `grafana-gateway-client`: Explicit projection of all file arms, including selected empties and filename presence.
- `gateway-unary-function-tools`: Admit file result content without enabling unrelated deferred tool families.
- `providerwire-v4-unary-runtime`: Replace blanket file/message-option rejection with the WP15 supported subset, shared by both execution modes.

## Impact

Apache SDK work affects `provider/`, root conversion/validation call sites, `providers/{grafana,anthropic,bedrock,openai,openai-compatible}/`, reusable observability/logger middleware, and related tests/docs/examples. AGPL Gateway work stays in `ai-gateway/providerwire/v4`, host integration/privacy tests, and `ai-gateway/test/providerwire-v4`.

WP7 and WP8 are present; existing WP11/12 tool-history paths supply the integration point for nested file results without requiring WP13/14. SDK prerequisite commits must be published at immutable refs before dependent Gateway module pins are updated; standalone module validation remains mandatory.

Authority: `test/conformance/upstream.yaml` and `test/conformance/PARITY.md`, currently provider 4.0.17, Gateway 4.0.87, Anthropic 4.0.58, Bedrock 5.0.88, OpenAI 4.0.71, and OpenAI-compatible 3.0.53. This proposal does not upgrade the baseline.
