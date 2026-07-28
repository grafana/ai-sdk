## Why

Applications need a reusable way to audit and debug LLM provider calls without wiring orchestration callbacks at every `StreamText` or `GenerateText` call site. The existing provider middleware layer already wraps any `provider.LanguageModel`, including registry-resolved models, so a structured logging middleware can solve this cross-cutting concern once at model construction time while preserving provider behavior.

## What Changes

- Add a new nested Go module `middleware/logger` that exports a provider-layer structured logging middleware using the standard library `log/slog` package.
- Provide `logger.Middleware(opts)` and `logger.Wrap(base, opts)` APIs that compose with `middleware.Wrap`, `middleware.WrapLanguageModel`, `registry.WithLanguageModelMiddleware`, `fallback`, and `middleware/sigil`.
- Log both direct `LanguageModel.DoGenerate` calls and streamed `LanguageModel.DoStream` calls through `WrapGenerate` and `WrapStream` hooks.
- Emit stable, machine-parseable event names and `ai_sdk.*` attribute keys for start, finish, error, and optional per-stream-part records.
- Default to privacy-safe summaries: model identity, durations, scalar request settings, usage, finish reason, warning/error summaries, response metadata, and stream part counts; prompts, generated content, reasoning, tool payloads, files, raw chunks, headers, request/response bodies, provider options, and provider metadata require explicit capture options.
- Add capture and redaction controls so opt-in payload logging can still redact known secret-bearing keys.
- Document how this provider-layer logger differs from future core telemetry and how middleware ordering affects composition with Sigil.

## Capabilities

### New Capabilities

- `structured-logging-middleware`: Independent structured logging middleware for provider language model calls, including API shape, privacy/redaction behavior, generate and stream logging semantics, module isolation, and documentation/validation expectations.

### Modified Capabilities

(none — the existing `language-model-middleware`, `provider-registry`, and `sigil-middleware` capabilities are reused without changing their requirements.)

## Impact

- **New nested module:** `middleware/logger` with module path `github.com/grafana/ai-sdk/middleware/logger`, `replace github.com/grafana/ai-sdk => ../../`, and dependencies limited to the root ai-sdk module plus the Go standard library.
- **Root module unchanged:** the root `github.com/grafana/ai-sdk` module does not import `middleware/logger`, so consumers who do not opt in gain no new dependencies or API surface.
- **Provider behavior unchanged:** no changes to `provider.LanguageModel`, `provider.CallOptions`, `provider.GenerateResult`, `provider.StreamPart`, stream wire format, UI message chunks, or orchestration callbacks.
- **Affected implementation areas:** new files under `middleware/logger/`, package godoc, tests in the nested module, and user-facing docs under `docs/guides/` plus a link from `docs/guides/middleware.md`.
- **Parity posture:** this observes existing provider calls and must forward results/stream parts unchanged. It should validate against the registered upstream baseline only for governance (`test/conformance/upstream.yaml`), not attempt to port the broader upstream telemetry integration registry.
