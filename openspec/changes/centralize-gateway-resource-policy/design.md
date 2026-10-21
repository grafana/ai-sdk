# Design

## Context

See proposal.md for motivation. This change builds on #428 (`nrbrd/app-platform`), where the Gateway is an App Platform plugin:
- `internal/gateway/routes.go` owns a `ServeMux` for the `config` and `language-model` routes, behind the access middleware. It sends configured and BYOK callers to separate ProviderWire handlers.
- The host delivers each request body as one route RPC message, limited by the plugin SDK's default 16 MiB gRPC receive size. The platform also ends routes after 60s and applies front-door limits.
- There is no `http.Server`, listener, JWKS client or container image any more. Aggregate admission belongs to the plugin host; #100 is closed.

Limits on the request/response path today:

| Limit | Where | Default |
| --- | --- | --- |
| Front door, route RPC message size, 60s route deadline | plugin host | platform-owned, 16 MiB receive |
| `max_request_bytes` setting | ProviderWire `readBody` (`LimitReader`) | 1 MiB |
| unary response bytes (constant) | `unarySuccessPreflight`, final encode, error docs (`min(…, 64 KiB)`), summary `sourceBytes` | 8 MiB |
| stream frame bytes (constant) | `streamEventPreflight`, warning count, source/reasoning/custom sizing, fixed-frame construction check | 1 MiB |
| stream parts (constant) | per-request part counter in consume and drain | 100,000 |
| `maxRawUsageBytes`, `maxErrorResponseBytes` | ProviderWire constants | 1 MiB, 64 KiB |
| `ProviderResponseBytes`, `ProviderResponseHeaderTimeout` (constants) | shared outbound `boundedTransport` (all providers, BYOK) | 16 MiB, 10s |
| `max_model_duration` setting; idle and drain durations (constants) | ProviderWire runtime | 120s/30s/1s |
| BYOK `maxCredentials` | `byok.DecodeRequest` (all arrays) and `byok.New` | 8 |
| Go client `Limits` | `providers/grafana` | discovery 4 MiB, unary 16 MiB, error 64 KiB, stream 64 MiB, event 1 MiB, 100k events, raw 1 MiB |

Settings are `GF_PLUGIN_*` keys from Grafana's plugin section; only `max_request_bytes` and `max_model_duration` are configurable limits. Every Gateway-constructed model uses the shared outbound client from `gateway.go`. The Anthropic and OpenAI SDKs do not set `Accept-Encoding`, so `http.Transport` decompresses transparently, and `boundedBody` already counts decompressed bytes.

## Goals / Non-Goals

**Goals:**
- Shared bounds that every adapter inherits: the route request-body bound and the outbound response bound. Execution deadlines stay; host-owned bounds are documented.
- Adapters keep correctness validation and encode once. They hold no size accounting.
- The server and the Go client accept the same outputs by default.

