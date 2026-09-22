## Context

The repository currently publishes two coupled legacy surfaces: `gateway/providerwire`, a tolerant HTTP/JSON/SSE transport over provider-domain JSON methods, and `providers/grafana`, a nested remote-provider module for that transport. Their verification is spread across package tests, `test/interop`, Grafana conformance, workspace/task registration, CI, parity documentation, active OpenSpec capabilities, and user guides.

The registered compatibility baseline is `@ai-sdk/gateway@4.0.52` with `@ai-sdk/provider@4.0.7`. The planned strict ProviderWire V4 adapter uses explicit private DTOs, schema validation, bounded processing, safe errors, and canonical identity. Keeping the tolerant transport beside that work would create ambiguous APIs and compatibility claims. This change therefore removes the old transport before introducing any strict replacement.

Root tag `v0.1.0-alpha.1` contains both legacy directories, but it versions only the root module; no module-prefixed Grafana tag exists.

## Goals / Non-Goals

**Goals:**

- Remove all production code that exposes or consumes the tolerant legacy transport.
- Remove tests, conformance claims, tooling, active specifications, and documentation whose only subject is that transport.
- Keep transport-independent provider-domain, provider implementation, catalog, fallback, registry, middleware, UI SSE, and integration capabilities intact.
- Leave the repository build, module graph, parity checks, docs navigation, and CI coherent after removal.
- Record the available rollback scope without overstating nested-module availability.

**Non-Goals:**

- Implement any part of the strict ProviderWire V4 adapter, schema, server, service, or Go client.
- Preserve source or wire compatibility through aliases, deprecated wrappers, copied codecs, or a legacy mode.
- Change provider-domain JSON behavior or concrete provider request/response conversion.
- Treat provider-domain JSON marshalers as HTTP protocol authority.
- Publish or promise a new `providers/grafana` module version without evidence of an external consumer need.
- Replace legacy interop coverage with speculative strict-protocol fixtures before the strict contract work package lands.

## Decisions

### Scope rollback guidance to what is actually versioned

The archived migration record identifies `github.com/grafana/ai-sdk@v0.1.0-alpha.1` as the rollback point for root-module/server deployments. It does not claim that `github.com/grafana/ai-sdk/providers/grafana@v0.1.0-alpha.1` is fetchable: Go nested modules require module-prefixed tags, and none exists. Grafana-client implementation source remains inspectable at repository tag `v0.1.0-alpha.1` and in Git history.

A module-specific Grafana tag is deferred unless a real external consumer requires an independently installable rollback. The implementation check found known consumers pinned to nested-module pseudo-versions, and each discovered revision resolved through the public Go proxy. Those immutable revisions already provide their existing rollback path, so no need for a new tagged release was established. If that need appears later, release preparation and external module validation must be handled explicitly rather than added as an unverified promise to this retirement change.

Alternative: add a nested tag solely to satisfy the original wording. Rejected because there is no established external-consumer requirement and tag publication is irreversible. Alternative: present the root tag as versioning both modules. Rejected because Go module resolution does not work that way.

### Delete both ends of the legacy transport atomically

`gateway/providerwire` and `providers/grafana` will be removed in the same code change. The Grafana module has no supported transport after the server codec is retired, and leaving either side would advertise an unusable or ambiguous API.

Alternative: retain deprecated packages until the strict implementation exists. Rejected because it permits new releases and consumers to keep adopting a protocol that the technical plan explicitly supersedes, and it increases the risk that strict code reuses tolerant behavior accidentally.

### Provide no compatibility layer

The removed import paths will not contain aliases, forwarding shims, build-tagged legacy code, or mode switches. Git history and the root repository tag remain source references.

Alternative: leave stubs that return unsupported errors. Rejected because stubs preserve discoverability without functionality and do not help pinned deployments.

### Remove verification by ownership, not by broad directory category

Delete `test/interop` because its server and scenarios exist to verify the legacy public handler. Delete `test/conformance/grafana` and Grafana module dependencies because they exercise the removed client. Retain `test/integration`, provider-independent UI fixtures, all other provider conformance, and provider-domain JSON tests because they validate capabilities that remain supported.

Provider-domain JSON behavior and tests remain unchanged. Only current comments or godoc that present provider structs or marshalers as ProviderWire HTTP authority will be reworded. The dedicated Grafana and ProviderWire transport rows will be removed from `test/conformance/PARITY.md` without creating a replacement transport classification.

