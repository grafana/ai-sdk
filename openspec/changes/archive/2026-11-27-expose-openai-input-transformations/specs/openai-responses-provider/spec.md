## ADDED Requirements

### Requirement: Responses input transformation metadata

The adapter SHALL project `input_transformations` to `inputTransformations` in the existing provider metadata namespace of unary results and in finish parts for `response.completed`, `response.incomplete`, and `response.failed`. Entries SHALL retain their order, optional nullable `id`, and string-valued `type`, `action`, and `reason`, including unrecognized strings. Other response metadata and finish behavior SHALL remain unchanged.

#### Scenario: Terminal response reports transformed items

- **WHEN** a unary or terminal stream response reports input transformations
- **THEN** the result or finish part includes those entries under the active provider metadata namespace
- **AND** an omitted entry ID stays omitted, and an explicit null ID stays null

#### Scenario: Empty and unreported transformations are distinct

- **WHEN** a unary or terminal stream response contains an empty transformations array
- **THEN** the metadata contains `inputTransformations: []`
- **AND** the metadata omits the key for missing or null arrays
