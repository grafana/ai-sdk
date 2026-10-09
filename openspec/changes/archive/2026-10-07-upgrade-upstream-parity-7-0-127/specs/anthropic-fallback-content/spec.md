## ADDED Requirements

### Requirement: Preserve fallback output

The Anthropic adapter SHALL emit each server-side fallback marker in unary and streaming output as custom content with kind `anthropic.fallback` and Anthropic metadata containing type `fallback`, from.model and to.model, preserving its position relative to other content.

#### Scenario: Model hop between reasoning blocks

- **WHEN** the provider emits reasoning, a fallback marker, and further reasoning
- **THEN** the marker appears between the reasoning blocks with both model identities intact

### Requirement: Replay validated fallback boundaries

The Anthropic adapter SHALL replay valid `anthropic.fallback` assistant custom parts as fallback blocks without cache control. It SHALL warn and omit invalid fallback metadata. Reasoning normalization SHALL NOT merge or reorder reasoning across a fallback boundary.

#### Scenario: Valid continuation

- **WHEN** assistant history contains a fallback marker between signed reasoning blocks
- **THEN** the outbound request preserves the marker, reasoning boundaries and signatures

#### Scenario: Invalid continuation

- **WHEN** fallback metadata lacks type fallback or string from.model or to.model
- **THEN** the adapter omits the marker and returns an other warning describing the required metadata
