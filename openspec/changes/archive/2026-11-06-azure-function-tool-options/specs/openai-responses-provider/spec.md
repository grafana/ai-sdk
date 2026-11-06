## ADDED Requirements

### Requirement: Function-tool options follow model identity
The Responses provider SHALL select function-tool options by model identity: `azure` when the identity contains `azure`, otherwise `openai`. Absent or null Azure objects SHALL fall back to `openai`; present objects SHALL win without field merging. Declarations and output-schema history SHALL use the same selection. Typed and JSON-decoded options SHALL behave identically. Default OpenAI models SHALL ignore Azure function-tool options even when accepting legacy call-level Azure options.

#### Scenario: Azure options take precedence
- **WHEN** an Azure-identified model receives function-tool options under both keys
- **THEN** generation and streaming declarations use only the Azure object, including explicit false values and empty objects
- **AND** history text is JSON-string encoded only when the selected object declares an output schema

#### Scenario: Azure options fall back
- **WHEN** an Azure-identified model has an absent or null Azure function-tool options object
- **THEN** generation and streaming declarations and history use the OpenAI object

#### Scenario: Default OpenAI ignores Azure tool options
- **WHEN** a default OpenAI model receives Azure function-tool options, including alongside Azure-only call-level options
- **THEN** only the OpenAI function-tool options affect declarations and history

#### Scenario: Typed namespace attachment survives JSON
- **WHEN** callers attach `OpenAIToolOptions` directly as the `azure` map value and then round-trip the map through JSON
- **THEN** namespace selection and outbound requests match the original typed options
