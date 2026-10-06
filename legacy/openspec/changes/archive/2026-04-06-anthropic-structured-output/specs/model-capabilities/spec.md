## MODIFIED Requirements

### Requirement: Model capabilities lookup
The system SHALL provide an unexported `getModelCapabilities` function that accepts a model ID string and returns a capabilities struct containing `maxOutputTokens` (int), `supportsAdaptiveThinking` (bool), `supportsStructuredOutput` (bool), and `isKnownModel` (bool).

The function SHALL use substring matching on the model ID, checked in specificity order:
- IDs containing `claude-sonnet-4-6` or `claude-opus-4-6`: 128000 max output, adaptive thinking supported, structured output supported, known
- IDs containing `claude-sonnet-4-5`, `claude-opus-4-5`, or `claude-haiku-4-5`: 64000 max output, adaptive thinking not supported, structured output supported, known
- IDs containing `claude-opus-4-1`: 32000 max output, adaptive thinking not supported, structured output supported, known
- IDs containing `claude-sonnet-4-` (catch-all for other sonnet 4.x): 64000 max output, adaptive thinking not supported, structured output not supported, known
- IDs containing `claude-opus-4-` (catch-all for other opus 4.x): 32000 max output, adaptive thinking not supported, structured output not supported, known
- IDs containing `claude-3-haiku`: 4096 max output, adaptive thinking not supported, structured output not supported, known
- All other IDs: 4096 max output, adaptive thinking not supported, structured output not supported, not known

#### Scenario: Known model claude-sonnet-4-6
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-6`
- **THEN** it SHALL return maxOutputTokens=128000, supportsAdaptiveThinking=true, supportsStructuredOutput=true, isKnownModel=true

#### Scenario: Known model with date suffix
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-5@20250929`
- **THEN** it SHALL return maxOutputTokens=64000, supportsAdaptiveThinking=false, supportsStructuredOutput=true, isKnownModel=true

#### Scenario: Known model claude-opus-4-1
- **WHEN** `getModelCapabilities` is called with model ID `claude-opus-4-1`
- **THEN** it SHALL return maxOutputTokens=32000, supportsAdaptiveThinking=false, supportsStructuredOutput=true, isKnownModel=true

#### Scenario: Known model claude-3-haiku
- **WHEN** `getModelCapabilities` is called with model ID `claude-3-haiku`
- **THEN** it SHALL return maxOutputTokens=4096, supportsAdaptiveThinking=false, supportsStructuredOutput=false, isKnownModel=true

#### Scenario: Older sonnet model
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-0`
- **THEN** it SHALL return maxOutputTokens=64000, supportsAdaptiveThinking=false, supportsStructuredOutput=false, isKnownModel=true

#### Scenario: Unknown model
- **WHEN** `getModelCapabilities` is called with model ID `some-future-model`
- **THEN** it SHALL return maxOutputTokens=4096, supportsAdaptiveThinking=false, supportsStructuredOutput=false, isKnownModel=false
