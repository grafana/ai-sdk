## MODIFIED Requirements

### Requirement: Bounded response-local source identity
The Gateway SHALL preserve the provider source ID unchanged in unary and streaming output rather than generate response-local IDs or hashes. IDs SHALL remain nonempty valid UTF-8 and at most 1024 bytes, checked before representation. Repeated IDs, including an equal ID on different source variants, SHALL preserve their values and provider order rather than inventing uniqueness. Unary input-string/cardinality and streaming part/frame bounds SHALL remain effective; metadata accounting SHALL use #280's reviewed aggregate bounds over every namespace. No response-local identity map SHALL be needed.

#### Scenario: Repeated source
- **WHEN** the provider emits the same URL ID twice and a document with the same native ID
- **THEN** all three SHALL retain the original ID and order without response-local renaming

#### Scenario: Source ID exceeds its cap
- **WHEN** a source ID is empty, invalid UTF-8 or longer than 1024 bytes
- **THEN** existing safe adaptation failure SHALL occur before emitting the invalid source

### Requirement: Closed public source metadata
Source metadata SHALL use #280's supported ordinary providerMetadata contract at its original position, preserving unknown object-valued namespaces/keys and nested values within reviewed bounds instead of generating numeric-only citation metadata. Malformed or oversized metadata SHALL follow the bounded adaptation/protocol failure policy, not silent selective omission. The existing known-namespace 8192-byte projection cap SHALL be replaced by #280's reviewed whole-metadata/enclosing-document budget.

Native document title/filename and URL title/URL SHALL remain caller output, including OpenAI/Azure file_path identity. The Gateway SHALL NOT replace file_path display with Document or omit a supplied nonempty filename just to conceal the selected backend. Optional-empty string normalization, required empty document title, Title-before-legacy-Text and inactive-field omission SHALL remain as defined by Registered source output. Metadata SHALL not be recursively sanitized for secret-looking application strings; known credential-bearing transport is separately excluded under gateway-caller-response-policy.

#### Scenario: Native file path
- **WHEN** OpenAI or Azure file_path supplies a native file ID as title and filename and supported metadata
- **THEN** the original ID/title/filename and metadata SHALL survive, not Document/source-N/numeric citation substitutes

#### Scenario: Unknown nested metadata
- **WHEN** a source carries a future object-valued namespace containing null, empty objects/arrays and unknown fields within the reviewed bounds
- **THEN** both clients SHALL preserve those values under #280 rather than discard unrecognized citation details

### Requirement: Independent consumption and evidence

The Apache Go client SHALL decode both registered variants independently of Gateway implementation imports, preserve native source identity/display and ordinary supported metadata, require document title presence while accepting its empty string, and normalize optional strings. Its existing bounded HTTP/SSE consumption SHALL apply. The maintained exact-pinned Vercel/Go differential suite SHALL exercise unary and streaming sources through the handler and authenticated command. Raw schemas and privacy tests SHALL independently verify output because the registered Vercel client parses permissively. Provider-independent UI tests SHALL prove source assembly. Synthetic transport responses SHALL NOT be presented as recorded provider evidence. Published dependency tests and disposable workspace overlay tests SHALL be reported separately.

#### Scenario: Both clients
- **WHEN** the pinned Vercel and independent Go clients consume the same supported native source output
- **THEN** URL/document fields, metadata and order SHALL agree after the declared optional-empty normalization
