# Proposal

## Why

`middleware.DefaultSettings` is the Go port of upstream `defaultSettingsMiddleware`, which returns `mergeObjects(settings, params)`: a call value wins when defined, nested plain objects merge recursively, and arrays and primitives replace. The Go middleware replaces whole provider-option namespaces and whole response formats, so a caller that adds one provider option silently drops the defaults for that provider. The capability spec also contradicts the code about empty slices.

Reference: registered baseline `ai@7.0.159` (commit `028a0253`): `packages/ai/src/middleware/default-settings-middleware.ts`, `packages/ai/src/util/merge-objects.ts` and their tests.

## What Changes

- Provider options from the defaults and the call merge recursively inside each namespace, with the same rules as upstream `mergeObjects`, including ignoring `__proto__`, `constructor` and `prototype` keys. The root module already ports this algorithm for step provider options; both callers share one implementation. Only namespaces set on both sides take the merged raw form; a namespace set on one side keeps its original, possibly typed, value.
- Response format merges member by member: an unset caller schema, name or description takes the default's when the result is JSON. A text result takes no JSON-only members from the default, because the Go type is a flat union and the Grafana client rejects inactive members.
- Tool choice keeps whole-value replacement, recorded as equivalent: upstream deep merge differs only by carrying an inactive tool name.
- Empty caller slices (`StopSequences`, `Tools`) keep winning over defaults, as upstream does. The spec is corrected to say only nil slices are filled.
- `Reasoning` stays as a Go addition and is recorded in `knownGaps` together with the union-member adaptation.
- ProviderWire decoding is unchanged. Its spec normalizes empty tools and text format to ordinary text behavior, and no Gateway path on `main` composes `DefaultSettings` after ProviderWire decoding.
- Caller-visible behavior change (patch, `fix:`): a caller namespace no longer removes default keys it does not restate, only namespaces set on both sides reach models as `RawProviderOption`, and a JSON response format without a schema inherits the default schema.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `builtin-middleware-default-settings`: corrects the unset-field rule, makes provider options deep-merge, and adds response-format, tool-choice and reasoning merge rules.

## Impact

- **Code:** `middleware/default_settings.go`; the root step provider-option merge moves to a shared internal package used by `StreamText` and `DefaultSettings`.
- **APIs:** no signature change. `DefaultSettings` can now return a marshaling error from `TransformParams`.
- **Tests:** upstream `default-settings-middleware.test.ts` and `merge-objects.test.ts` cases ported as Go unit tests, plus Go-specific union and typed-option cases.
- **Parity records:** one `knownGaps` entry. `PARITY.md` is unchanged. No conformance fixtures change; the middleware is core-layer with no provider or UI wire boundary.
- **Release:** root module patch release.
