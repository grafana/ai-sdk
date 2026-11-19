## MODIFIED Requirements

### Requirement: Route table matches the manifest
The production route table SHALL declare exactly the method and path pairs of the committed manifest's custom routes. A request that matches no entry, including another method on a declared path, SHALL perform no access decision, discovery, selection or provider work, and SHALL be labeled unmatched.

#### Scenario: Production table and manifest agree
- **WHEN** the production route table is compared with the committed manifest
- **THEN** both SHALL contain only `GET namespaces/{namespace}/config`, `POST namespaces/{namespace}/language-model`, `POST namespaces/{namespace}/v1/chat/completions`, and `GET` `config`, `POST` `language-model` and `POST` `v1/chat/completions` under `namespaces/{namespace}/modelcatalogs/{name}/`

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the route prefix, or a declared path with another method, including `HEAD` on either `config` route
- **THEN** no catalog, selection or provider call SHALL occur, and the response SHALL NOT be a Gateway error document
