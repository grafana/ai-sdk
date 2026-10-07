## Why

[Issue #166](https://github.com/grafana/ai-sdk/issues/166): the `concurrent-tool-execution` spec requires `step.ToolResults` to follow call order, but a step that mixes a rejected or denied call with an executed one lists results in handling order. Calls `[good, missing]` produced `[missing, good]`, and `[good, dangerous]` with `dangerous` denied by policy produced `[dangerous, good]`. Readers of `Steps()` saw an order that depended on how each result arose. Upstream lists `step.toolResults` in arrival order, and neither `upstream.yaml` nor `PARITY.md` recorded which order Go keeps.

## What Changes

- After a step's tools finish, a new `sortToolResultsByCall` stable-sorts `step.ToolResults` by the position of each result's call in `step.ToolCalls`.
- Record the intentional difference from upstream's arrival order in `test/conformance/upstream.yaml`.
- Extend the call-order requirement to name every kind of result, with a scenario for rejected and denied calls.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `concurrent-tool-execution`: State that call order holds for provider-executed, rejected, denied and executed results alike, and record the upstream difference.

## Impact

`streamtext.go` and its tests, one `upstream.yaml` entry and the spec. UI chunks, `step.Content` and the next provider request do not change: the request already followed call order, content is built from recorded response content, and conformance and cross-language integration suites pass unchanged.
