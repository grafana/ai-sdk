# Tasks

## 1. Root unsupported-functionality sentinel

- [x] 1.1 Export `provider.ErrUnsupportedFunctionality` with godoc that states it is pre-I/O and distinct from APICallError. Add root tests showing wrapped identity survives `errors.Is` and that `errors.As` finds no APICallError. Verify with `go test ./provider/...`.
- [x] 1.2 Add retry tests showing StreamText and GenerateText make exactly one model call and no backoff for a wrapped sentinel. Verify with `go test -run Retry ./...`.
- [x] 1.3 Add fallback tests for unary and stream setup:
  - the default decider advances on the sentinel to a succeeding candidate;
  - all-reject exhaustion keeps every sentinel identity, with one call per candidate;
  - a custom decider returning false stops.

  Verify with `go test ./fallback/...`.

## 2. Anthropic custom alignment

- [x] 2.1 Convert a custom tool-result entry with `anthropic.type: "tool-reference"` to a raw `tool_reference` block (raw `toolName`, omitted when absent), and warn `other` / `unsupported custom tool content part` for other custom entries. Send `content: []` when non-empty content loses every entry. Add request-snapshot unit tests labeled synthetic: mixed text/file/custom order, missing and non-string `toolName`, and a single non-reference entry. Verify with `mise run test-anthropic`.
- [x] 2.2 Skip unknown assistant custom kinds without a warning while keeping `anthropic.fallback` replay. Update the existing warning tests to assert absence. Verify with `mise run test-anthropic`.
- [x] 2.3 Emit unary `anthropic.container_upload` custom content with `anthropic.fileId` in block order; streaming stays unchanged. Add synthetic response-conversion tests. Verify with `mise run test-anthropic` and `mise run test-conformance`, with existing fixtures unchanged.

## 3. Bedrock, OpenAI-compatible and OpenAI custom alignment

