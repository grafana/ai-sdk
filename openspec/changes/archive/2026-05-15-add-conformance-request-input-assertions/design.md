## Context

The conformance suite currently has two layers of fixtures: provider response fixtures (`input*.chunks.txt`) and expected upstream TypeScript UI output (`expected.jsonl`). The TypeScript tooling already runs upstream `streamText` against a local replay server, and the Go runner runs the same scenario against a Go replay server, so both sides have a natural point where the provider HTTP request can be observed.

The missing assertion is the request sent into the provider boundary. For Anthropic, that request includes the provider API body and behavior-affecting headers such as beta flags. For Grafana provider-wire conformance, the suite already validates the Grafana transport headers and can still assert the downstream Anthropic request produced after the hosted endpoint decodes `provider.CallOptions`.

## Goals / Non-Goals

**Goals:**

- Capture upstream TypeScript provider requests during fixture generation and recording.
- Compare Go provider requests against those snapshots in conformance tests.
- Ignore JSON object field ordering by comparing decoded JSON values, not raw bytes.
- Keep array order strict for ordered request data such as messages, content blocks, stop sequences, and multi-step request sequences; normalize tool declaration arrays by tool identity because Go exposes tools as a map.
- Include behavior-affecting headers while avoiding volatile transport headers and committed secrets.
- Make failures useful by reporting the request index and semantic diff context.

**Non-Goals:**

- Do not introduce broad normalization for SDK default differences yet. Missing, extra, or different values should fail until we decide a concrete exception is justified.
- Do not compare raw byte-for-byte request bodies.
- Do not make provider-wire JSON byte-compatible with upstream TypeScript provider types.
- Do not expand every conformance config option up front; add request-producing config fields as fixtures need them.

## Decisions

### Capture the actual HTTP request at the replay server boundary

Both TypeScript and Go conformance flows already send provider requests to a local replay server before receiving fixture responses. Recording the request in that server validates the actual serialized provider request, including provider conversion logic, SDK defaults, and provider SDK header behavior.

Alternative considered: compare `provider.CallOptions` before provider conversion. That would be simpler for Go and provider-wire, but it would miss bugs in Anthropic request conversion, beta header synthesis, provider option translation, and API body serialization. The goal is conformance at the provider API boundary, so HTTP request capture is the better assertion point.

### Store one request snapshot per provider call in `expected-requests.jsonl`

Each line represents one normalized request snapshot in request order. A single JSONL file mirrors the existing `expected.jsonl` pattern and naturally handles multi-step cases without adding per-step file naming rules.

Snapshot shape:

```json
{"method":"POST","path":"/v1/messages","headers":{"anthropic-beta":"..."},"body":{"model":"..."}}
```

Alternative considered: `expected-request.json` plus numbered files for multi-step cases. That mirrors response fixture names, but it spreads one assertion over several files and makes request sequence validation less direct.

### Compare decoded JSON semantically

The Go test runner should unmarshal expected and actual request bodies into generic JSON values and use structural equality. This ignores object field ordering but keeps other differences visible, including missing fields, extra fields, scalar differences, nested object differences, ordered array differences, and request count mismatches. Tool declaration arrays are the narrow exception: they are sorted by tool name/type before comparison because Go callers provide tools as a `ToolSet` map while upstream TypeScript preserves object insertion order.

Alternative considered: canonicalize JSON to sorted strings and compare strings. That can work, but comparing decoded values better expresses the intended semantics and avoids raw formatting concerns. Stable stringification is still useful when writing fixtures for reviewable diffs.

### Normalize only headers that influence behavior

Request snapshots should lowercase header names, trim values, redact sensitive values, and include only provider-owned or behavior-affecting headers. Volatile transport headers such as `host`, `content-length`, `user-agent`, `accept-encoding`, and connection management headers should be excluded.

Anthropic beta header values should be treated as unordered comma-separated sets. Anthropic tool-result JSON content should be compared semantically across the upstream raw-string shape and Go SDK text-block shape.

For Anthropic, the initial allowlist should include headers such as `content-type`, `anthropic-version`, `anthropic-beta`, and redacted auth headers when present. Provider-specific allowlists can be added when more providers join conformance.

Alternative considered: compare all headers except a denylist. That will fail on harmless HTTP client/runtime differences and make conformance noisy without improving provider behavior coverage.

### Keep default differences visible for now

The initial strategy should not normalize provider default differences beyond object field ordering and volatile headers. If TypeScript includes a default and Go omits it, or vice versa, the request assertion should fail. Each failure can then be resolved by aligning behavior or adding a narrow documented exception.

Alternative considered: maintain an ignore/default normalization list from day one. That would make the suite easier to land but risks hiding exactly the request drift this change is meant to expose.

### Treat Grafana provider-wire separately from upstream provider request conformance

The upstream comparison target is the provider API request produced by the TypeScript provider. Grafana provider-wire is Go-to-Go and has a different body shape by design, so its conformance path should continue validating provider-wire method, route, transport headers, auth, and decodability separately. The downstream Anthropic request produced by the fake hosted endpoint can still be captured and compared against the same upstream `expected-requests.jsonl` snapshots.

Alternative considered: compare Grafana provider-wire bodies to upstream request snapshots. That would conflate transport-boundary validation with provider API conformance and would fail by design because provider-wire serializes `provider.CallOptions`, not Anthropic API params.

## Risks / Trade-offs

- SDK default drift may produce many initial failures -> Land the assertion with generated snapshots and fix or document differences case by case rather than broad-normalizing them.
- Header comparison can become noisy across HTTP clients -> Use provider-specific allowlists and redact secrets before writing fixtures.
- Existing fixtures need migration -> Regenerate `expected-requests.jsonl` with the TypeScript generator and update recorded fixture generation to write the file going forward.
- Request snapshot failures may be harder to read than chunk diffs -> Include request index, method/path/header/body sections, and pretty-printed expected/actual JSON in failure output.

## Migration Plan

- Add request capture and snapshot writing to TypeScript generation and recording tools.
- Regenerate request snapshots for existing conformance fixtures without changing provider response fixtures or `expected.jsonl` unless necessary.
- Add Go replay request capture and semantic comparison.
- Run conformance tests for direct Anthropic and Grafana provider-wire packages, fixing only concrete mismatches surfaced by the new assertions.

## Open Questions

_None for the initial implementation. Header allowlists can evolve provider by provider as new concrete differences appear._
