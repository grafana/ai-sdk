## Context

PR #373 is the dormant foundation for #322, above #372. The owner requested a full simplification after reviewing the JSON projector, allocation machinery and native error processing, and approved the revised design. No service or handler imports this package yet. Runtime activation remains #370.

The baseline remains ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. The evidence namespace is a Grafana extension, not a private-service parity claim. PARITY.md's Gateway runtime/provider-metadata boundaries apply.

## Goals / Non-Goals

**Goals:** Tell callers which candidates actually ran, which was selected and why attempts failed, using simple bounded facts and standard Go JSON.

**Non-Goals:** Change fallback decisions, retries, stream readers, cleanup ownership or operator capture; introduce a generalized redaction/configuration framework; implement native success transport debugging or producer fixes.

## Decisions

- Immutable model wrappers record actual entry and return. Request-local synchronized state records attempts, selection, fallback intent, completion, native identity and the current error. Sealing prevents late mutation. The collector does not encode metadata or whole snapshots on mutation.
- Snapshot returns isolated facts. A separate metadata assembler owns typed output DTOs, namespace relocation and essential/optional allocation checks. It uses ordinary json.Marshal. Collection bounds attempt count and individual fact sizes; optional retention uses an incremental complete-component byte count rather than rescanning prior attempts.
- Normalize attributable single-chain APICallError sources using encoding/json with UseNumber, then transform the decoded error data to remove known credential/tenant/signing fields and detect configured credential/endpoint echoes. Derive message/type/code and details from that same protected value. Do not traverse aggregate errors or serialize arbitrary causes, request bodies or headers as a native error struct.
- Protection is limited to native error data. Ordinary URLs, endpoints, application request fields and token-looking strings are not rejected merely by generic names. Configured source echoes and actual credential fields remain protected. Opaque provider metadata uses the existing transport unchanged.
- Owner approval supersedes the old lexical-preservation and duplicate-rejection rules for rewritten native diagnostics: standard Go duplicate-member processing applies, property/string escapes and lone surrogates normalize, and numeric precision is preserved with UseNumber. Only the normalized, protected representation is published; the original raw source is never reused after protection. Client HTTP/SSE byte retention and opaque metadata are unchanged.
- Keep the approved numeric ceilings: 16 retained attempts, 1 MiB source, 128 KiB complete component, 256 KiB retained/success diagnostics, 32 KiB mandatory evidence and 24 KiB optional error diagnostics. Complete runtime error/frame/response limits remain the protocol adapter's responsibility. Overflow produces complete dispositions, not truncated messages or JSON.
- Remove jsontext and restore the existing Go 1.26.3 Gateway/workspace minimum. Keep the Gateway namespace schema as documentation and contract-test evidence, not runtime validation or an independent client dependency.

## Migration Plan

Refactor the dormant package and update its tests/specs in #373. #370 must consume a facts snapshot through the metadata assembler rather than call metadata assembly on mutable state; its carriers, classification, emergency output and event ordering remain unchanged. Do not restack or edit that PR as part of this foundation update.

## Risks / Trade-offs

Standard normalization deliberately changes diagnostic JSON spellings and duplicate acceptance. Number precision and meaningful null/false/empty values remain intact. Unknown producer fields stay absent rather than inferred. Package/schema tests do not prove handler integration or live-provider safety. The original #321 decision remains historical; this scoped owner-approved revision supersedes its diagnostic lexical/duplicate policy, not attribution, credentials or bounds.
