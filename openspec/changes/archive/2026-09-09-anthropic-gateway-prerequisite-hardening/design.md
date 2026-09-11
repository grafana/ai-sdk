## Context

The ProviderWire V4 handler owns fixed protocol error documents, but later host composition also needs to emit authentication, permission, and internal failures without copying bytes or exposing verifier/error details. Separately, the Anthropic SDK loads ambient environment defaults during client construction unless explicitly disabled; applying that option later per request is too late.

These prerequisites are implemented at this change's head. The runnable authenticated Anthropic Gateway service is not.

## Goals / Non-Goals

**Goals:**

- Reuse package-owned fixed ProviderWire failure documents through a closed host-facing API.
- Make the explicit direct-Anthropic API key and reviewed request options authoritative over ambient SDK environment state.
- Prove both behaviors with focused API and poisoned-environment tests.

**Non-Goals:**

- Service configuration, outbound transport policy, JWKS authentication, discovery, routing, telemetry, process lifecycle, or real-command integration.
- Changing the Vertex construction path or removing explicit caller request options.
- Adding dynamic messages, arbitrary statuses, schema compilation, or byte-limit configuration to the host-safe writer.

## Decisions

### Keep the host-safe writer closed and package-owned

The exported API accepts only authentication, permission, or internal categories and reuses the exact fixed documents already owned by `ai-gateway/providerwire/v4`. Invalid categories select the same internal fallback. Callers cannot provide messages, causes, status, type, code, retryability, or byte limits.

### Disable Anthropic environment defaults at construction time

The direct path passes `option.WithoutEnvironmentDefaults()` and then `option.WithAPIKey(apiKey)` directly to `anthropic.NewClient`. This ordering prevents the SDK from loading base URL, credential/profile/federation, identity-token, organization, or custom-header environment state while preserving later explicit request options. Vertex retains its separate explicit Google-auth path.

## Risks / Trade-offs

- Ignoring `ANTHROPIC_BASE_URL` is an intentional security deviation from the registered TypeScript provider and is documented in parity evidence.
- The host-safe writer is deliberately less flexible than a generic HTTP error API; that is the privacy and wire-authority boundary.
- The future service change must reference these archived prerequisites rather than re-add or re-archive their capability deltas.
