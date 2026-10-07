## Context

Stack 1/3 of #317 separates Apache client/capture changes from Gateway execution.
The unchanged registered baseline is Gateway 4.0.96, ai 7.0.118 and Provider 4.0.18
at `5d12eaa6caa193d3901cbab98a734403eb6bf622`. Reviewed matching
`gateway-language-model.ts`, its tests, `gateway-provider-options.ts` and
`gateway-provider.ts`: clients serialize BYOK maps and retain `request.body`.
This proves projection, not Vercel hosted-service credential behavior.

## Goals / Non-Goals

Protect opt-in automatic capture while preserving caller-owned metadata and
ordinary content. Make client-owned authentication unambiguous and permit native
selectors. Do not activate BYOK, change Gateway listeners or add account retries.

## Decisions

- Reject authentication overrides rather than silently merge them. This is an
  intentional Go safety deviation from the pinned client's permissive merging.
- Apply a transport bound rather than copy catalog grammar into the client.
- Reuse JSON encoding/decoding for typed SDK captures; redact only the credential
  subtree on the decoded copy before sink emission. Keep ordinary gateway fields
  and providerTimeouts.byok. Validate request-body objects at their capture boundary.
- Support typed provider options and JSON request/error bodies, including nested
  provider-options fields. Do not recursively interpret arbitrary Go values or
  nested byte slices as serialized JSON; omit malformed/opaque request captures.
- Keep original parameters/results untouched; inspect enrichment and Agent
  Observability separately instead of adding unsupported capture APIs.
- Keep TypeScript/application capture caller-owned rather than ship another
  redaction helper. Keep Apache tests independent of AGPL Gateway imports;
  Gateway-hosted test assets are only projection evidence.

## Risks / Trade-offs

- Caller-owned metadata still contains keys: direct application logging remains
  caller-owned and must use structural protection.
- Invalid request captures can be omitted; model execution must not fail.
  Existing JSON capture limits bound emitted data, not normalization allocations.
- Client projection does not make the existing command support BYOK.

## Migration Plan

No compatibility aliases are required for this WIP SDK. Land before the engine
and activation changes. No deployment changes are made here.

## Open Questions

None for this slice. Provider account execution and deployment isolation belong
to the following stack changes.
