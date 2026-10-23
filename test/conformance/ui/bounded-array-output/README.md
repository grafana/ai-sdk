# Bounded array output

Provider-independent core fixtures for `Output.array({ minItems, maxItems })`.
`input.jsonl` in each case is synthetic `LanguageModelV4` text output, not a
provider recording. `generate.mts` replays it through `streamText` from the
[registered baseline](../../upstream.yaml) and writes:

- `expected.jsonl`: the UI message stream;
- `expected-output.json`: the `responseFormat` the model received, the
  elements the element stream delivered, and the innermost
  `TypeValidationError` cause from the element stream and from `output`, or
  `null`.

| Case | Bounds | Elements in text |
|---|---|---|
| `within` | min 1, max 2 | 2 |
| `overflow` | max 2 | 3 |
| `below-minimum` | min 2 | 1 |

Go compares the UI chunks and the request format exactly, the elements in
order, and checks that each Go error ends with the upstream cause; the
wrapping messages differ by runtime. Upstream errors the element stream where
Go closes `ElementStream` and reports the cause through `ElementStreamError`,
which is recorded as `array-element-stream-overflow`.

From `test/conformance/tools`, regenerate with:

```sh
pnpm exec tsx ../ui/bounded-array-output/generate.mts
```
