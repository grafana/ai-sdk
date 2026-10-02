## MODIFIED Requirements

### Requirement: Scoped file options preserve semantic JSON
Supported message-level and ordinary file-part provider options, including nested tool-result file options, SHALL preserve namespace objects and opaque nested JSON at their original scope. Reserved host namespaces `gateway`, `grafana`, and `grafana-ai-sdk` SHALL fail safely rather than reach native providers unless an owning host feature consumes them. File-entry and function-tool options SHALL obey consumption-backed protections under gateway-native-provider-options, without selected-backend namespace or field filtering. Existing call-level options, body headers, and text-part options SHALL retain their mapping and host protections. This capability SHALL NOT enable output-level tool-result options or non-file nested result options.

#### Scenario: Scoped opaque values survive
- **WHEN** supported message/file scopes carry ordinary provider options containing nested null, false, zero, empty strings, arrays, or objects
- **THEN** the model SHALL receive semantically identical values at the same scopes, including explicitly empty namespace objects

#### Scenario: File options remain available to native interpretation
- **WHEN** file-entry or function-tool options contain both the selected backend's namespace and an unrelated ordinary namespace or an unlisted ordinary field
- **THEN** all ordinary namespaces and fields SHALL reach the model at their original scopes without Gateway inventory filtering
- **AND** native consumption and concrete bypass protections SHALL determine behavior

#### Scenario: Host namespace is not forwarded
- **WHEN** a supported message or file scope includes a reserved host namespace not consumed by an owning feature
- **THEN** mapping SHALL fail with a fixed safe response before resolution or invocation and SHALL NOT promote it to headers or call options

### Requirement: File validation preserves processing order and bounds
Malformed role/data/reference/media-field shapes, typed null, mixed arms, missing required fields, and schema-invalid file members SHALL fail before the execution boundary, resolution, and model invocation. Schema-valid runtime-deferred branches SHALL remain safe unsupported failures. The existing complete-request byte limit SHALL bound file strings and decoding work, including JSON escaping and base64 overhead. Native-provider restrictions SHALL be enforced before external provider I/O rather than guessed by the protocol mapper. The Gateway SHALL NOT retrieve file URLs or broaden the fallback capability subset. Retaining ordinary empty message-option namespaces SHALL NOT narrow existing text fallback eligibility; the gateway-ordered-text-fallback contract SHALL govern this no-op distinction. No selected-backend inventory SHALL sanitize requests to make them eligible for fallback.

#### Scenario: Invalid request has no effects
- **WHEN** a file request has conflicting arms, a forbidden reference member, a missing mediaType, a file directly in a tool-role content array, or a typed null filename
- **THEN** complete validation SHALL fail with zero execution-boundary, resolver, and model calls

#### Scenario: Encoded request byte boundary
- **WHEN** otherwise supported file requests are below, exactly at, and one byte above the configured complete-body limit
- **THEN** only the first two SHALL reach mapping and the oversized body SHALL fail without unbounded buffering or provider invocation

#### Scenario: Existing route restriction remains effective
- **WHEN** newly admitted files, active preserved options, or disallowed tool history target a text-only fallback route
- **THEN** that restriction SHALL remain effective before any physical candidate invocation
