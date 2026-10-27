# Partial output

Provider-independent core fixtures for structured-output partials. The inputs
are synthetic `LanguageModelV4` text output, not provider recordings.
`generate.mts` replays them through `streamText` from the
[registered baseline](../../upstream.yaml) and writes:

- `expected.jsonl`: the UI message stream;
- `expected-partials.json`: every value `partialOutputStream` delivered.

| Case | Output | What it pins |
|---|---|---|
| `empty-array` | `Output.array` | The first partial, before any valid element, is `[]` |
| `multi-step-repeat` | `Output.object` | Step 2 republishes the partial step 1 ended on, and its text-delta boundaries follow its own partials |

`multi-step-repeat` reads `input-1.jsonl` and `input-2.jsonl`, one per model
call, and registers a `lookup` tool that returns `{}`.

Go compares the UI chunks exactly and the partials as JSON, in order.
