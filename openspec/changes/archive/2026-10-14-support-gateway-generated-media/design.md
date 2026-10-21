# Design

## Context

See proposal.md for motivation. Planning starts at canonical `47055dee685ae30cfd656d61912e51361d3a07f2`; `test/conformance/upstream.yaml` registers commit `eb77f09e3c06c28e860d92e0de941b143c2eecec`, ai 7.0.127, Gateway 4.0.103 and provider 4.0.21. Sources were read with `git show <registered-commit>:<path>` from an upstream checkout whose working HEAD differs and was deliberately not used as authority:

- `packages/provider/src/language-model/v4/language-model-v4-file.ts`: only type, mediaType, data and providerMetadata; data/URL union, no filename.
- `packages/provider/src/shared/v4/shared-v4-file-data.ts`: data is bytes/string; URL has optional `originalUrl` for opaque URI fidelity.
- `packages/gateway/src/gateway-language-model.ts`: unary and SSE successful payloads use permissive `z.any()`; generated payload is not schema-normalized. JSON transports byte data as base64 and URLs as strings. This proves public client behavior, not private service behavior.

Existing `reasoning.go` contains bounded data/URL projection that can be shared, but ordinary `ContentFile`/`PartFile` is rejected by response and stream switches and Grafana client decoders. `DataContent` and `StreamFileData` represent bytes/base64/URL but currently discard `originalUrl`. Existing provider metadata transport is opaque and bounded. PARITY.md classifies Gateway runtime and provider metadata as mixed evidence; file input and reasoning transport are delivered. Existing root generated-file downloader and frontend scenarios establish downstream file mechanics, not Gateway generated output.

## Goals / Non-Goals

**Goals:** Complete one generated LanguageModel file capability through unary and streaming clients, including malformed/oversized handling and useful opaque values.

**Non-Goals:** New model families, media synthesis engines, Gateway downloads/URL rewriting, MIME or base64-content censorship, native provider expansion, privacy policy overhaul, diagnostic carriers, raw events, fallback algorithm replacement, and unpublished-module support claims.

## Decisions

1. **Explicit strict output DTOs.** Add `generated_file.go` under `ai-gateway/providerwire/v4` or generalize the already concrete reasoning-file helper without adding a generic serializer. Unary ContentFile and SSE PartFile share the registered data/URL projection. Only registered fields are emitted. Provider-local Filename is ignored, including its length and invalid UTF-8, because it is not output evidence. Media type/base64/URL/metadata remain unchanged; bytes become standard base64 only at the bounded encoder. Reject missing data, mixed arms, unsupported reference/text, malformed relevant fields and invalid opaque metadata through existing safe adapter failure paths.

2. **URL fidelity prerequisite.** Add `OriginalURL *string` to `provider.DataContent` and `provider.StreamFileData`: the exact registered `SharedV4FileDataUrl` definition requires this optional representation. Nil means omitted; a pointer to an empty string means present and empty. Preserve omission versus explicit empty and raw URI bytes; it belongs only to the URL arm. Include it in provider JSON, strict Gateway DTO/schema and Grafana decode, and all relevant unary/stream conversions. Do not reinterpret originalUrl as authority or fetch it. Avoid a general input-file overhaul; only update shared conversions necessary to avoid losing the new field. Specifically preserve it in `middleware/simulate_streaming.go` (GenerateContentPart to StreamPart), `unaryReasoningFile` or its shared replacement, and Grafana unary reconstruction from decoded stream-file data. Shared reasoning-file helpers must preserve it consistently when reused. Reject an in-memory OriginalURL on a non-URL arm rather than silently emitting it. Do not add OriginalURL to the high-level GeneratedFile API: pinned `resolve-generated-file-data.ts` downloads `data.url`, and high-level file values contain resolved bytes/base64 rather than the original URL union. This is Apache functionality and must precede dependent Gateway code. The registered baseline is not upgraded.

