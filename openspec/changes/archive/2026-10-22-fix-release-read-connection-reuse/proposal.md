# Proposal

## Why

Live root authorization completed library validation but its final GitHub read
failed on a closed pooled HTTP connection. Synchronous child validation prevents
the client from processing the idle socket close before the next request.

## What Changes

- Request fresh connections in the existing read-only GitHub transport.
- Reproduce server-side socket closure during blocked client execution.
- Repeat live root authorization without a diagnostic preload.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. Existing authorization and fail-closed requirements stay unchanged.

## Impact

Release tooling transport/tests and operational documentation only. No runtime
parity behavior, dependency versions, GitHub write permissions or activation changes.