Parity metadata will remove the two legacy ProviderWire rows and baseline tooling will stop treating the deleted interop workspace as a package-version consumer. The registered package versions remain unchanged; this is retirement of a local compatibility surface, not an upstream baseline upgrade.

Alternative: retain the interop workspace as future scaffolding. Rejected because dormant tests and dependencies would still imply current coverage and can be recreated from history when the strict contract package lands.

### Retire every active normative dependency

Delta specifications will remove all requirements from `gateway-providerwire-server`, `grafana-provider`, `grafana-provider-options`, and `gateway-error-normalization`. Cross-capability deltas will remove or rewrite requirements in conformance, parity governance, API-call errors, documentation structure, gateway catalog, Prometheus middleware, and context enrichment. Files under `openspec/specs/` are current normative contracts and must never be classified as ignorable historical references; only archived changes may retain historical descriptions.

### Clean the complete repository registration graph

Remove the Grafana module and interop testserver from `go.work`; remove Grafana test/build/vet/tidy tasks and the interop task; remove the interop package from test dependency sources and pnpm workspace/lockfile; remove the dedicated CI interop job; and update baseline scripts that enumerate TypeScript consumers. Dependency files will be regenerated only as a consequence of deleting those registered consumers.

A repository-wide import/reference check will guard against orphaned registration. The check must distinguish generic Grafana organization references, provider-native “wire name” terminology, archived OpenSpec history, and frontend interoperability documentation from current references to the retired package and harness.

### Preserve transport-independent gateway and UI documentation

Delete the legacy server guide and Grafana Cloud provider guide. Edit navigation and adjacent pages to remove links to those pages. Keep `gateway/catalog` documentation, but remove handler-specific examples and describe it only as a reusable resolver/catalog. Keep the root UI-message SSE documentation and remove only statements that conflate it with provider wire.

Alternative: remove every document mentioning gateways or Grafana. Rejected because catalog, middleware, and UI SSE are independent supported capabilities.

### Validate removal without inventing replacement parity evidence

Run formatting, docs lint, retained TypeScript integration tests, module/build/test/vet/lint checks, `mise run validate-parity-baseline`, and `mise run parity-check` after registration cleanup. No conformance fixtures or provider payloads will be added or regenerated for this retirement. The parity classification is a deliberate removal of an automated local transport surface, while the remaining provider and frontend coverage continues unchanged.

## Risks / Trade-offs

- [Root tag is mistaken for a nested Grafana module version] → State the scope explicitly and do not publish a `go get .../providers/grafana@v0.1.0-alpha.1` command.
- [An unknown external Grafana-client consumer needs an installable rollback] → Confirm consumer need before implementation; if found, stop and design a real nested-module release rather than silently inventing a tag.
- [Immediate source break for users of either public package] → Retain the archived rollback record and Grafana source history; do not imply an in-place migration target exists yet.
- [Over-deletion removes useful provider or frontend coverage] → Classify files by dependency on the retired transport and explicitly retain `test/integration`, non-Grafana conformance, provider-domain tests, catalog, fallback, registry, and middleware.
- [Provider comments continue implying HTTP authority] → Reword only those comments or godoc while preserving provider JSON behavior and tests.
- [Orphaned active specs or module references contradict the diff] → Update every active capability found by repository-wide search and treat only archived OpenSpec changes as historical.
- [Documentation navigation breaks when pages disappear] → Update the docs index, provider overview/adjacent footers, cross-links, and run both structural and Markdown linting.
- [Removing interop weakens confidence before strict V4 exists] → Treat the gap as intentional sequencing: the next work package establishes exact-pinned contract evidence rather than preserving misleading legacy evidence.

## Migration Plan

1. Confirm whether any known external consumer imports the nested `providers/grafana` module and needs an independently installable rollback. If one exists, stop and plan a real module-specific release before removal.
2. Implement and validate the repository retirement, including retained TypeScript integration tests.
3. Root/server deployments roll back to `v0.1.0-alpha.1`; no independently fetchable Grafana client rollback is promised by this change.

## Open Questions

None. Known consumers retain proxy-fetchable pinned pseudo-versions; no consumer requirement for a new module-specific rollback release was established.
