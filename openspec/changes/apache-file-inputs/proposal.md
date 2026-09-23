## Why

[Issue #109](https://github.com/grafana/ai-sdk/issues/109) needs an Apache SDK/provider contract that can represent every selected ordinary LanguageModelV4 file-data arm, preserve filename presence, and convert file requests without silently dropping supported content. The Gateway service is a separate module: these public producer changes must be independently mergeable and published before its runtime adopts them.

## What Changes

- Add public bytes, base64, URL, provider-reference, and text file-data construction and inspection. Preserve selected empty data/text and reject ambiguous or malformed arms before provider I/O.
- **BREAKING:** Make request-file and tool-result-file filenames `*string`, including ordinary UI file input, so absent and explicitly empty remain distinct. Leave generated/source descriptive filenames unchanged.
- Align Anthropic/Vertex, OpenAI Responses, OpenAI-compatible, and Bedrock native conversions with the pinned upstream source and provider-specific support. Preserve Anthropic inline-text documents and URL/PDF tool-result content.
- Update the independent Go Gateway client, reusable Agent Observability and logger, centralized input docs, examples, and deterministic cross-language/UI evidence. Do not import Gateway production code into Apache modules.
- Publish immutable, proxy-resolvable module revisions and pass standalone readonly dependency and parity checks. Do not hand-author provider recording inputs.

## Capabilities

### New Capabilities

- `provider-file-data-selection`: public selected-arm construction, validation, and native provider conversion.

### Modified Capabilities

- `provider-v4-content-model`: presence-aware ordinary request-file filenames.
- `v4-tool-result-alignment`: selected file arms and presence-aware tool-result filenames.
- `ui-message-conversion`: ordinary UI filename presence through model-message conversion.
- `grafana-gateway-client`: explicit request projection of file arms and filename presence.

## Impact

Apache-side provider/root, native adapter, independent client, reusable middleware, docs, examples, conformance witness, and UI integration tests change. The Gateway runtime mapper, fallback, HTTP bounds, logical host privacy, and generated-media/reasoning runtime are **not** implemented by this PR; the stacked `gateway-file-inputs` change owns their acceptance. Reference versions remain those in `test/conformance/upstream.yaml`, commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`.
