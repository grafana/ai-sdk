## Context

`gateway/providerwire` is a public JSON, HTTP, and SSE transport whose request decoder remains intentionally tolerant of canonical LanguageModelV4 JSON and historical Go encodings. Its request helpers rely on JSON methods attached to provider-domain types. That is appropriate for compatibility, but it does not provide a narrow validation boundary for untrusted canonical requests or isolate canonical bytes from future provider JSON changes.

The registered provider contract defines TypeScript discriminated unions for call options, messages, content, tools, tool results, file data, provider options, and model settings. Some runtime boundary rules—rejecting typed nulls, private transport fields, malformed provider references, and reserved gateway controls—must be enforced by this repository because TypeScript types alone do not validate incoming JSON.

## Goals / Non-Goals

**Goals:**

- Introduce an independent strict request codec at `gateway/providerwire/v4`.
- Emit canonical LanguageModelV4 request JSON from `provider.CallOptions`.
- Strictly decode canonical request JSON into `provider.CallOptions` for package-internal consumers.
- Keep canonical wire decisions in private DTOs and explicit conversion code.
- Reject invalid, ambiguous, legacy-only, private, or reserved values during strict request conversion so a later transport boundary can fail before provider invocation.
- Reject unknown standard request fields before model invocation while preserving inactive sibling-arm fields and explicit opaque extension boundaries.
- Preserve every supported request semantic value.
- Preserve the existing provider-wire package's public API and tolerant behavior.

**Non-Goals:**

- Result or stream decoding, error envelopes, SSE framing, HTTP handlers, routes, timeouts, or transport limits.
- Model catalog lookup or model invocation.
- Grafana provider transport selection.
- A public DTO model or a public general-purpose strict request decoder.
- Changes to existing provider-wire production code.

## Decisions

### 1. Use an independent sibling package

The strict codec will live in `gateway/providerwire/v4` with package name `providerwirev4`. Production code in this package will depend only on the standard library and `provider`; it will not import the legacy provider-wire package or higher gateway layers.

Keeping the implementations independent prevents compatibility behavior in the legacy decoder from becoming implicit strict behavior. Reusing the legacy helpers was rejected because their accepted dialect and provider marshaler coupling are intentional but incompatible with a strict canonical boundary.

### 2. Own the wire shape with private DTOs

Request objects and nested unions will use unexported DTOs. Conversion to and from `provider.CallOptions` will copy fields explicitly. Opaque JSON values—schemas, tool inputs, tool arguments, provider options, and JSON tool outputs—may remain `json.RawMessage` after validation.

Directly marshaling `provider.CallOptions` was rejected because provider-domain JSON methods accept compatibility forms and can change canonical bytes outside this package. Exporting DTOs was rejected because it would create a second public provider data model.

### 3. Export encoding but keep strict decoding internal

`EncodeCallOptions` will be the request codec's public entry point. Canonical request decoding will remain unexported until an HTTP boundary needs it. This establishes canonical outbound bytes without presenting an unversioned public decoder whose validation policy would become an external compatibility promise.

### 4. Validate raw objects before typed conversion

Decoding will first inspect each JSON object as `map[string]json.RawMessage`, require the discriminator and active-arm fields, reject typed nulls where the contract requires a concrete value, and reject field names outside the complete understood field set for that request object. It will then decode only fields selected by the active discriminator, so known fields belonging exclusively to inactive sibling arms remain harmless and ignored.

Provider-options namespaces, provider references, headers, schemas, tool inputs, tool arguments, and JSON tool outputs remain explicit opaque or keyed extension boundaries and are validated according to their own contracts rather than a fixed inner-field allowlist. This selective approach is preferred over `DisallowUnknownFields`, which cannot distinguish standard request objects from extension maps or inactive union fields, and over direct `json.Unmarshal`, which would silently discard unsupported caller intent or turn missing, null, or malformed union values into unsafe zero values.

### 5. Enforce request semantics at the DTO boundary

The converter will enforce the role/content matrix, canonical tool and tool-result unions, required object boundaries for schemas and examples, provider-qualified identifiers, provider-reference objects, supported reasoning and literal choice values, and valid opaque JSON. Known legacy encodings and request-private provider fields will be rejected even when Go provider types can represent them.

JSON `null` remains valid only where the canonical value itself is opaque and nullable, such as tool-call input and JSON tool-result output. Optional typed fields that are present as `null` are invalid.

### 6. Treat gateway options as reserved control data

At the top-level provider-options map, an absent or empty `gateway` object will be removed before producing provider options. A non-empty, null, or non-object top-level value will fail. Any nested `gateway` namespace will fail, including an empty object.

This keeps routing controls out of provider-visible options and prevents nested data from bypassing the single top-level control boundary. Introducing a public gateway-control DTO was rejected because request routing is outside this codec.

### 7. Preserve tagged empty file data explicitly

File data conversion will preserve each canonical tagged variant. In particular, empty inline data and `{"type":"text","text":""}` must survive decode and re-encode without collapsing to an absent or different variant. Provider references must remain open provider-keyed JSON objects with string values.

This requires an explicit boundary convention rather than provider JSON methods: empty inline data uses a non-nil empty byte slice, while a present zero-valued `DataContent` in a full file or tool-result file arm represents canonical empty inline text. Reasoning-file conversion does not use that convention because its contract excludes text data.

### 8. Prove strict and legacy behavior separately

Focused strict-package tests will cover canonical request JSON, every supported union arm, encoding rejection, strict decoding rejection, unknown standard fields, inactive sibling-arm tolerance, opaque extension boundaries, typed nulls, privacy fields, reserved namespaces, and empty file data. Dependency tests will enforce package independence. Existing-package tests will compile its exported API and confirm its tolerant request path remains unchanged.

Parity validation will run against the registered provider baseline in addition to focused Go tests. The parity coverage map will classify the canonical-only codec and its reserved gateway-namespace restriction. No frontend interop scenario is required because this change does not add an HTTP route or alter bytes emitted by the existing transport.

## Risks / Trade-offs

- **Canonical definitions can drift from the registered provider contract** → Keep request cases exhaustive, run parity checks, and compare contract changes before extending the codec.
- **A newer caller can send a standard request field this baseline does not understand** → Reject it before model invocation instead of silently changing caller intent; adopt the field through an explicit pinned-baseline codec update.
- **Fail-fast validation reduces forward availability for harmless request additions** → Preserve designated opaque extension boundaries and inactive sibling-arm fields; prefer an explicit compatibility error over executing with unknown semantics.
- **Go zero values can erase tagged empty file variants** → Use explicit DTO presence and round-trip tests for empty inline data and text.
- **Two request codecs increase maintenance cost** → Keep the strict package small and independent; preserve the legacy package rather than mixing incompatible policies.
- **A provider type may represent values outside canonical LanguageModelV4** → Reject ambiguous or unrepresentable values during strict encoding instead of choosing a lossy representation.

## Migration Plan

This is an additive package introduction. Existing callers continue using `gateway/providerwire` unchanged. The strict request codec can be consumed by a separately designed transport boundary after its request contract is established. Rollback consists of removing the new package and its tests; no persisted data or existing API requires migration.

## Open Questions

None.
