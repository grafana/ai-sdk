## Why

AI Gateway requires an internal JWT. Deployments behind an authenticating reverse proxy need a separate mode that accepts trusted identity headers.

## What Changes

- Add a Gateway-owned request-authentication boundary with startup-selected `access-token` and `cloud-gateway` implementations. Preserve internal JWT behavior without fallback between trust models.
- Validate forwarded Cloud identity headers without verifying Cloud Access Policy tokens or repeating edge scope checks. Retain target stack, policy organization, and policy identifier separately.
- Require deployment-verified proxy-only API ingress before Cloud-mode activation. The application does not inspect cluster policies or establish proxy identity from header syntax.
- Construct authentication dependencies by mode. Cloud mode must not construct or retrieve JSON Web Key Sets (JWKS).
- Separate API and operational listeners in Cloud mode. Preserve the default internal combined listener and cancel-first shutdown.
- Keep provider credentials and routing configuration server-owned. Add fixed-value authentication observations and privacy assertions.
- Extend real-command tests with the registered low-level Gateway client and a test-only edge shim. Keep real edge authorization and network enforcement outside application proof.
- Add `ai-gateway/docs/cloud-authentication.md`, linked from `ai-gateway/README.md`. Document the application's required headers, proxy isolation, settings, and client limitations. Exclude private proxy configuration and deployment procedures.

The implementation follows five phases. Application implementation does not authorize deployment activation.

## Capabilities

### New Capabilities

- `ai-gateway-cloud-authentication`: Startup-selected authentication, distinct trusted Cloud identity, independent dependencies, listener isolation, coordinated lifecycle, and client-composition privacy.

### Modified Capabilities

None. Existing ProviderWire contracts and internal defaults remain unchanged. The Gateway-owned guide is an explicit scope exception to general SDK documentation placement, not a repository-wide documentation reorganization.

## Impact

Implementation affects `ai-gateway/cmd/grafana-ai-gateway/internal/{auth,config,outbound,service,process}` and their tests. Command evidence belongs in `ai-gateway/test/providerwire-v4/gateway-command.test.ts`; completed coverage belongs in `test/conformance/PARITY.md`.

The registered baseline remains `@ai-sdk/gateway@4.0.52` and `ai@7.0.65` at Vercel commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. High-level `generateText` and `streamText` remain unsupported. Model-call evidence uses low-level `doGenerate` and `doStream` with explicit `maxOutputTokens`.

No reusable SDK, provider module, authlib dependency, provider fixture input, or package version changes are required. Deployment resources, ingress enforcement, image publication, and rollout remain separate work. No alternate Cloud-header name or implementation is introduced. The main specification is synced only after implementation and verification.
