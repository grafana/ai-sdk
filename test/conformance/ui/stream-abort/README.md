# Stream abort UI lifecycle

These provider-independent expectations come from `streamText(...).toUIMessageStream()` in the registered `ai@7.0.173` package ([source](https://github.com/vercel/ai/tree/0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1/packages/ai)). The generator uses `MockLanguageModelV4` and an `AbortController`; there are no provider recordings or external requests.

With the repository's locked test dependencies installed, regenerate from the repository root:

```sh
pnpm -C test/conformance/tools exec tsx stream-abort.mts
```

| Scenario | Abort point | Expected chunks |
| --- | --- | --- |
| `no-output` | The provider stream has supplied `stream-start` and is asked for its next part. | `start`, `abort` |
| `partial-output` | The consumer has received the first text delta. | `start`, `start-step`, `text-start`, `text-delta`, `abort` |
| `pending-output` | The provider has queued text but cancellation is signaled before the provider result is handed back. | `start`, `abort` |

`TestStreamTextAbortUIConformance` verifies the same sequences and the UI finish callback's aborted state with controlled Go streams.
