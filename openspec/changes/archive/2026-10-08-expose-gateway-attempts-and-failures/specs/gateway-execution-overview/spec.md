## MODIFIED Requirements

### Requirement: Dormant foundation and owned schema

The Gateway SHALL define and test a compact namespace schema independently of production service/handler activation, without a new stream reader or retry/cleanup owner, a runtime schema dependency or an Apache client import. The implementation SHALL remove the old Gateway collector, model wrappers and diagnostic allocation machinery. Root SDK observation SHALL remain reusable without Gateway dependencies. Minimum Go versions SHALL remain unchanged.

#### Scenario: Only the foundation is applied
- **WHEN** this change is applied without the activation change
- **THEN** existing runtime output SHALL remain unchanged while focused Go and strict TypeScript schema tests exercise the new projection
