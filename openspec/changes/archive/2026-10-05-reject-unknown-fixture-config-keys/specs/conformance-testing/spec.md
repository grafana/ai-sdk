## ADDED Requirements

### Requirement: Fixture configuration rejects unknown keys

The TypeScript generation and recording tools and the Go replay loader SHALL reject a `config.yaml` key that no config type declares, at the top level and in every structured nested value, including map-valued entries such as `tools.<name>` and `providerTools.<name>`, message entries and their content parts, model-output content items, approvals, stream options, the tool choice and the response format. The error SHALL name the key and where it appears, and SHALL stop generation, recording or replay before any snapshot is produced or consumed. Payload values, including provider options, JSON schemas, tool inputs and outputs, provider tool arguments, headers and UI message parts, SHALL accept any keys. The two loaders SHALL accept the same keys, enforced by a shared fixture that lists every key at every level and that both loaders must accept.

#### Scenario: Misspelled top-level key

- **WHEN** a fixture sets `stopWhenStepCoun: 2`
- **THEN** both loaders reject it and name `stopWhenStepCoun`

#### Scenario: Misspelled key in a message part

- **WHEN** a configured tool-call content part sets `toolCallID` instead of `toolCallId`
- **THEN** both loaders reject it and name `toolCallID`, including the Go path that decodes message content in a custom unmarshaler

#### Scenario: Payload maps stay open

- **WHEN** a fixture puts arbitrary keys inside `providerOptions`, a tool `inputSchema`, a tool-call `input` or a UI message part
- **THEN** both loaders accept it

#### Scenario: Key sets stay aligned

- **WHEN** one loader declares a key the shared all-keys fixture does not contain, or the other loader rejects a key the fixture contains
- **THEN** that language's alignment test fails
