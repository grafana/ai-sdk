## MODIFIED Requirements

### Requirement: Bounded response-local source identity

Source scalar identity/validation SHALL remain governed by #320/PR #325 rather than this metadata transport change. The current base maps (sourceType, native ID) to response-local source-N IDs with a 1024-byte native-ID bound; those facts describe the base and SHALL NOT renew a native-identity suppression policy. On rebase/merge, this requirement and its tests SHALL preserve #325's native source identity behavior instead of restoring response-local substitution. Unary preflight SHALL bound total input strings and all original metadata namespace key/value bytes by UnaryResponseBytes, and content/metadata cardinality SHALL bound map growth. Source metadata SHALL share the aggregate result/content unary budget, not reset it for each source. Streaming SHALL use StreamParts and complete-frame bounds. Identity entries SHALL be inserted only after successful bounded source encoding.

#### Scenario: Repeated source
- **WHEN** a provider emits the same URL source ID twice and a document with the same native ID
- **THEN** identity SHALL follow the separately owned scalar policy without changing source metadata semantics, and integrated #320/PR #325 native source IDs SHALL NOT be replaced by source-N IDs

### Requirement: Closed public source metadata

Source providerMetadata SHALL preserve all supplied native object-valued namespaces and nested JSON under gateway-provider-metadata, with no synthetic citation namespace, numeric-only projection, recognized-namespace byte cap or key inventory. Absence SHALL remain absent and an explicit empty object SHALL remain present. All original namespace bytes including whitespace, namespace key bytes and cardinality SHALL participate in aggregate unary or complete-frame preflight before validation and encoding. Malformed JSON, invalid UTF-8, null/non-object namespaces or excess budget SHALL fail the response/event explicitly rather than selectively omitting fields. Useful native IDs, cited text and unknown fields inside ordinary provider metadata SHALL NOT be discarded merely because operator capture excludes them. Concrete credential and tenant-source protections SHALL remain independent from ordinary metadata transport.

Source title, URL and filename behavior SHALL remain owned by #320/PR #325. The current base replaces file_path titles with Document and omits filenames; this is implementation context, not renewed privacy policy. Metadata-only work SHALL leave scalar code unchanged on that base and SHALL preserve #325's native display/filename behavior when it integrates, reconciling copied requirements/tests rather than restoring substitutions. Ordinary source strings SHALL NOT acquire a generic redaction policy.

#### Scenario: Native file path
- **WHEN** OpenAI or Azure file_path supplies a native file ID as both title and filename
- **THEN** bounded native metadata SHALL survive independently of the separately owned display contract, and integrated #320/PR #325 native title/filename/identity behavior SHALL be preserved without restoring Document/filename substitution
- **AND** a valid native index SHALL remain in its original namespace rather than being renamed to citation

#### Scenario: Malformed unknown namespace fails explicitly
- **WHEN** a source has a previously unknown namespace with malformed or non-object JSON
- **THEN** the unary response SHALL fail before HTTP success or the committed stream SHALL reject the event through its existing terminal adaptation path, without numeric-only or metadata-free success
