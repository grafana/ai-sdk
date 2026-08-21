## Context

Phase 1 pinned the compatibility target to `@ai-sdk/provider@4.0.7`, `@ai-sdk/gateway@4.0.52`, and upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. Its semantic captures and external-package witnesses demonstrate six provider-domain losses: three `number` fields narrowed to integers, absent versus explicit false for two booleans, absent versus explicit empty for seven optional strings, and an empty inline-text file-data arm that external code cannot construct or inspect.

The immediate parent and legacy compatibility baseline is commit `32e5ab7f1ab9e524477cc0ece04c690a89854a24`. Its Go `int` request settings can encode exact 64-bit integers outside JavaScript's exactly representable integer range. Replacing those fields directly with `float64` would satisfy the pinned fraction evidence but violate the program's unconditional byte-stability guarantee for historical Go values.

The provider package is a transport-neutral input contract shared by root orchestration, middleware, fallback, direct providers, and the hosted Grafana client. The deployed `gateway/providerwire` package is a tolerant legacy transport whose request bytes and parent-decoder compatibility must remain stable while the future strict `gateway/providerwire/v4` codec is developed separately. Existing provider request structs and custom JSON methods currently blur those boundaries by describing generic `encoding/json` output as protocol authority.

## Goals / Non-Goals

**Goals:**

- Represent every provider-model distinction established by the pinned request evidence and archived loss analysis in public Go values.
- Preserve every historical Go integer exactly while representing every finite JavaScript-number value emitted by the pinned Gateway client.
- Keep request types normalized around language-model meaning rather than ProviderWire envelopes, headers, or routes.
- Make root orchestration and all supported providers consume redesigned values intentionally, without silent truncation, omission, concatenation, or presence loss.
- Preserve source/response filename APIs, nil versus non-nil empty collections, and opaque provider-option JSON.
- Preserve legacy ProviderWire request bytes for every value accepted by the parent encoder, and preserve parent-decoder compatibility for the subset that the parent decoder accepted.
- Replace temporary loss witnesses with durable positive public request-contract coverage, then retire the completed handoff artifacts.
- Leave the provider contract ready for an explicit strict V4 mapper.

**Non-Goals:**

- Implementing `gateway/providerwire/v4`, strict syntax/schema validation, a strict handler, or a reusable V4 client.
- Redesigning response-domain types, source filename representation, stream parts, UI chunks, service configuration, authentication, discovery, or policy.
- Making the provider model itself enforce every role or inactive-arm rule from the strict JSON Schema.
- Introducing a generic optional-value or general union framework, generated provider types, or dual-dialect tolerance in provider request values.
- Preserving source compatibility with the old request structs.

## Decisions

### Use one focused exact number type for the three evidenced settings

The provider package introduces this public API:

```go
type LanguageModelNumber struct {
    // private representation
}

func LanguageModelNumberFromInt(value int) LanguageModelNumber
func LanguageModelNumberFromInt64(value int64) LanguageModelNumber
func LanguageModelNumberFromFloat64(value float64) (LanguageModelNumber, error)

func (n LanguageModelNumber) Int64() (int64, bool)
func (n LanguageModelNumber) Float64() (float64, bool)
```

`CallOptions.MaxOutputTokens`, `CallOptions.TopK`, and `CallOptions.Seed` become `*LanguageModelNumber`.

The representation has private integer and floating variants. Integer constructors preserve the exact signed integer. The float constructor rejects NaN and positive or negative infinity. A finite float that is mathematically integral, within `int64`, and round-trips exactly through `int64` is canonicalized to the integer variant, including negative zero to integer zero; every other finite float retains its IEEE-754 value.

`Int64` succeeds only for the integer variant. `Float64` succeeds for the floating variant and for an integer exactly representable as `float64`; it fails for a large exact historical integer that conversion would round. The zero value is invalid and protocol/provider adapters must reject it. Retained compatibility JSON methods encode the integer variant as its exact decimal integer and the floating variant as a finite JSON number. Decoding first preserves a plain decimal integer token exactly when it fits `int64`; otherwise it parses the token as a finite `float64` and applies normal canonicalization. It rejects null, non-number input, malformed numbers, and non-finite results. This preserves both large historical `int64` values and finite JavaScript numbers outside `int64`.

This type is a focused response to one concrete three-field representation need, not a generic number or union framework.

Alternatives rejected:

