## Context

This change owns #370's runtime activation of #373's compact execution projection. The existing activation, transport-owned fit checks, request-local capture and native-summary audit are implemented. The owner-approved follow-up addresses configured API-key echoes in Gateway-produced failure summaries, not arbitrary response content.

Reference: registered ai 7.0.127, Gateway 4.0.103, Provider 4.0.21 and Provider Utils 5.0.53 at eb77f09e3c06c28e860d92e0de941b143c2eecec. The upstream client preserves provider metadata and error data; it does not define the private service's credential-disclosure policy. This is an intentional Gateway security adaptation, with synthetic service/client evidence rather than live-provider parity.

## Goals / Non-Goals

**Goals:** Preserve useful compact native diagnostics while removing exact configured server API-key matches from their message, type and string code. Apply the same policy to configured and BYOK callers and to failure, successful fallback and committed-error carriers. Preserve caller-owned BYOK echoes unless their values also match configured keys.

**Non-Goals:** Generic response filtering, generated text/tool/metadata scanning, stream-fragment buffering, arbitrary encoded-secret detection, request credential inventories, model wrappers, account-provenance APIs, new collectors, routing/retry changes, schema/client changes or transport-budget redesign.

## Decisions

### Immutable startup policy

The service resolves configured credentials at startup, with no hot reload. Collect actual configured API-key values there, including unused configured provider accounts, rather than deriving keys from the selected model or scanning requests. The ProviderWire handler receives configured keys separately from its selector and builds an immutable redactor. Empty values are ignored; duplicates and overlapping values have deterministic handling. Caller mutation of the configuration slice cannot change the policy. A future reload must replace the handler and credentials together, not mutate in-flight policy.

The protected set consists of configured provider API keys and the enabled exporter credential. The owner explicitly excludes static Gateway access keys: their authentication-owned hash-only storage remains unchanged, with no raw copies retained for summary matching.

The policy is handler-owned, not part of public catalog identity, discovery, SDK fallback events or request options. BYOK selection cannot disable it. There is no per-request secret extraction and no global mutable inventory.

### Projection followed by mandatory summary protection

Keep the foundation's pure native extraction and exact numeric-code decoding. Protect only the newly projected Failure objects before execution metadata or current-error data reaches encoding. Both overview attempt failures and event-local nativeError use one redactor. It replaces exact matches in decoded message, type and string code with a fixed marker; numeric code and status are unchanged. Raw source errors, observations and opaque provider metadata are never mutated. If a marker would itself leave a configured key in a field, omit that field rather than expose the key.

All supplied configured keys apply to every summary, including plain request selections without configured attribution. Values that are also caller-supplied remain protected because configured ownership takes precedence. JSON escaping is handled by normal decoding and encoding; URL/base64 or other transformations are not inferred.

### Existing lifecycle and transport boundaries

Immutable executionRequest values retain copied configured identity, the existing source bound and immutable summary policy. attemptCapture keeps its existing synchronized enter/observe/seal/discard lifecycle. Projection and protection run outside capture locks. Configured direct outcomes remain locally derived; no one-candidate fallback or unowned late outcome is introduced.

Unary successes and stream finish retain optional providerMetadata.gateway.execution; unary/setup failures retain optional top-level providerMetadata; committed errors retain optional error.data.providerMetadata and event-local nativeError. Original primary validation, complete-response/frame fit and the existing HTTP error-read bound remain unchanged pending #394. Optional enrichment may be omitted, but a Gateway-produced summary must never revert to its unredacted form. Native metadata remains opaque even when enrichment is omitted; this narrow contract does not claim to sanitize it.

Selection, classification/retryability, SDK observers, first-part commitment, finish authority and bounded cleanup are unchanged. Plain/request-account unary/setup summary and attempt-overview gaps remain #317; no new carrier or identity is invented.

## Evidence

Focused tests exercise empty/duplicate/overlapping keys, replacement-marker collisions, JSON-escaped values, string versus numeric codes, immutable configuration, source nonmutation and preserved BYOK-only diagnostics. Handler tests cover direct and exhausted failures, successful fallback unary/finish, configured and plain committed errors, original metadata/content preservation and unchanged classification/counts/order. Both registered clients exercise the real command with dummy configured keys independently of operator capture. Existing frontend assembly tests remain applicable because the wire shape does not change.

## Risks / Trade-offs

- Exact matching is deliberately not arbitrary transformation detection or general output DLP.
- Every configured API-key source must be registered by host composition; custom hosts own that configuration.
- Startup-only key policy avoids mutable shared state; no reload mechanism is added.
- Optional fit failure may remove diagnostics, not expose an unredacted Gateway summary or replay execution.
- Synthetic tests prove the disclosure boundary, not that a real provider currently echoes credentials.

## Migration Plan

Reuse this existing change and current specs, retaining prior completed tasks as history. Implement the follow-up on current #370, leave #373 unchanged, validate and publish, then coordinate #369 from its recorded exact parent. No new proposal, schema, dependency or minimum-Go change. No merge or deployment action.

## Open Questions

None for the approved narrow summary policy.
