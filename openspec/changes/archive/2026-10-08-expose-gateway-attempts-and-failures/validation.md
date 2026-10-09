# Validation

## Scope and reference

#370 activates #373's compact execution overview for configured routes. Stack #379 orders #367 → #368 → #373 → #370 → #369; changes to #370 require a coordinated #369 rebase. This PR owns one synced/archived OpenSpec change, not the historical collector/budget design.

Registered reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. Matching upstream Gateway source/tests establish client carrier preservation. Request capture and private overview serialization are Go adaptations, not private-service parity.

## Current implementation and regression evidence

- Catalog selection supplies copied private configured identity/credentials. Plain request selections install no Gateway attempt observer and omit both overview/current summaries, preserving opaque native metadata and independently registered SDK observers.
- Explicit request inputs, a synchronized short-lived capture and finalized views separate collection from projection. Projection inspects native errors after releasing the capture lock. Tests cover concurrent isolation, late callbacks, reentrant/panicking inspection and stable finalized output.
- Existing invocation ownership determines direct/fallback outcomes. Rejected unary outcomes discard unowned fallback history; locally owned direct cancellation remains observable. First error parts establish selection, not completion. Physical counts, fallback policy and error/content/finish order remain unchanged.
- Protection covers actual configured/inbound/body credentials, case-distinct and mixed-type MCP headers, known MCP URL credentials and native other-tenant fields. Ordinary URLs/application data remain useful; no raw diagnostic trees or token-pattern DLP are introduced.
- Adapter-local typed public errors preserve canonical HTTP/SSE classifications, retry fields and fixed bytes. Current stream summaries remain event-local and take priority over optional overview metadata.
- Namespace assembly only builds candidate metadata. Transports validate/encode primary output first, independently check enrichment and write accepted bytes. Production-handler tests cover exact enriched fit, one-byte shortage, exact-original fit, native namespace preservation and invalid primary metadata/encoding in unary and finish paths. HTTP errors retain the existing 64 KiB complete-read bound.
- Encoder tests use direct metadata/failure inputs; handler tests prove lifecycle integration. Namespace tests retain input isolation and malformed-candidate coverage rather than implementing synthetic transport envelopes.
- Authenticated native-fake command tests compare both clients' configured direct/fallback delivery and actual invocation counts. Low/high-level/middleware access and schema-parsed frontend tests preserve ordered error/content/finish assembly without automatic diagnostic forwarding. Go GenerateText uses streaming.

## Validation of the simplification

All checks passed with readonly dependencies where applicable:

- Full Gateway `go test -race ./...`, `go vet ./...` and `golangci-lint run ./...` (zero issues).
- Go 1.26.8 full Gateway races/vet and standalone Grafana-client races/vet.
- `mise run test-providerwire-v4` and `mise run test-integration`: strict schemas, runtime/client interoperability and frontend assembly.
- `mise run test-ai-gateway-command`: 74 tests, no skips.
- `mise run parity-check`: registered replay checks; the optional provider-shape report still skips unavailable registered source.
- Docs, Gateway boundary, SDK isolation, source workspace, merged-pin, strict OpenSpec and whitespace checks.

The pre-refactor execution/ProviderWire race suites also passed. The new exact-original finish boundary initially used a frame smaller than the handler's mandatory canonical-error minimum; its native fixture was expanded so the test exercises a valid configured limit. No production limit was relaxed.

The preceding restack additionally passed root and isolated-foundation races/vet on Go 1.26.8. An inherited standalone client/logger test dependency was repaired by #367/#368's owner, moving the composition witness into the existing SDK-only integration harness without changing published manifests. Both foundation and activation replay ranges were patch-equivalent after that repair.

## Review and disposition

The historical independent review loop reached its three-round cap, not a clean verdict. It fixed credential-source gaps and a pre-publication cancellation ownership race. The last ownership fix and subsequent owner-approved refactors have parent regression validation but no fresh independent follow-up; no clean or merge-ready claim is made.

No provider input fixtures, wire shapes, SDK contracts, dependency pins or workspace checksums changed in this simplification. Synthetic/schema evidence does not establish live-provider acceptance or deployed/private-service parity. Overview absence does not prove no attempts; a native gateway namespace does not establish provenance. Selection and caller retries do not establish completion, exactly-once generation or replay safety.

The owner authorized spec sync/archive on 2026-10-08; all 15 implementation tasks are complete. Merge requires separate approval. Native transport diagnostics #323, discovery #324, routing #316, BYOK #317, producer/core #299 and broader client acceptance #375 remain separate.