- `*float64`: loses historical integers above the IEEE-754 exact range and cannot satisfy parent byte stability.
- `json.Number`: admits invalid lexical strings and makes JSON lexical representation part of a transport-neutral provider API.
- Keeping `*int`: cannot carry the valid pinned fractional requests.
- Globally narrowing historical compatibility to safe integers: contradicts the fixed legacy coexistence guarantee without an approved migration exception.

### Use pointers only for evidenced scalar presence and split request filename from source filename

`CallOptions.IncludeRawChunks` and request-side `ContentPart.ProviderExecuted` become `*bool`. Response-format name and description, function-tool description, approval reason, tool-result file filename, and execution-denied reason become `*string`.

`ContentPart.Filename string` remains unchanged for generated response files and sources, preserving the response/source API. Prompt request-file filename presence uses a new direction-specific field and helper:

```go
type ContentPart struct {
    // ContentPartTypeFile only.
    FilePartFilename *string `json:"-"`

    // Generated response files and ContentPartTypeSource only;
    // existing response/source behavior remains unchanged.
    Filename string `json:"filename,omitempty"`
}

func FilePart(mediaType string, data DataContent) ContentPart
func FilePartWithFilename(mediaType string, data DataContent, filename string) ContentPart
```

`FilePart` means request filename absent. `FilePartWithFilename(..., "")` means explicitly present and empty. Request codecs and providers read `FilePartFilename`; source and generated-response handling continue to write `Filename`. `ToResponseMessages` converts a generated file into the next request by copying a present response `Filename` into `FilePartFilename` and clearing `Filename`. A request file with non-empty response/source `Filename`, a source with non-nil `FilePartFilename`, or a value with both fields populated is invalid at a request mapping boundary.

`ContentPart` compatibility JSON becomes arm-aware and request-directional for file parts. A request file emits `filename` from `FilePartFilename`, including explicit empty; a source emits it from `Filename`. To retain historical generated-file bytes, a file with nil `FilePartFilename` may encode non-empty response `Filename` as `filename`, but file decoding always normalizes that member into `FilePartFilename`; source decoding populates `Filename`. Therefore generic JSON round-trip of a generated response file is intentionally a response-to-request normalization, not structural response equality. Compatibility encoding rejects both fields populated rather than choosing one. Generated response APIs and response/stream codecs do not use this generic provider-message decode path and remain unchanged. Legacy ProviderWire does not delegate to this method: its private adapter applies the parent migration projection and preserves the parent's permissive bytes independently.

The finite production migration inventory is:

| Area | Files | Required ownership |
| --- | --- | --- |
| Provider model and compatibility JSON | `provider/content.go`, `provider/message.go` | Constructors and file-arm JSON use `FilePartFilename`; source/generated-response values retain `Filename`. |
| Root producers and consumers | `convert.go`, `to_response_messages.go`, `streamtext.go` | UI/request input and next-step prompts write `FilePartFilename`; generated file content retains `Filename` until converted. |
| Direct providers | `providers/anthropic/convert_request.go`, `providers/anthropic/convert_citations.go`, `providers/bedrock/convert_messages.go`, `providers/openai/convert_messages.go`, `providers/openai-compatible/convert_request.go`, `providers/openai/convert_provider_tool_continuation.go` | Prompt file conversion and citation tracking read `FilePartFilename`; generated sources keep response filenames; OpenAI tool-result files default only nil filenames and preserve explicit empty. |
| Request inspection and privacy | `middleware/logger/logger.go`, `middleware/agentobservability/map_request.go`, `middleware/agentobservability/media.go` | Request mapping and media inference read `FilePartFilename`; generated media reads `Filename`; privacy redaction clears both filename fields. |
| Transport and harnesses | `gateway/providerwire/request.go`, `test/conformance/runner.go`, `test/interop/testserver/main.go` | Legacy migration is explicit; request fixtures/probes construct and inspect `FilePartFilename`. |

The matching tests in those packages are part of the migration inventory and must be updated or added before the field change is complete. A final repository search for `ContentPartTypeFile`, `FilePart`, `ToolResultContentValue`, and `Filename` must classify every remaining occurrence as request filename, tool-result request filename, generated response filename, source filename, or unrelated response/stream type.

Slices, maps, already-pointer scalars, required strings, and unrelated response fields stay unchanged. Nil and non-nil empty collections remain distinguishable in memory. Future strict codecs decide required wire presence explicitly; the tolerant legacy encoder retains the parent's `omitempty` collapse for non-nil empty collections to preserve parent bytes, while its decoder preserves explicit empty members received on the wire.

