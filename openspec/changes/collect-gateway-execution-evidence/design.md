## Context

This is the internal foundation slice of #322, stacked above the independent client change and #332. It extracts unchanged code from 39354e21. It intentionally has no service catalog or ProviderWire handler call sites yet; package tests explicitly drive the observation seams. Production activation is owned by expose-gateway-attempts-and-failures.

The approved #321 decision defines Grafana-specific evidence, not private Vercel-service parity. The unchanged baseline is ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. Existing pinned metadata and error carriers remain opaque. PARITY.md's Gateway runtime/provider-metadata boundaries apply; collection tests do not establish carrier delivery.

## Goals / Non-Goals

**Goals:** Review request-local attribution, source protection, bounded immutable snapshots and namespace shape independently of handler integration.

**Non-Goals:** Change fallback decisions, read provider streams, activate response fields, alter client retries, implement native success transport debugging or repair missing producer fields.

## Decisions

- Model wrappers retain immutable configuration; request state lives in context behind a mutex. Candidate entry records actual work, not configured candidates. Existing fallback decisions supply selection/intent; direct part observation supplies selection and completion when invoked by a future handler.
- Capture candidate-local structured errors before cancellation replacement. Do not infer attribution from joined errors or parse wrapper prose. Keep current event error separately without an unbounded error history.
- Preserve identities separately and construct a normalized essential snapshot. Metadata assembly copies the map and relocates a native gateway namespace under nativeMetadata without trusting it.
- Check source bytes before parsing, complete serialized components before retention, and aggregate/essential sizes before returning snapshots. Above 16 attempts retain the observed count, not a partial history. Sealing rejects late mutations.
- Use standard jsontext for structure and duplicate detection; remove known-source credential fields and check surviving configured echoes. Preserve raw string/key and number lexemes instead of ordinary decode/re-encode normalization. Reject duplicate components, including discarded subtrees.
- Require Go 1.27 only in the Gateway module/workspace, as already approved. The existing configured toolchain is Go 1.27.1. No external JSON dependency or client-baseline increase is needed.
- Ship the namespace schema and strict acceptance/rejection tests here. The final PR validates actual emitted handler carriers; schema acceptance alone is not runtime support.

## Risks / Trade-offs

- Internal package temporarily has no production callers → make the activation dependency explicit; keep this PR independently compiling and tested.
- Source protection cannot infer absent producer semantics → omit unavailable facts, preserve ordinary useful identifiers, and leave producer improvements to #299.
- Essential evidence can overflow → return an assembly error; the later handler integration owns canonical emergency output.
- Package/schema tests do not prove lifecycle wiring → reserve service, real-handler, both-client and frontend integration tests for the final PR.

## Migration Plan

Introduce the dormant internal foundation and schema, then activate it in the next stack entry. No runtime response changes or data migration occur here. Rollback removes the unused package/schema and restores the Gateway baseline.

## Open Questions

None. No new product or public API decision is introduced by the split.
