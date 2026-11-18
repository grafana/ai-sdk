# Verification notes

Evidence is grouped by kind. Do not read one kind as another.

## Automated, synthetic (runs in CI, needs no key)

- `go test ./anthropic/...` in `ai-gateway`: the official `anthropic-sdk-go`
  client against the handler with a fake model. Covers unary, streaming with tool
  use, tool-result continuation, an unknown model, the request boundary, a
  provider failure carrying a secret sentinel, `disable_parallel_tool_use` per
  target, and the Claude Code request fixture.
- Route-table, route-adapter and manifest tests in the gateway package.
- The fixture `anthropic/messages/testdata/claude_code_request.json` keeps the
  structure of a Claude Code 2.1.326 request with placeholder text. It is a
  client request fixture, not a recorded provider response, so the provider
  fixture provenance rules do not apply.

## Live provider, manual (one session)

- Claude Code 2.1.326 ran a multi-turn read-and-edit task through a local plugin
  against a live Anthropic model. The task completed and five `POST v1/messages`
  calls returned 200. Run with a developer's own key and a local Grafana from the
  CI-pinned image digest. Not repeatable in CI.
- Authentication was checked live: a bearer token works and `x-api-key` gets 403
  from the platform before the plugin runs.

## Not verified

- Claude Code beyond a short tool session: adaptive and interleaved thinking, long
  sessions, compaction, and whether it calls `count_tokens`.
- Any behavior of the fields listed as accepted and not applied.
- Other Claude Code versions, and the Claude Code variable names for the
  background model.
- Non-Anthropic targets for `disable_parallel_tool_use` beyond the option
  translation tests; no live OpenAI or compatible backend was called.
- Cloud-mode access behavior for this route. It follows the shared selector, as
  Chat does, and has no Messages-specific test.
