## MODIFIED Requirements

### Requirement: Continuation metadata replacement
The service and independent Go client SHALL preserve bounded opaque metadata with arbitrary object-valued namespaces under gateway-provider-metadata, including absence versus an empty object and nested null encrypted content, and retain metadata at its original event position. Namespace/key/type inventories SHALL NOT select continuation fields or discard unknown metadata. Core assembly SHALL replace the previous object with the latest non-nullish object rather than recursively merge it. Continuation values SHALL never enter metadata-only telemetry.

#### Scenario: Signature arrives on end
- **WHEN** an end event carries a signature or final encrypted content
- **THEN** the next assistant history and native provider request SHALL retain that final value

#### Scenario: Empty object replaces without deep merge
- **WHEN** reasoning metadata initially contains native continuation values and a later registered event supplies an empty object or a disjoint replacement object
- **THEN** both clients' assembled reasoning SHALL use the later complete object, while an omitted later object SHALL retain the previous metadata
