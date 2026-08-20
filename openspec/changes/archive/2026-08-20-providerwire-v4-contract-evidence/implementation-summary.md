# Implementation summary

## Baseline and source evidence

- Repository head: `c0e19b316edda4cfde8bb08f0398950a576d68b9`.
- Registered upstream commit: `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`.
- Exact workspace pins: `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/provider-utils@5.0.27`.
- Source comparison used the registered commit object in `/home/nara/src/ai` through `git show`; the checkout's current branch was not treated as authority.
- `artifacts/source-equivalence.json` records byte-identical SHA-256 evidence for the explicit 88-file closure maintained in `src/source-equivalence.ts`: provider LanguageModelV4 declarations, the Gateway language-model/configuration/error path, and provider-utils request/header/response helpers. Exact package pins remain the broad guard. Ordinary checks validate these installed hashes without consulting the upstream checkout.

## Contract and generated evidence

- `src/request-coverage.ts` is the sole maintained request-surface ledger. Per-arm `satisfies` constraints enforce finite keys and discriminators, and the generic evidence validator consumes its exact/presence observations.
- `classification.json` is generated from that typed ledger as a reviewer-facing artifact; its baseline metadata is derived from `upstream.yaml` and the workspace manifest.
- `schema/providerwire-v4-request.schema.json` is the maintained draft 2020-12 request schema. It closes standardized objects, represents JSON Schema 7 including `$defs`, excludes provider-reference `type`, and leaves provider option JSON opaque.
- `artifacts/semantic-requests.json` contains 24 deterministic scenarios and 25 ordered requests generated through the public pinned client. It covers unary, streaming, multi-step tool continuation, complete prompt/tool unions, presence-sensitive values, arbitrary/configured/protocol/observability headers, and supported versus excluded content-type collisions.
- Response inputs remain locally authored smoke probes for unary JSON, clean-EOF JSON SSE, and one safe non-2xx normalization path. They are not provider recordings or exhaustive response/server evidence.

## Phase 2 handoff

`phase2-delta.md` and the external-package tests in `provider/providerwire_v4_loss_test.go` retain only distinctions an explicit V4 mapper cannot recover from transport-neutral Go values: fractional numeric domains, optional false/empty scalar presence, and typed construction of empty inline-text file data. Required members and nil versus non-nil empty collections are explicit codec responsibilities, not provider-model deltas. The remaining losses form one coherent Phase 2 provider request-contract correction.

## Validation

The following commands passed after the final artifact generation:

- `mise run update-providerwire-v4-artifacts`
- `mise run check-providerwire-v4`
- repository content hash comparison before and after the non-mutating check, after retaining the required `go.work.sum` additions
- focused ProviderWire workspace typecheck/tests and baseline-tooling tests
- `go test ./provider -run '^TestProviderWireV4Loss_'`
- `go test ./gateway/providerwire`
- `(cd providers/grafana && go test ./...)`
- `mise run validate-parity-baseline`
- `mise run parity-check`, which now requires the ProviderWire check
- `mise run test`
- `mise run test-integration`
- `mise run test-interop`
- `mise run test-conformance`
- `mise run vet`
- `mise run lint`
- `mise run lint-docs`
- `openspec validate --all --strict`
- `git diff --check`

The first lint invocation encountered stale golangci-lint cache data referencing a removed sibling worktree. After `golangci-lint cache clean`, the unchanged source tree passed with zero issues across every module.

## Review and simplification

The evidence-gated review/fix loop ran for three rounds with one forked oracle and fresh correctness, validation, and simplification reviewers. Validated findings corrected git visibility, per-arm key and role-specific discriminator exhaustiveness, exact empty and deterministic observations, source-closure gaps, metadata shape checks, semantic Go witnesses, and stale documentation. The final review round found no blocker; one remaining executed response-helper closure gap was fixed before final validation.

A separate fresh-context `simplify-changes` reviewer then identified two worthwhile local simplifications. Header scenario construction and focused schema/multi-step assertions are now table-driven; regeneration confirmed identical semantic request bytes. The suggestion to centralize parity-consumer lists was not applied because the validation and upgrade workflows intentionally own separate lists and focused omission tests enforce both registrations.

## Residual gaps and scope confirmation

- No strict ProviderWire V4 production decoder, handler, client, model resolver, authentication, policy, lifecycle, or server response taxonomy is implemented.
- The deployed tolerant `gateway/providerwire` dialect and `providers/grafana` transport are unchanged and remain covered by their existing tests.
- The five demonstrated Go semantic request-model loss groups remain intentionally unfixed for Phase 2; codec-only presence behavior is excluded.
- Response probes establish only pinned-client consumption of the three represented inputs.
- No provider API, provider implementation, frontend wire output, provider recording, or conformance fixture input changed in this phase.
