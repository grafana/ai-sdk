# Spec Delta

## ADDED Requirements

### Requirement: Anthropic container skills request mapping
When `AnthropicOptions.Container.Skills` is non-empty, the request SHALL send `container` with `id` (when set) and each skill's `type`, `skill_id` and `version` (when non-empty). Skill `type` SHALL be `anthropic` or `custom`; any other type SHALL fail request building. An `anthropic` skill SHALL use `skillId`.

#### Scenario: Unknown skill type
- **WHEN** a skill has a type other than `anthropic` or `custom`
- **THEN** request building SHALL return an error and SHALL send no request

### Requirement: Anthropic custom skill reference resolution
A `custom` skill SHALL take its `skill_id` only from `providerReference["anthropic"]`. A custom skill without `providerReference`, or whose reference has no `anthropic` entry, SHALL fail request building and list the available providers. A custom skill whose `anthropic` entry is empty or `null` SHALL fail request building with an empty-reference error instead of sending an empty `skill_id`.

#### Scenario: Custom skill provider reference
- **WHEN** a request sends `{type: custom, providerReference: {anthropic: "skill_01X"}}` beside `{type: anthropic, skillId: pptx}`
- **THEN** the body SHALL contain `skills: [{type: anthropic, skill_id: pptx}, {type: custom, skill_id: skill_01X}]`

#### Scenario: Reference without anthropic entry
- **WHEN** a custom skill's `providerReference` has only an `openai` entry
- **THEN** request building SHALL return an error naming provider `anthropic` and the available providers, and SHALL send no request

#### Scenario: Custom skill without providerReference
- **WHEN** a custom skill sets `skillId` and no `providerReference`
- **THEN** request building SHALL return the same error as a reference without an `anthropic` entry, and SHALL send no request

#### Scenario: Empty anthropic reference
- **WHEN** a custom skill sends `providerReference: {anthropic: ""}` or `providerReference: {anthropic: null}`
- **THEN** request building SHALL return an empty-reference error for provider `anthropic`, and SHALL send no request

### Requirement: Anthropic container skills betas and warning
A non-empty skills list SHALL add the `code-execution-2025-08-25`, `skills-2025-10-02` and `files-api-2025-04-14` betas, deduplicated with tool and explicit betas. When no `anthropic.code_execution_20250825` or `anthropic.code_execution_20260120` tool is declared, the call SHALL return an `other` warning with message `code execution tool is required when using skills` and SHALL still send the request.

#### Scenario: Skills without a code-execution tool
- **WHEN** a request has skills and no code-execution provider tool
- **THEN** the `anthropic-beta` header SHALL contain all three betas once and the warnings SHALL contain the code-execution message

#### Scenario: Skills with code execution 20250825
- **WHEN** a request has skills and the `anthropic.code_execution_20250825` tool
- **THEN** the header SHALL contain `code-execution-2025-08-25` once and no code-execution warning SHALL be returned

#### Scenario: Skills with code execution 20260120
- **WHEN** a request has skills and the `anthropic.code_execution_20260120` tool
- **THEN** the header SHALL contain `code-execution-2025-08-25` once and no code-execution warning SHALL be returned
