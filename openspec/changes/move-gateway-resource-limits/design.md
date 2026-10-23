# Design

## Context

See proposal.md for motivation. This change builds on #479, which removes the Go client limits, and on #428 (`nrbrd/app-platform`), where the Gateway runs as an App Platform plugin.

Limits on the request and response path today:

| Limit | Where | Value |
| --- | --- | --- |
| Front door, route RPC message size, 60s route deadline | plugin host | platform-owned, 16 MiB receive |
| `max_request_bytes` setting | ProviderWire `readBody` (`LimitReader`) | 1 MiB default |
| unary response bytes | `unarySuccessPreflight`, final encode, error documents, summary `sourceBytes` | 8 MiB, fixed |
| stream frame bytes | `streamEventPreflight`, warning count, source and reasoning-file sizing, tool ID and result checks, fixed-frame construction check | 1 MiB, fixed |
| stream parts | per-request counter in consume and drain, and the tool map size | 100,000, fixed |
| `maxRawUsageBytes`, `maxErrorResponseBytes` | ProviderWire constants | 1 MiB, 64 KiB |
| `ProviderResponseBytes`, `ProviderResponseHeaderTimeout` | `outbound.NewAnthropicClient` (every provider, BYOK) | 16 MiB, 10s |
| `max_model_duration`; idle and drain durations | ProviderWire runtime | 120s default; 30s; 1s |
| BYOK `maxCredentials` | `byok.DecodeRequest` (every array) and `byok.New` | 8 |

Relevant code facts:
- `gateway.newHandler` builds one `ServeMux` for the `config` and `language-model` routes. Access middleware wraps each route, and `telemetry.Middleware` wraps the mux. Access refusal writes the permission document without reading the body.
- The testserver (`test/providerwire-v4/testserver`) is in the same Go module, but it cannot import `cmd/grafana-ai-gateway/internal/...`. It serves three ProviderWire handlers on its own mux.
- Many preflight helpers also run validity checks: metadata key and value UTF-8, the `{` prefix, warning UTF-8 and a non-empty approval ID. `encoding/json` does not reject invalid UTF-8 inside a `RawMessage`, so these checks have to stay explicit.
- `execution.Metadata` already relocates a native `gateway` value. The callers drop it only when the enriched output does not fit.
- The Anthropic and OpenAI SDKs do not set `Accept-Encoding`, so `http.Transport` decompresses transparently, and `boundedBody` counts decompressed bytes.

## Goals / Non-Goals

**Goals:**
- Each retained limit has one owner: the route wrapper, the outbound transport, the execution durations or the host.
- ProviderWire keeps every correctness check and does no size accounting.
- Output is encoded once, without truncation or fit-based omission.

