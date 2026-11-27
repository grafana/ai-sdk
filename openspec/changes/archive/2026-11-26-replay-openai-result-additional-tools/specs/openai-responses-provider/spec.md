## ADDED Requirements

### Requirement: OpenAI additional tools from function result history

An ordinary tool-role function result with a nonempty `additionalTools` list in its part provider options SHALL emit a developer `additional_tools` input item immediately after its `function_call_output`. Typed and raw part options SHALL use the same rules. An omitted or empty list SHALL NOT emit an item. This replay SHALL NOT change request-level tools or local execution eligibility.

#### Scenario: Active part namespace controls replay

- **WHEN** an Azure request has result-part options in both namespaces
- **THEN** an Azure list SHALL take precedence, including an explicitly empty list
- **AND** the OpenAI list SHALL be used only when that field is absent in the active namespace, including when the active namespace value itself is null

#### Scenario: Grouped parallel results preserve child positions

- **WHEN** a conversation or previous-response continuation groups a complete parallel result set
- **THEN** the single wrapper output SHALL be followed by one item for each nonempty child list, in child-index order
- **AND** later occurrences of those child parts SHALL NOT emit duplicate items

#### Scenario: Definitions have the result-history shape

- **WHEN** a participating result supplies definitions
- **THEN** the list SHALL accept function definitions with nonempty names and object parameters, custom definitions with nonempty names and optional text or regex/lark grammar formats, and namespace definitions with nonempty names, string descriptions and nonempty function-only child lists
- **AND** malformed definitions, unknown definition or format fields, and null lists SHALL fail locally before HTTP
- **AND** unrelated namespace fields SHALL NOT be validated by this option

#### Scenario: Other history paths do not apply the option

- **WHEN** the history converter emits a native non-function output, replays an assistant result, or skips an approval-linked denial
- **THEN** it SHALL NOT emit additional tools for that part
- **AND** it SHALL NOT read this option from an output's provider options
