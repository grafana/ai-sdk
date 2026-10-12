## 1. Pin evidence and establish regressions

- [x] 1.1 Use eb77f09e source/tests and existing registered client harness; add failing both-client unary/stream format-presence witnesses and remove the old differential deletion of empty responseFormat.name.
- [x] 1.2 Add focused schema fidelity, invalid roots/types/malformed JSON and no-invocation assertions; distinguish native schema rejection from transport validation.

## 2. Implement request and native semantics

- [x] 2.1 Migrate ResponseFormat.Name to *string across every producer/consumer/test/example; make output WithName("") explicit, update native OpenAI/compatible absent-only defaults and verify other adapters.
- [x] 2.2 Decode/pass JSON response-format schema/name/description in Gateway; normalize description, preserve schema RawMessage semantics and existing text behavior; keep native validation and unchanged result/stream codecs.
- [x] 2.3 Prove native fake HTTP requests for Anthropic and OpenAI Responses/compatible, preserving native options, warnings/errors/metadata/usage/finish and no extra mapped-fallback refusal.

- [x] 2.4 Preserve response-schema numbers in scoped native OpenAI/Anthropic/Bedrock decoding and serialization; retain strict single-JSON validation and intentional native transforms; add raw-native numeric fidelity and malformed/trailing-input regression tests.

## 3. Complete integration evidence

- [x] 3.1 Exercise both clients in unary and streaming modes with no schema and schema, absent/empty/nonempty names, normalized descriptions and semantic schema edge values from design matrix.
- [x] 3.2 Prove TS generateText/streamText and Go GenerateText/StreamText output parsing and invalid final output validation at client layer.
- [ ] 3.3 Prove configured fallback secondary success, noneligible/all-failed actual attribution and bounds, first-part commitment/no replay and cancellation cleanup for structured requests; report any unresolved existing attribution gap explicitly.
- [x] 3.4 Prove operator/consumer capture independence with structured request/output and errors, one logical observation, useful consumer opt-in capture and unchanged caller responses.

## 4. Documentation and validation

- [x] 4.1 Update relevant user guide/API migration guidance and stable PARITY coverage/limits without claiming synthetic HTTP as provider recordings or private-service parity.
- [x] 4.2 Run affected root/output/provider/Gateway module tests and build checks, required cross-language integration and mise run parity-check; record exact commands/outcomes and any environment blockers.
- [x] 4.3 Run OpenSpec validation and independent implementation review; address findings and leave incomplete items unchecked at the hard stop. Never stage local implementation reports.
