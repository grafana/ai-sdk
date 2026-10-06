## Why

The OpenAI Responses adapter currently turns every `allowedTools.toolNames` entry into a function choice, even when the request declares hosted, custom, or MCP tools. This can send incorrect `tool_choice.allowed_tools` shapes; the registered `@ai-sdk/openai@4.0.72` implementation resolves choices against prepared declarations instead.

## What Changes

- Resolve `allowedTools` names against declared, emitted tool kinds and canonical provider-name aliases; prefer a direct declared name over an alias, and preserve selected order and duplicates.
- Emit the correct Responses allowed-tool entries for function, supported hosted, custom, and MCP tools; warn/drop ambiguous aliases and ineligible declared selections. If no entries remain, fail before HTTP. Unknown names warn but are still emitted as function entries, matching the registered upstream baseline.
- If there are no declared tools, omit both tools and `tool_choice`; otherwise `allowedTools` takes precedence over the ordinary choice and defaults its mode to `auto` when absent. Limit mode to the upstream typed `auto`/`required` contract without introducing new runtime validation in tool preparation.
- Add focused request/warning/error tests; do not fabricate recorded provider events.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Specify declaration-aware Responses `allowedTools` resolution, warning/drop and error cases, and choice precedence.

## Impact

OpenAI Responses tool preparation and its existing provider options/request tests (`providers/openai/prepare_tools.go`, `options.go`, `prepare_tools_test.go`, and focused unary/streaming request tests as needed). No core provider interface or frontend wire-format change. Ordinary forced tool choices for shell, local-shell, and tool-search remain tracked separately under #32. This supersedes #218's older 4.0.71-era demand to reject every unknown, ambiguous, empty, or invalid-mode selection: the registered 4.0.72 behavior is the acceptance reference. Existing provider fixture provenance rules apply; synthetic HTTP tests do not establish live API acceptance.
