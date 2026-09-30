## Context

At HEAD `ec204b9164e9c7bdc74cb0cc416d55c0dedaffe1`, `test/conformance/tools/common.mts:524-544` records only `URL.pathname`, while `test/conformance/runner.go:898-913` records only `r.URL.EscapedPath()`. `request_snapshot_test.go:36-52` sends `?stream=true` and expects `/v1/messages`. `CompareRequestSnapshots` (`runner.go:1482-1504`) already compares `Path` exactly; capture, not comparison, erases the signal. An inventory of the 139 committed `expected-requests.jsonl` files found no query-bearing `path` values.

This started as conformance-harness work; owner-approved native Anthropic target alignment is included to resolve the provider-boundary mismatch exposed by strict capture (Decision 5). The registered reference (`test/conformance/upstream.yaml`) is commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`, including `ai@7.0.109`, `@ai-sdk/openai@4.0.72`, `@ai-sdk/anthropic@4.0.59`, and `@ai-sdk/amazon-bedrock@5.0.90`. At that exact commit, `packages/azure/src/azure-openai-provider.ts:245-277` conditionally adds `api-version`; `azure-openai-provider.test.ts:155-176,275-284` asserts default `v1`, explicit `2025-04-01-preview`, and a query-bearing Responses URL. This supersedes the issue's older source citation and confirms the premise remains current. Azure is not a registered npm consumer: its commit-aligned source/tests are contextual evidence, not a claim of Azure package parity or support.

`test/conformance/PARITY.md` records provider/request evidence as mixed and baseline/inventory evidence as automated. Synthetic harness tests prove comparison sensitivity, not real provider acceptance. Existing provider inputs must remain untouched; generation does not establish recording/import provenance.

## Goals / Non-Goals

**Goals:** Retain behavior-affecting queries and their escaping/order identically across capture implementations; keep the JSON schema; provide a committed cross-language request snapshot and an executable mismatch witness; preserve queryless expectations; align the native Anthropic default Messages target to registered upstream behavior.

**Non-Goals:** New Azure/Vertex support, provider routing changes beyond removal of the native Anthropic SDK's implicit beta query, baseline upgrades, public module releases, new stream/UI fixtures, broad URL canonicalization, query sorting, or provider response recording. This change does not redefine the URL parsers' existing handling of non-wire inputs such as relative dot-segment paths or malformed URLs.

## Decisions

### 1. Extend `path` to contain the escaped request target

Keep `{method,path,headers,body}` and make `path` the escaped pathname plus `?` and a nonempty query, excluding scheme, authority and fragment. For already serialized HTTP request URLs, preserve query spelling and all pair ordering, including interleaved repeated keys. Do not parse query pairs or decode/re-encode them. Percent-escape spelling, `+` versus `%20`, key-only pairs and empty values remain observable. A terminal empty `?` is omitted; an empty pathname is `/`.

In TypeScript retain the existing `new URL(rawURL ?? "/", "http://localhost")` for pathname handling, but append the original nonempty query suffix: remove the fragment portion from the input first, locate `?` only in that pre-fragment target, and retain its suffix verbatim unless it is just `?`. Do not append `url.search`: WHATWG special-URL parsing rewrites a literal apostrophe in a valid HTTP query (for example `q=O'Reilly`) to `%27`, whereas Go `RawQuery` preserves it. Do not use `URLSearchParams` either. In Go use `EscapedPath()` (with `/` fallback), then append `"?" + RawQuery` only when `RawQuery` is nonempty; ignore `ForceQuery`. Keep fragment out. The domain is valid HTTP(S) serialized request URLs in origin or absolute form, not general URI parsing. These changes stay in the current shared capture seams, so recording and replay benefit together. Do not change `CompareRequestSnapshots` or its exact path comparison.

Examples:

| Input URL | Snapshot `path` |
| --- | --- |
| `/v1/messages` | `/v1/messages` |
| `/v1/messages?` | `/v1/messages` |
| `/v1/messages?api-version=2024-10-21&feature=a&feature=b` | unchanged |
| `/v1/a%2Fb?q=a%26b%3Dc&space=a+b&space=a%20b` | unchanged |
| `https://example.test?x=1#ignored` | `/?x=1` |
| `/v1/messages?q=O'Reilly` | unchanged |
| `/v1/messages#ignored?not=a-query` | `/v1/messages` |

