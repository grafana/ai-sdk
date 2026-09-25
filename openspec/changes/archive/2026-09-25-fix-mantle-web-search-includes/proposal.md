## Why

Issue [#227](https://github.com/grafana/ai-sdk/issues/227) remains a request-construction defect against the registered upstream reference: `providers/openai/apply_options.go` automatically includes `web_search_call.action.sources` whenever a web tool is present, while `providers/bedrock/mantle/provider.go` reuses that builder without disabling the include unsupported by Mantle. The issue and pinned upstream source identify the incompatibility; no live Mantle web-search call was verified.

## What Changes

- Make OpenAI Responses web-search source auto-inclusion conditional on a default-enabled model capability and an optional per-call `includeWebSearchSources` switch. Keep caller-specified `include` values intact, even if either switch is false.
- Configure only the Mantle Responses adapter to disable automatic web-search source inclusion. Preserve web tool emission and other independent includes, metadata and attribution.
- Add request-level OpenAI option/capability matrix coverage and strict synthetic Mantle generate and stream endpoint tests that reject the unsupported automatic include; verify the standalone published-module dependency boundary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Specify automatic web-search source include precedence, optional per-call override and unchanged explicit includes for shared Responses models.
- `bedrock-mantle-responses-provider`: Require Mantle Responses to opt out of automatic web-search source inclusion in both generate and stream while retaining explicitly requested includes.

## Impact

`providers/openai/options.go`, `model.go`, `convert_request.go`, `apply_options.go` and focused tests; `providers/bedrock/mantle/provider.go` and transport tests; published OpenAI producer dependency in `providers/bedrock/go.mod`/`go.sum` upon consumer adoption. Baseline is `test/conformance/upstream.yaml` commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d` (`@ai-sdk/openai` 4.0.71, `@ai-sdk/amazon-bedrock` 5.0.88); provider-adapter and published-dependency proof boundaries are in `test/conformance/PARITY.md`. The exported Go option and field names were approved separately before implementation; this change does not alter the frozen baseline or provider fixtures, expand Mantle Chat support, or merge #207 assistant history or #164 reasoning-summary work.
