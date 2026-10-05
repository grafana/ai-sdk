## Context

The registered baseline is `ai@7.0.116` (`test/conformance/upstream.yaml:7`). Issue #314 cites `7.0.118`, which open PR #315 proposes to register; the parsers this change touches are identical in both, and the implementation follows the registered version. `test/conformance/PARITY.md` places core output parsing in the orchestration layer, whose proof is Go unit coverage rather than provider fixtures, so no conformance input changes.

Upstream `packages/ai/src/generate-text/output.ts` at tag `ai@7.0.116`:

- `array.parseCompleteOutput` parses the text, then requires a non-null object with an `elements` property that `Array.isArray` accepts, and validates each element against the element schema. Unrelated wrapper properties are untouched.
- `choice.parseCompleteOutput` requires a non-null object whose `result` property is a string in the option set.
- `choice.parsePartialOutput` returns nothing unless the parsed value is an object whose `result` is a string, then prefix-filters the options: a successful parse publishes only an exact option, and a repaired parse publishes only a unique prefix match.

Go at `43527ef` validated `o.wrappedSchema` in both complete parsers and decoded `result` into a `string` field, which hides absence and null behind the zero value. `ArrayOutput.ParsePartial` already matched upstream and stays as it is.

## Goals / Non-Goals

**Goals:** Match the registered runtime's extraction and partial-presence semantics for wrapped array and choice outputs; keep request schemas strict; prove the singleton-option bug with a regression that a multi-option set would mask.

**Non-Goals:** Array bounds (`minItems`, `maxItems`), which issue #313 owns; the repair callback of #260; object and JSON output modes; any public API change; ignoring caller-supplied element or object schemas.

## Decisions

### 1. Check the required field, not the wrapper schema

`ParseComplete` decodes into `map[string]json.RawMessage` and inspects the one required field. A non-object document, including `null`, fails there, which preserves the existing `ErrNoObjectGenerated` contract for malformed responses. The alternative, relaxing the wrapper schema itself, was rejected: the schema is also the request's response format, where upstream keeps `additionalProperties: false`.

### 2. Presence through a pointer

`json.Unmarshal` of `null` into a `string` succeeds and leaves the zero value, so presence and type are checked by decoding into `*string`. A missing key, `null`, and a non-string value are then indistinguishable from the caller's point of view, which is what upstream returns for all three.

### 3. Element validation moves into the loop

With the wrapper schema no longer validated, each element is validated against the element schema before being unmarshalled into `T`, and the first invalid element fails the parse. This keeps the existing "array with invalid element" behavior that the spec already requires.

## Risks / Trade-offs

A response carrying an unexpected wrapper property now parses instead of failing. That matches the registered runtime, and the strict request schema still tells the model what to produce. Element validation now runs per element rather than through one wrapper validation, on the final text only, so the cost is bounded by the array the model returned.
