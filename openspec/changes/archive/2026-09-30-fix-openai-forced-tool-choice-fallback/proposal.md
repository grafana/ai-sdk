## Why

[Issue #32](https://github.com/grafana/ai-sdk/issues/32) remains valid: Go classifies all canonical provider-tool names as hosted forced choices, but the current registered `@ai-sdk/openai@4.0.72` still sends ordinary forced `shell`, `local_shell`, and `tool_search` selections as function choices. Existing auto-choice conformance snapshots miss this request-encoding bug; the issue's `4.0.25` reference is historical, not the implementation target.

## What Changes

- Restrict ordinary forced hosted choices to the registered upstream allowlist; serialize the three affected kinds as `{ "type": "function", "name": "<canonical name>" }`.
- Preserve configured alias-to-canonical resolution, provider tool declarations, supported hosted/custom/function choices, and the separate `allowedTools` override.
- Add focused canonical/alias regressions and credential-free unary/streaming HTTP request tests with inline expectations derived from the registered TypeScript 4.0.72 source. These are synthetic request evidence, not provider recordings or proof of live OpenAI acceptance.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Specify the ordinary forced hosted-choice allowlist and canonical function fallback, including shell/local-shell/tool-search aliases and unchanged allowed-tools behavior.

## Impact

The implementation is confined to `providers/openai/prepare_tools.go`, focused provider tests, and inline provider-module request assertions. No public Go API, dependency, baseline, conformance-harness, response/SSE, frontend, or other-provider change is required. Existing conformance provider inputs remain byte-identical. This fixes observable request JSON rather than introducing a new compatibility policy; consumers forcing these tools will receive the registered upstream shape.
