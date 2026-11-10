# Design

## Context

- Not parity-sensitive (see proposal). `test/conformance/PARITY.md` routes the AI Gateway to `gateway-*` specs; no conformance fixture or upstream package is involved.
- Base: the `ModelCatalog` kind with `stackcatalog.Parse(spec, keys) (config.File, error)`, and the composition seam: `execution.Selection.Source`, `execution.Config.Compose` and `service.ModelObservers` with one chain per source.
- `service.BuildCatalog` constructs every operator candidate at startup; `Catalog.selectRouted` composes a request chain over those instances with `routing.Compose`.
- `execution.Runtime` holds one `SummaryRedactor` built from `Config.ConfiguredAPIKeys`; `attemptCapture` uses it for overview attempt errors and `View.Current`.
- `ModelObservers` registers the Prometheus middleware once. Its `NormalizeModel` buckets by provider (`grafana` keeps the model ID, others become `byok`). Each `middleware.Middleware` sees the identity of the model directly beneath it.
- `requestObservation` holds correlation ID, caller service and namespace, snapshotted by the context bridge and read by the logger (`observationLogAttrs`) and Agent Observability (`observationMetadata`, `allowedAgentMetadataKeys`).
- `config.Settings.Validate` requires `model_catalog`; `service.NewSelector` returns nil for a nil catalog, which `execution.New` refuses.
- Grafana gives catalog routes the parent object, its resource version and `resource.DecryptedSecureValues` (`map[string]RawSecureValue`, which redacts itself in formatting and JSON) through `resource.RouteRequestInfo`.
- ProviderWire documents are fixed per category; the Go client accepts only registered status, type and code combinations.

## Goals / Non-Goals

**Goals:**

- One selection function that the catalog routes call per request, returning an ordinary `Selection` the runtime, composer and overview already handle.
- No startup cost or retained state per catalog.
- Keep edits in shared hotspots (`execution/selection.go`, `model_observability.go`, the category lists) additive.

**Non-Goals:**

