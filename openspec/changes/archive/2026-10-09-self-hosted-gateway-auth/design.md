# Design

## Context

Base: draft #369 at 323b6524f1c02cf2f4cf0a91b61e409a3741c88f, stacked on nrbrd/failure-visibility and #368/#367. Restacked during implementation from the previously reviewed 55a80621b3c27d966e1bb8b86cb6b67a65cad844 after the dependency advanced. Preserved new execution attribution, all dependency tests, and current listener defaults: private 8082, Cloud 8080, operational 8081. #248 remains a draft overlapping startup/router code. Preserve inherited activate-gateway-auth-and-request-byok artifacts unchanged, including its separate deployment gate. Our feature diff introduces and archives only this change.

Registered upstream: vercel/ai eb77f09e3c06c28e860d92e0de941b143c2eecec, @ai-sdk/gateway 4.0.103 (inherited baseline upgrade, not changed by this feature). Source gateway-provider.ts and tests confirm explicit apiKey becomes Authorization Bearer. Server authentication/discovery are Grafana extensions with no upstream private-service oracle; this changes host composition, not ProviderWire framing. Existing Go NewWithAccessToken supplies X-Access-Token.

## Goals / Non-Goals

Goals: production standalone admission and same-permissions identity attribution, strict startup validation, existing Cloud policy isolation, both clients and restart proof.
Non-goals: key issuance, database, hot reload, overlapping secrets, per-model ACLs, JWT semantics for opaque keys, deployment activation or network-isolation certification.

## Decisions

Extend existing bounded strict YAML with presence-aware auth and server.cloud.enabled. Reject null/empty auth, unknown/duplicate keys and YAML merge keys and aliases, unselected provider blocks and multiple documents using safe decoder diagnostics. Omitted auth alone preserves legacy settings; explicit auth conflicts with enabled legacy unsafe or nonempty legacy JWKS.

Validate common scalars first, load routes, resolve selected auth/active listeners, validate selected endpoints and runtime bounds, then resolve secrets/build dependencies and bind. JWT-only limits and timeout contribution apply only to JWT. Unsafe remains development and loopback-only. Cloud defaults enabled but disabled Cloud constructs no handler or listener. Use paired addresses/handlers and existing readiness/rollback/shutdown.

Static entries are sorted, bounded to 1–64 and named with 1–64 ASCII alphanumeric/dot/underscore/hyphen characters. Environment references follow shell identifier syntax. Resolve each distinct reference once; reject missing, empty, invalid and duplicate keys. Keys are exact 1–4096 byte RFC6750 bearer strings. Store SHA256 digests, copy constructor slices and compare every entry using constant-time digest comparison without early-match return. No secure erasure promise.

Reject both credential alternatives, duplicate/case-colliding/coalesced values, malformed tokens and all Cloud/acting-user assertions. Valid callers use static-key source, configured account policy, service name and local:name subject, with empty namespace and zero stack. Middleware keeps fixed 401 and precedes body/catalog/provider work. Telemetry uses bounded source/outcome labels only.

Move production export requirement into runtime validation with one exception: static-key and Cloud disabled. Enabled export retains all TLS/endpoint/bounds/ambient checks; disabled export resolves no credential and creates no workers.

## Risks / Trade-offs

- Draft dependency changes → rebase and repeat affected checks; do not claim Cloud deployment readiness.
- Restart rotation → running snapshots remain unchanged, replicas may temporarily accept different keys, admitted work is not reauthenticated.
- Local image tests require Linux Docker → implement proof, report local skips distinctly from Linux CI acceptance.
- Shared keys give shared permissions → names only attribute calls, never confer tenancy or ACL isolation.

## Migration Plan

Omitted YAML preserves legacy JWT/unsafe behavior. Operators explicitly select static-key and disable Cloud; private TLS termination and isolated unauthenticated operational ingress remain deployment responsibilities. Rotate via coordinated environment replacement and replica restart with possible disruption. No deployment is performed here.

## Temporary credential buffer cleanup

Boot and request hashing share a bounded mutable scratch buffer. Validate exact syntax and the 4096-byte limit before copying, hash only the populated bytes, then explicitly clear that application-owned buffer. Provisioning still uses keyEnv; environment and net/http-owned strings remain untouched. Compiler/runtime/hash internals may retain other copies, so this is limited temporary-buffer hygiene, not secure memory erasure.
