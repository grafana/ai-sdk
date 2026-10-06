## Context

Google Cloud documents native JSON output on Claude 4.5 and later through `output_config.format`. Organizations must allow `structured_outputs` in `constraints/vertexai.allowedPartnerModelFeatures`.

## Decisions

Enable only the native-output transport capability and retain the existing model gate. Gate automatic structured-output beta injection on direct beta support to preserve Vertex header behavior. Explicit JSON-tool mode and older-model fallback remain available.

Compared request conversion and tests against registered `@ai-sdk/anthropic` 4.0.59 at `4e8c387622ee1bb0d55841664416d38754d5c9a3`. This is a provider-configuration adaptation of the existing upstream capability gate. No baseline upgrade or response mapping change is required.

## Validation

Synthetic request serialization reproduces forced tool choice before the fix and checks native schemas, absent synthetic tools, and absent forced choice after it, for generation and streaming. Provider tests and lint pass. No live Vertex recording is claimed.

## Risks

Organizations without the structured-output policy grant will receive a provider error and must enable the feature. Strict function-tool support is outside this fix.
