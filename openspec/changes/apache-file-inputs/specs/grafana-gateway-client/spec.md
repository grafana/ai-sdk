## MODIFIED Requirements

### Requirement: Explicit request projection and presence
The client SHALL explicitly map the complete current `provider.CallOptions` shape into the registered LanguageModelV4 Gateway request projection without importing server DTOs or calling server validators. It SHALL preserve every representable absent, explicit zero, explicit false, empty string, empty array, empty object, nested null, selected empty union arm, URL string, and supported binary-to-base64 distinction. It SHALL omit the Go zero value `ReasoningProviderDefault` and encode every non-zero registered reasoning value. It SHALL reject invalid UTF-8, non-finite numeric values, invalid raw JSON, unknown discriminators, conflicting selected arms, and any other value without an unambiguous registered representation before authentication or network I/O. Ordinary prompt and tool-result file projection SHALL support data, URL, reference, and text arms, preserving selected empties and absent/empty/non-empty filenames. Message and file-part options SHALL retain their registered scopes. Reasoning files SHALL retain their narrower data/URL-only projection without implying server runtime support.

#### Scenario: Presence-sensitive text request is encoded
- **WHEN** a text/scalar call contains explicit zero, false, empty collections, empty strings, and opaque nested JSON
- **THEN** the semantic body SHALL match the equivalent request emitted by the exact `@ai-sdk/gateway` version registered in `test/conformance/upstream.yaml` for every Go-representable distinction

#### Scenario: File data is representable
- **WHEN** a selected binary, URL, reference, or text arm is supplied in an ordinary prompt or tool-result file
- **THEN** bytes SHALL become standard base64, URLs SHALL remain strings, selected empty payloads SHALL remain selected, and filename absence SHALL remain distinct from explicit empty
- **AND** the strict server SHALL own current runtime capability rejection

#### Scenario: Reasoning uses the Go default
- **WHEN** `Reasoning` is `ReasoningProviderDefault`
- **THEN** the body SHALL omit `reasoning` and parity evidence SHALL classify the unrepresentable explicit JavaScript `provider-default` presence as a parity-preserving Go adaptation

#### Scenario: Provider-domain input is invalid
- **WHEN** any selected value cannot be mapped unambiguously to the registered projection
- **THEN** both unary and streaming setup SHALL fail before token acquisition or HTTP and SHALL not silently omit or reinterpret the value

#### Scenario: Reasoning file uses a forbidden arm
- **WHEN** a reasoning file selects reference or text, or supplies an inactive filename
- **THEN** client encoding SHALL fail before authentication or network I/O