**Non-Goals:**
- Host admission, capacity or plugin gRPC settings.
- Request structural validation, which #395 owns. The MCP option caps in `nativeoptions` stay unchanged.
- Decoder shape simplification in `providers/grafana`, which #375 owns. This change only touches limit plumbing there.
- JSON helpers (#396), raw output (#116), telemetry sink caps, configuration-file and discovery-catalog limits, and the `fallback` package buffer limits.
- New or renamed settings. The existing `max_request_bytes` and `max_model_duration` keep their names; fixed constants stay fixed.

## Decisions

### D1. Route request-body bound on the mux
`gateway.newHandler` wraps both route patterns with `http.MaxBytesHandler`, using the existing `max_request_bytes` setting (1 MiB default). It moves from `Settings.ProviderWire.RequestBytes` to a route-level settings field.
- **Order:** the wrapper sits inside the access middleware, so refused callers never have their bodies read.
- **ProviderWire:** `readBody` becomes `io.ReadAll` plus close. A `*http.MaxBytesError` maps to the fixed invalid request (400), keeping today's classification; any other read error maps to internal.
- **Test server:** the ProviderWire testserver wraps its handler the same way.
- **Godoc:** the package godoc states that hosts own the body bound.

The check is cheap because the platform has already buffered the body, so it doesn't bound memory. It still matters: it bounds UTF-8 scanning, schema validation and mapping work for every adapter, below the 16 MiB route message size.

- *Alternative:* no Gateway-side limit, relying on the 16 MiB host message size. Rejected: that would let JSON of up to 16 MiB reach schema validation and mapping, and a future adapter served elsewhere would have no bound at all.
- *Alternative:* keep `RequestBytes` in each adapter. Rejected, because every new adapter would duplicate it.
- *Alternative:* raise or lower the plugin gRPC receive size. Rejected, because that belongs to the host.

### D2. Outbound transport stays per-response with fixed bounds
`outbound.NewAnthropicClient` becomes `outbound.NewModelClient`, since it serves every provider type. `config.ProviderResponseBytes` (16 MiB, below the Anthropic SDK's 32 MiB scanner limit) and `config.ProviderResponseHeaderTimeout` (10s) stay fixed constants. A gzip test pins the decompressed-bytes behavior.

- *Alternative:* an aggregate byte budget per logical call, across attempts. Rejected: attempts are already bounded by candidate count (16 configured, 8 BYOK) and the shared deadline, and it would need context plumbing through the fallback package.

### D3. Remove adapter output budgets and the part counter
`Limits` keeps only `ModelDuration`, `StreamIdleDuration` and `StreamDrainDuration`. This change removes:
- `RequestBytes`, `UnaryResponseBytes`, `StreamParts`, `StreamFrameBytes` and their settings;
- the canonical-frame construction check;
- `unarySuccessPreflight`, `streamEventPreflight`, `warningsPreflight`, `streamWarningCountFits`, `sourcePreflight`, `reasoningFileFits`, `responseIdentityFits`/`responseStringsFit`, the custom-part sizing and `minimum*Bytes`;
- `maxRawUsageBytes` and `maxErrorResponseBytes`;
- the `streamPartCounter`.

The drain loop is bounded by `StreamDrainDuration` alone, and the initial stream-state map capacity becomes a constant.

- *Alternative:* keep one final unary/frame cap. Rejected: it cannot prevent the allocation it measures, it can only fail a committed stream, and it duplicates the upstream bound.
- *Alternative:* keep `StreamParts`. Rejected: part volume derives from upstream bytes, which are bounded, and time is bounded by the total and idle deadlines (and the 60s platform deadline). Drain does not need a count once it has a deadline.

### D4. Validation is separated from sizing
The preflight functions are replaced by small validity helpers, called in place on every mapping path:
- UTF-8 strings;
- metadata namespaces: a valid UTF-8 name and a valid object (`{`-prefixed and `utf8.Valid`);
- raw usage: one valid UTF-8 JSON object;
- custom kinds;
- timestamps, token ranges, discriminators, and tool/text/reasoning lifecycle.

These checks must stay explicit because `encoding/json` passes invalid UTF-8 inside `RawMessage` through unchanged. Behavior differs only where size used to be the reason for failure.

### D5. Encode once; optional data falls back only on encoding failure
- **Unary:** validate and map, attach `execution.Metadata` when an overview exists, then `json.Marshal` once. If marshalling fails, return the fixed internal error.
- **Finish:** the same, using one `encodeStreamFrame`.
- **HTTP and SSE errors:** build the base response plus optional metadata and `nativeError`, then marshal once. Only if that fails, emit the base document, which always encodes.

`execution.Metadata` always relocates a native `gateway` value. It returns nil only when the overview is absent or cannot be encoded.

- *Alternative:* keep the two-pass original-then-enriched approach. Rejected: the issue removes fit-based omission, and the second pass only existed to measure fit.

### D6. Execution projection drops `sourceBytes`
`execution.Project` and `execution.Summary` lose their `sourceBytes` parameter, along with `boundedText` and the oversized-body branches. Native bodies are bounded by the outbound transport. `executionRequest`/`executionView` drop `sourceBytes`, and the `sourceBytes == 0` disable branch goes too, so summaries are always eligible.

### D7. BYOK account bound is execution policy
`byok.DecodeRequest` keeps rejecting empty arrays and drops the `> maxCredentials` check. `byok.New` keeps the check, which only sees selected-provider accounts. `service.NewBYOKSelector` maps a `byok.ErrInvalidRequest` from `New` to `providerv4.ErrInvalidBYOK`, so the response remains the fixed BYOK credentials invalid-request error.

- *Alternative:* truncate to the first eight. Rejected, because it silently ignores caller credentials.

### D8. Go client limits: `DiscoveryBytes` + `ResponseBytes`
`Limits{DiscoveryBytes int64; ResponseBytes int64}`, with defaults 4 MiB and 64 MiB:
- discovery success and error bodies use `DiscoveryBytes`, which keeps parity with the TS helper;
- model-route unary success, non-2xx error bodies and the cumulative SSE stream use `ResponseBytes`.

`eventReader` drops `eventBytes`, `count` and their checks, and keeps the cumulative total. `wireUsage` drops `maxRawUsageBytes`. Pre-decode UTF-8/JSON validation stays. The 64 MiB default is four times the default outbound bound, which covers JSON re-encoding expansion in normal output.

- *Alternative:* no client limits, matching Vercel's fetch. Rejected: Go `io.ReadAll` on an untrusted endpoint needs a memory bound.
- *Alternative:* keep the per-event and count limits. Rejected: they reject server-accepted output and duplicate the cumulative bound.

These limit changes touch the same functions that #375 will simplify, so the two need reconciling when they meet.

### D9. Limit ledger in `ai-gateway/README.md`
The "Limits" section gets the ledger, next to the existing platform notes and the `max_request_bytes`/`max_model_duration` settings rows. Each group of work in tasks.md updates its own ledger rows:

| Retained limit | Owner |
| --- | --- |
| front door, route RPC message size (16 MiB receive), 60s route deadline, termination grace | plugin host |
| `max_request_bytes` | route handler |
| provider response bytes (16 MiB) and header timeout (10s), no-redirect, connection pool | outbound transport |
| `max_model_duration`, stream idle (30s) and drain (1s) durations | execution runtime |
| model file size (1 MiB), YAML string cap | configuration loader |
| catalog route ceilings (1,024 IDs, 16 candidates, 128 aliases, 2,048-byte strings) | discovery configuration |
| BYOK eight accounts (selected provider) | execution attempt policy |
| `agento11y.*` queue/batch/payload, physical-record and diagnostic caps | telemetry sinks |
| MCP option caps | request validation (#395) |
| Go client `DiscoveryBytes`, `ResponseBytes` | independent client |

### D10. One PR with ordered commits
Everything lands in one PR on #428, so the server and client change together. Commits follow the task groups in tasks.md, and each leaves `mise run test-ai-gateway` and `mise run test-grafana` passing:
1. route bound;
2. outbound;
3. BYOK;
4. client;
5. execution and unary;
6. streaming;
7. coverage and docs.

The client commit comes before the server stops limiting output, so the intermediate states stay coherent.

## Risks / Trade-offs

- **Per-request memory grows with the outbound bound, encoding expansion and concurrency.** Standard `json.Marshal` HTML-escapes `<>&`, up to 6×. → Documented. Admission belongs to the host, and the outbound bound is a single shared constant to tune if needed.
- **ProviderWire library users with providers outside the shared transport lose adapter-level bounds.** → Package godoc and the ledger state that hosts own request, upstream and deadline bounds.
- **An in-process provider that floods parts without I/O.** → The total and idle deadlines and the platform route deadline still bound processing, and drain is time-bounded. No Gateway-constructed provider behaves this way.
- **Pathological escaping can exceed the client's 64 MiB default.** → Documented and configurable. The TS client has no limit.
- **Breaking API.** ProviderWire `Limits` and `providers/grafana.Limits` change; no plugin setting changes. → The PR title carries `!`; release automation is still inactive (#245).
- **Large single PR.** → Ordered commits per concern, and a PR description that maps commits to spec deltas.
- **Test churn.** Existing preflight and over-limit tests and TS cases assert size failures. → Convert them to large-success or validity tests. Recorded provider fixtures stay untouched.
- **Coordination with #375 and #248.** → Both touch the same files. Rebase whichever lands later.

## Migration Plan

1. Open one draft PR against `nrbrd/app-platform` (#428). Retarget it to main after #428 merges.
2. Operators change nothing: `max_request_bytes` and `max_model_duration` keep their names and meaning.
3. ProviderWire library users drop the removed `Limits` fields and apply a request-body bound in their host. Go client users replace the removed `Limits` fields with `ResponseBytes`.
4. Rollback means reverting the PR. No persisted state changes.

## Open Questions

- Whether the 1 MiB default request-body bound suits file-input workloads can be tuned through configuration later, without changing the approach.
