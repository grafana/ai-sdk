## ADDED Requirements

### Requirement: Catalog is exclusive to configured-account access
The Gateway host SHALL use catalog resolution and listing only for configured-account access. The catalog SHALL describe configured models, aliases and provider candidates; it SHALL NOT define BYOK model support or request routing. BYOK selectors SHALL NOT receive catalog instances or resolve configured provider references. Future authentication mechanisms can grant configured access without changing catalog semantics.

#### Scenario: Configured alias is used internally
- **WHEN** a configured-access request names a catalog alias
- **THEN** catalog resolution SHALL preserve canonical identity and the configured provider/account selection

#### Scenario: BYOK request coincides with a configured ID
- **WHEN** a BYOK request's provider/model text also exists as a configured catalog ID
- **THEN** selection SHALL parse the request independently and SHALL make zero catalog calls

#### Scenario: Catalog changes independently
- **WHEN** configured aliases, defaults, providers or fallback routes change
- **THEN** an otherwise identical BYOK request SHALL retain the same provider, native model and request-credential selection
