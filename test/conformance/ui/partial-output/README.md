# Partial output

Provider-independent core fixtures for structured-output partials and step
text. The inputs are synthetic `LanguageModelV4` text output, not provider
recordings. `generate.mts` replays them through `streamText` from the
[registered baseline](../../upstream.yaml) and writes:

- `expected.jsonl`: the UI message stream;
- `expected-partials.json`: every value `partialOutputStream` delivered;
- `expected-result.json`: the final `text`, the final `output`, and the
  innermost cause of the output error, or `null`.

| Case | Output | What it pins |
|---|---|---|
| `empty-array` | `Output.array` | The first partial, before any valid element, is `[]` |
| `multi-step-repeat` | `Output.object` | Step 2 republishes the partial step 1 ended on, and its text-delta boundaries follow its own partials |
| `interleaved-text` | `Output.object` | Partials parse only the first text part while another part's delta arrives between its deltas, and step text joins whole parts |
| `first-text-start` | `Output.object` | The first text part is chosen at its `text-start`, not at its first delta |

`multi-step-repeat` reads `input-1.jsonl` and `input-2.jsonl`, one per model
call, and registers a `lookup` tool that returns `{}`.

Go compares the UI chunks and the final text exactly, and the partials and
final output as JSON. The two text-part cases end with a JSON parse error whose
upstream cause is a JavaScript message, so Go only checks that the output
failed.