Alternative: sort keys or reserialize pairs via `URLSearchParams`/Go `url.Values.Encode`. Rejected: encoding conventions differ, duplicate ordering can carry meaning, and canonicalization could erase the mismatch being tested. Raw preservation is deterministic for a given transmitted target, not semantic equivalence across differently spelled targets. A new `query` or `target` field would unnecessarily migrate every snapshot consumer.

### 2. Use shared synthetic harness cases, not invented provider fixtures

Add `test/conformance/testdata/request-snapshots/cases.json` with named `{name,url,expectedPath}` cases and `expected-requests.jsonl`. Every case uses fixed nonsecret `POST`, `content-type: application/json`, body `{}` and the existing `anthropic` normalization label. Cases include the issue's API-version/repeated-value example, interleaved duplicates, percent-encoded path/query delimiters and UTF-8, plus versus encoded space, literal apostrophes, empty/key-only values, no query, empty query marker, and absolute URL/root/fragment handling (including `?` appearing only inside a fragment). Cover apostrophe queries in both absolute and origin forms. TS-only fallback coverage additionally exercises absent `url`. These are local request-target tests, not Azure or Anthropic provider evidence.

Add a small explicit tool `test/conformance/tools/request-snapshots.mts` that reads these cases, calls exported `normalizeRequestSnapshot`, asserts each result against the independently declared `expectedPath`, and writes the committed JSONL through existing `writeRequestSnapshots` only with `--write`. Normal invocation computes into a temporary file and compares its bytes with the committed JSONL, failing on stale expectations without modifying repository files. Commands from the tools directory: `pnpm exec tsx request-snapshots.mts --write` to regenerate and `pnpm exec tsx request-snapshots.mts` to check. Resolve paths relative to the module, not caller cwd.

`common.test.mts` uses the shared cases to assert explicit target values and calls the check path so its existing `test:baseline` inclusion enforces golden freshness. Go's table-driven `request_snapshot_test.go` loads cases and `LoadExpectedRequests` from the same directory, constructs matching requests, runs `newRequestSnapshot`, asserts the independently declared targets, and passes the whole ordered list to `CompareRequestSnapshots`. For fragment-bearing URL-reference cases, a test helper uses `url.Parse` and assigns the parsed URL to `req.URL`; reserve direct `httptest.NewRequest` construction for fragment-free incoming HTTP request targets. Fragments are URL-reference components, not transmitted request-target components: `httptest.NewRequest` uses `http.ReadRequest` and `url.ParseRequestURI`, which assumes no fragment suffix. Retain the fragment-exclusion assertions without stripping literal raw server request data or changing the production `EscapedPath()` plus `RawQuery` algorithm. No Node process or regeneration occurs during Go tests. This checks actual TS serialization/Go loading without adding provider config fields, fake provider events, INDEX entries or credentials. Alternative: add an Azure provider replay just for query evidence; rejected as a much larger, unsupported surface.

### 3. Exercise the actual comparator's failure path

Because `CompareRequestSnapshots` accepts `*testing.T` and calls fatal assertions, keep the production API unchanged. Add a subprocess/helper mode within `TestRequestSnapshot_QueryMismatch`: use the test executable (`os.Executable`) with a narrowly selected `-test.run` and an environment guard to invoke the real comparator. The parent test expects a nonzero exit and `request 0 path mismatch`. Helpers load the committed TS expectation and capture an actual request whose only difference is (a) changed API version, (b) reversed repeated values, or (c) missing query. Keep method, headers and body identical. A matching control must pass. This proves mismatch rejection without failing the normal suite or merely asserting string inequality.

Alternative: refactor the comparator into a new pure helper solely for testability. Rejected for this small harness fix; the subprocess witnesses current production comparison without widening its API or duplicating equality logic.

### 4. Use a red/green sequence and existing gates

First add explicit failing target tests on both implementations. Then change TS capture and generate the harness expectation; confirm Go loading/capture against that golden fails before changing Go. Finally change Go capture, update the old header test's query expectation, and prove positive and negative comparator cases. Query-only failure diagnostics must retain the request index and `path mismatch` wording.

