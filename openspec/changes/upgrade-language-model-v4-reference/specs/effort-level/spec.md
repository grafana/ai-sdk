# Spec Delta

## ADDED Requirements

### Requirement: Portable maximum reasoning

The portable reasoning contract and Gateway SHALL accept maximum reasoning. Anthropic and Anthropic Converse adaptive thinking SHALL map it to native maximum effort; budget-based models SHALL map it to a 95 percent reasoning budget under existing bounds. OpenAI and compatible adapters SHALL retain their existing provider-specific capability gates. Nova 2 Lite SHALL map maximum and extra-high portable effort to high with a compatibility warning.

#### Scenario: Adaptive and budget maximum
- **WHEN** maximum reasoning is supplied to an adaptive Anthropic model or a budget-based Anthropic model
- **THEN** the request SHALL use native maximum effort or a 95 percent bounded budget respectively

#### Scenario: Gateway and Nova projection
- **WHEN** maximum reasoning passes through either Gateway client to a Nova 2 Lite model
- **THEN** the portable value SHALL survive request mapping and native effort SHALL become high with a compatibility warning
