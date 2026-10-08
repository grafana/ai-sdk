## Context

This is stack 3/3. Client capture and request-only engine contracts are owned by
`protect-gateway-client-byok-capture` and `add-gateway-request-byok-engine`.
The original combined proposal is preserved on `nrbrd/byok-before-stack`;
the three scoped changes replace it, not three copies of the same umbrella plan.
Archive/sync the changes in stack order. This change adds authenticated-service
requirements alongside the preceding change's engine requirements; it does not
modify a requirement that has not yet been synchronized into the main specs.

The upstream reference remains Gateway 4.0.96 / ai 7.0.118 / Provider 4.0.18 at
`5d12eaa6caa193d3901cbab98a734403eb6bf622`. Client projection is not an oracle
for Vercel service authentication, retry policy or deployed Grafana isolation.

The original research inspected deployment-tools `5e3acd0dc1b7aa32cc5391131e50d3e59aa7c2ea`,
backend-enterprise `4d042d837c312b80f78e341558dc0f7b38059ef4`, auth token/exchange
documentation and the pinned consuming authlib verifier. These are repository
observations, not live Kubernetes evidence.

## Goals / Non-Goals

Activate disjoint private/configured and Cloud/request-only access atomically.
Preserve native content, options, continuation and returned identity. Bound
lifecycle and automatic operator capture. Do not add public JWT ingress, a CAP
validator, service allowlists, compatibility flags, credential storage or advanced
routing. Do not absorb #318–#324/#280.

## Decisions

- Bind private JWT, proxy-only Cloud and operational listeners before readiness.
  Replace auth.mode and the combined listener; partial startup closes all binds.
  Shutdown withdraws readiness and cancels both populations under one deadline.
- Use authlib signature/type/expiry/audience verification against explicit regional
  JWKS. Accept one access header or bearer JWT, never mixed/duplicate credentials.
  Reject Cloud assertions on the private listener before application body reads.
- Accept concrete positive-int64 stack namespaces and service-level wildcard
  access. Only a verified acting-user token with a bound concrete namespace adds
  user context. Service identity is optional attribution, not an allowlist.
- Keep CAP scope/realm/stack/IP verification and credential stripping at the
  existing edge. Cloud application requests require trusted assertions and reject
  surviving auth credentials; they never fall back to JWT verification.
- Derive immutable account policy from authentication. Configured selectors and
  discovery enforce configured access. BYOK selectors have no catalog dependency;
  their discovery handler returns the closed HTTP 400 unsupported error.
- Wrap the prepared credential-attempt model once with logical observers after
  extraction. Keep configured canonical identity; retain bounded request identity
  for BYOK logs/exports and use a fixed model metric bucket. Do not cache observers
  by unbounded request model IDs or rewrite native response identity.
- Keep listener activation and authorization in this PR rather than expose a
  transitional Cloud port with configured credentials. Earlier stack branches
  deliberately retain the old command composition.

## Rebase alignment

Main's #326/#328 now preserve opaque options and mapped configured fallback
capabilities. Keep those paths and their tests intact rather than reinstate field
inventories or a text-only wrapper. BYOK Anthropic composition reuses the same
consumption-backed native guard before credential attempts: invalid consumed MCP
configuration/history, container skills and provider-side fallback are rejected.
Valid bounded MCP configuration/history, inert local markers, harmless fields and
ordinary namespaces remain available to the adapter. This guard has no catalog
or configured-account dependency.

## Risks / Trade-offs

- A reachable trusted-header port can be spoofed: deployment-tools must deny
  direct Cloud-port access by internal JWT workloads and all non-edge callers.
- Any trusted ai-sdk JWT grants configured access: provisioning that audience is
  an authorization decision; private networking does not replace verification.
- Unknown precommit failures can follow paid work. The engine's default fallback
  policy is inherited, not a promise of exactly-once execution.
- Dummy edge/native TLS tests establish application behavior, not live CAP
  enforcement, provider acceptance or deployed network isolation.

## Migration Plan

No active-consumer compatibility layer is required. Update application flags,
Docker/CI probes and docs together. Deliver the separate Services/JWKS/NetworkPolicy
handoff before activating an environment; an incomplete deployment must remain
unavailable rather than restore Cloud access to configured accounts.

## Open Questions

The service uses the reviewed engine decoder and explicit account constructors;
OpenAI organization/project settings are preserved. It permits native default
endpoints only and does not introduce custom destination-policy configuration.
The structured logger keeps its existing 2,048-byte model-identity field bound
locally; this is not an execution selector limit.

The deployment delivery still needs an owner/link and actual isolation evidence.
No repository-local test substitutes for that external gate.
