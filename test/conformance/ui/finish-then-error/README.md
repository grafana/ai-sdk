# Finish then error

Provider-independent core fixtures for a provider `error` part that arrives
after a `finish` part in the same model call, for example a Gateway stream that
times out after a written finish. `input.jsonl` in each case is synthetic
`LanguageModelV4` output, not a provider recording. `generate.mts` replays it
through `streamText` from the [registered baseline](../../upstream.yaml) and
writes `expected.jsonl`, the UI message stream.

| Case | Parts after the first finish |
|---|---|
| `terminal` | an error, then close |
| `next-segment` | an error, then a new response segment and a second finish |
