# Spec Delta

## MODIFIED Requirements

### Requirement: Strict OpenAI startup configuration failures
Configuration SHALL remain strict YAML, and every failure SHALL occur during startup before readiness. Operator model IDs and aliases SHALL follow the shared public-ID grammar, including `@` and `+` after the first character and the 320-byte bound.

#### Scenario: Strict OpenAI startup configuration failures
- **WHEN** an OpenAI provider YAML contains an unknown key
- **THEN** strict configuration SHALL fail during startup before readiness
