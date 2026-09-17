## 1. WP11 and exact-client evidence

- [x] 1.1 Apply and validate WP11 first; verify accepted prerequisites and exact registered upstream stream types, Gateway transport and ai tool-loop tests.
- [x] 1.2 Add failing pinned Vercel streaming call/input and two-step real-handler cases; pair with Go providers/grafana plus existing root orchestration.
- [x] 1.3 Capture expected next-request call/result history and deterministic local execution count; disable incidental retries in acceptance fixtures.

## 2. Apache stream support

- [x] 2.1 Audit current Go stream decoder and provider StreamPart tool fields; add/fix only gaps proven by exact-client event cases.
- [x] 2.2 Verify affected native provider tool-input/call conversion against registered fixtures/tests and extend reusable normalized observation where needed.
- [x] 2.3 Verify root Go client orchestration completes the loop; add parseJsonEventStream/uiMessageChunkSchema integration coverage if UI chunk behavior changes.
- [x] 2.4 Commit Apache prerequisites separately and pin the resulting approved immutable versions before isolated Gateway release validation.

## 3. Gateway state machine

- [x] 3.1 Enable WP11 request subset for streaming without weakening deferred-family checks.
- [x] 3.2 Add bounded per-tool-ID input/call/result state, standalone calls, interleaved inputs, name matching, metadata placement and finish validation.
- [x] 3.3 Add allowlisted private event DTOs and schema fixtures for input events, calls and basic results; preserve empty deltas and non-null JSON result semantics.
- [x] 3.4 Add invalid order/duplicate/unmatched/deferred marker tables, complete-frame boundary and part-budget tests.
- [x] 3.5 Verify non-terminal errors during open input, cancellation/timeout, finish/EOF, write failure and bounded cleanup under hostile channel behavior.
- [x] 3.6 Retain WP9 effect guard and test zero physical invocation for streaming definitions/choice/history.

## 4. Complete milestone evidence

- [x] 4.1 Run both authenticated clients through at least two real-handler HTTP generations with local function execution and native continuation assertions; separately test basic result transport.
- [x] 4.2 Inspect actual metadata-only exports, logs and metrics for one canonical generation per request and no tool-content/private markers.
- [x] 4.3 Run affected Go module tests, focused race tests, committed-pin Gateway GOWORK=off tests, ProviderWire contract checks and mise run parity-check; run test-integration when UI behavior changes.
- [x] 4.4 Update narrative scope, PARITY and existing activation smoke when applicable; record provider execution/approval/media and effectful fallback deferrals.
- [x] 4.5 Strictly validate this OpenSpec change, run git diff --check and report WP12 separately from WP11.

Apply handoff: exact-client loops, safe-JWKS command/native streaming continuation,
basic-result transport, and focused hostile open-input/between-step cancellation
now pass. The authenticated real-command matrix verifies streaming definitions,
choice, assistant call history, and complete continuation are rejected before
either physical candidate runs for both clients; direct tool loops still pass.
On 2026-09-17, the committed-pin Gateway module tests, focused Gateway/service
race tests, Go Gateway client tests, complete ProviderWire contract checks, and
`AI_SDK_UPSTREAM_ROOT=/Users/luccas/code/ai mise run parity-check` passed. The
upstream checkout supplies the registered commit, not its current HEAD. Real
command tests additionally cover automatic streaming success/error execution,
full native continuation history and required-to-none tool choice. Both clients
preserve empty, malformed and escaped input strings; focused decoder assertions
distinguish an empty input string from an absent required input field.

After dependency-order restacking, WP12 is based on the completed WP11 head
`6f3af90`; the exact-client and committed-pin suites below validate that
integrated prerequisite. The existing immutable root SDK pin is
`v0.1.0-alpha.1.0.20260916154023-07aacebe97a2`; the native Anthropic pin is
`v0.0.0-20260916162302-e9128cc3b35a`. Isolated tests verify these versions resolve
and execute, and the Apache prerequisite remains committed separately in
`69000b2`. The dependency and pin reconciliation is complete; production
activation remains a separate gate.
No root UI chunk behavior was changed; no reusable observation extension was needed.
