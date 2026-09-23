## Why

The public Grafana AI Gateway authenticates Cloud credentials at cortex-gw, but the Go provider only sends internal access-token JWTs. Its `NewWithCloudAuth` name obscures that incompatibility: exchanging a CAP token does not produce a credential accepted by the deployed Cloud route.

## What Changes

- Add `NewWithCloudCredentials` and `CloudCredentialsConfig` for direct stack-ID/CAP-token authentication through the public authenticating proxy, without token exchange.
- **BREAKING**: rename `NewWithCloudAuth` to `NewWithTokenExchange` and `CloudAuthConfig` to `TokenExchangeConfig`, with no compatibility aliases. Preserve exchange behavior and `NewWithAccessToken`.
- Make authentication selection explicit and protect Cloud credentials and identity headers from request-level overrides. Reject acting-user tokens in the Cloud-credential flow rather than implying user delegation is supported.
- Add Go and pinned Vercel client evidence for discovery, unary generation and streaming through the existing Cloud-mode command and a deterministic edge shim.
- Rewrite authentication documentation around a shared Go/Vercel guide: choosing credentials and endpoints, working client examples, policy scope/realm prerequisites, constructor migration, troubleshooting and the limits of local evidence. Link existing client/server guides to this single explanation.

## Capabilities

### New Capabilities

None; this extends the existing public client capability.

### Modified Capabilities

- `grafana-gateway-client`: add direct CAP authentication, explicitly name token exchange, and specify mode-aware construction, header ownership, regression evidence and a shared Go/Vercel authentication guide.

## Impact

- Code: `providers/grafana`, its internal capture executable, ProviderWire differential/command tests, and repository-owned constructor call sites.
- Documentation: a shared guide under `docs/`, its index/navigation, the provider guide, package reference and existing Gateway Cloud authentication guide. Keep API signatures in godoc and server operational requirements in the server guide. Update parity coverage only where stable evidence or support boundaries change.
- Upstream reference: `@ai-sdk/gateway@4.0.87`, `ai@7.0.107`, commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, as registered in `test/conformance/upstream.yaml`. No baseline upgrade or Vercel upstream change.
- External rollout: `deployment_tools` must provision correctly scoped policies and configure consumers; consumer repositories must adopt the new API or renamed exchange API. Those changes are separate owner-approved work, not edits in this proposal's implementation scope.

## Non-goals

- No Gateway server authentication, listener, network-policy or ProviderWire payload changes.
- No cortex-gw JWT support, new Auth API exchange mechanism, automatic authentication fallback, or browser credentials.
- No migration of k6 delegated workers to long-lived/system CAP credentials. Their short-lived session contract remains a separate workstream.
- No live credential provisioning, secret access, deployment changes or hosted-support claim from local tests.
