# Proposal

## Why

Extensions had two applicability filters: key ownership (`Keys`) and selection source (`Sources`). Every source has exactly one ownership, so `Sources` could only narrow `Keys`, needed its own consistency rule, and tied every extension to a source list that grows as the Gateway adds ways to configure models. No registered extension uses it.

## What Changes

- **BREAKING** (v0 API): `extension.Definition.Sources` and the `extension.Sources` list are removed. `Keys` is the only applicability filter.
- The host keeps one ordered extension list per key ownership instead of one per source; a call uses the list of its source's ownership (`Source.Keys()`, unchanged).
- The registration rule that every declared source has an ownership in `Keys` is removed with the field.
- `Call.Source` stays as an informational label. Its documentation states that the set may grow, that a new source keeps its key ownership, and that extensions should not branch on it to decide where they apply.
- No behavior change for any extension that does not declare `Sources`, which is every registered extension.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-extensions`: definitions no longer declare sources, and applicability is decided by key ownership alone.

## Impact

- **Code:** `ai-gateway/pkg/extension` (`Definition`, `Source` docs, `Call` docs); `ai-gateway/pkg/internal/extensionhost` (registration checks, lists keyed by ownership).
- **Docs:** `docs/guides/gateway-extensions.md`, the Gateway glossary.
- **Parity:** not parity-sensitive; the extension contract is Grafana-owned.
