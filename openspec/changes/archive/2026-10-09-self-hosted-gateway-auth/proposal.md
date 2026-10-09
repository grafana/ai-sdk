# Proposal

## Why

Self-hosted operators need production configured-model access without Cloud accounts, CAP exchange or a JWT issuer. The approved design adds boot-selected local authentication while preserving the configured-account versus request-BYOK boundary.

## What Changes

- Select JWT, static-key or explicit development unsafe authentication in strict YAML.
- Snapshot environment-backed named opaque credentials once and authenticate before protected work.
- Allow disabling Cloud ingress; permit disabled production export only with static-key and Cloud disabled.
- Prove Go/Vercel interoperability, restart behavior and image bootstrap; document operator responsibilities.

## Capabilities

### New Capabilities

- `gateway-self-hosted-auth`: boot configuration, static admission, local identity and optional Cloud ingress.

### Modified Capabilities

None. This additive capability specializes the inherited draft unified-authentication contracts for explicit self-hosted configuration; omitted configuration retains their behavior.

## Impact

Gateway internal config/auth/process/service, command/client/image tests and operator guides. No new public SDK API or upstream version change. No issuance, dynamic rotation, ACLs, proxy trust plugin, deployment or release work.

Temporary raw hashing buffers are bounded and explicitly cleared at boot and per request without changing environment provisioning or promising runtime memory erasure.
