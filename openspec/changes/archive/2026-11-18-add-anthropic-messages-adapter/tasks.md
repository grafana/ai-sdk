# Tasks

This change is the route, the text and client-tool subset, and honest
documentation. Applying the accepted-but-ignored fields and the client evidence
that would support a compatibility claim are follow-ups, listed at the end and
tracked under #427.

## 1. Route and manifest

- [x] 1.1 Construct the Messages handler on the shared runtime and register
  `POST v1/messages` in the production route table with the `anthropic_messages`
  label and the Anthropic host-error writer. Verified by the gateway package
  tests, including the route-table and route-adapter suites.
- [x] 1.2 Add the route to `kinds/manifest.cue`, regenerate the manifest with
  `pnpm --filter ./ai-gateway generate:kinds`, and update the manifest and
  route-table expectations. Verified by `go test ./pkg/internal/manifestdata/...
  ./pkg/internal/gateway/...` passing and the regeneration changing only the
  manifest files.

## 2. Adapter core

- [x] 2.1 Implement request mapping, unary responses, SSE streaming and fixed
  error documents in `ai-gateway/anthropic/messages`. Verified by
  `go test ./anthropic/...`.
- [x] 2.2 Cover unary, streaming with tool use, tool-result continuation, unknown
  model, the request boundary, provider failures with a secret sentinel and a
  Claude Code-shaped request with the official `anthropic-sdk-go` client and
  recorded-shape request bodies. Verified by the same test run.
- [x] 2.3 Run Claude Code 2.1.326 through a local plugin with a live model on a
  multi-turn read-and-edit task. Verified by the task completing and five
  `POST v1/messages` calls returning 200. Recorded as live-provider evidence in
  `verification.md`, not as automated coverage.

## 3. Request behavior in stage A

- [x] 3.1 Apply `disable_parallel_tool_use` through a per-target option
  translator, the way Chat applies `parallel_tool_calls`. Verified by
  `TestHandler_DisableParallelToolUse` for the Anthropic, OpenAI and
  OpenAI-compatible targets, an explicit false, and a call that sets nothing.
- [x] 3.2 Commit a request fixture that keeps the structure of a real Claude Code
  request with placeholder text, and replay it through the handler. Verified by
  `TestHandler_ClaudeCodeRequestFixture` and the fixture holding no client system
  prompt text.

## 4. Documentation

- [x] 4.1 Write the client guide (`docs/guides/anthropic-messages.md`): base URL,
  bearer token (not `x-api-key`), Claude Code environment variables, supported
  subset, each accepted-but-not-applied field and each refused content type.
  Verified by review against the code. The Claude Code environment variable names
  for the background model are not yet verified against a pinned client.
- [x] 4.2 Update the Gateway architecture walkthrough, the glossary, the docs
  index and the `gateway-plugin-host` purpose line. Verified by a diff review
  against the code.
- [x] 4.3 Say in the local-development README section that any Anthropic API key
  works and that tests need none. Verified by the README diff.
- [x] 4.4 Record the verification notes for this change in `verification.md`.

## 5. Spec integration

- [x] 5.1 Run `openspec validate add-anthropic-messages-adapter --strict` and fix
  findings. Verified by a clean validation.

- [x] 5.2 Sync the deltas into `openspec/specs` and archive the change. Verified
  by `openspec list` and the synced specs.

## Follow-ups (not part of this change; tracked under #427)

Order and sizes come from the provider-mapping research, smallest first.

- Report the matched `stop_sequence` from provider metadata, unary and SSE.
- Map `output_config.effort` and ordinary thinking settings; report top-level
  usage details.
- Preserve cache markers on system blocks, content, tool results and tools.
- Preserve signed and redacted thinking in history and emit thinking blocks and
  signatures in responses. Needs a decision on how `max_tokens` relates to a
  thinking budget first.
- Provider options for request `cache_control`, `context_management`,
  `metadata.user_id` and `service_tier`, then the adapter mapping.
- Decide how unsupported controls warn on non-Anthropic targets and where a
  Messages client sees warnings.
- Compaction (#296) and a `count_tokens` decision from a longer recorded Claude
  Code session.
- Command-level suite with the official Go client against the built plugin, wired
  into CI.
- Scripted Claude Code end-to-end scenario with a pinned client, passing in CI
  without a key.
