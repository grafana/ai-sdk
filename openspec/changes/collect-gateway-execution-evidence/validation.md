# Validation

## Scope and decisions

This single PR-owned OpenSpec change covers the Apache shared fallback capture stage and the AGPL compact Gateway projection. The temporary `capture-fallback-attempts` change has been merged into this change and removed.

The original Gateway evidence subsystem is removed: no collector, model wrappers, selected index, diagnostic count/byte allocations, retention accounting, raw details, disposition taxonomy, completion/replay-risk claims or post-selection error history remains. The foundation is still dormant; service/handler activation and #370 restacking are not included.

Reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at `5d12eaa6caa193d3901cbab98a734403eb6bf622`. Pinned APICallError sources keep native data separate from causes. No matching upstream fallback implementation exists: observation is a Go adaptation and the overview is a Grafana extension, not private-service parity. No provider fixture inputs or recordings changed.

Owner-approved policy: optional enrichment that cannot fit existing complete-response/frame limits preserves original metadata, including an opaque native gateway namespace when relocation alone cannot fit. Namespace presence alone is not provenance or authority, and omitted overview is not evidence of no attempts. Original primary validation must pass before enrichment is accepted.

## Shared SDK evidence

Implemented in `f8dffbfe`. Tests first failed because context observation and SourceErr did not exist (`/tmp/373-overview-design/sdk-red.log`).

- Request/model observers receive the same ordered snapshots, with independent panic recovery and child observer replacement/disablement.
- Twenty-four independent request contexts share one fallback model under races without callback leakage.
- Cancellation/deadline during a decider preserves SourceErr while existing decision/returned errors preserve the context cause.
- Leading error parts remain selected stream content; setup/validation/EOF and exclusion of unowned late native setup results preserve existing ownership.
- GenerateText/StreamText provide callback access to preceding failures while preserving selected provider metadata and isolating a panicking operator observer.
- Root tests/races/vet/lint, full root Go 1.26.8 races, Gateway tests/races, parity, docs and module boundaries passed for this stage. Logs: `/tmp/373-overview-design/sdk-{root-race,gateway,go126,parity,boundaries}.log`.

## Gateway projection evidence

Tests first failed because the pure projector/assembler did not exist (`/tmp/373-overview-design/projection-red.log`).

- Complete ordered actual decisions, configured candidate/account identity and selected/failed/canceled outcomes are projected. Unrun candidates, mixed indices, impossible progression and unfinished advancement are omitted.
- Thirty-two observed attempts remain a full array; no diagnostic attempt cap erases history.
- Safe messages survive API URL/cause presence. Bare/enveloped errors, standard duplicate/escape handling, precise large numeric codes, empty string codes and body fallback are covered.
- Known credential/other-tenant scalar echoes and supplied actual sources are protected field-by-field. Raw body/header/request/cause trees never become output. Native inputs remain unchanged.
- Malformed/invalid UTF-8/oversized diagnostic sources under the caller's existing read bound degrade to available safe status; there is no new allocation or normalization error.
- Shared fallback-to-projector composition preserves setup failure attribution and exact error/text/finish order with disabled, record-dropping or panicking model observers. The record-dropping callback is a unit witness, not a saturated production queue proof.
- Metadata inputs/other namespaces are isolated. Fitting native namespace collisions relocate intact; exact complete-response and SSE-frame boundaries, one-byte overflow and original-size-only fit preserve original metadata and successful content. Optional encoding failures and originally invalid namespaces cannot become successful primary responses through relocation.
- Go-produced namespaces validate against the compact schema; strict TypeScript schema tests reject selected-error history, selected indices, completion claims, details and dispositions. Schema validation alone does not establish invocation order, complete observation or production delivery.

## Passed final commands

- Full Gateway source-workspace `go test ./...`.
- Gateway execution/ProviderWire/service `go test -race`, full `go vet` and golangci-lint: zero issues.
- Full Gateway source-workspace tests on Go 1.26.8 with readonly dependencies.
- Full root races/vet/golangci-lint: zero issues; prior full root Go 1.26.8 races remain applicable to the unchanged SDK stage.
- ProviderWire TypeScript typecheck and strict schema tests: 134 tests, no failures.
- `mise run parity-check`: registered schema/client/runtime checks and provider replays.
- `mise run test-ai-gateway-command`: 74 tests, no skips; production output remains unchanged.
- Docs lint, Gateway workspace/module/license boundary, SDK isolation and merged-pin gates.
- `MODULE=providers/grafana mise run verify-published-module`: readonly public dependencies pass; the independent client is unchanged.
- `openspec validate --all --strict`: 93 items, zero failures.
- Whitespace checks and byte-for-byte preservation of the unrelated `go.work.sum` patch.

Final logs: `/tmp/373-overview-design/projection-{gateway,root,go126,schema,parity,command,boundaries}.log`. A final source-protection check also covers credentials in a single-chain URL cause; execution/ProviderWire races, Gateway vet/lint and Go 1.26 focused tests passed again in `projection-final-focus.log`.

## Migration and remaining proof

#370 must replace the removed evidence wrapper/state with finalized call-local fallback observation, feed one invocation's decisions/descriptors/actual protected sources into `execution.Project`, and call `execution.Metadata` with existing complete-envelope primary validation/fit checks. Direct routes retain their own lifecycle, not one-candidate fallback. Current committed errors stay in existing event payloads; no finish history duplicates them. Existing error/content/finish ordering, operator independence and no replay after first-part commitment require fresh runtime/both-client/frontend proof in that activation change.

No production overview delivery, live-provider acceptance or fresh independent implementation review is claimed. The optional provider-shape report skips unavailable registered provider package source; passing parity does not establish that drift report. Source-workspace tests establish same-revision Gateway adoption, not a newly published SDK dependency. No Go minimum or module pin changed. OpenSpec sync/archive and PR merge remain pending owner approval.
