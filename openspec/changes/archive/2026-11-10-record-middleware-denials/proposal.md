# Proposal

## Why

A gateway or policy layer that refuses a call surfaces that refusal as the call's error, so the structured logger and Prometheus model middleware record it as an `error`. Denials then inflate error rates and error-level alerts although no provider failed. The Gateway's extensions (`ErrDenied`) need a distinct outcome, and the middlewares have no way to tell a denial from a failure.

## What Changes

- `middleware/prometheus.Options` gains `IsDenied func(error) bool`. A call error or stream error part it reports as a denial uses status `denied` with error type, status code and finish reason `none`, before every other classification rule.
- `middleware/logger.Options` gains the same `IsDenied`. A denied call's terminal record uses outcome and error type `denied`, logged at `Level` instead of `ErrorLevel`; a denied stream error part's per-part record also uses `Level`.
- Both default to nil, which leaves classification unchanged. A panicking classifier reports no denial.
- Package documentation and the middleware guides describe the option.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `prometheus-middleware`: the status vocabulary gains `denied`, chosen by the optional denial classifier.
- `structured-logging-middleware`: the options gain the denial classifier, and denied calls log outcome `denied` at the success level.

## Impact

- **Code:** `middleware/prometheus` (options, classification, stream observation), `middleware/logger` (options, generate and stream terminal records, per-part error records).
- **APIs:** one new optional field in each module's `Options`; both modules get a minor release. No behavior change when the field is nil.
- **Docs:** `docs/middleware/prometheus.md`, `docs/middleware/structured-logging.md`, package godoc.
- **Parity:** not parity-sensitive. Both middlewares are Grafana-owned and have no upstream counterpart; no stream part, chunk or provider request changes.