Alternatives rejected:

- Changing shared `ContentPart.Filename` to `*string`: creates an unevidenced source/response API break.
- A generic `Optional[T]`: unnecessary because the evidenced scalar states need only pointer presence and a value.
- A separate sealed file-part hierarchy: Phase 1 found no broader union representation defect.

### Define an exact public DataContent selection API without changing response values

`DataContent` remains the shared request/response value with its existing exported payload fields and private selection state. The provider package adds an exported discriminator type, constructors, and an inspector:

```go
type DataContentType string

const (
    DataContentTypeData      DataContentType = "data"
    DataContentTypeURL       DataContentType = "url"
    DataContentTypeReference DataContentType = "reference"
    DataContentTypeText      DataContentType = "text"
)

type DataContent struct {
    Bytes     []byte          `json:"bytes,omitempty"`
    Base64    string          `json:"base64,omitempty"`
    URL       string          `json:"url,omitempty"`
    Reference json.RawMessage `json:"reference,omitempty"`
    Text      string          `json:"text,omitempty"`
    // private selection state only when zero-value inference is impossible
}

func BytesDataContent(data []byte) DataContent
func Base64DataContent(data string) DataContent
func URLDataContent(url string) DataContent
func ReferenceDataContent(reference json.RawMessage) DataContent
func TextDataContent(text string) DataContent

func (d DataContent) DataType() (DataContentType, bool)
```

Bytes and raw JSON inputs are copied. `DataType` first uses private selection state when an empty payload requires it; otherwise it infers exactly one arm from legacy payload fields: non-nil bytes or non-empty base64, non-empty URL, non-empty reference, or non-empty text. `DataContent{}` and conflicting values are invalid. On conflict, `DataType` returns the selected or first inferred candidate with `ok == false`; callers must not treat that candidate as valid.

The data arm permits empty bytes and empty base64; `Base64DataContent("")` uses the established non-nil empty-byte representation so selection and structural round trips remain stable. It rejects simultaneous non-nil bytes and non-empty base64. `URLDataContent("")` and `TextDataContent("")` record private URL or text selection. The reference arm requires a non-null JSON object whose values are strings and permits `{}`. Every selected or inferred arm rejects non-zero or non-nil payloads belonging to another arm.

Existing `MarshalJSON` and `UnmarshalJSON` remain production compatibility methods that emit and accept the established tagged union and preserve current structural response round trips. Decoding leaves private selection empty whenever a non-empty legacy field or non-nil bytes can infer the arm; it records private selection only for otherwise-uninferable empty URL or text arms. Protocol adapters call `DataType` and inspect payload fields directly; they never inspect private state or use generic JSON as protocol authority. `GenerateContentPart.Data`, response/source construction, response codecs, existing untagged response literals, and `reflect.DeepEqual` response tests remain unchanged.

Alternatives rejected:

- Adding an exported `Type` field: decoding existing untagged response values would populate new structural state and break inherited response equality.
- A constructor without a public inspector: an external strict V4 mapper still could not identify an empty selected arm.
- Inferring the arm only from non-zero payload fields: reproduces the empty-text loss.
- Reusing `StreamFileData`: prompt data additionally supports reference and text arms and has a different contract.

### Keep flat request values transport-neutral and move wire authority to codecs

`Message`, `ContentPart`, `Tool`, and `ToolResultOutput` remain flat discriminated domain structs because all valid registered arms are representable after the targeted changes. Provider documentation and canonical specs stop claiming their generic JSON form is a strict protocol. A strict V4 mapper must validate the selected role/arm and map every required field explicitly; in particular, it maps one valid system string without concatenating arbitrary parts and rejects invalid inactive-arm combinations before provider invocation.

Existing request custom JSON methods on `Message`, `DataContent`, `ToolResultOutput`, `ToolResultContentValue`, and related request values remain as compatibility behavior for existing generic JSON consumers. `ContentPart` gains the minimal arm-aware compatibility mapping required to serialize `FilePartFilename` as `filename` for request file arms and retain `Filename` for source arms. File decoding is explicitly request-directional; generic encoding and decoding of a generated response file normalizes its filename into `FilePartFilename` and is covered as such rather than promised as structural response round-trip. `provider/upstream_encode_compat_test.go` and `provider/upstream_decode_compat_test.go` remain and are documented as compatibility tests. `provider/call_options_wire_test.go` is renamed to a compatibility-oriented name and no longer claims generic JSON is protocol authority. No new permissive decode dialect is added to provider request values.

