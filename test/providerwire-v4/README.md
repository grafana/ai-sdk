# ProviderWire V4 contract evidence

This private workspace records the request contract emitted by the exact Vercel AI SDK packages registered in [`../conformance/upstream.yaml`](../conformance/upstream.yaml). It covers `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/provider-utils@5.0.27` at upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`.

## Claim boundary

The committed request captures are deterministic observations from the pinned Gateway client. The canonical typed coverage map and independently maintained JSON Schema define the reviewed strict request contract. The source-equivalence attestation compares an explicit, grouped closure of relevant provider declarations, Gateway language-model request/error paths, and provider-utils transport helpers with the registered upstream commit; exact workspace pins remain the broad package guard.

The response fixtures are locally authored smoke inputs proving only that the pinned client consumes minimal unary JSON, SSE events ending at clean EOF, and one safe non-2xx error. They are not provider recordings, upstream provider fixtures, exhaustive server-response oracles, or production runtime tests.

This workspace is separate from:

- recorded and imported fixtures under `test/conformance`;
- frontend and legacy ProviderWire interoperability under `test/integration` and `test/interop`;
- the tolerant production `gateway/providerwire` package;
- any future strict ProviderWire V4 server implementation.

No artifact here establishes compatibility with Vercel's private Gateway server, another package version, or real provider output.

## Artifacts

Maintained inputs:

- `src/request-coverage.ts`: canonical typed request-surface coverage map;
- `schema/providerwire-v4-request.schema.json`: independent normative draft 2020-12 request-body schema;
- `phase2-delta.md`: human-reviewed semantic Go provider-model handoff;
- scenario selection, exclusions, focused negative cases, response probes, and the explicit source path closure.

Generated artifacts:

- `classification.json`, derived from the typed coverage map for review;
- `artifacts/semantic-requests.json`;
- `artifacts/source-equivalence.json`.

Artifact metadata is derived from or checked against `upstream.yaml` and this workspace's exact package pins.

Semantic outer-header captures retain every emitted header except transport-generated `host`, `connection`, `content-length`, `accept`, `accept-language`, `accept-encoding`, and `sec-fetch-mode`. Authentication and user-agent values are normalized; deterministic protocol, caller, configured, and observability values remain evidence.

`mise run check-providerwire-v4` is non-mutating. It regenerates request evidence in temporary storage, compares it with committed artifacts, validates exact pins and source-equivalence hashes against installed npm inputs, runs schema and observation checks, Go loss witnesses, and smoke probes.

`mise run update-providerwire-v4-artifacts` is the explicit request-artifact writer. Source-equivalence evidence is refreshed only by the coordinated parity-upgrade workflow because it compares installed package sources with the registered upstream commit.
