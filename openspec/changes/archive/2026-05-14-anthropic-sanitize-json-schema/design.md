## Context

The Anthropic provider's `applyResponseFormat`
(`providers/anthropic/convert_request.go:21`) currently passes the JSON schema
from `ResponseFormat.Schema` straight through to the SDK helper
`anthropic.BetaJSONSchemaOutputFormat(schemaMap)` when the model supports
native structured output. Anthropic's constrained decoder rejects a documented
set of JSON Schema validation keywords (numeric ranges, string length, regex
`pattern`, array cardinality, object property counts, `not`, and unsupported
`format` values), so passing a schema produced by typical generators (Zod-like
helpers, `invopop/jsonschema`) often results in an HTTP 400.

Upstream addresses this in `@ai-sdk/anthropic` via a `sanitize-json-schema.ts`
helper applied only at the constrained-decoder call site. The full untouched
schema remains available for AI SDK result validation in the orchestration
layer; only the payload Anthropic sees on the wire is relaxed. The Go port
mirrors the same call site (`OutputConfig.Format` assignment) but currently
lacks the sanitizer.

Stakeholders are downstream users wiring structured outputs against Sonnet
4.x / Opus 4.7 via this provider, plus the conformance suite which exercises
non-trivial schemas.

## Goals / Non-Goals

**Goals:**
- Strip JSON Schema validation keywords Anthropic rejects from the schema
  written to `OutputConfig.Format` while preserving the same constraints as
  human-readable hints in `description`.
- Match upstream's exact wording and ordering of the description appendix so
  conformance against the TypeScript reference stays straightforward.
- Recurse the schema (composition, items, properties, definitions/`$defs`).
- Preserve unsanitized schema for the orchestration layer's result validation
  by operating on a deep copy.
- Cover behavior with table-driven unit tests aligned with upstream snapshots.

**Non-Goals:**
- Sanitizing the tool-fallback `inputSchema` (upstream leaves it alone; the
  tool input path uses a different Anthropic validator that accepts the full
  draft-7 surface).
- Sanitizing schemas attached to user-defined tools.
- Introducing a typed `JSONSchema` Go struct. The provider already represents
  the schema as `map[string]any` to match the SDK helper signature.
- Two-way semantic preservation of removed constraints; description text is
  advisory for the model.

## Decisions

### D1: Sanitize at `applyResponseFormat`'s native path, not centrally

Apply `sanitizeJSONSchema` immediately before the call to
`anthropic.BetaJSONSchemaOutputFormat(schemaMap)`. This matches upstream's call
site (`anthropic-language-model.ts:471`) and keeps the orchestration layer's
schema (used by `output.Object` / `output.Array` / `output.Choice` validators)
untouched.

**Alternatives considered:**
- Sanitize earlier in `buildParams` and reuse for the tool fallback. Rejected:
  the tool fallback path uses the schema as a tool `input_schema`, which
  Anthropic's tool validator accepts; mirroring upstream avoids
  over-sanitization.
- Sanitize at the orchestration layer before calling the provider. Rejected:
  that would alter what AI SDK validates against and is provider-specific
  knowledge that belongs in the provider package.

### D2: Operate on `map[string]any`

The Go port already unmarshals `ResponseFormat.Schema` into `map[string]any`.
Sanitizing on that representation avoids introducing a typed JSON Schema model
just for this concern and keeps the function decoupled from
`invopop/jsonschema`.

**Alternatives considered:**
- Introduce a `JSONSchema` struct and walk it typed. Rejected as YAGNI; the
  upstream TS file is itself one self-contained function operating on a
  schema-shaped object.

### D3: Non-mutating, deep-cloning by construction

The sanitizer builds a new `map[string]any` for each node rather than
mutating the input. Upstream tests assert non-mutation explicitly. The Go port
follows the same contract so the schema retained by the caller (for result
validation) is unchanged.

### D4: Description appendix wording

Match upstream's text exactly:
- Constraint name camelCase is split into space-separated lower-case words
  (`minLength` → `min length`, `exclusiveMinimum` → `exclusive minimum`).
- Values are rendered with `formatConstraintValue`:
  - strings render verbatim,
  - everything else is JSON-encoded.
- Items joined with `"; "`, trailing `"."`.
- Existing `description` is preserved and the appendix is appended on a new
  line (`description\nappendix`).

