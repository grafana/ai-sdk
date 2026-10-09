## Context

This is the service-activation slice, now last in the combined stack
#367 → #368 → #373 → #370 → #369. Client capture and request-only engine contracts are owned by
`protect-gateway-client-byok-capture` and `add-gateway-request-byok-engine`.
The original combined proposal is preserved on `nrbrd/byok-before-stack`;
the three scoped changes replace it, not three copies of the same umbrella plan.
Archive/sync the changes in stack order. This change adds authenticated-service
requirements alongside the preceding change's engine requirements; it does not
modify a requirement that has not yet been synchronized into the main specs.

The frozen restack reference is Gateway 4.0.103 / ai 7.0.127 / Provider 4.0.21 at
`eb77f09e3c06c28e860d92e0de941b143c2eecec`. Main target `28e08a45` supplies that
baseline and native/UI continuation changes; earlier versioned evidence is historical. Client projection is not an oracle
for Vercel service authentication, retry policy or deployed Grafana isolation.

The design references token/exchange documentation, the pinned authlib verifier
and the existing edge/deployment contract. Repository observations do not establish
live Kubernetes behavior.

## Goals / Non-Goals

Activate disjoint private/configured and Cloud/request-only access atomically.
Preserve native content, options, continuation and returned identity. Bound
lifecycle and automatic operator capture. Do not add public JWT ingress, a CAP
validator, service allowlists, compatibility flags, credential storage or advanced
routing. Do not absorb #318–#324/#280.

## Decisions

- Keep Cloud on port 8080 and operations on 8081; add private JWT on 8082 by
  default. This preserves existing Cloud/operational wiring. Bind all listeners
  before readiness.
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

## Execution observation integration

Configured selection delegates to `CatalogSelector` and preserves its private
candidate/protection context. #373's shared SDK observer and #370's execution
projection remain independent of logical operator logging, metrics and exports.
Request-only selection supplies model/identity without configured attribution:
the Gateway registers no request collector, adds no public execution overview or
current native failure summary, and preserves an independently registered SDK
observer. This is an explicit support boundary, not evidence that no attempts
occurred; original native metadata stays opaque. BYOK public attribution remains
separately scoped and would need request-account protection before activation.

Account-access refusal and unsupported discovery reuse #370's typed fixed error
definitions without restoring the former byte-document implementation or changing
public status/type/code/message fields. No listener/route redesign is included.

## Risks / Trade-offs

- A reachable trusted-header port can be spoofed: deployment policy must deny
  direct Cloud-port access by internal JWT workloads and all non-edge callers.
- Any trusted ai-sdk JWT grants configured access: provisioning that audience is
  an authorization decision; private networking does not replace verification.
- Unknown precommit failures can follow paid work. The engine's default fallback
  policy is inherited, not a promise of exactly-once execution.
- Dummy edge/native TLS tests establish application behavior, not live CAP
  enforcement, provider acceptance or deployed network isolation.

## Migration Plan

No active-consumer compatibility layer is required. Update application flags,
Docker/CI probes and docs together. Deliver the separate same-cluster Services/JWKS/NetworkPolicy
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
