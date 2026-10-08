## Why

Applications need to capture fallback decisions for one request without mutating a shared model or depending on an operator observer. Cancellation can currently replace a candidate error before observation, hiding why that candidate failed.

## What Changes

- Add a context-scoped attempt observer alongside the existing model observer.
- Preserve the candidate setup or validation error in `Attempt.SourceErr` before decision-time cancellation normalization.
- Keep candidate ordering, decider inputs, returned errors, first-part commitment, callback timing and cleanup unchanged.
- Keep capture in-process: no automatic metadata serialization, credential transformation or Gateway-specific identity in the Apache package.

## Capabilities

### New Capabilities

- `fallback-attempt-capture`: Request-scoped capture with candidate-local source errors and independent observer isolation.

### Modified Capabilities

None. Existing fallback selection and error-propagation requirements remain unchanged.

## Impact

Apache SDK `fallback`, its tests and the fallback guide. The dormant Gateway foundation will consume this API in `collect-gateway-execution-evidence`; runtime delivery remains a separate change. No dependency or LanguageModelV4 interface changes.
