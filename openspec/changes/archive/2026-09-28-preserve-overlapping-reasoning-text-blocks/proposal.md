## Why

When a provider interleaves text blocks with different IDs, `ExtractReasoning` can lose or misattribute delayed text starts and reuse reasoning IDs. This remains reproducible by inspection of `middleware/extract_reasoning.go` and diverges from the registered `ai@7.0.109` upstream implementation (commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`), which keys delayed starts by text ID and assigns reasoning IDs across the stream; see grafana/ai-sdk#254.

## What Changes

- Preserve each overlapping text block's start, delta and end association when extracting reasoning, including reasoning-only blocks.
- Allocate distinct reasoning segment IDs across interleaved blocks without changing per-block tag buffering, separators, or single-block behavior.
- Add focused stream-event regressions and provider-independent frontend assembly evidence for interleaved IDs.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `builtin-middleware-extract-reasoning`: require correct text lifecycle and unique reasoning segment IDs for overlapping/interleaved streaming text IDs.

## Impact

- `middleware/extract_reasoning.go` streaming state and `middleware/extract_reasoning_test.go`; potential core StreamText/UI fixture and cross-language `test/integration/` scenario to establish frontend chunk compatibility.
- No public API, generate behavior, dependencies, or provider adapters change. Provider-independent synthetic inputs are appropriate; no fabricated provider recordings.
