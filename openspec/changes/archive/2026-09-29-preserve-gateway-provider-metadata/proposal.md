## Why

Issue #280's unchanged direct conformance goldens expose lost `openai.itemId` on text start/end and `anthropic.caller` on function calls, which prevents faithful UI output and tool continuation through both Gateway clients. ProviderWire deliberately excludes all provider metadata today, so this requires a reviewed, privacy-safe response contract change rather than a test normalization.

## What Changes

- Permit a closed, validated projection of supported provider-part metadata on streamed text and basic function-tool events and unary text/function-call content; update the strict response schemas and both clients' consumption.
- Keep public response identity canonical and gateway-owned routing, authentication, backend identity, errors, logs, metrics and metadata-only observability isolated. Unknown/future metadata is not forwarded without a separate review.
- Bound metadata and entire unary/SSE responses before emission or delivery; invalid supported fields fail safely, without partial events or leaked detail. Continue omitting finish and top-level unary metadata without evidence that they are needed for supported conversation continuation.
- Add independently green server/client and cross-language privacy, boundary, continuation and cleanup tests. Preserve the unchanged OpenAI simple-text and Anthropic tool-call UI/backend-request goldens as the subsequent replay contract on #201 after it rebases onto this change; report unrelated tool-result differences separately. No claim about Vercel's private service.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providerwire-v4-streaming-runtime`: bounded, field-specific per-part metadata and unchanged canonical response identity/terminal behavior.
- `providerwire-v4-unary-runtime`: bounded content metadata on supported unary output while preserving minimal top-level response.
- `gateway-streaming-function-tools`: function call/result metadata and privacy-safe stateless continuation.
- `grafana-gateway-client`: strict bounded adoption of the approved metadata projection in unary and streaming results.

## Impact

ProviderWire DTOs and response JSON schemas (`ai-gateway/providerwire/v4/`), Grafana client response decoding (`providers/grafana/`), registered Vercel client schema/consumption tests (`ai-gateway/test/providerwire-v4/`), and a documented handoff to the existing two-client matrix on unmerged PR #201, which rebases after this change merges. No public Go method signatures, request schema, provider fixture inputs or direct goldens change. Reference: `test/conformance/upstream.yaml` pins `ai@7.0.109`, `@ai-sdk/gateway@4.0.88`, provider v4 `4.0.17`, Vercel commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`; the public client forwards successful stream parts and spreads unary result fields, not evidence of private service behavior.
