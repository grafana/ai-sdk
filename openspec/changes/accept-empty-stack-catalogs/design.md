# Design

## Context

`stackcatalog.Validator` computes the keys the platform will store, then calls `stackcatalog.Parse`. Selection, discovery, health and inventory also use that parser, with keys that were decrypted for the verified route parent. The parser currently calls `config.File.ValidateShape`, which shares the operator catalog's two non-empty checks. The app builds a temporary spec and key map on **New catalog** to satisfy those checks.

The registered Vercel baseline is `vercel/ai@0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1` (`ai@7.0.173`, `@ai-sdk/gateway@4.0.146`). At that pin, [GatewayFetchMetadata](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/gateway/src/gateway-fetch-metadata.ts) reads the usual `/config` response, and its schema accepts an empty model array. The [Gateway provider tests](https://github.com/vercel/ai/blob/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/gateway/src/gateway-provider.test.ts) use an empty list as a successful mocked response. Stack catalog management is Grafana-specific, under the mixed **AI Gateway / Go extension** surface in `test/conformance/PARITY.md`. Neither the client contract nor the coverage map changes.

## Design decisions

### Relax only the collection minimums

Separate the operator's collection minimums from the shared validation of entries and references. Keep one validator for provider shape, required model fields, primary and fallback references, unique candidates, IDs and aliases, per-destination policies, and bounds. The operator's strict YAML validation keeps both minimums and its `apiKeyEnv` rules. Stack parsing keeps its provider policy and the one-to-one key check, including orphan-key refusal. Removing a provider and its key must be a single update; omitting an unchanged key still keeps it.

Use the existing wire schema: it requires `spec.providers` and `spec.models` to be maps but has no minimum size. Send explicit empty maps, not nulls or omitted fields. A name-only create has no `secure` member.

### Route preflight still fails closed

`parentCatalog` currently represents a typed decode failure by passing a zero spec and no keys. That would now be valid. Carry an explicit invalid-spec state to the shared runtime preflight, with no decoded keys on failure. Treat a missing or null required map in the stored wire parent as malformed, not as an explicit empty map. Keep the existing identity check, caller authorization, request validation and error precedence. Authorized, otherwise valid requests against a malformed spec receive the existing fixed configuration error (`424`); missing or mismatched parents keep the internal host error.

After successful preflight, use the existing lister, index, and projection. `GET config` returns `200 {"models":[]}` for zero configured models, and a valid `POST language-model` returns the existing `404 model_not_found` document for any requested ID. Neither path constructs a candidate or contacts a provider; neither consults the operator catalog or provider inventory.

### On-demand action outcomes

| Checked saved state | Health (explicit action, admitted run) | Available models (explicit action) |
| --- | --- | --- |
| No providers and no models | `200 {"providers":{},"models":{}}`; no provider requests | A valid selector returns the existing fixed `404`; no provider requests |
| Connected provider with no targets | Check its key and listing with the normal status vocabulary. A valid first page gives `ok`; do not traverse more pages with no targets to match. No target statuses for that provider. | List that account using its saved key and the existing bounds and outcomes. It need not have served models. |
| Provider used only as a fallback | Probe as for any configured target, including pagination as needed. Return fallback statuses under the relevant models. | Same as for any connected provider. |

A catalog with no models has an empty `models` map in health even if it has providers. Keep the same authorization, preflight and run budgets. Neither action runs on create, connect, save, or discovery.

### Edit the saved catalog in stages

Keep the name check, URL preview, and platform error display on **New catalog**. Submit once with the empty spec, then navigate to the saved page after success. Remove the draft provider/model/key path and its create-only picker hint. From the saved page, **Connect a provider** stores type and key in one update; the provider card is immediately usable even with no model rows. Manual add remains available and the permission-gated picker can browse before the first model is added. Keep provider-level controls separate from the model rows and quick-add form, so a later provider setting can be added on that card without revisiting creation. Do not add a placeholder setting or any new persistence abstraction.

Each confirmed change uses the loaded resource version and continues from the returned object. Removing a model, including the last one, leaves its provider and key alone. Confirm provider removal with the affected primary model IDs and the key deletion. In one update remove the provider, its key and all models whose primary it was, including those with outbound fallbacks. Do not promote a fallback. Keep the existing block when a model that would remain falls back to the provider being removed.

## Open questions

| Question | Recommendation |
| --- | --- |
| Should a name-only create also omit the maps or relax the kind schema? | No. Submit explicit empty maps in the unchanged schema. The form needs only the name; the spec still has a well-defined shape. |
| Should the app offer **Check health** when there are no providers? | No. Show the empty state instead. Keep the API's empty success for authorized, admitted checks without bypassing its existing run limits. |
| Should health skip a connected provider with no targets? | No. The first listing page tests whether the account accepts the saved key before a writer adds a model. Keep the provider badge distinct from key storage state. |
| Should provider removal promote fallbacks or rewrite other models? | No. Name the primary models to delete in the confirmation, leave surviving models unchanged, and block removal while any of them references it. |
