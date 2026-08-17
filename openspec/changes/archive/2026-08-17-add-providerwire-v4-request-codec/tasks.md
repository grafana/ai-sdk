## 1. Request Contract Tests

- [x] 1.1 Create `gateway/providerwire/v4` package scaffolding and add request-focused tests for full canonical call-options conversion, explicit empty prompts, literal tool choices, response formats, reasoning values, and all scalar request settings.
- [x] 1.2 Add canonical message and content tests for every role-permitted content arm, provider options, private-field rejection, role mismatches, and canonical-only system strings.
- [x] 1.3 Add file-data tests for inline data, URLs, provider references, inline text, reasoning-file restrictions, ambiguous provider values, and preservation of empty inline data and empty inline text.
- [x] 1.4 Add function-tool, provider-tool, tool-call, tool-result output, and tool-result content tests covering every canonical discriminator, required object boundary, explicit false or empty values, nullable opaque JSON, output-level provider-options eligibility, and legacy-form rejection.
- [x] 1.5 Add table-driven strict-decoding tests for required fields, typed nulls, unknown discriminators, unknown standard fields, malformed active fields, inactive sibling-arm fields, opaque extension boundaries, qualified identifiers, provider options, and top-level or nested reserved gateway namespaces.
- [x] 1.6 Split any mixed codec coverage so request tests do not depend on result, stream, error, HTTP, SSE, catalog, or client implementation.

## 2. Private DTO and Validation Foundation

- [x] 2.1 Implement private request DTOs and shared JSON helpers for object decoding, required-field presence, typed-null rejection, active-field selection, complete JSON values, string maps and arrays, and provider-qualified identifiers.
- [x] 2.2 Implement strict provider-options conversion that requires object-valued namespaces, copies opaque JSON, and restores decoded values as `provider.RawProviderOption`.
- [x] 2.3 Implement canonical file-data conversion with explicit tagged variants, base64 byte encoding, provider-reference validation, reasoning-file restrictions, and empty tagged-value preservation.
- [x] 2.4 Implement reserved gateway-option cleanup: remove an empty top-level object, reject every other top-level form, and reject the namespace at every nested provider-options location.

## 3. Canonical Request Conversion

- [x] 3.1 Implement field-by-field message and content conversion with the canonical role/content matrix, system-string handling, required active-arm fields, fail-fast unknown-field handling, inactive sibling-arm tolerance, and privacy rejection.
- [x] 3.2 Implement field-by-field function-tool and provider-tool conversion, including schemas, examples, strict tri-state values, required provider-tool args, and provider-qualified IDs.
- [x] 3.3 Implement field-by-field tool-choice, response-format, reasoning, tool-call, tool-result output, and tool-result content conversion with canonical discriminators, arm-specific provider-options eligibility, and opaque JSON validation.
- [x] 3.4 Implement `EncodeCallOptions` and the unexported strict request decoder, ensuring `prompt` is always encoded and required on decode without invoking nested provider-domain JSON methods.
- [x] 3.5 Verify invalid or ambiguous provider-domain values return contextual errors without partial or silently normalized output.

## 4. Isolation and Legacy Compatibility

- [x] 4.1 Add a production-import test proving the strict package does not depend on the legacy provider-wire package or another gateway layer.
- [x] 4.2 Add external-package compile checks for the strict request encoder while confirming DTOs and strict request decoding are not exported.
- [x] 4.3 Add or retain compile checks for the existing provider-wire public surface and focused handler coverage proving canonical requests and currently supported legacy request forms still reach the model unchanged.
- [x] 4.4 Confirm no existing `gateway/providerwire` production file or provider-domain JSON behavior changes as part of the strict package introduction.

## 5. Validation and Specification Completion

- [x] 5.1 Run formatting and `go test -race ./gateway/providerwire ./gateway/providerwire/v4`, then run `go vet ./gateway/providerwire/...`.
- [x] 5.2 Update `test/conformance/PARITY.md` to classify the canonical-only request codec, its strict local validation rules, and its reserved gateway-namespace restriction; then run `mise run validate-parity-baseline` and `mise run parity-check`.
- [x] 5.3 Run `openspec validate add-providerwire-v4-request-codec --type change --strict`, `openspec validate --all --strict`, and `git diff --check`.
- [x] 5.4 Review the completed diff against every `gateway-providerwire-v4` request scenario and confirm it claims no result, stream, HTTP, SSE, catalog, or client capability.
- [x] 5.5 Synchronize the verified `gateway-providerwire-v4` specification into the canonical spec set, archive the completed change, and confirm `openspec list --json` reports no active changes.

## 6. Fail-Fast Unknown Request Fields

- [x] 6.1 Add focused tests that reject unknown top-level and nested standard request fields while preserving inactive sibling-arm fields and explicit opaque extension boundaries.
- [x] 6.2 Implement shared unknown-field validation across every strict request object without widening the public API or changing legacy production behavior.
- [x] 6.3 Update the proposal, design, canonical and archived specifications, and parity classification to distinguish fail-fast request fields from response-side additive tolerance.
- [x] 6.4 Run focused race tests, vet, parity checks, strict OpenSpec validation, `git diff --check`, and a final request-only scope review.
- [x] 6.5 Reject the reserved `type` key in provider references, document Gateway controls as intentionally unsupported for this phase, and rerun focused and parity validation.
- [x] 6.6 Preserve explicit empty `tools` and `stopSequences` across strict request encoding and decoding so downstream defaults remain disabled, then rerun focused and parity validation.
- [x] 6.7 Preserve both value and pointer forms of object-valued `provider.RawProviderOption` without exposing Go wrapper fields, and reject nil pointers.
