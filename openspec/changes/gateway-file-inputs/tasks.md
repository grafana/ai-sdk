## 1. Establish regression evidence

- [x] 1.1 Reconfirm `test/conformance/upstream.yaml`, `PARITY.md`, and the pinned provider/Gateway/native conversion sources and tests identified in `design.md`; record any baseline changes before implementation.
- [x] 1.2 Add focused registered-client request cases for all four ordinary file arms in user, assistant, and supported tool-result positions, including binary/base64 equivalence, empty text/data, filename absence/empty/non-empty, and scoped opaque options. Regenerate only the affected semantic goldens with `mise run update-providerwire-v4-goldens`.
- [x] 1.3 Add production Go golden replay assertions for selections, filename presence, scoped options, and invocation counts; confirm these fail on the current mapper. Retain unchanged comprehensive-golden rejection for unrelated deferred families.
- [x] 1.4 Add failing provider-domain/native-request tests for empty selected text/data, conflicting arms, malformed/null/non-string/reserved references, and native absent-versus-empty filename defaults. Include tagged SDK decoding with inactive empty/null arm members and mixed legacy aliases so conflicts cannot disappear before validation.
- [x] 1.5 Add failing ordinary UI file JSON/conversion tests for absent/empty/non-empty filenames in user and assistant roles, using pinned `ai` UI types, validation, and conversion as the reference.

## 2. Complete provider-domain selection and filename presence

- [x] 2.1 Add public bytes/URL/reference/text constructors alongside the existing base64 constructor and complete arm inspection. Unify arm resolution, validation, and tagged JSON behavior; reject conflicting members during decoding before dropping them while retaining unambiguous supported legacy forms.
- [x] 2.2 Change `ContentPart.Filename`, `ToolResultContentValue.Filename`, and ordinary UI `FilePart.Filename` to `*string`, preserving absent/empty/non-empty values through JSON, `ConvertToModelMessages`, nested file content, and client projection.
- [x] 2.3 Migrate root orchestration, source helpers, examples, and test call sites. Add generated-to-input and source-normalization regressions without changing generated/source filename APIs or UI `SourceDocumentPart`; do not add filename fields to generated file SSE chunks.
- [x] 2.4 Verify direct-caller structural validation covers ordinary and nested file inputs, with zero outbound provider requests for invalid selections or references; keep provider-specific support decisions in native converters.

## 3. Align native provider request conversion

- [x] 3.1 Update Anthropic/shared Vertex conversion and citation bookkeeping to use selected arms and filename presence. Assert native empty text/data and absent/empty title behavior, message/file option precedence, references, and supported media.
- [x] 3.2 Update OpenAI Responses request and tool-result file conversion; assert native JSON defaults only absent filenames, retains explicit empty values, preserves reference/provider-option behavior, and rejects unsupported arms before HTTP.
- [x] 3.3 Update OpenAI-compatible file conversion; assert PDF filename presence, supported media/data/URL behavior, and pinned rejection of unsupported reference/text arms.
- [x] 3.4 Update Bedrock file and tool-result conversion for selected empty data/text and pointer filenames while preserving native name sanitization/defaulting, S3/media constraints, reference rejection, and the documented tool-result URL deviation.
- [x] 3.5 Run affected native provider tests and provenance-valid conformance replays. Classify any remaining differences and identify request-snapshot or live-provider evidence gaps without synthesizing recorded inputs.

## 4. Extend the independent Go client and reusable observation

- [x] 4.1 Update `providers/grafana/request.go` to use public arm inspection and presence-aware filenames at ordinary/nested file positions. Preserve binary-to-base64, URLs, references, text, and scoped options without importing Gateway DTOs or validators.
- [x] 4.2 Extend Go-client request-contract and zero-auth/zero-network rejection tests, including selected-empty conflicts, null/malformed references, inactive fields, and forbidden reasoning-file arms/filename.
- [x] 4.3 Migrate Agent Observability file mapping and logger redaction to selection-aware data and pointer filenames. Test capture-enabled and metadata-only behavior without fetching URLs or interpreting text/references as empty binary.
- [x] 4.4 Run root, Go-client, Agent Observability, logger, and affected reusable middleware tests; keep filename/URL/reference values out of metric labels.

## 5. Establish independent module consumption

- [x] 5.1 Arrange Apache prerequisite commits before dependent Gateway commits and obtain owner-approved publication of immutable SDK/provider refs. Stop for publication approval/access if unavailable.
- [x] 5.2 Update affected module dependencies, including Gateway pins, to proxy-resolvable prerequisite versions without committed replacements or Gateway registration in the root workspace.
- [x] 5.3 Verify affected modules with readonly `GOWORK=off` checks and `mise run verify-module-resolution`; verify Apache builds/imports remain independent of the Gateway subtree.

## 6. Enable bounded Gateway file mapping