`ToolResultOutput.Reason` becomes a pointer for request-domain presence, but the legacy stream decoder's `legacyToolResult` projection retains its historical flat result: nil and explicit-empty reasons both produce the JSON string `""`, while a non-empty pointer produces that string. This is the only permitted internal stream-path adjustment and does not change any valid historical stream bytes or stream-part API.

Response-domain provider types, dedicated response custom JSON methods, response codecs, stream wire representations, error codecs, and their inherited round-trip behavior otherwise remain unchanged. The request-directional generic `ContentPart` normalization above is not a response codec guarantee.

### Match pinned provider request behavior field by field

Provider conversion follows the exact registered sources rather than a generic integer-SDK policy:

| Provider | Exact numeric behavior |
| --- | --- |
| Anthropic and Vertex Anthropic | Forward `maxOutputTokens` and `topK` exactly when the pinned model/reasoning path supports them; preserve pinned model-cap and reasoning-budget adjustment/clamping, unsupported-model `topK` handling, thinking-mode sampling omission, and seed warning behavior. Fractional values use SDK extra-field overrides where generated integer fields cannot carry them. |
| Bedrock | Forward `maxOutputTokens` and `topK` exactly through the repository-owned JSON request representation when supported; preserve pinned Anthropic-thinking budget arithmetic and `topK` omission/warning plus seed warning behavior. |
| OpenAI Responses | Forward `maxOutputTokens` exactly, using the SDK extra-field override when the generated integer field cannot represent it; keep pinned unsupported warning/omission for `topK` and `seed`. |
| OpenAI-compatible | Forward `maxOutputTokens` and `seed` exactly through its map-based request body; keep pinned unsupported warning/omission for `topK`. |

Provider request tests and request snapshots cover a fraction for every supported forwarding path and the pinned omission, warning, arithmetic, and clamping paths by model capability and reasoning state. Integer variants preserve existing integral request bodies. A provider never silently rounds or truncates. If the SDK escape hatch fails to produce the pinned semantic request, implementation stops for an explicit deviation decision rather than substituting warning/omission silently.

Large historical integers continue through integer-capable paths. Provider-specific model or backend limits remain provider behavior and may warn or clamp only where the exact pinned implementation does so.

### Isolate the deployed tolerant request wire behind a parent-pinned adapter

`gateway/providerwire` keeps its public API and tolerant dialect, but request encoding/decoding maps through a complete private request-only transport representation instead of directly marshaling `provider.CallOptions`. The adapter explicitly handles exact numbers, pointer presence, parent-compatible collection emission, explicit collection presence during decode, system-message legacy behavior, file-data arms, tool results, and opaque provider options. Its encoder intentionally collapses non-nil empty collections exactly as the parent did because the provider value carries no provenance that could distinguish a historical value from a newly constructed one. It does not alter or wrap response, stream, or error codecs.

The byte-stability domain is defined as every `provider.CallOptions` value for which parent `EncodeCallOptions` at `32e5ab7f1ab9e524477cc0ece04c690a89854a24` returned bytes. The redesign migration projection replaces each changed parent field with its equivalent redesigned value, including moving a parent request-file `Filename` into `FilePartFilename`; it otherwise preserves all populated flat fields. This domain explicitly includes mixed inactive-arm fields and any `DataContent.Reference` containing valid JSON that the parent encoder emitted. The legacy adapter reproduces the parent's field emission, `omitempty`, and `DataContent` arm-precedence behavior for those values instead of applying strict request validation.

Before request-domain types change, a committed compatibility corpus is generated using that parent. Fixture metadata records the exact commit, parent-produced canonical bytes, the migration projection, and the parent decoder outcome. Successful parent decodes include the parent semantic projection; failed parent decodes record the exact rejection. After redesign, the new encoder must produce byte-identical output for every row. Byte equality proves encoder compatibility across representative field/arm partitions; previous-decoder acceptance is claimed only for rows whose parent decode succeeded and is proved from their recorded parent projection. The redesigned tolerant decoder reads every parent-produced corpus row that its legacy dialect supports; strict validity is not inferred from corpus acceptance.

