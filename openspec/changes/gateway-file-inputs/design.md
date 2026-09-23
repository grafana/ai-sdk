## Context

This implements WP15 / [#109](https://github.com/grafana/ai-sdk/issues/109) from `~/ai-sdk-ai-gateway-plan.md`, using the already-landed WP7 client and WP8 logical middleware. WP11/12 already provide ordinary tool-result history, so nested file results can be added without implementing provider-executed tools or approvals.

Current gaps:

- `provider/content.go` has private selection state, only `Base64DataContent`, `IsData`, and `IsURL`, and value-based reference/text dispatch. JSON decoding can represent an empty text arm that public Go construction cannot select. `MarshalJSON` can choose the first populated arm without validating conflicts.
- `ContentPart.Filename`, `ToolResultContentValue.Filename`, and ordinary UI `FilePart.Filename` are strings. `providers/grafana/request.go` omits empty filenames; native conversions use empty-string defaulting.
- `ai-gateway/providerwire/v4/request.go` rejects file parts and non-empty message/part options. `function_tools.go` accepts only text within content results. The complete schema already describes all registered file arms.
- Provider conversions and reusable recording inspect non-empty payloads in several places. Filename migration also touches root/UI conversions, source conversion, logger redaction, examples, and tests.

### Registered reference and evidence

`test/conformance/upstream.yaml` pins commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`. Planning inspected that exact Git object in `~/src/ai`, not its different checked-out HEAD. Package versions at the pinned commit match ai 7.0.107, provider 4.0.17, Gateway 4.0.87, Anthropic 4.0.58, Bedrock 5.0.88, OpenAI 4.0.71, and OpenAI-compatible 3.0.53. No baseline mismatch was substituted.

Reference paths at that commit:

- `packages/provider/src/shared/v4/shared-v4-file-data.ts` and `packages/provider/src/language-model/v4/language-model-v4-prompt.ts`: four file-data arms, optional filenames, registered roles/options, and the narrower data/URL-only reasoning-file union.
- `packages/ai/src/ui/{ui-messages,convert-to-model-messages,validate-ui-messages}.ts` and conversion tests: ordinary UI file filenames are optional strings and are forwarded without collapsing explicit empty values.
- `packages/gateway/src/gateway-language-model.ts`: `maybeEncodeFileParts` converts byte arrays for file/reasoning-file parts and nested tool-result files; ordinary JSON preserves empty selections and filename presence.
- `packages/anthropic/src/convert-to-anthropic-prompt.ts` and tests: text documents, reference resolution, options, and optional document titles.
- `packages/openai/src/responses/convert-to-openai-responses-input.ts` and tests: `filename ?? part-N.pdf` for request PDFs, `filename ?? 'data'` for tool-result files, and rejection of text-file inputs where unsupported.
- `packages/openai-compatible/src/chat/convert-to-openai-compatible-chat-messages.ts`: PDF `filename ?? 'document.pdf'`, with provider-specific media/arm restrictions.
- `packages/amazon-bedrock/src/convert-to-amazon-bedrock-chat-messages.ts` and tests: inline text documents, restricted S3 URLs, unsupported references, and document-name sanitization/defaulting that intentionally treats an empty name like absence.

Per `test/conformance/PARITY.md`, this spans the mixed provider contract/provider-adapter surfaces, automated ProviderWire projection, and mixed Gateway/client/host observation coverage. Request snapshots and client-emitted goldens are primary proof; schema acceptance alone is not runtime support and synthetic HTTP responses are not live-provider recordings.

## Goals / Non-Goals

**Goals:**

- Preserve every selected ordinary file arm, including empty text/data, across construction, projection, mapping, provider conversion, and permitted observation.
- Preserve absent, empty, and non-empty request/tool-result filenames until provider-specific conversion decides their meaning.
- Support file-bearing unary and streaming requests through both public clients, with bounded validation and safe errors.
- Preserve registered message and file-part options without enabling arbitrary call-level policy controls.

**Non-Goals:**

- Generated media responses (WP16), reasoning-file runtime support (WP17), custom content, tool approvals, provider-executed result history, or new fallback/replay semantics.
- File uploads/storage, Gateway URL fetching, content sniffing in the protocol mapper, or a cross-provider MIME allowlist.
- A baseline upgrade, a new wire dialect, a generic capability registry, or a general redesign of provider content types.

## Decisions

### 1. Explicit selection with the existing Go data container

Complete the public constructor family for bytes, base64, URL, reference, and text, retaining `Base64DataContent` and adding matching inspection for reference/text alongside `IsData`/`IsURL`. All constructors record selection even for empty payloads. Keep the existing fields and private typed selection mechanism rather than introducing interfaces or multiple new content structs.

Use one arm-resolution/validation path across inspection, JSON, and consumers. A selected arm and its own payload are one source, not two; bytes and base64 are alternative representations of the same data arm and cannot both be populated. An unselected zero value, a selected arm with another arm's payload, or two populated sources is invalid. Existing unambiguous field literals remain useful, but empty string arms require a constructor. Malformed, null, non-string-valued, or reserved-`type` references fail validation; `{}` remains a selected reference and provider-specific resolution determines whether it is usable.

Marshaling, SDK decoding, and direct conversion must not pick a winner among conflicting arms. Tagged decoding checks for other arm members before discarding fields, including inactive members whose values are empty or null; for example, `{"type":"text","text":"","url":""}` is invalid. This is a focused union check, not a copy of the Gateway schema. Existing supported legacy forms remain decodable when unambiguous, but a legacy alias cannot override or supplement a tagged payload. Do not use a JSON round trip to construct an empty text arm or infer text as the default branch.

Alternative rejected: relying solely on non-empty fields cannot represent selected empties. A full exported tagged-union redesign would unnecessarily expand migration scope.

### 2. Filename presence is confined to the input boundary

Change `ContentPart.Filename`, `ToolResultContentValue.Filename`, and ordinary UI `FilePart.Filename` to `*string`: nil means absent, pointer-to-empty means supplied empty. Update private DTOs and the independent client encoder accordingly. Native converters use nil-based defaults only where pinned upstream uses nullish defaulting; Bedrock retains its own empty-name normalization and sanitization rather than inheriting an invented global default rule.

Ordinary UI file parts are request inputs, not merely descriptive metadata: preserve all three filename states through UI JSON, `ConvertToModelMessages`, the provider domain, and Go Gateway projection for user and assistant roles. Add deterministic cross-language conversion coverage against pinned `ai`; filename presence must not disappear before the provider boundary.

`ContentPart` also carries source data, so source helpers explicitly normalize the pointer representation at that flat-struct boundary. `SourceInfo`, generated content, stream source/file fields, and UI `SourceDocumentPart` retain existing descriptive-string semantics. Generated-to-input conversions explicitly normalize these string-only sources; they cannot invent lost filename presence. Keep those normalization tests separate from lossless UI input tests. This does not add filenames to generated file SSE chunks or enable WP16 output.

Alternative rejected: a parallel `FilenameSet` flag produces contradictory states and adds more public API than a pointer. Making every descriptive filename optional would contradict the technical plan.

### 3. Enable only ordinary file-input branches

Extend private wire DTOs and shallow mapping switches after complete schema validation. Map ordinary user/assistant files and file entries in tool-role content results using public selected-arm constructors. Preserve order, media type, filename presence, and all four arms. Reuse file-data conversion for nested result files without routing through provider JSON marshalers.

The current tool-result subset remains otherwise unchanged: assistant provider-executed history, approvals, execution-denied, and custom result content stay unsupported. Reasoning files remain runtime-deferred to WP17; their existing data/URL-only schema and Go-client projection are regression-tested. The comprehensive union golden contains unrelated deferred capabilities: replay it unchanged for schema/support rejection and exercise its ordinary file cases in focused client-generated requests that can reach the model. Do not claim the complete comprehensive request becomes executable.

Alternative rejected: enabling all siblings encountered in a comprehensive golden would silently implement several other work packages.

### 4. Preserve scoped options without opening call-level policy

Map registered message-level options for supported messages and ordinary file-part options, including nested file result entries, to `provider.RawProviderOption`. Preserve opaque nested null/false/zero/empty values and namespace objects. Keep this independent from top-level call options, body headers, non-file part options, and deferred output-level tool-result options.

Reuse the existing function-tool namespace policy: reserved `gateway`, `grafana`, and `grafana-ai-sdk` namespaces cannot be forwarded as provider options at these scopes; reject them safely until an explicit host consumer owns them. Ordinary options remain attached to their original scope, never promoted to call options or outer HTTP headers. Do not drop an empty namespace object in newly supported scopes merely because it is operationally a no-op.

### 5. Validate at the boundary that owns the rule

The strict schema owns missing/null values, mixed/inactive members, reference object shape, and role unions. Mapping failures occur before the execution boundary, resolver, and model. The existing request byte limit bounds inline strings and any later decoding; test below/at/above the complete encoded-body limit including escaping/base64 overhead. No new arbitrary per-file limit or file-count quota is necessary.

The provider domain owns unambiguous selection and direct-caller structural validation. Native converters own supported roles, media detection, URL schemes, provider-reference lookup, native filename rules, and provider-specific warning/error behavior. Validate invalid/conflicting direct inputs before outbound provider HTTP. A structurally valid reference missing the selected provider is not a schema error; resolve it in the native converter. Preserve upstream-supported bare top-level/wildcard media types and do not globally reject base64-like strings that pinned providers intentionally interpret as file IDs.

“Before provider invocation” means zero model calls for invalid/unsupported protocol branches. Native-provider-specific unsupported content is detected during conversion before external provider I/O; the provider-independent mapper cannot predict every model's support. Preserve established upstream warning behavior and registered deviations rather than inventing a universal error policy. In particular, the existing Bedrock tool-result URL warning/omission deviation remains separately documented.

No Gateway or observer fetches a submitted URL. Preserve existing text fallback eligibility when newly retained ordinary message-option namespaces contain only empty objects. The current mapper drops these namespaces, while the route guard rejects any non-empty map; preserving representation therefore requires the guard to distinguish empty objects from active options without mutating the request. A namespace with any member, including nested null/false/zero/empty values, remains active. Invalid or reserved namespaces do not qualify. Test authenticated text failover with `{"vendor":{}}`, retaining that value at each candidate; files, active options, and effectful history remain rejected. This preserves the established text subset rather than enabling file fallback.

### 6. Observation stays metadata-only in the Gateway

Update reusable Agent Observability mapping and logger redaction for selection and pointer filenames. Retain existing payload-capture controls and deliberate omission of unsupported media representations; reference/text files must not be reinterpreted as empty binary or missing content. Gateway composition continues to export one canonical logical generation with approved metadata only. Use hostile markers in data, text, URLs/query strings, references, filenames, and provider options to verify none appear in records/logs/metrics/errors. No high-cardinality filename/URL/reference metric labels.

### 7. Evidence precedes behavior changes

Add focused requests emitted by the exact registered TypeScript Gateway client, including paired omitted/empty filenames and empty text/data in both ordinary and nested file positions. Update committed semantic goldens only through the explicit update task. Confirm the current Go replay fails before enabling the mapper. Differential tests exercise the independent Go client against the same cases and retain zero-auth/zero-network checks for ambiguous Go values. SDK JSON tests reject mixed tagged arms before information is lost. Root integration tests send user/assistant UI files with absent/empty/non-empty filenames through Go JSON and model conversion, comparing pinned TypeScript conversion and resulting client projection. File SSE chunks remain unchanged; use the registered UI-message validation/conversion contract to prove filename presence, not an unregistered chunk field.

Native request assertions cover Anthropic/Vertex conversion, Bedrock, OpenAI Responses, and OpenAI-compatible behavior, with targeted pinned-upstream expectations for empty selection and filename defaults. Use existing provenance-valid replay inputs where sufficient; otherwise use clearly synthetic unit/native-request tests and document any remaining provider-boundary coverage gap. Never modify provider recordings or label invented events as upstream evidence.

## Risks / Trade-offs

- [Cross-module source-breaking filename migration] → Migrate all call sites, publish Apache prerequisites first, then pin proxy-resolvable immutable versions in Gateway and dependent modules. A local workspace is not merge evidence.
- [Go SDK `omitempty` erases an explicit empty native title/filename] → Assert raw native request JSON, not only intermediate structs; use supported native-SDK presence mechanisms or a narrow explicit encoding fix.
- [Shared filename field affects sources and continuation] → Test source normalization, generated-to-input conversion, and tool-result continuation separately without widening generated/source APIs.
- [Provider support differs by arm, role, and media] → Keep protocol acceptance distinct from native support, compare pinned source/tests, and classify observed differences as bugs, Go adaptations, documented deviations, or coverage gaps.
- [Sensitive files/options enter the existing text telemetry chain] → Preserve metadata-only Gateway policy and assert markers across success, failure, cancellation, unary, and streaming paths.
- [Overly broad golden is blocked by another deferred family] → Retain the original and add focused registered-client captures; do not hand-edit evidence into an executable request.

## Migration Plan

1. Add regression evidence and implement the Apache data/filename migration, providers, independent client, and reusable middleware. Update affected examples and centralized input documentation.
2. Publish approved immutable SDK/provider prerequisite commits; update dependent module pins and run readonly `GOWORK=off` checks. Stop for owner action if publication is unavailable; do not commit workspace replacements.
3. Enable AGPL file mapping and scoped options; extend handler, authenticated-service, privacy, bounds, and differential tests. Keep existing response/SSE families unchanged.
4. Run providerwire, parity, module-resolution, build, lint, and relevant integration checks. Update stable parity coverage only if its evidence boundary changes. Extend deployment smoke when this capability is activated and retain existing license/source-offer obligations.
5. Roll back by deploying the previous coherent Gateway/module set. Requests containing files revert to safe unsupported failures; there is no storage migration and no legacy compatibility mode.

## Open Questions

No product decision blocks this proposal. Implementation must verify raw native-SDK encoding for explicit empty filenames/titles and the current publication path before claiming completion. If those checks require a design change, stop for approval rather than widening scope or accepting workspace-only proof.
