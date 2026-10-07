## Context

This client-only slice of #322 implements the error-retention portion of approved #321. The owner approved splitting the completed implementation into independently validated stacked PRs. #332 has since merged; this branch is rebased onto main at 1be616c3.

The reference remains ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. The pinned Gateway extract-api-call-response implementation prefers cause.data, and GatewayError derives retryability from HTTP status. PARITY.md classifies the affected layer as Gateway runtime/Go-client boundary; injected responses are client evidence, not server production evidence.

## Goals / Non-Goals

**Goals:** Preserve already-supplied opaque extensions under existing read limits, exact committed JSON and public classifications.

**Non-Goals:** Produce Gateway evidence, interpret its schema, add an accessor, change retries/core error policy, raise client Go requirements or import Gateway code.

## Decisions

- Decode HTTP and SSE error envelopes directly into private typed DTOs with encoding/json.Unmarshal. The owner explicitly approved ordinary Go case-insensitive field matching instead of maintaining exact-name error member selection. Unknown additive fields are ignored by typed decoding but remain in the retained HTTP body or opaque SSE data. This is an intentional Go acceptance deviation for noncanonical property casing, not a change to the server's emitted dialect.
- Retain SSE data as json.RawMessage while decoding the original event, bypassing the generic selected-field re-marshaling path. Remove the error-only member-decoding abstraction. Other response-family decoding stays outside this focused simplification.
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