- [x] 3.1 In Bedrock Converse, return a wrapped sentinel error naming the custom tool content type while converting user/tool-role results. The existing pre-conversion filter already removes tool content when `hasActiveTools` is false. Add DoGenerate and DoStream tests asserting zero HTTP requests and `errors.Is`, filtering without rejection for no tools and for tools with `toolChoice: none`, unchanged warnings for existing file cases, and an unchanged Mantle warning. Verify with `mise run test-bedrock`.
- [x] 3.2 Wrap the existing Converse conversion errors for provider-reference files, non-S3 URLs and unsupported image/video/file media types with the sentinel, keeping their messages. Add `errors.Is` and zero-HTTP tests, and confirm warning sites are unchanged. Verify with `mise run test-bedrock`.
- [x] 3.3 Skip Bedrock Converse assistant custom parts without a warning, with a request test asserting no warning. Verify with `mise run test-bedrock`.
- [x] 3.4 Skip OpenAI-compatible assistant custom parts instead of returning an error, with a request-snapshot test. Verify with `mise run test-openai-compatible`.
- [x] 3.5 Align OpenAI Responses compaction replay with upstream. Use a local raw-presence check for `itemId` (an empty string counts; null or absent doesn't) without changing the exported options type. Make `encryptedContent` optional, building the item with an override that omits the key. Add request-snapshot tests asserting the key is absent. Verify with `mise run test-openai`.
- [x] 3.6 Document the Converse rejection and sentinel classification, and the Mantle difference, in `docs/providers/bedrock.md`. Record the scoped AGENTS-convention exception there and in a code comment, not in PARITY.md. Verify with `mise run lint-docs`.

## 4. Grafana client custom consumption

- [x] 4.1 Decode unary `custom` content (kind, order, metadata presence), failing explicitly on a missing or non-string kind and accepting an empty kind, with model tests. Repoint the `TestModel_UnaryFailures` "unknown family" case to a still-unsupported family such as `file`. Verify with `mise run test-grafana`.
- [x] 4.2 Decode stream `custom` events between text deltas with absent, empty and opaque metadata. A malformed kind emits one bounded PartError. Add stream tests. Verify with `mise run test-grafana`.
- [x] 4.3 Add request-projection regression tests confirming custom history and custom tool-result entries keep their current projection. Verify with `mise run test-grafana`.

## 5. Gateway request mapping

- [x] 5.1 Map assistant custom parts (kind plus scoped options, reserved-namespace refusal) and custom tool-result entries (scoped options, empty allowed). Remove the `custom-content` capability and its error document, and update `schema_test.go` and `runtime_test.go` accordingly. Keep the message in the test-only `schema/error.json` enum, following the `files` precedent. Flip the `custom` row in `TestRuntimeUnsupportedCapabilities` to a success expectation. Approval-response options remain uninterpreted. Verify with `mise run test-ai-gateway`.
- [x] 5.2 Add a `custom-content` request golden family emitted by the registered Vercel client, covering assistant custom history and a custom tool-result entry in unary and streaming mode. Generate it with `mise run update-providerwire-v4-goldens`. Add Go replay rows expecting 200, one model call and the exact mapped kinds and options. Verify with `mise run test-providerwire-v4` and `mise run test-ai-gateway`.
- [x] 5.3 Extend native-options and fallback tests so custom history and tool-result options reach every candidate unchanged on direct and configured-fallback routes, including an OpenAI-compatible candidate receiving compaction history. Verify with `mise run test-ai-gateway`.
- [ ] 5.4 Record the consumption audit in the PR description for Anthropic tool-reference `toolName`, Anthropic fallback replay `from`/`to`, and OpenAI compaction `itemId`/`encryptedContent`, including the stored item-reference case. Also record OpenAI-compatible stringification of custom entries into tool text and the OpenAI Responses drop-with-warning. Each gets a disposition and paired bypass/harmless tests. The stored item-reference case is recorded as accepted for internal configured accounts; under BYOK (#368) it resolves in the caller's own account. Verify with `mise run test-ai-gateway` and `mise run test-openai`.

## 6. Gateway unary and stream custom output

- [x] 6.1 Add a private unary custom DTO with kind validation, kind bytes in the content preflight, and the shared metadata budget. Add a custom arm to the test-only `schema/unary_success.json`, with valid fixtures and invalid ones (undotted or missing kind, extra members). Relabel the `response_test.go` empty-kind `ContentCustom` case as invalid-kind. Test custom-only paid success, metadata absent/`{}`/opaque, and fixed internal error for invalid kind or over-budget output before HTTP 200. Verify with `mise run test-ai-gateway`.
- [x] 6.2 Add the atomic stream custom event with complete-frame preflight; it sets content-started and leaves block state unchanged. Add a custom arm to the test-only `schema/stream_event.json`, with valid and invalid fixtures. Test:
  - custom between open text and tool blocks;
  - the existing `stream_test.go` custom case with kind `"private"` moves to the invalid-kind cases, and a dotted-kind case passes;
  - response metadata after custom fails;
  - invalid kind or oversized frame produces one terminal error;
  - a first-part custom commits the fallback candidate;
  - a synthetic `anthropic.message_start`-shaped custom event (`anthropic: {id, model, usage}`) immediately after response metadata and before text passes unchanged. It is labeled synthetic, and no baseline native emission is claimed.

  Verify with `mise run test-ai-gateway`.
- [x] 6.3 Add metadata-only telemetry tests showing custom options and metadata stay out of operator logs and Agent Observability records while caller responses retain them. Assert the intended disposition: AO omits custom parts, the logger records no custom kind, and Prometheus counts custom as first output. Verify with `mise run test-ai-gateway`.

## 7. Gateway unsupported-functionality classification

- [x] 7.1 Add the all-branches sentinel predicate in the shared provider error mapping. It goes last, after the APICallError, `url.Error` and `net.Error` checks and immediately before the internal default. Add ProviderWire handler tests with fake models returning the wrapped sentinel, for unary and stream setup:
  - direct rejection returns 400 with no client retry;
  - all candidates rejecting returns 400;
  - rejection then success returns the success;
  - rejection with a 503, in both orders, keeps the existing classification, and so does rejection with an unknown error;
  - an APICallError whose cause wraps the sentinel keeps API classification;
  - cancellation wins.

  Verify with `mise run test-ai-gateway`.

## 8. Two-client and cross-language evidence

- [x] 8.1 Add TS workspace tests where the registered Vercel client consumes unary and streaming custom output (known and unknown kinds, metadata presence), plus Go-client differential scenarios. Verify with `mise run test-providerwire-v4`.
- [x] 8.2 Add authenticated command tests with native OpenAI and OpenAI-compatible fakes for compaction round trips through both clients on direct and fallback routes. Add native Anthropic fakes for fallback-marker output and history replay, labeled format-conversion evidence because the command refuses enabling server-side fallback. Verify with `mise run test-ai-gateway-command`.

## 9. Docs and parity

- [x] 9.1 Update `docs/providers/grafana-gateway.md` to describe custom content support and remove "custom tool-result formats are not supported". Update the PARITY.md Gateway rows and the synthetic-evidence gaps for `tool_reference` and `container_upload`, and note that no command-served adapter produces the rejection yet. Verify with `mise run lint-docs` and `mise run validate-parity-baseline`.
- [x] 9.2 File `upstream-sync` issues for the pre-existing native gaps listed in the design's non-goals. One covers Anthropic (content-entry cache control, dropped-file warnings, request-wide MCP routing); one covers Bedrock assistant-role tool results. Verify both exist and are linked in the PR.

## 10. Integration checks

- [ ] 10.1 Run `mise run check`, `mise run lint`, `mise run parity-check`, `mise run test-ai-gateway-source-integration`, `mise run verify-ai-gateway-boundary`, `mise run verify-sdk-gateway-isolation` and `mise run verify-merged-pins`. Confirm they pass, and report any standalone module-pin readiness gap without claiming publication readiness.

## Workflow follow-up

- Notify the upgrade automation once merged so the target `anthropic.message_start` emitter and server-side fallback beta can rebase onto this codec.
- Mark the Bedrock Converse breaking change in the PR title or footer.
- Tell the upgrade automation that Prometheus counts custom as first output, so `message_start` will set time-to-first-output.
- Archive the change after review.