Run `mise deps` before implementation builds/tests. Focused checks: `cd test/conformance/tools && pnpm exec tsx --test common.test.mts`; `cd test/conformance && GOWORK=off GOFLAGS=-mod=readonly go test -tags conformance -run TestRequestSnapshot ./...`; explicit golden check; `mise run typecheck-conformance`; then `mise run parity-check` (includes baseline validation and conformance). No frontend integration scenario is needed: no frontend wire format is changed. Update the tooling README; update `PARITY.md` only if its stable evidence boundary needs to mention harness-only query sensitivity, never as an issue log or expanded provider-support claim.

### 5. Align the native Anthropic default target revealed by strict capture

The full parity check now exposes `/v1/messages?beta=true` from every native Go Anthropic replay, while registered TypeScript expectations contain `/v1/messages`. At the exact registered commit, `packages/anthropic/src/anthropic-language-model.ts:989-993` builds `${baseURL}/messages`; unary `:1076` and streaming `:1664` use that URL, with beta features represented in headers. The Go provider calls `Beta.Messages.New` and `NewStreaming` in `providers/anthropic/model.go`; `anthropic-sdk-go@v1.57.0/betamessage.go:72,109` adds `beta=true` implicitly. This is a previously hidden provider-boundary mismatch, not a reason to relax snapshot comparison. The owner approved the narrow provider alignment during implementation.

Initialize the direct `New` model's request options with verified SDK `option.WithQueryDel("beta")` before applying caller `WithRequestOptions`. Both unary and streaming paths already consume this list, so no transport wrapper or duplicated request-path rewriting is needed. Preserve beta headers, conversion-generated body options and explicit caller query/header options; callers may deliberately override the default using the existing raw SDK options. Do not change `NewVertex` or its auth/routing path. Focused HTTP tests cover both operations, no beta and feature-beta headers, unrelated ordered repeated caller query values, and explicit caller beta overrides. Existing authentic Anthropic replay inputs and TypeScript request expectations stay untouched; the now-query-sensitive replay is the red/green provider-boundary proof. The Gateway command integration's fake native server and explicit request-target assertion must also require `/v1/messages`, rather than the SDK's old implicit query, while keeping all auth and body checks intact. Validate that consumer boundary through `mise run test-integration`.

Run the Anthropic module's full tests and vet, then the focused harness suites, typecheck and full `mise run parity-check`. This is a private provider default correction with no new API or dependency; candidate-source CI exercises it directly. Publishing an updated Anthropic module remains a later release activity, not a prerequisite for this self-contained source change.

## Risks / Trade-offs

- [Exact query order/spelling can expose semantically harmless differences] → Preserve the conservative signal; investigate concrete failures instead of introducing silent sorting or decoding.
- [Queries can contain credentials or signed, volatile values] → Use only controlled nonsecret harness targets; inventory/review generated request targets before committing. Existing header redaction remains unchanged. Future query-authenticated recording requires an explicit provider-specific secret/volatility policy before committing its snapshots; generic filtering is not part of this change.
- [Golden and capture could share the same bug] → Assert explicit `expectedPath` values on both sides, require query-bearing JSONL, and run version/duplicate-order/missing-query comparator witnesses.
- [Synthetic request evidence mistaken for provider acceptance] → Keep testdata outside `recorded/` and `upstream/`, label it in the README, and make no claims about live Azure behavior or newly supported providers.
- [URL parser differences for non-wire input] → Scope raw preservation to serialized HTTP request URLs; retain existing parser semantics rather than introducing a general URL normalization library. Exercise escaped transport inputs and root fallback explicitly.

## Migration Plan

Land capture changes, focused tests and generated harness expectations together. Re-inventory provider request paths after TS generation when warranted; review every changed expectation. Existing queryless paths should be byte-identical. Any existing query-bearing case must regenerate `expected-requests.jsonl` with registered tooling; no backwards-compatible path-only matching is allowed, because it would restore the blind spot. Do not alter provider `input*.chunks.txt`, `input.response.json`, stream/object expectations, fixture provenance or pins for this task. Land the native Anthropic default-target correction with the stricter capture so the change remains independently green; do not alter its upstream expectations to accommodate the implicit SDK query. Roll back the capture, provider default correction and harness expectations as one unit if needed; no public module publication or production deployment ordering is involved.

## Open Questions

None blocking the proposal. Raw query order, empty-marker handling, synthetic evidence location and comparator witness strategy are specified above; implementation must recheck the baseline/inventory at its then-current revision.
