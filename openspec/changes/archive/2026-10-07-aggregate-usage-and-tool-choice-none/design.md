## Context

`aggregateUsage` dates from the flat `Usage` type. The V4 reshape kept summing only the totals. The Anthropic `none` handling was ported from upstream `prepareTools`, which removes the tools.

## Decisions

### 1. Keep totals non-nil

Upstream returns `undefined` for an unreported total. Go has always returned a non-nil total, and callers dereference it, so totals stay set to zero. Breakdown fields were always nil, so nil-when-unreported keeps them safe and matches upstream.

### 2. Send `tool_choice: none` instead of dropping tools

Changing `tool_choice` invalidates only cached message blocks, while removing tools invalidates the whole cache. `disableParallelToolUse` does not apply to `none`.

### 3. Keep only the `json` tool when the fallback forces tool use

The fallback overrides the choice to `required`. With the caller's tools present, the model could call one of them despite `none`. Upstream sends them; Go keeps the earlier behavior of sending only `json`.
