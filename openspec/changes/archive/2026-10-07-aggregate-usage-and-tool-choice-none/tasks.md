## 1. Usage aggregation

- [x] 1.1 Add a table-driven `aggregateUsage` test for no steps, unreported steps and mixed nil/non-nil breakdowns; confirm it fails on `main`.
- [x] 1.2 Sum every field with zero totals and nil-when-unreported breakdowns.

## 2. Anthropic tool choice none

- [x] 2.1 Update converter and HTTP request tests for direct, Vertex, `disableParallelToolUse` and the JSON fallback.
- [x] 2.2 Keep tools and send `tool_choice: none`; send only the `json` tool with the fallback.
- [x] 2.3 Add the `anthropic-tool-choice-none` entry to `test/conformance/upstream.yaml`.
- [x] 2.4 Update the Gateway command test that expected a `none` step to drop tools.

## 3. Validate

- [x] 3.1 `mise run test`, `mise run vet`, `mise run lint`, `mise run test-ai-gateway-command`.
- [x] 3.2 `mise run validate-parity-baseline`, `mise run test-conformance`.