3. **Bounds before amplification and writes.** Extend unary preflight and stream preflight to account for all emitted media fields and metadata using overflow-safe subtraction and byte-to-base64 expansion checks before allocation. Check actual escaped encoded JSON/frame bytes before any successful body/frame write. The preflight may be a conservative raw-size/allocation guard followed by exact encoded-byte validation; it need not implement another full JSON size calculator. Tests must target the encoded limit (not merely raw string length). For the new ContentFile branch, budget each emitted field once and bypass the existing generic scalar loop: that loop counts Filename and repeats MediaType after file-specific accounting. Ignore unregistered Filename (and other omitted generic fields) rather than letting an omitted value consume the response budget. Keep any reasoning preflight adjustment limited to a shared-helper regression demonstrated by the new code. Oversize unary fails before 200; oversize file frames produce only the existing bounded terminal error, never a partial media frame or success finish. Retain cancellation and drain/close ownership.

4. **Existing lifecycle and selection.** Route files through the existing stream emission state and fallback commitment semantics; file output counts as selected output, is valid without opening a text ID, and preserves relative position. Do not retry a later candidate after emitted file or selected encoding failure. Test preselection eligible failure separately from selected-output failure. No Gateway text-only restriction is added.

5. **Independent Apache decode and capture.** Extend `providers/grafana/model.go`, `stream.go` and a small file decoder (reusing reasoning data decoding where appropriate); never import AGPL DTOs. Preserve file metadata and selected empty data. Keep operator capture and consumer wrapper capture independently configured; no response scrubbing based on telemetry flags. Use ordinary secret-looking application values as fidelity controls, without inventing credential classification.

6. **Evidence and ownership.** Add synthetic responses to the existing AGPL HTTP testserver and TypeScript client differential/runtime tests. Assert both low-level client results, raw output against production schemas and high-level generated-file availability; High-level URL tests use a deterministic data: URL; localhost HTTP downloads are intentionally blocked by the existing generated-file downloader and must not be enabled for this feature. Extend the existing `test/integration/generated-file.test.ts` with one scenario using the Apache Grafana provider and a local fake Gateway response, parsed with `parseJsonEventStream`/`uiMessageChunkSchema`, to prove the newly connected frontend path. Reuse its existing assembly assertions; do not duplicate the existing direct-model scenario or create a parallel suite. Native provider output wiring needs a focused existing producer case if available; no invented recording goes under recorded/upstream fixture directories. Opaque metadata parity is observed at provider boundaries; high-level SDK transformations must be separately described.

### File ownership

- Apache prerequisite: `provider/content.go`, `provider/stream_part.go`, corresponding domain codec tests, and `middleware/simulate_streaming.go` plus its focused URL-conversion regression test.
- Apache client: `providers/grafana/model.go`, `stream.go`, `reasoning.go` or `generated_file.go`, focused generated-file/metadata/capture tests.
- AGPL service: `ai-gateway/providerwire/v4/{response.go,stream.go,reasoning.go,generated_file.go}`, `schema/{unary_success.json,stream_event.json}`, focused mapping/preflight/lifecycle/fallback tests.
- AGPL proof: `ai-gateway/test/providerwire-v4/{testserver/*,runtime-integration.test.ts,go-client-differential.test.ts,client-consumption.test.ts,schema*.ts}` and existing capture helpers; add files only when existing suites become clearer.
- Cross-language frontend: `test/integration/testserver/scenario_generated_file.go` or adjacent focused scenario and `test/integration/generated-file.test.ts`.
- Contract/docs: this OpenSpec change, `docs/providers/grafana-gateway.md`, `test/conformance/PARITY.md` only for stable evidence/support changes.

## Risks / Trade-offs

- [Apache OriginalURL prerequisite needs new published refs] → local overlays can prove candidate source only. Never commit replace directives, lie about published adoption or push without authorization. Record any immutable-ref/pin requirement as a concrete delivery blocker.
- [Permissive TypeScript parsing masks invalid service output] → independently validate raw JSON/SSE with strict production schemas.
- [Existing reasoning helper has narrower assumptions] → retain reasoning regression tests and avoid widening its accepted arms accidentally.
- [Large media allocates before rejection] → boundary and escaping tests assert preflight expansion and atomic writes; do not introduce arbitrary separate media ceilings.
- [URL high-level processing can perform network I/O] → provider transport stays opaque; frontend tests use deterministic data: URLs and keep the established download policy unchanged.

## Migration Plan

Add Apache prerequisite first, then independent client, then AGPL transport/tests/docs. Run candidate-source and committed-pin checks separately with GOWORK=off where required by CONTRIBUTING. Keep the feature local pending authorized publication; rollback removes the file support mapping without changing input-file contracts. No deployed state migration is involved.
