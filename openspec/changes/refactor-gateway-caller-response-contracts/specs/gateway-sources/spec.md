## MODIFIED Requirements

### Requirement: Bounded response-local source identity
The Gateway SHALL preserve provider source ID unchanged rather than generate response-local IDs/hashes. IDs SHALL remain nonempty valid UTF-8 and at most 1024 bytes. Repeated/equal-cross-variant IDs SHALL preserve native values/order rather than invent uniqueness. Existing unary cardinality/string and stream part/frame bounds remain. No response-local identity map SHALL be needed; current metadata transport/bounds remain unchanged until #280.

#### Scenario: Repeated source
- **WHEN** a provider emits the same URL ID twice and a document with the same native ID
- **THEN** all SHALL retain native ID/order without response-local renaming

### Requirement: Closed public source metadata
This foundation SHALL retain the current bounded citation projection unchanged as a temporary capability boundary: only citation index/startPageNumber/endPageNumber/startCharIndex/endCharIndex integers in 0..1000000000, originating from the existing OpenAI/Azure/Anthropic fields. Current known-namespace 8192-byte/input/frame preflight, malformed/null/value omission and absent-empty behavior SHALL remain. #280 exclusively owns replacing that projection with ordinary supported metadata server/client/schema transport and source integration; this limitation SHALL NOT be justified as permanent shared-account concealment or claimed as native metadata parity.

Source display SHALL preserve native URL/title and document title/filename, including OpenAI/Azure file_path values. The Gateway SHALL not substitute Document/omit a nonempty filename merely to conceal the backend. Required empty title, optional-empty normalization, Title-before-legacy-Text, inactive fields and no URL fetch/canonicalization remain. This display revision SHALL not broaden metadata fields or introduce another metadata helper/filter.

#### Scenario: Native file path display
- **WHEN** file_path supplies a native file ID as title/filename
- **THEN** native ID/title/filename SHALL survive while the current metadata projection remains unchanged
- **AND** full source metadata acceptance SHALL remain with #280 rather than block this foundation

### Requirement: Independent consumption and evidence
Independent Apache Go and exact-pinned TS clients SHALL preserve delivered source ID/display/order/required-empty/optional-empty semantics through handler and authenticated command. Strict raw schema/bound/security tests and schema-parsed frontend assembly SHALL be independent of permissive TS parsing. Current metadata remains unchanged; future metadata-dependent source integration is a #280 handoff, not foundation acceptance. Synthetic provider responses SHALL not become recorded/upstream evidence; #201 later owns full authentic matrix.

#### Scenario: Both clients consume native display
- **WHEN** supported sources pass through both clients
- **THEN** native identity/display/order SHALL agree after documented optional-empty normalization without claiming future metadata parity