- [x] 6.1 Extend private file DTOs and shallow mapping for user/assistant files and supported tool-result file entries in both execution modes, preserving selection, order, media type, and filename presence through public constructors.
- [x] 6.2 Map registered message and file-entry provider options with opaque JSON and explicit empty namespace preservation. Reuse the reserved-host-namespace rule without enabling top-level options, body headers, or unrelated part/output options.
- [x] 6.3 Make focused golden replays pass; update supported/unsupported runtime assertions without enabling reasoning files, generated outputs, custom content, approvals, or provider-executed history.
- [x] 6.4 Add negative handler tests for missing/null/inactive members, mixed arms, reference shape, role unions, and reserved namespaces, asserting zero execution-boundary/resolver/model calls.
- [x] 6.5 Add complete-request byte-limit tests below/at/above the limit for inline data, text, and escaping/base64 overhead, plus cancellation and no-URL-fetch regressions. Retain existing safe error and response/SSE contracts.
- [x] 6.6 Adapt the fallback route guard to preserve text eligibility for retained ordinary message-option namespaces containing only empty objects, without removing those namespaces. Add authenticated unary/streaming failover assertions that every candidate receives the same empty objects. Continue rejecting files, active/invalid options, reserved namespaces, and effectful history; do not add file fallback semantics.

## 7. Prove cross-client and host behavior

- [x] 7.1 Extend differential request capture to compare registered Vercel and Go semantic bodies for file selections, filename presence, and scoped options in both execution modes.
- [x] 7.2 Add authenticated real-handler/command scenarios for file-bearing unary and streaming calls and nested file tool-result continuation, asserting native request mapping and unchanged supported response consumption.
- [x] 7.3 Add hostile payload markers across every file arm, filename, reference, URL/query, and provider option; assert metadata-only records/logs/metrics and safe errors remain private for success, failure, and cancellation with one logical generation per admitted HTTP call.
- [x] 7.4 Run `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, and `mise run test-ai-gateway`; verify focused file goldens, existing deferred-family checks, privacy, bounds, and lifecycle regressions pass.

## 8. Documentation and completion gates

- [x] 8.1 Update centralized SDK input guidance/examples and Gateway-owned support/operator documentation for constructors, filename migration, provider-specific support, and deferred WP16/17 behavior. Extend production smoke if file inputs are being activated and retain source-offer/license obligations.
- [x] 8.2 Review fixture provenance and consider affected request/UI/object expectations explicitly; regenerate only justified expectations. Update `PARITY.md` only for stable evidence-boundary changes and register actionable gaps in upstream-sync issues.
- [x] 8.3 Run `mise run build`, `mise run test`, `mise run vet`, `mise run lint`, `mise run fmt-check`, `mise run build-examples`, and `mise run test-examples` for the migrated module set.
- [x] 8.4 Add a deterministic Go testserver scenario and matching Vitest test proving user/assistant UI file filename absence/empty/non-empty survives Go JSON, model-message conversion, and client projection against pinned TypeScript UI validation/conversion. Run `mise run test-integration`; do not invent filename fields on file SSE chunks, and parse any SSE used by the scenario with `parseJsonEventStream` and `uiMessageChunkSchema`.
- [x] 8.5 Run `mise run parity-check`, repeat readonly standalone module verification against committed pins, and run `openspec validate gateway-file-inputs --strict`. Record exact results, remaining evidence gaps, and any intentional deviations before declaring WP15 complete.

## Verification

- Registered reference: `test/conformance/upstream.yaml` commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`; the local upstream checkout HEAD differs, so all comparisons used that exact Git object and the pinned npm workspace.
- Passed: `mise run build`, `mise run test`, `mise run vet`, `mise run lint`, `mise run fmt-check`, `mise run build-examples`, `mise run test-examples`, `mise run test-integration`, `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, `mise run test-ai-gateway`, `mise run parity-check`, `mise run verify-module-resolution`, `mise run verify-ai-gateway-boundary`, `mise run lint-docs`, and `openspec validate gateway-file-inputs --strict`. The final clean-tree formatting check and pinned-version checks were repeated after implementation commits.
- Published Apache prerequisites: root `f224e3d5`, adapters/observers `252f0507`, Bedrock `94f83881`, contract/UI evidence `3dcc232d`, and Anthropic file correction `20642796`. The Gateway module pins the corrected Anthropic version and the published root, OpenAI-compatible, logger, and Agent Observability versions; the Bedrock module pins the published OpenAI adapter. Fresh-cache readonly module tests passed without Gateway imports in Apache production code.
- Provenance: no provider `recorded/` or `upstream/` input was added or changed. `file-inputs.json` is generated by the exact registered Gateway client, not a provider recording. Native request tests and the authenticated command use synthetic deterministic transports; they do not prove live-provider acceptance or deployed ingress behavior.
- Support boundary: generated media outputs (WP16), reasoning-file runtime (WP17), and file fallback remain deferred. The existing Bedrock tool-result URL warning/omission deviation remains as documented in `PARITY.md`; no new intentional deviation was accepted.

## Review follow-up

Three review rounds found that the earlier Anthropic prerequisite silently omitted inline-text files with non-text media types and URL/PDF tool-result files that the registered upstream converts. The corrected native module at `20642796` passed focused red/green tests and was published as an immutable proxy-resolvable prerequisite. Gateway now pins that revision; authenticated Vercel and Go unary/streaming calls assert native text, URL, and PDF sources through the standalone service. Gateway replay additionally asserts media types, and `PARITY.md` names stable file-input evidence. Fresh-cache readonly module checks, full tests, lint, vet, parity, and integration pass. No provider recording was added; live-provider and deployed-ingress acceptance remain unproven.
