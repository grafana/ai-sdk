## Why

The legacy ProviderWire transport has been retired, leaving no executable contract for the strict HTTP dialect consumed by the registered `@ai-sdk/gateway@4.0.52` LanguageModelV4 client. Before implementing a Go runtime, the repository needs durable baseline-pinned evidence of the complete request surface, the client's real HTTP projection, and its response-consumption behavior.

## What Changes

- Establish top-level `ai-gateway/` as the separate `github.com/grafana/ai-sdk/ai-gateway` module under AGPL-3.0-only while the reusable SDK and root license remain Apache-2.0.
- Add explicit repository/Gateway license, contribution, provenance, and legal-readiness documentation plus blocking one-way module-boundary verification.
- Add an exact-pinned private `ai-gateway/test/providerwire-v4` TypeScript workspace registered against `test/conformance/upstream.yaml`.
- Add exhaustive compile-time coverage for every finite registered LanguageModelV4 request key and request/response discriminator relevant to the Gateway language-model route.
- Add a complete hand-authored draft 2020-12 request schema at `ai-gateway/providerwire/v4/schema/request.json`, covering the entire registered request projection rather than only the first runtime subset.
- Capture compact semantic request goldens from the real registered Gateway client, including method, route, protocol headers, streaming mode, presence semantics, header composition, and file-data transformation.
- Add focused registered-client consumption probes for unary success, clean-EOF SSE success, tolerated `[DONE]`, raw-part filtering, response-metadata timestamps, and non-2xx errors while keeping raw HTTP/server DTO assertions authoritative for fields the client overwrites.
- Add schema validation, in-memory golden drift checks, a non-mutating aggregate ProviderWire verification task, and a separate explicit command that updates only reviewed request goldens.
- Update active parity governance and the retired-wire boundary to register this new strict versioned contract without restoring or claiming compatibility for the deleted tolerant transport.
- Do not implement request decoding, Go mapping, model invocation, response encoding, the service, or the Go V4 client in this change.

## Capabilities

### New Capabilities

- `providerwire-v4-http-contract`: Defines the complete baseline-pinned LanguageModelV4 request schema, real Gateway HTTP projection evidence, client-consumption probes, and contract verification workflow.

### Modified Capabilities

- `provider-wire`: Clarify that the retired unversioned tolerant transport remains absent while a new explicit `ai-gateway/providerwire/v4` contract namespace may define strict protocol artifacts independently of provider-domain JSON marshalers.
- `upstream-parity-governance`: Register `ai-gateway/test/providerwire-v4` as an exact-pinned parity consumer and require its compile-time, schema, golden, and client-consumption checks in parity validation.

## Impact

- Adds the isolated AGPL-3.0-only `ai-gateway/` module without registering it in the root `go.work` or root module graph.
- Keeps the root Apache-2.0 license unchanged and requires Grafana legal confirmation before merge or deployment without altering prior grants.
- Adds `ai-gateway/providerwire/v4/schema/request.json` as production protocol authority for the registered request projection.
- Adds a new private TypeScript test workspace, exact AI SDK dependency pins, package-manager lockfile entries, baseline validation inputs, and `mise` tasks.
- Adds committed semantic HTTP request goldens and registered-client response-consumption tests, but no provider payload fixtures or executable Go server.
- Establishes the contract dependency required by the later strict unary and streaming runtime work packages.
