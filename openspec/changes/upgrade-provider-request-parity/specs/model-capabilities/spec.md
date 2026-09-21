## MODIFIED Requirements

### Requirement: Model capabilities lookup
The system SHALL provide an unexported `getModelCapabilities` function that accepts a model ID string and returns a capabilities struct containing `maxOutputTokens` (int), `supportsAdaptiveThinking` (bool), `supportsStructuredOutput` (bool), `rejectsSamplingParams` (bool), `supportsXHighEffort` (bool), `rejectsThinkingDisabledAboveHighEffort` (bool), and `isKnownModel` (bool).

The function SHALL use substring matching on the model ID, checked in specificity order:
- IDs containing `claude-opus-5`: 128000 max output, adaptive thinking supported, structured output supported, sampling parameters rejected, xhigh effort supported, thinking-disabled effort above high rejected, known
- IDs containing `claude-opus-4-8`, `claude-opus-4-7`, `claude-fable-5`, or `claude-sonnet-5`: 128000 max output, adaptive thinking supported, structured output supported, sampling parameters rejected, xhigh effort supported, known
- IDs containing `claude-sonnet-4-6` or `claude-opus-4-6`: 128000 max output, adaptive thinking supported, structured output supported, known
- IDs containing `claude-sonnet-4-5`, `claude-opus-4-5`, or `claude-haiku-4-5`: 64000 max output, adaptive thinking not supported, structured output supported, known
- IDs containing `claude-opus-4-1`: 32000 max output, adaptive thinking not supported, structured output supported, known
- IDs containing `claude-sonnet-4-` or `claude-sonnet-4@` (catch-all after more-specific Sonnet models): 64000 max output, adaptive thinking not supported, structured output not supported, known
- IDs containing `claude-opus-4-` or `claude-opus-4@` (catch-all after more-specific Opus models): 32000 max output, adaptive thinking not supported, structured output not supported, known
- IDs containing `claude-3-haiku`: 4096 max output, adaptive thinking not supported, structured output not supported, known
- Other legacy Claude instant, 2, and 3 IDs: 4096 max output, adaptive thinking not supported, structured output not supported, not known
- Other IDs containing `claude-`: 128000 max output, adaptive thinking supported, structured output supported, sampling parameters rejected, xhigh effort supported, thinking-disabled effort above high rejected, not known
- All other IDs: 4096 max output, adaptive thinking not supported, structured output not supported, not known

Except for dated Vertex Claude 4 recognition, these entries preserve the current registered-baseline Go behavior; this change SHALL NOT introduce a new model-capability family.

#### Scenario: Known model claude-opus-4-7
- **WHEN** `getModelCapabilities` is called with model ID `claude-opus-4-7`
- **THEN** it SHALL return maxOutputTokens=128000, supportsAdaptiveThinking=true, supportsStructuredOutput=true, rejectsSamplingParams=true, supportsXHighEffort=true, isKnownModel=true

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

#### Scenario: Dated Vertex Sonnet 4
- **WHEN** capabilities are resolved for `claude-sonnet-4@20250514`
- **THEN** the model SHALL be known with maximum output 64000, without adaptive thinking, structured output, sampling rejection, or xhigh effort

#### Scenario: Dated Vertex Opus 4
- **WHEN** capabilities are resolved for `claude-opus-4@20250514`
- **THEN** the model SHALL be known with maximum output 32000, without adaptive thinking, structured output, sampling rejection, or xhigh effort

## ADDED Requirements

### Requirement: Dated Vertex request defaults
Generate and stream request conversion for dated Vertex Sonnet 4 and Opus 4 SHALL use the recognized model capabilities for default output limits, thinking-budget calculation, clamping, and warnings, without changing the requested model identity. More-specific model families SHALL retain their existing capabilities.

#### Scenario: Default output limit at the request boundary
- **WHEN** a generate or stream request omits `MaxOutputTokens` for `claude-sonnet-4@20250514` or `claude-opus-4@20250514`
- **THEN** the serialized `max_tokens` SHALL be 64000 or 32000 respectively
- **AND** conversion SHALL NOT emit an unknown-model warning or choose future-Claude capabilities

#### Scenario: Explicit output and thinking budget
- **WHEN** an explicit output limit plus an enabled thinking budget exceeds the dated Vertex model's maximum
- **THEN** the final request SHALL clamp to that maximum and emit the existing explicit-limit warning
- **AND** a valid explicit limit without excess budget SHALL remain unchanged

#### Scenario: Existing models are unaffected
- **WHEN** the request targets a more-specific Claude 4.5/4.6 model, a legacy model, or a future-Claude model
- **THEN** this dated-ID correction SHALL NOT change its existing lookup or request behavior
