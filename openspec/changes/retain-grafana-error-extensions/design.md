## Context

This client-only slice of #322 implements the error-retention portion of approved #321. The owner approved splitting the completed implementation into independently validated stacked PRs. #332 is the stack base, not a merged prerequisite.

The reference remains ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. The pinned Gateway extract-api-call-response implementation prefers cause.data, and GatewayError derives retryability from HTTP status. PARITY.md classifies the affected layer as Gateway runtime/Go-client boundary; injected responses are client evidence, not server production evidence.

## Goals / Non-Goals

**Goals:** Preserve already-supplied opaque extensions under existing read limits, exact committed JSON and public classifications.

**Non-Goals:** Produce Gateway evidence, interpret its schema, add an accessor, change retries/core error policy, raise client Go requirements or import Gateway code.

## Decisions

- Decode exact-name object members into private named DTOs. This avoids case-insensitive extension selection and redundant filter/marshal/decode passes.
- Keep the complete validated HTTP source as Data and ResponseBody; preserve SSE error.data directly as raw JSON. Reconstructing an envelope would lose extension members and raw lexical fidelity.
- Leave SSE ResponseBody empty because there is no HTTP error response to retain. Keep category/status/code validation and explicit SSE retryability consistency checks before publishing data.
- Retain malformed/oversized-response rejection and ordered error/content/finish behavior. The client does not redact arbitrary server extensions; Gateway source protection belongs to the later producer changes.

## Risks / Trade-offs

- Broader retained data is caller-visible → keep bounded reads and do not promote it into error text or UI messages.
- Native retry facts differ from hop retryability → preserve status-derived retry behavior; callers choose SDK retries explicitly.
- Synthetic envelopes do not prove Gateway output → reserve authenticated production-carrier integration for the final PR.

## Migration Plan

Extract only client decoding/tests and client guidance from the reviewed implementation. Rollback restores prior extension loss without affecting Gateway execution. No releases or generated versions are created. The separate collection and integration changes own their own specs and validation.

## Open Questions

None. Contract and split are owner-approved.
