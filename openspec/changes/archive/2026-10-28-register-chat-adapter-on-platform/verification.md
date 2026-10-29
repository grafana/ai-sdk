# Runtime/host integration evidence — 2026-10-28

Source is the integration worktree based on #581 `c9c802b8`, with the rebased
Chat cuts and route/client integration. Results below are current scoped
checks, distinct from the October 27 prototype results in the earlier archives.

## Current checks

- Official OpenAI Go v3.76.0: `go test -race -count=1 -v
  ./ai-gateway/test/openai-chat-completions-adapter` passed in 78.643s.
  Evidence: `/tmp/chat-sdk-28-go-final.log`.
- Official npm openai 6.27.0: package test passed 4/4 in 58.58s; TypeScript
  typecheck passed. Evidence: `/tmp/chat-sdk-28-ts.log` and the client agent's
  handoff `/tmp/chat-sdk-28-handoff.md`.
- Gateway route and manifest packages passed in 21.359s; manifest test result
  was cached. Evidence: `/tmp/chat-sdk-28-routes-final.log`. Explicit route-table
  and manifest invariants were also rerun successfully.
- The streaming implementer reported focused race tests passing, including
  second-native-segment refusal, idle/runtime cleanup and writer handling.
  Command: `go test -race ./ai-gateway/openai/chatcompletions -run
  "Test(OfficialSDKStreaming|Stream|RuntimeFailure|LateSetup|NativeContract_Streaming|ClaimedStream)"`,
  using the candidate workspace. Output was reported by the streaming agent
  without a saved log. The refusal is present in
  `TestRuntimeFailureBoundsAndCancellation`.
- Canonical manifest generator v0.60.16 ran twice with identical generated
  artifacts. `scripts/module-policy.sh workspace`, Python documentation lint
  and diff whitespace checks passed in the client integration agent's run.

Go and plugin builds used the integration checkout's absolute `go.gateway.work`
through GOWORK/GATEWAY_TEST_GOWORK, with a dedicated temporary Go cache. The TS
suite used that same candidate workspace. These are plugin-protocol runs with
signed test delegation and deterministic local Responses, Anthropic and
compatible servers, not real Grafana or commercial provider calls.

## Corrected initial failures

The initial Go run expected the prototype's 403 for a cloud BYOK caller without
controls; the shared selector correctly returns invalid-BYOK 400. The initial
self-hosted stream test expected `hello`, while its fixture emits `first` and
`second`. Tests were corrected to the intended shared-policy/fixture contracts;
the final logs above pass. The earlier failing logs are not presented as final
evidence or silently replaced with a claim of an uninterrupted green run.

After rebasing on #581's updated head, the first published CI run found that
both official-client command harnesses built their child binaries with
`GOWORK=off` when no test-only override was supplied. The current repository
uses `internal/streamrecover` from the local root module, so those child builds
could not resolve it in readonly module mode. Both harnesses now use the tracked
root `go.gateway.work` by default. The Go command suite passed with `-race`
and no `GATEWAY_TEST_GOWORK` override in 82.983s; the TypeScript command suite
passed 4/4 with `GOWORK=off` in its parent environment in 58.135s. The failing
CI run is superseded by these local checks and requires a new CI run on the
amended commit.

## OpenSpec and review cuts

OpenSpec 1.14.3 validates the three revised archived proposals/deltas and this
route change strictly. An isolated sequential replay applied unary, streaming
and compatibility deltas; each archive/sync and resulting intermediate spec
validated successfully. Thus earlier review cuts need not acquire route or
client requirements before the fourth change.

- Unary archive: Runtime.Call/Call.Generate, Translator and category mapping;
  no production route or streaming claim.
- Streaming archive: Call.Stream/Next/Close, Chat framing/identity and explicit
  refusal of additional native segments; no plugin/client-process claim.
- Compatibility archive: writer and known Anthropic metadata behavior only.
- This change: route table, manifest, access/error boundary and official
  Go/TypeScript protocol-hop evidence.

## Remaining limits

The repository's pnpm 12.15.0 package was unavailable from the public registry.
The coordinator explicitly authorized pnpm 10.22.0 for this run with package
manager auto-switching disabled. A dependency-document frozen install passed
and the complete lockfile was restored. Native pnpm 12 validation was not run.
This is a tooling limitation, not a hidden credential or private-cache fallback.

No deployed Grafana/E2E, live provider, plugin package/release or full parity
suite result is claimed by this scoped change. Scenario coverage differs by
language/backend; it is not an exhaustive cross-product. Current lifecycle
checks are still partly local to Chat. #483 shared validation remains pending
before readiness, and #583's ProviderWire segments must not silently truncate
Chat. Runtime integration and this archive do not close #394.
