## Context

See proposal.md - Why. The Bedrock provider signs SigV4 requests with a fixed service constant (`bedrock`) in `signRequest`. Bedrock Mantle is a distinct AWS service whose credential scope requires `bedrock-mantle`; the endpoint host is `bedrock-mantle.<region>.api.aws` and is reached today via `WithBaseURL`. Existing constraints: default behavior must not change (parity), bearer-token auth must be unaffected, and the change must stay additive (no upstream `@ai-sdk/amazon-bedrock` counterpart).

## Goals / Non-Goals

**Goals:**
- Allow SigV4 requests to Mantle to be signed with the correct `bedrock-mantle` credential scope.
- Zero-config for the common case: pointing `WithBaseURL` at a Mantle host should sign correctly without a second option.
- Preserve the default `bedrock` service for every non-Mantle endpoint.

**Non-Goals:**
- Converting Converse-shaped requests into Mantle's OpenAI-/Anthropic-compatible request bodies and paths (separate follow-up).
- Adding a dedicated Mantle base-URL builder or region-to-URL helper.
- SigV4a (asymmetric, multi-region) signing.

## Decisions

- **Expose a `WithSigningService(service string)` option plus host inference, rather than a `WithMantle()` boolean.** A free-form service name is the AWS-native primitive (SigV4 "signing name") and covers proxy/VPC hosts that do not encode the service. Inference keeps the common Mantle case config-free. A boolean would be less general and would still need an escape hatch for non-standard hosts. Precedence is explicit override → host inference → `bedrock` default.
- **Infer from the endpoint host prefix `bedrock-mantle.`** parsed from the resolved endpoint URL. This matches AWS's documented Mantle host shape (`bedrock-mantle.<region>.api.aws`) and avoids matching lookalikes such as `bedrock-mantle-proxy.example.com`. A malformed URL falls back to the default service (no signing-service inference), leaving existing error paths unchanged.
- **Resolve the service per request inside `signRequest`** via `resolveSigningService()`, not at construction. Keeps the resolution adjacent to signing, requires no new construction-time validation, and reads cleanly with the existing lazy-credential flow. Bearer-token requests return before the signer runs, so the service name never affects them.

## Risks / Trade-offs

- **Auth works but request shape does not (Mantle uses OpenAI/Anthropic APIs).** → Documented explicitly in godoc and `docs/providers/bedrock.md` as a scope limitation; the option is a correct building block, and request-shape routing is a tracked follow-up.
- **Host inference could surprise a caller who intentionally points a Mantle host at a `bedrock`-scoped shim.** → `WithSigningService` always wins, giving a deterministic override; default (non-Mantle) behavior is unchanged.
- **Go-specific divergence from upstream.** → Additive option with an unchanged default; noted in godoc as an endpoint-agnostic signing extension, and no committed conformance fixtures change.