Newly representable values use already-recognized request members, but are tested separately and are not described as historical byte compatibility. Invalid `LanguageModelNumber` zero values, non-finite inputs, and redesigned values with no parent or legacy representation return errors rather than being rewritten. Mixed inactive arms and parent-encodable reference payloads are not rejected by the tolerant legacy adapter; direct provider request conversion and the future strict V4 mapper retain their own selected-arm validation.

The Grafana hosted client and reusable handler continue to call `EncodeCallOptions` and `DecodeCallOptions`; neither gains a strict mode or imports future V4 types.

Alternatives rejected:

- Changing `gateway/providerwire` to strict V4: mixes deployed tolerant and new strict dialects.
- Adding strict/tolerant flags to one codec: creates a dual-dialect implementation.
- Continuing direct `json.Marshal(opts)`: leaves HTTP transport authority in provider structs.
- Proving compatibility with only the redesigned decoder: does not establish that the parent decoder accepted the emitted bytes.

### Retire the provider-model handoff after establishing positive coverage

The Phase 1 loss witnesses become positive external-package request-contract tests in `provider/request_contract_external_test.go`. The tests prove the exact number, optional scalar, and public file-data capabilities through only the exported provider API and run through normal Go test workflows.

The completed `test/providerwire-v4/phase2-delta.md` handoff and its markdown-to-test-name validation are removed after implementation. The archived Phase 1 OpenSpec change retains the loss rationale and handoff, the retired row-level table remains recoverable from repository history, and the evidence README and parity map describe the durable positive Go contract coverage. The ProviderWire V4 check remains focused on immutable pinned-client captures, classification, schema, source equivalence, and response probes; those artifacts remain unchanged unless their registered client inputs change.

## Risks / Trade-offs

- [The exact number type is more complex than `float64`] → Keep it private internally, expose only five constructors/accessors, and limit it to the three evidenced settings.
- [Provider SDK integer fields hide fractional parity] → Use existing extra-field overrides or repository-owned JSON DTOs and assert final semantic request bodies.
- [Large integer arithmetic can overflow in provider reasoning logic] → Branch on `Int64`/`Float64`, use checked integer arithmetic, and return the provider's established error/warning behavior rather than wrap.
- [Legacy bytes drift while request authority moves] → Generate the corpus before changing types, pin it to the exact parent, and compare redesigned bytes directly.
- [Two filename fields can be misused or old readers can silently read the wrong one] → Maintain the finite producer/consumer inventory, make compatibility JSON arm-aware, require providers and next-step conversion to use `FilePartFilename`, clear both fields during privacy redaction, and reject mixed state at request boundaries.
- [Shared request/response values need empty-arm selection without new structural response state] → constructors record private selection only for uninferable empty arms, while `DataType` provides the public inspection boundary.
- [Flat structs still permit other invalid inactive fields] → Preserve parent-encodable forms only in the tolerant legacy adapter; direct provider request mappers and the later strict V4 mapper reject invalid arms before model invocation.
- [Pointer presence can be collapsed by root helpers or middleware] → Add table-driven copy/merge tests for nil, explicit zero/false/empty, and non-nil empty collections.

## Migration Plan

1. Confirm exact pinned source equivalence and generate the parent-encoded/parent-decoded legacy request corpus at `32e5ab7f1ab9e524477cc0ece04c690a89854a24`.
2. Convert the Phase 1 witnesses into failing positive target assertions and add finite type-shape tests.
3. Implement `LanguageModelNumber`, exact `DataContent`, request-only filename presence, and the other pointer fields.
4. Update root producers and copy/merge middleware without changing response/source APIs.
5. Update each direct provider and prove the field-by-field pinned numeric behavior at final request bodies.
6. Introduce the request-only private legacy adapter and lock parent bytes plus recorded parent-decoder evidence.
7. Update the Grafana client/server paths, replace the temporary loss witnesses with stable external request-contract tests, retire the completed handoff artifacts, and update the evidence README and parity map.
8. Run focused module tests followed by ProviderWire, conformance, integration, interop, repository, vet, lint, and strict OpenSpec validation.

Rollback is a normal branch revert before deployment. No data migration or runtime flag is required; the legacy HTTP endpoint remains the deployed transport throughout this phase.

## Open Questions

None. If an exact provider request cannot be emitted through the verified SDK override or repository-owned request representation, or any parent request byte cannot be preserved, stop for an explicit migration or intentional-deviation decision as required by the program plan.