**Non-Goals:**
- Moving the execution runtime out of ProviderWire (#481), or the route table and host composition (#482).
- New settings, or renaming `max_request_bytes` and `max_model_duration`.
- Telemetry sink caps, configuration-file and discovery-catalog ceilings, and the MCP option caps.
- A Gateway concurrency or aggregate-memory bound (#104).

## Decisions

### D1. Route request-body bound wraps the mux once
`gateway.newHandler` wraps the mux in `http.MaxBytesHandler(mux, settings.MaxRequestBytes)`, inside the telemetry middleware. `MaxBytesHandler` only replaces `r.Body` with a bounded reader, so:
- access refusal still reads nothing;
- every route registered on the mux, now or later, inherits the bound.

The setting moves from `Settings.ProviderWire.RequestBytes` to a route-level `Settings.MaxRequestBytes`, which must be positive. In ProviderWire, `readBody` becomes `io.ReadAll` plus `Close`. A `*http.MaxBytesError` maps to the fixed invalid request (400), and any other read or close error maps to internal, as today.

The testserver wraps its mux in `main.go` with `http.MaxBytesHandler` and a local 1 MiB constant. Its handler tests do the same.

The platform has already buffered the body, so the check does not bound receive memory. It does bound UTF-8 scanning, schema validation and mapping work for every adapter, below the 16 MiB route message size.

Alternatives:
- *Rely on the 16 MiB host message size.* Rejected: up to 16 MiB of JSON would reach schema validation, and a handler served elsewhere would have no bound.
- *Keep the bound in each adapter.* Rejected: every new adapter would duplicate it.
- *A shared package for the testserver constant.* Rejected (YAGNI): it would be one constant around a stdlib one-liner.

### D2. Outbound transport keeps fixed per-response bounds
`outbound.NewAnthropicClient` becomes `outbound.NewModelClient`, with the same signature. `config.ProviderResponseBytes` (16 MiB) and `config.ProviderResponseHeaderTimeout` (10s) stay fixed constants. New tests pin two behaviors:
- a gzip response whose decompressed size exceeds the bound fails, even though its compressed size is within it;
- two sequential responses through one client, each near the bound, both succeed.

*Alternative: an aggregate byte budget per logical call, across attempts.* Rejected: attempts are already bounded by candidate count and the shared deadline, and the budget would need context plumbing through `fallback`.

### D3. BYOK bound checks only the selected provider
`byok.DecodeRequest` still validates every supplied array: it must be non-empty and each account must be well-formed. It applies `len > maxCredentials` only to `accountsByProvider[selected]`. The error stays `ErrInvalidRequest`, which the selector maps to `ErrInvalidBYOK` and the fixed BYOK credentials 400. `byok.New` keeps its own check as a constructor invariant.

Alternatives:
- *Drop the check from `DecodeRequest` and map the `byok.New` error* (as in the #473 draft). Rejected: the selected provider is already known during decoding, and keeping the check there keeps the error mapping unchanged.
- *Truncate to eight accounts.* Rejected: it would silently ignore caller credentials.

### D4. ProviderWire keeps only duration limits
`v4.Limits` keeps only `ModelDuration`, `StreamIdleDuration` and `StreamDrainDuration`. `validateLimits` checks only that they are positive, and the canonical-frame construction check goes away. This change removes:
- `RequestBytes`, `UnaryResponseBytes`, `StreamFrameBytes` and `StreamParts`;
- `unarySuccessPreflight`, `streamEventPreflight`, `warningsPreflight`, `streamWarningCountFits`, `mapStreamWarnings`'s budget, `sourcePreflight`, `reasoningFileFits`, `responseIdentityFits`, `responseStringsFit`, `minimumTextPartBytes` and `minimumWarningBytes`;
- the `limit` parameters of `mapSource`, `mapUnarySuccess`, `encodeStreamFrame` and `validRawUsage`;
- the tool ID length check and the `len(state.tools) >= StreamParts` check in `stream_tools.go`;
- `maxRawUsageBytes` and `maxErrorResponseBytes`;
- `streamPartCounter` and `streamWaitPartLimit`.

`drainStream` loops until the channel closes or the drain timer fires. `newStreamState` uses a small constant initial map capacity.

`config.Settings` keeps `ProviderWire providerv4.Limits` for the durations. `Validate` still constructs a throwaway handler to check them.

Alternatives:
- *Keep one final unary or frame cap.* Rejected: it cannot prevent the allocation it measures, it can only fail a committed stream, and the outbound bound already covers it.
- *Keep `StreamParts`.* Rejected: part volume comes from upstream bytes, which are bounded, and time is bounded by the total, idle and platform deadlines. Drain needs no count once it has a deadline.

### D5. Validity checks replace preflights in place
Small helpers replace the size-and-validity preflights, and each mapping path calls them where the preflight ran:
- `validMetadata`: every key is valid UTF-8, and every value is valid UTF-8 starting with `{` after trimming;
- warnings: a registered type with UTF-8 active fields (today's `mapWarnings`);
- raw usage: `utf8.Valid`, `json.Valid` and an object;
- the existing checks for custom kinds, reasoning files, response identity and timestamps, token ranges, tool and text lifecycles, and non-empty approval IDs.

Behavior changes only where size was the reason for a failure. The existing invalid-value tests are the regression contract.

### D6. Encode once
- **Unary success:** validate and map. When an overview exists, set `mapped.Metadata = execution.Metadata(overview, mapped.Metadata)`, then `json.Marshal` once. If that fails, the result is the fixed internal error carrying `view.metadata()`.
- **Finish:** the same, through a single `encodeStreamFrame`. A failure makes the stream attempt its terminal internal error.
- **HTTP errors** (`encodeHTTPError`): marshal the response with its metadata. Only if that fails, marshal the base document, which always encodes.
- **SSE errors** (`encodeStreamError`): build `error.data` with both `providerMetadata` and `nativeError`, and marshal it. Only if that fails, return the fixed base frame.
- `execution.Metadata` returns nil only when there is no overview or it cannot be encoded. Its godoc drops the fit wording.

For success and finish, a failed enriched encode is not retried without the metadata. The overview is Gateway-built and already proven encodable by `Metadata`, so any failure comes from primary native data, and the unenriched document would fail too. This keeps one encode per output and guarantees that a failed primary never becomes a success.

*Alternative: keep the original-then-enriched two-pass approach.* Rejected: the second pass only existed to measure fit.

### D7. Execution projection loses `sourceBytes`
`execution.Summary(err)` and `execution.Project(requested, canonical, decisions, candidates)` drop their size parameter. `boundedText` and the oversized-body branches go too. `executionRequest` and `executionView` drop `sourceBytes`, and `current()` no longer has a `sourceBytes == 0` disable branch. Native body size is bounded by the outbound transport.

### D8. Limit ledger in `ai-gateway/README.md`
The existing "Limits" section becomes the ledger:

| Retained limit | Owner |
| --- | --- |
| 60s route deadline, route message size, admission and concurrency | plugin host |
| `max_request_bytes` (1 MiB default) | route wrapper |
| Provider response bytes (16 MiB, decompressed, per response), header timeout (10s), redirect rejection | outbound model transport |
| `max_model_duration`, stream idle (30s) and drain (1s) durations | language-model handler |
| BYOK accounts (8, selected provider) | BYOK selection |
| Model file size (1 MiB), catalog entries, aliases and fallbacks | model catalog loader |

The ledger lists the catalog limits without changing them, because operators meet them when writing the catalog. Telemetry sink caps stay out, because they bound asynchronous export rather than a request or the catalog.

It then names each removed limit (generated response size, stream event size, stream event count, raw usage, error response and error summary sizes) and states that the outbound bound and the durations replace them. It also states the trade-offs (see Risks).

Every task group that changes a limit updates its ledger row in the same commit.

### D9. Documentation owners
- **`providerwire/v4/doc.go`:** hosts own the request-body bound, upstream response bounds and admission. The handler owns only durations.
- **`docs/providers/grafana-gateway.md`:**
  - the raw-events paragraph no longer mentions part budgets or frame limits;
  - generated files no longer "count toward the response size limits";
  - overviews are not missing because of size;
  - "Bound work and handle errors" no longer says the Gateway caps stream frames.
- **`docs/concepts/gateway-architecture.md`:** the handler-bounds paragraph and the "bounded complete SSE frame" and "oversized frame" wording are replaced by the ownership split, linking the README ledger.
- **`docs/concepts/gateway-glossary.md`:** changes only where it describes handler byte limits.

### D10. Removed-identifier check
The acceptance `rg` matches whole words over `ai-gateway/` and `docs/`, excluding `CHANGELOG.md`, so identifiers that only contain a name, such as the logger middleware's public `LogStreamParts` option, do not match; archived OpenSpec changes sit outside both. `ai-gateway/test/providerwire-v4/developer-evidence-bounds.test.ts` declares an unrelated local `sourceBytes` constant for the #321 design witness, which models a proposed developer-evidence budget and does not exercise the Gateway. The constant is renamed to `witnessSourceBytes`. Its `reason: "sourceBytes"` string stays, because the archived #321 decision lists `sourceBytes` among its closed reason identifiers; it is the one historical reference the check still finds. The `gateway-developer-evidence-decision` spec requires those witnesses, so the file is not deleted. The root module and other providers have unrelated identifiers named `StreamParts` or `Preflight`, which are out of scope for the check.

- *Alternative: exclude the file from the check.* Rejected: a carve-out in the acceptance command is harder to review than a one-line rename.

### D11. Parity
Gateway resource policy is a Go extension: `@ai-sdk/gateway@4.0.103` only defines the client, which has no server budgets. No `knownGaps`, conformance input or expectation changes. `mise run validate-parity-baseline` confirms the baseline is untouched.

## Risks / Trade-offs

- **Per-request memory grows with the outbound bound, encoding expansion and concurrency.** Standard `json.Marshal` HTML-escapes `<`, `>` and `&` (up to 6×), base64 adds a third, and the plugin route recorder holds a unary body about twice more. go-plugin route messages have no gRPC send cap, so no host limit fails them first.
  - Mitigation: documented in the ledger. Admission belongs to the host (#104), and the outbound bound is one constant to tune.
- **ProviderWire library users without the shared transport or route wrapper lose adapter-level bounds.**
  - Mitigation: the package godoc and the ledger state that hosts own these bounds. There is no outside-module consumer.
- **An in-process provider that floods parts without I/O.**
  - Mitigation: the total, idle and platform deadlines bound processing, and drain is time-bounded. No Gateway-constructed provider behaves this way.
- **Large-output tests are slow under `-race`.**
  - Mitigation: use a few large values for byte cases and tiny parts for the 100,001-part case. Keep command-test outputs just above the former limits and well below 16 MiB.
- **Breaking Go API** (`v4.Limits`, `execution.Summary` and `execution.Project`).
  - Mitigation: the PR title carries `!`. Only this module uses them.

## Migration Plan

1. Operators change nothing: `max_request_bytes` and `max_model_duration` keep their names, defaults and meaning.
2. Go code that builds `v4.Limits` drops the removed byte and part fields. Callers of `execution.Summary` and `execution.Project` drop the size argument.
3. Rollback is a revert of this PR. There is no persisted state.
