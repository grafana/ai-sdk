# Spec Delta

## MODIFIED Requirements

### Requirement: Opaque metadata at supported registered scopes
The Gateway SHALL preserve ordinary providerMetadata at every registered metadata position of its currently supported output families: unary result and text/reasoning/reasoning-file/source/function-call/custom content; streaming text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result, custom and finish.

#### Scenario: All supported scopes retain opaque values
- **WHEN** supported unary content/results and stream parts carry unknown namespaces with nested arrays, objects, null, false, zero and empty strings
- **THEN** raw schema-valid server output and both clients SHALL retain those values at their original scopes without a raw-event option or native-key inventory

#### Scenario: Custom metadata presence
- **WHEN** unary or streamed custom content supplies no metadata, an empty metadata object, or an unknown namespace with nested values
- **THEN** raw server output and both clients SHALL respectively omit metadata, retain `{}`, or retain the namespace unchanged

#### Scenario: Unsupported family is not activated
- **WHEN** an otherwise unsupported provider-executed tool or generated-content variant supplies metadata
- **THEN** its existing explicit unsupported-family behavior SHALL remain in force rather than admitting the content because a metadata codec exists
