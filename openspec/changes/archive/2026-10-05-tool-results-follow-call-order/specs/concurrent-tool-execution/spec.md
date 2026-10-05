## MODIFIED Requirements

### Requirement: Tool results preserve call order

The `step.ToolResults` slice SHALL contain results in the same order as the original `step.ToolCalls` entries, regardless of completion order and of how each result arose: provider-executed, rejected as invalid, denied by an approval policy, or executed. This ensures deterministic message construction for subsequent steps. Upstream lists `step.toolResults` in arrival order; Go keeps call order, and `test/conformance/upstream.yaml` records the difference. A result for a call that is not in the step SHALL keep its relative position ahead of the step's own results, and results for the same call SHALL keep their order.

#### Scenario: Results ordered by call position

- **WHEN** a step has tool calls [A, B, C] and they complete in order [B, C, A]
- **THEN** `step.ToolResults` contains results in order [A, B, C]

#### Scenario: Skipped tools do not occupy result slots

- **WHEN** a step has tool calls [A, B, C] where B is provider-executed
- **THEN** `step.ToolResults` contains results for [A, C] only, in that order

#### Scenario: Rejected and denied calls keep their call position

- **WHEN** a step has tool calls [A, B, C] where A names a tool missing from the tool set, B executes, and C is denied by an approval policy
- **THEN** `step.ToolResults` contains results in order [A, B, C], although A was handled while the model stream was read and C before any tool started

