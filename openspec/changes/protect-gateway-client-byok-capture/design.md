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
- Structurally replace the entire known BYOK subtree before serialization or
  truncation; omit unsafe capture representations rather than scrub substrings.
- Keep original parameters/results untouched; inspect enrichment and Agent
  Observability separately instead of adding unsupported capture APIs.
- Provide a tested copy-based TypeScript helper. Keep Apache tests independent
  of AGPL Gateway imports; Gateway-hosted test assets are only projection evidence.

## Risks / Trade-offs

- Caller-owned metadata still contains keys: direct application logging remains
  caller-owned and must use structural protection.
- Bounded sanitization can omit an attribute; model execution must not fail.
- Client projection does not make the existing command support BYOK.

## Migration Plan

No compatibility aliases are required for this WIP SDK. Land before the engine
and activation changes. No deployment changes are made here.

## Open Questions

None for this slice. Provider account execution and deployment isolation belong
to the following stack changes.
