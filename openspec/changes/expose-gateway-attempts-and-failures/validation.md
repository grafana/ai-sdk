# Validation

## Scope and reference

Runtime rewrite based on #373 at ea668bef, not the historical 4d0f9bfc collector implementation. The old branch is retained as backup/failure-visibility-before-overview-rewrite. This PR owns one active OpenSpec change; the foundation remains synced/archived.

Registered reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. Matching upstream Gateway/model/error source informed carrier/access behavior. Shared capture and private Gateway overview serialization are Go adaptations, not a private-service parity claim.

## Publication

Runtime commit 6399d319 was pushed to nrbrd/failure-visibility with an explicit lease against historical 4d0f9bfc. Draft #370 remains based on #373; its description/title now reflect this rewrite, not the obsolete collector or historical equivalence. Exactly one active PR-owned OpenSpec change was verified. The unrelated four-line go.work.sum patch remains unchanged and uncommitted.

## Regression evidence

- Catalog copies configured candidates and private sources on construction/resolution; credentials are excluded from discovery/serialized resolution.
- Direct/fallback result and failure carriers, noneligible/exhausted failures, leading error selection and error/content/finish ordering preserve invocation counts and no replay.
- Request isolation, direct cancellation/late native failure exclusion and saturated operator-output independence pass under races.
- Exact fitting response/frame enrichment succeeds; one-byte-short optional enrichment preserves the original native namespace/output. Primary-invalid JSON metadata and complete encoded primary failures remain adaptation failures.
- Configured credentials, protected request headers, explicit Anthropic/OpenAI MCP credentials and native other-tenant fields are protected without raw diagnostics, token-pattern DLP or general application-data scanning.
- Complete HTTP error enrichment stays within the existing Go-client default 64 KiB read bound; oversized optional information preserves the original classified error.
- Go-produced namespace and unary/setup/committed carriers validate against strict Gateway-owned schemas.
- Native-fake authenticated command tests compare Go/registered TypeScript carriers with actual candidate counts, public classifications, request conversion, cancellation and operator privacy.
- Real-handler low/high-level/middleware tests cover streaming errors followed by content/finish and both generation APIs. Go GenerateText uses streaming, not unary DoGenerate.
- Frontend scenario parses UI chunks with parseJsonEventStream/uiMessageChunkSchema and verifies ordered errors/content/finish and assembled text without automatic raw-diagnostic forwarding.

## Checks

- Root/Gateway/Grafana-client tests and races; root/Gateway/client vet and lint.
- Go 1.26.8 root/Gateway races and standalone readonly Grafana-client races/vet; minimums and dependency pins unchanged.
- mise run test-providerwire-v4: 134 schema and 112 contract/runtime tests.
- mise run test-ai-gateway-command: 74 authenticated command tests, no skips.
- mise run test-integration: 24 frontend files, 133 tests, plus Gateway dependency suites.
- mise run parity-check: registered replays pass; optional provider-shape drift reporting skips unavailable registered package source.
- Docs, module/license boundaries, SDK/Gateway isolation, source workspace, merged pins and published Grafana module gates.
- Strict OpenSpec: 95 items, zero failures. Whitespace and unrelated go.work.sum checksum preservation checked.

## Limits and disposition

No provider input fixture was added or changed; synthetic endpoints/UI scenarios are scoped behavior witnesses, not recorded inputs, live-provider acceptance or deployed/private-service parity. Publication does not claim fresh independent review. Native transport diagnostics #323, discovery #324, routing #316, BYOK #317, producer/core fixes #299 and broader client acceptance #375 remain separate.

Overview absence does not prove no attempts; native namespace presence does not establish Gateway provenance. Mixed/incomplete capture can be omitted, selection is commitment rather than completion, and caller retries/preselection fallback do not establish exactly-once generation or remote effects.

Sync/archive and merge remain pending separate approval.
