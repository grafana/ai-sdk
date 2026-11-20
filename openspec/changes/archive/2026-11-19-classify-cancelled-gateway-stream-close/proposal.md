# Classify cancelled Gateway stream closes

## Why

A provider can close its stream without a finish or an error part after the caller cancels. The extension host can receive that close before it handles the cancellation and incorrectly record an error.

## What Changes

- Record an unfinished stream close as cancelled when the caller's context has already ended.
- Keep unexpected closes with a live context classified as errors. Do not change the precedence of denial, invalid output, or observed in-band errors.

## Capabilities

### Modified Capabilities

- `gateway-extensions`: Specify observation outcomes for unfinished stream closes.

## Impact

Only the outcome in Gateway extension observation records changes. The wire response and provider stream are unchanged.
