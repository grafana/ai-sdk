## MODIFIED Requirements

### Requirement: Opaque metadata at supported registered scopes
The Gateway SHALL preserve ordinary providerMetadata at every registered metadata position of its currently supported output families: unary result and text/reasoning/reasoning-file/source/function-call content; streaming text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result and finish. Both the registered TypeScript client and independent Go client SHALL retain unknown/future namespace objects and nested JSON semantics without IncludeRawChunks. Except for optional result/finish gateway.execution enrichment and fitting native gateway relocation at the runtime feature boundary, the service SHALL NOT rename namespaces, select keys by inventory, relocate metadata to another event, or expand unsupported output unions. Response-metadata, stream-start, error and raw SHALL NOT acquire an unregistered providerMetadata field.

#### Scenario: All supported scopes retain opaque values
- **WHEN** supported unary content/results and stream parts carry unknown namespaces with nested arrays, objects, null, false, zero and empty strings
- **THEN** raw schema-valid server output and both clients SHALL retain those values at their original scopes without a raw-event option or native-key inventory

#### Scenario: Unsupported family is not activated
- **WHEN** an otherwise unsupported provider-executed tool or custom/generated-content variant supplies metadata
- **THEN** its existing explicit unsupported-family behavior SHALL remain in force rather than admitting the content because a metadata codec exists

### Requirement: Metadata adaptation preserves commitment and ownership
Invalid or oversized original unary metadata SHALL fail the whole response before HTTP 200 through the existing adaptation error. Invalid committed stream metadata SHALL prevent writing that event, cancel provider work, and use the existing bounded terminal error when the writer is usable. Optional Gateway overview enrichment that cannot fit SHALL preserve the original fitting response/event. Native namespace relocation SHALL NOT repair primary-invalid output. The service SHALL NOT silently omit native metadata, return normalized-only success, read ahead for selection, restart fallback after a selected result/part, or introduce another cleanup owner. Part budgets, IDs, non-terminal provider errors, cancellation, deadlines, authoritative finish and bounded drain SHALL retain their delivered semantics.

#### Scenario: Selected result cannot replay after adaptation
- **WHEN** a selected direct/fallback unary result or committed stream has invalid or oversized metadata
- **THEN** the existing failure path SHALL apply and no later candidate SHALL be invoked to hide metadata loss or replay generation

#### Scenario: Finish remains authoritative
- **WHEN** an in-limit finish carrying metadata is written and a later provider part is available
- **THEN** finish and its metadata SHALL remain final, later output SHALL be suppressed and existing bounded cleanup SHALL occur
