## Context

The registered reference is Vercel AI SDK commit
`b7f6255acaae7724adc0fd37aac3dae3f3466578`, package
`@ai-sdk/amazon-bedrock` 5.0.118. Its chat options schema accepts optional
`z.record(z.string(), z.string())`; its request converter removes requestMetadata
from opaque options and emits the validated record at the top level for both
Converse APIs. Upstream tests cover valid, omitted, and non-string values. The
initial focused Go regression test fails because all ten malformed shapes are
accepted in both supported namespaces.

## Goals / Non-Goals

Match that string-record contract, expose an idiomatic Go map, retain the
already working wire representation, and reject malformed metadata before HTTP.
Do not change namespace resolution, unrelated option semantics, model defaults,
response metadata, or enforce constraints mentioned only in upstream comments.

## Decisions

- Use `map[string]string` with `json:"requestMetadata,omitzero"` in public options
  and the internal Converse request. Nil means absent and a non-nil empty map
  means `{}`. Existing Go minimum supports `omitzero`.
- Decode the raw metadata field separately within the existing options JSON
  decoder so JSON null and null record entries cannot silently become empty Go
  string values. Return a fixed contextual error without record keys or values.
  Typed Go maps need no runtime value validation.
- Exclude canonical requestMetadata from opaque pass-through; validated typed
  request conversion owns its placement. AdditionalModelRequestFields remains
  opaque and independent, matching upstream.
- Keep modern namespace precedence unchanged, including an empty modern object
  blocking legacy values and null/absent modern options falling back. Preserve
  ordinary encoding/json struct-field matching. Decode the metadata field and
  its entries as raw JSON before validation so duplicate members keep the last
  value, as existing opaque pass-through and upstream JSON.parse do.
- Classify the Go map/nil representation as parity-preserving Go adaptation.
  Focused synthetic request-capture tests prove both HTTP paths; no new recorded
  or upstream fixture is invented. Existing provider-adapter coverage remains
  mixed in PARITY.md, so no coverage-map status change is claimed.

## Risks / Trade-offs

Callers supplying invalid raw metadata now receive an error before HTTP. This is
the intended alignment. Valid raw requests and typed map round trips retain
empty-map presence. Metadata is explicitly supplied for Bedrock invocation logs;
this provider adds no logging or response metadata projection. Real AWS invocation
log ingestion is outside credential-free tests.

## Validation

Reproduce invalid raw values first, then verify typed value/pointer and raw
options in both namespaces; omitted/empty maps, null/absent/empty modern options,
invalid winning namespace, ignored invalid legacy options, JSON round trips,
unchanged top-level shape, and errors without metadata contents. Run Bedrock
module tests, race, vet, lint, relevant parity checks, docs lint, OpenSpec
validation and archive the completed change. Full-repository CI remains the
publication gate.
