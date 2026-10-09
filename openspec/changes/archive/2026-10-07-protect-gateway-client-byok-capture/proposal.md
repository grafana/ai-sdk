## Why

BYOK client requests contain credentials in caller-owned metadata. Before enabling
request-only accounts in the Gateway, clients need transport-safe selectors,
unambiguous authentication headers and structural capture protection.

## What Changes

- **BREAKING:** reject reserved authentication-header overrides in every Go constructor and call.
- Accept bounded native model selectors without client-side catalog discovery.
- Protect credential subtrees in supported SDK captures before log emission.
- Prove Go and registered Vercel request projection, metadata ownership and capture safety.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: header ownership, native selectors and BYOK request metadata.
- `structured-logging-middleware`: bounded structural BYOK protection.

## Impact

Apache client/middleware code and pinned-client projection tests.
No Gateway authentication, account policy, listener or native execution changes.
This is stack 1/3 for #317; `add-gateway-request-byok-engine` and
`activate-gateway-auth-and-request-byok` follow it.
