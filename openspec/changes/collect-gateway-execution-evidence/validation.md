# Collection foundation validation

## Scope and reference

PR #373 remains stacked above #372 at 4e254ca0. The owner approved a full simplification after reviewing the original collector/JSON/budget design. The foundation still has no production service/handler callers; #370 owns public carrier delivery.

Reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. Reviewed the pinned APICallError and Gateway error mapping together with current Go native producer wrappers. Attribution is a Grafana extension, not a private Vercel-service parity claim. The original #321 decision used an earlier baseline and remains historical; this scoped owner-approved diagnostic normalization policy supersedes its lexical/duplicate policy.

## Refactor

- Removed error_json.go, essential.go, retention.go and protected_sources.go. No jsontext, token-level codec, complete-state encoding on mutation or prior-attempt retention rescan remains.
- State records synchronized request-local facts; Snapshot returns isolated copies. Metadata is a separate assembler operating on those facts with ordinary typed JSON encoding.
- Native normalization uses standard encoding/json with UseNumber and a decoded-data credential transformation. Summary and details use the same protected value. Completed diagnostic components are retained as RawMessage so their encoded byte size is directly measurable, without an encoded-size cache.
- Standard duplicate-member processing and normalized string/key escapes (including lone surrogates) are intentional owner-approved adaptations. Numeric precision, null/false/empty values, immutable source data and known-source credential protections remain covered. Ordinary URL/endpoint/request/header/private-named application values survive unless they actually contain a protected source.
- Attempt/source/component/retained-detail bounds apply before retention; mandatory aggregate/encoding and success/error output allocations apply at assembly. Mandatory memory is bounded by the capped attempt count and per-fact limits, rather than repeated whole-state marshaling. The final complete protocol document/frame budget remains the adapter's responsibility.
- Removed the Go 1.27 increase from ai-gateway/go.mod and go.gateway.work. Both keep their existing Go 1.26.3 minimum. The namespace schema remains a contract-test/documentation artifact, not a runtime validator or client dependency.

## Proof and passed checks

Package regressions cover 16/17 attempts, observed selection beyond the history limit, cancellation/sealing, current-versus-retained failures, optional-detail replacement accounting, isolated snapshots (including raw JSON and optional booleans), namespace collision immutability, and assembly rejection of escaped/aggregate mandatory overflow.

Native-error regressions cover ordinary and protected nested data, configured credential echoes, escaped/duplicate credential names, last-value normalization, trailing documents/malformed input, original invalid UTF-8, normalized surrogate strings/keys, large integer and numeric-lexeme precision, source/component limits, HTML expansion and exact optional allocations. All inputs are focused synthetic unit-test data, not provider recordings.

Passed after the refactor:

- Full Gateway tests; collector, ProviderWire and service race tests.
- Gateway vet and golangci-lint: zero issues.
- Full Gateway tests with Go 1.26.8, local toolchain and readonly dependencies in the candidate workspace. This proves minimum-toolchain source compilation, not standalone published Gateway adoption.
- mise run parity-check: typecheck, 133 schema tests and 110 client/runtime tests.
- mise run test-ai-gateway-command: 74 tests, no skips; the dormant package does not change production output.
- Docs lint, Gateway boundary, SDK/Gateway isolation, candidate workspace, merged-pin ancestry and independent published-client gates.
- Strict OpenSpec validation: 93 items; whitespace checks.

## Downstream migration and limits

#370 must replace State.Metadata(original, gateway, minimal) with evidence.Metadata(state.Snapshot(), original, gateway, minimal), and use one consistent snapshot when assembling multiple fields of an envelope. NativeError.Details is now a complete encoded component, not a mutable Component pointer. Essential and complete error-document limits belong at the protocol boundary; the old exported ErrorBytes/EssentialBytes implementation constants are no longer collector API. Update downstream tests for standard diagnostic normalization. Do not change error classification, client retry policy, event ordering, cleanup or fallback commitment.

This PR does not modify or restack #370. Package/schema tests do not establish real-handler carrier assembly, operator-sink independence or frontend interoperability. Original combined review receipts predate this refactor and are not a fresh independent review. The optional provider-shape report still lacks registered package source. No provider fixture inputs changed. Sync/archive and release remain outside this operation.