Identical wording keeps the Go output byte-comparable to upstream snapshots,
which simplifies cross-port regression testing.

### D5: `oneOf` becomes `anyOf`; `$ref` short-circuits; objects get `additionalProperties: false`

Mirror upstream:
- `oneOf` is rewritten to `anyOf` (upstream rationale: the constrained
  decoder doesn't enforce mutual exclusivity and Anthropic recommends
  `anyOf`).
- A node containing `$ref` returns `{ "$ref": <value> }` only; any sibling
  keywords are dropped (consistent with draft-7 semantics that siblings to
  `$ref` are typically ignored).
- Object nodes (`type == "object"` or `properties != nil`) always have
  `additionalProperties: false` set on the output, even if the input did not
  specify it.

### D6: Constraint detection rules

The constraint is included in the description appendix when the value is
"present and non-default":
- skip if the key is missing or holds Go `nil`,
- skip boolean `false` (matches upstream's `value === false` guard so e.g.
  `uniqueItems: false` isn't reported).
- `multipleOf` and other numeric keys pass through `formatConstraintValue`
  unchanged.

The `format` key receives special treatment: supported values
(`date-time`, `time`, `date`, `duration`, `email`, `hostname`, `uri`,
`ipv4`, `ipv6`, `uuid`) are preserved on the output; unsupported values are
dropped and appended to description as `format: <value>`.

## Risks / Trade-offs

- **[Risk]** A description appendix subtly differing from upstream could
  affect any downstream snapshot test that mirrors upstream output. →
  **Mitigation**: table tests reproduce the upstream snapshot strings
  verbatim.
- **[Risk]** Schemas with nested non-object types Anthropic accepts could
  still hit decoder limits we don't model (e.g., very long `enum`s). →
  **Mitigation**: out of scope here; orthogonal to validation-keyword
  stripping. Tracked separately if it becomes a real issue.
- **[Risk]** Forcing `additionalProperties: false` could surprise callers
  whose schemas relied on the open-object shape. → **Mitigation**: matches
  upstream behavior, which is the spec callers already comply with.
- **[Trade-off]** Sanitization runs on every native structured-output
  request. The schema is typically small (<100 nodes); the overhead is
  negligible compared to the network call.

## Migration Plan

No migration. The change is purely additive within the provider's native
structured-output path and is wire-compatible (it relaxes what we send to
Anthropic and adds informational text to `description`). Existing callers
benefit automatically. Rollback is a single-file revert.

## Addendum (post-archive): Bypass the SDK schema helper

Decision D1 above named `anthropic.BetaJSONSchemaOutputFormat(schemaMap)` as
the call site. Post-archive review showed that the helper is a thin wrapper
around `anthropic-sdk-go`'s internal `transformSchema`, which has a
`supportedSchemaKeys` whitelist that excludes upstream-preserved keywords:
`$schema`, `$id`, `definitions` (the older draft keyword; only `$defs` is
whitelisted), `allOf`, `enum`, `const`, and `default`. Those keys are moved
into `description` as Go-formatted text (`map[Foo:map[type:string]]` style),
which is not valid JSON Schema; `$ref` pointers into `definitions` then dangle
because the target block has been stripped; and a root schema whose only
composition is `allOf` (no `type`/`anyOf`/`oneOf`) is collapsed to `nil`
entirely (`schemautil.go:165-168`).

This second transform layer is incompatible with the requirement
"`applyResponseFormat` SHALL write a sanitized copy of `ResponseFormat.Schema`
to `OutputConfig.Format`" — the schema reaching the wire was actually
`transformSchema(sanitizeJSONSchema(...))`, not `sanitizeJSONSchema(...)`.

**Resolution.** Construct `anthropic.BetaJSONOutputFormatParam{Schema:
sanitizeJSONSchema(schemaMap)}` directly. The SDK's own tests use this
construction pattern (`betamessage_test.go:101`), and `Type` defaults to
`"json_schema"` on zero-value marshal, so the wire format matches upstream's
single-transform path (`anthropic-language-model.ts:467-473`).

Implementation lives at `providers/anthropic/convert_request.go:58` with
regression tests in `convert_request_test.go`:
`NativeMode_PreservesDefinitionsAndRefIntegrity`,
`NativeMode_PreservesRootAllOf`,
`NativeMode_PreservesMetadataAndValueConstraints`.
