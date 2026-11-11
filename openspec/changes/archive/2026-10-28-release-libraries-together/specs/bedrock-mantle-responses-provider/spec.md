## MODIFIED Requirements

### Requirement: Mantle published continuation boundary

Unary/streaming assistant reconstruction and stored continuation SHALL work with the Bedrock module's published dependencies, without workspace substitutions. The only replacement directives SHALL be local first-party replacements, which consumers ignore.

#### Scenario: Mantle published continuation boundary

- **WHEN** an external consumer uses a published Bedrock version with GOWORK=off and no replacements of its own
- **THEN** reconstruction SHALL use string-content encoding and stored assistant IDs SHALL still produce item references

### Requirement: Mantle candidate-source web-include evidence

Candidate-source checks SHALL exercise web-source automatic-include suppression with shared OpenAI implementation at the same revision through Bedrock's local first-party replacements, while Bedrock's declared OpenAI requirement equals the version manifest.

#### Scenario: Mantle candidate-source web-include evidence

- **WHEN** candidate checks use same-revision OpenAI source through Bedrock's local replacements
- **THEN** Mantle unary and stream web-tool requests SHALL omit automatic web sources and the declared OpenAI requirement SHALL equal its manifest version
