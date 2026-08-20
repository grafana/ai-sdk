# ProviderWire V4 contract evidence

This private workspace records the request contract emitted by the exact Vercel AI SDK packages registered in [`../conformance/upstream.yaml`](../conformance/upstream.yaml). The manifest is the source of truth for the package versions and upstream commit.

## Claim boundary

The committed request captures are deterministic observations from the pinned Gateway client. The canonical typed coverage map and independently maintained JSON Schema define the reviewed strict request contract. The source-equivalence attestation compares an explicit, grouped closure of relevant provider declarations, Gateway language-model request/error paths, and provider-utils transport helpers with the registered upstream commit; exact workspace pins remain the broad package guard.

The response fixtures are locally authored smoke inputs proving only that the pinned client consumes minimal unary JSON, SSE events ending at clean EOF, and one safe non-2xx error. They are not provider recordings, upstream provider fixtures, exhaustive server-response oracles, or production runtime tests.

This workspace is separate from:

- recorded and imported fixtures under `test/conformance`;
- frontend and legacy ProviderWire interoperability under `test/integration` and `test/interop`;
- the tolerant production `gateway/providerwire` package;
- any future strict ProviderWire V4 server implementation.

No artifact here establishes compatibility with Vercel's private Gateway server, another package version, or real provider output.

## Contract evidence model

The request contract is established from three independent views:

1. The generated captures record requests actually emitted by the registered Gateway client for the maintained scenarios.
2. The manually curated JSON Schema defines the repository's reviewed strict request-body contract. It is not generated from the upstream TypeScript declarations, captures, or coverage map. Every captured request body must satisfy it, while focused negative tests verify that known-invalid shapes are rejected.
3. The canonical typed coverage map inventories the finite request members and discriminators. Every supported item must point to an exact observation in a designated capture; unsupported items require an explicit exclusion.

Keeping these views separate reduces the chance that one implementation mistake defines both the evidence and its validator. Together they demonstrate that the curated schema represents the observed pinned-client requests and that the classified request surface was considered. They do not prove that every possible unobserved request combination is accepted or rejected exactly like an upstream private server; negative cases, installed-source equivalence, and human review constrain that remaining boundary.

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

## Request capture generation

`mise run update-providerwire-v4-artifacts` runs the maintained scenarios in `src/scenarios.ts`. Each scenario starts a local HTTP capture server, points the registered public Gateway client at it, and executes `doGenerate`, `doStream`, or the multi-step `streamText` flow. The server strictly parses each raw request body before semantic normalization and records the method, path, headers, body, and request order.

The command validates the observations before writing all of `artifacts/semantic-requests.json` and `classification.json`; neither file is hand-edited. Source-equivalence evidence is refreshed only by the coordinated parity-upgrade workflow because it compares installed package sources with the registered upstream commit.

Semantic outer-header captures retain every emitted header except transport-generated `host`, `connection`, `content-length`, `accept`, `accept-language`, `accept-encoding`, and `sec-fetch-mode`. Authentication and user-agent values are normalized; deterministic protocol, caller, configured, and observability values remain evidence.

`mise run check-providerwire-v4` is non-mutating. It regenerates request evidence in temporary storage, compares it with committed artifacts, validates exact pins and source-equivalence hashes against installed npm inputs, runs schema and observation checks, Go loss witnesses, and smoke probes.

## Go loss witnesses

[`provider/providerwire_v4_loss_test.go`](../../provider/providerwire_v4_loss_test.go) uses the external `provider_test` package so it can observe only the public Go provider contract. Its passing Phase 1 tests demonstrate distinctions that the current transport-neutral model cannot represent, including fractional numeric settings, optional false or empty scalar presence, and publicly selecting empty inline-text file data. Phase 2 should invert the relevant witness before changing the provider model.