- Routes, the `catalog` account type, decoding `RouteRequestInfo`, the parent check, stack-catalog discovery and its host error category (#758).
- Key ownership, a context argument for `ModelComposer`, extension errors through the composer, moving `Source` (#762).
- Caching parsed catalogs or built models across requests.

## Decisions

### Selection API for the catalog routes

```go
// package service
type StackCatalog struct {
	Name string
	Spec v1alpha1.Spec
	Keys resource.DecryptedSecureValues
}

type StackCatalogOptions struct {
	Client *http.Client
}

func SelectStackCatalog(catalog StackCatalog, modelID string, gateway json.RawMessage, options StackCatalogOptions) (execution.Selection, error)
```

- Inputs are what #758 has after its parent check: the decoded spec, Grafana's decrypted values unchanged, and the catalog name. `StackCatalogOptions` mirrors `BYOKOptions`.
- It returns `execution.StackCatalogSelection(canonicalID, catalog.Name, model, expected, keys)`: source `stack-catalog`, configured attribution (same overview as the operator catalog), the catalog name, and every non-empty decrypted value as request keys.
- Errors: `execution.ErrCatalogConfiguration`, `catalog.ErrUnknownModel`, `execution.ErrUnknownFallbackModel`, `ErrUnsupportedGatewayControl`, and the routing sentinels.
- #758 adds the `catalog` account branch to `NewSelector`, which calls this function with the catalog it decoded for the request.
- Namespace and resource version are not inputs. The observation already carries the route namespace, and the parent check (#758) ties the catalog to it. No consumer of the version exists until a cache keyed by it does.
- Alternative considered: a context carrier and a `NewSelector` branch in this change. Rejected, because without the `catalog` account the branch would dispatch on context presence alone, which #758 must then replace.

### Order of checks

1. `routing.Decode(gateway, routing.ConfiguredAccess)`: `byok` and unknown controls are refused as unsupported gateway controls, and malformed ones as routing controls. The caller can fix these, and the check is cheap and needs no catalog.
2. `stackcatalog.Parse(spec, keysWithValues)`, where `keysWithValues` are the secure keys with non-empty values. Any error becomes the bare `ErrCatalogConfiguration`. The `Parse` error is dropped, not wrapped: its messages carry spec identifiers.
3. Resolve the requested ID and each `models` entry through an ID and alias index built from the parsed file.
4. `routing.Compose` over the resolved groups with `config.MaxProviderAttempts`.
5. Construct only the placed candidates, compose them in order without a physical attempt observer, and give the chain `ConfiguredIdentity(canonicalID)`.

### Building only what the chain attempts

- The per-candidate construction in `buildCatalog` (type switch, native options validation, `execution.Translate`, `ConfiguredCandidate`) moves to one helper that both catalogs call.
- `selectRouted`'s group building, composition and expected-attempt projection move to one helper that takes how a placed candidate is obtained. The operator catalog returns its startup instance; the stack catalog constructs it from `config.ResolvedProvider{Type, APIKey}`.
- An unrouted stack call takes the same path with only the requested group. It yields the same expected attempts as `CatalogSelector` (requested ID as sent, canonical ID, candidates in order).
- Stack providers are `anthropic` or `openai` with no `baseURL` (policy), so candidates reach the native endpoints through the shared model client.
- Alternative considered: build the stack catalog into a `service.Catalog` per request and reuse `selectRouted`. Rejected: it constructs every candidate of every model.

### Request-scoped redaction

- `Selection` gains an unexported `keys []string`, set only by `StackCatalogSelection`. `Call` builds the capture with `r.redactor.With(selected.keys)`, which returns the runtime redactor unchanged when keys are empty, and otherwise a new one over both key sets.
- Overview attempt errors and `View.Current` therefore redact both. The merged redactor lives only in that call's capture.
- BYOK selections carry no keys, so the existing BYOK rule (caller-owned echoes are kept) is unchanged.
- All decrypted values are included, not just the placed providers', mirroring the operator rule ("including unused accounts").

### Metric label: a metric identity view, chosen by source

- The stack-catalog chain shares the startup bridge, Agent Observability recording, the operator-catalog logger and the single metrics middleware. Only how they are stacked differs: per call, the composer builds:

  `bind catalog → bridge → recording → logger → identity(grafana, canonical) → metrics → identity(grafana, "catalog") → selected model`

- Each middleware sees the identity of the model beneath it. Metrics therefore see `grafana`/`catalog`, which `NormalizeModel` keeps as is, while logs and Agent Observability see the canonical ID. The choice is made by source in `Compose`, without a second registration and without changing the reusable Prometheus middleware.
- The cost is a few wrapper allocations per call; no middleware or collector is constructed.
- Alternatives considered:
  - a distinct logical provider for stack catalogs: changes log and Agent Observability identity, and still decides by provider;
  - a context-aware `NormalizeModel`: needs a change to the published `middleware/prometheus` module.

### Catalog name in observation

- `Selection` gains an exported `Catalog string`: the stack catalog's name, empty for other sources. The composer reads it, so no context argument is needed.
- The per-call `bind catalog` wrapper stores a copy of the request observation with `catalog` set. The startup bridge snapshots it like the other fields.
- `observationLogAttrs` emits `catalog` and `observationMetadata` emits `gateway.catalog`, both under the existing 128-byte printable bound. The log allowlist accepts `catalog` under the configured policy; `allowedAgentMetadataKeys` gains `gateway.catalog`.
- The log key is `catalog`, following the existing pairs (`namespace` in logs, `gateway.namespace` in metadata). This deviates from the issue's wording, which names `gateway.catalog` for both surfaces.
- A name over the bound is omitted without a count: no counter of dropped observation values exists, and one field does not warrant adding one.
- Alternatives considered:
  - a separate stack-catalog log policy: no field differs from the configured policy;
  - having #758 put the name into the observation at route time: telemetry would then name the route's catalog, not the one selected.

### Optional operator catalog: an empty catalog, not a nil selector

- `Settings.Validate` no longer requires `model_catalog`; the size bound stays.
- When the setting is blank, `gateway.New` skips parsing, `File.Validate` (which requires at least one provider and model), endpoint validation and secret resolution, and calls `BuildCatalog(config.File{}, nil, ...)`. That yields an empty `service.Catalog`: discovery writes exactly `{"models":[]}`, resolution returns `ErrUnknownModel`, and `NewSelector` and `execution.New` are unchanged.
- Alternative considered: nil-tolerant `NewSelector` and discovery. Rejected: more branches for the same behavior.

### `catalog configuration` category

- `execution.ErrCatalogConfiguration` maps to `CategoryCatalogConfiguration` in `ClassifySelection`, appended after the existing categories.
- ProviderWire maps it to a fixed document: 424, `failed_dependency`/`failed_dependency`, message "model catalog is invalid or missing a provider key", not retryable.
  - 424 is the one registered combination that means "a prerequisite of this request failed" and is not retryable in either client.
  - 400 would blame the caller's request; 500 is retryable and reads as a Gateway fault.
  - The fixed message tells it apart from the provider-4xx `failed dependency` document.
- No host error category in this change. Only #758's discovery needs one, and it adds `host.ErrorCatalogConfiguration` with the same document.
- Categories live in `gateway-execution-runtime` and `providerwire-v4-unary-runtime`, not in `providerwire-v4-http-contract`, which covers client evidence. #762's extension categories belong in the same two specs.

### Physical attempt records

Stack-catalog calls write no private physical attempt records, like BYOK calls today (`byok.New` composes its chain without an attempt observer). The rule is key ownership: calls with customer-owned keys get no private attempt log, which also keeps customer-chosen provider names and backend model IDs out of operator logs beyond the catalog name. Per-call evidence stays in the execution overview the caller receives. This is a deliberate choice that can be revisited.

Alternative considered: record them through the same sink as the operator catalog. Rejected for the ownership rule above.

## Test plan

| Acceptance | Evidence |
|---|---|
| Starts without `model_catalog`; no configured models; BYOK works | `config` settings tests (unset, blank, oversized); `gateway` tests: `New` without the setting, `config` returns `{"models":[]}`, a configured call returns the model-not-found document, a BYOK call in `cloud` mode succeeds. |
| Builds only needed models; routing and fallback | `service` table tests over `selectStackCatalog` with an injected constructor counting builds: plain call, alias, `models`, `only`, `order`, 50-model catalog, duplicate destinations, attempt bound (zero builds), unknown model, unknown `models` entry, primary fails and fallback succeeds with overview; a stack call writes no physical attempt record. |
| BYOK controls refused | Same table: valid `byok` accounts and an unknown control fail with `ErrUnsupportedGatewayControl`, with zero builds and zero provider requests; a missing catalog key never uses an operator key. |
| Redaction of request keys | `execution`: `SummaryRedactor.With`, and `StackCatalogSelection` carrying keys. Runtime tests: a fake provider error echoing a catalog key, an unused-provider key and an operator key is redacted in the overview and `View.Current`, unary and stream. A later BYOK call echoing the same key is not redacted. |
| Catalog configuration category | `stackcatalog`-invalid specs and missing or empty values return exactly `ErrCatalogConfiguration` (`errors.Is` and an equal message, so no spec text). Classification test. ProviderWire unary and stream-setup document bytes. Go client mapping test, and a TypeScript non-success probe in `test/providerwire-v4`. |
| Bounded metrics; Agent Observability metadata only | `service` composition test with production `ModelObservers`, `SelectStackCatalog` and the runtime: unary and stream, each observer once. Logs carry `grafana`, the canonical ID and `catalog`. Agent Observability (testkit) carries the canonical ID and `gateway.catalog`, without content. The metric `model` label is `catalog`. 20 catalogs × 20 model IDs give the series count of one. Sentinel keys, provider names, aliases and backend IDs are absent from every sink. |
| Source chain | `ModelObservers.Compose` accepts `stack-catalog`; the unknown-source test still fails as internal. |
| Regression | `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-providerwire-v4`, `test-integration`, `verify-ai-gateway-boundary`, `lint-docs`; `openspec validate --strict`. No e2e locally; CI runs the Playwright suite. |

Cross-language scenarios on catalog routes belong to #758, which adds the routes. This change has no route that can return the new document.

## Risks / Trade-offs

- [An operator model whose canonical ID is `catalog` shares series with stack-catalog calls] → accepted and documented in `ai-gateway/docs/text-observability.md`, like the existing `byok` bucket; renaming that model avoids it.
- [Per-request `Parse` on large catalogs] → linear in a spec that admission already bounds, with the object size bounded by Grafana. A cache keyed by resource version can come later without changing the API.
- [Catalog names over 128 bytes lose log and Agent Observability attribution] → fails closed, like every other observation value; the correlation ID still links the attempt records.
- [The 424 document matches provider 4xx by status, type and code] → only the message differs. Dashboards grouping by status see both; a distinct code would need client changes.
- [#758] → See Follow-ups for #758.
- [#762 merge] → Text conflicts only, in:
  - `execution/selection.go`: the `SourceStackCatalog` constant, the `Catalog` and `keys` fields and the constructor, all moving with `Source` to its leaf package;
  - `model_observability.go`: one allowlist key and one chain entry. `Compose` keeps its signature here, so #762's context argument applies unchanged;
  - the appended `Category`, `safeCategories` and `publicErrors` entries;
  - the `Closed failure classification` MODIFIED block in `gateway-execution-runtime`.

  Stack-catalog calls are key ownership `customer`, derived from the source by #762.
- [#763 edits the same observation requirement and allowlists] → additive keys, sequential rebases.

## Follow-ups for #758

- Add `host.ErrorCatalogConfiguration`, written by ProviderWire with the same fixed 424 document, together with its consumer, stack-catalog discovery. This change adds only the runtime category, because no host code here can fail with it.
- Decode `RouteRequestInfo.Parent` into `v1alpha1.ModelCatalog` and check its group, kind, namespace and name against the route before calling `SelectStackCatalog`. A decode failure maps to `ErrCatalogConfiguration`.
- Add the `catalog` account branch to `NewSelector`, calling `SelectStackCatalog` with the decoded catalog and `RouteRequestInfo.DecryptedSecureValues`.
- Add a discovery lister that reuses `Parse` and the candidate projection without constructing models.
- Add the cross-language scenarios on the catalog routes, including the catalog configuration document.
- The PR description notes the `catalog` log key deviation from the issue's wording.

## Migration Plan

No migration: `model_catalog` becomes optional and existing settings keep working. Rolling back to an older build requires `model_catalog` again.
