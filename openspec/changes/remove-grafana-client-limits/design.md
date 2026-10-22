# Design

## Context

See proposal.md for the motivation. Current state in `providers/grafana`:

- `newProvider` resolves `*Limits` (nil selects `DefaultLimits()`), validates every field and stores the result on `Provider.limits`.
- `readJSON(ctx, resp, limit)` is the single buffered reader. It is used by discovery (`DiscoveryBytes`, 4 MiB), unary success (`UnaryBytes`, 16 MiB) and `readGatewayError` (`ErrorBytes`, 64 KiB). `readGatewayError` also covers non-2xx stream setup. `DoGenerate` lets an over-limit partial body take precedence over a read error, and context errors take precedence over both.
- `eventReader` applies `StreamBytes` (also through `io.LimitReader`), `StreamEventBytes` and `StreamEvents`. `errStreamLimit` is the only non-retryable stream read error.
- `wireUsage.UnmarshalJSON` rejects retained `usage.raw` above `maxRawUsageBytes` (1 MiB).

Upstream reference, `@ai-sdk/gateway@4.0.103` (provider-utils is a workspace dependency at the same tag):

- `createJsonResponseHandler` and `createJsonErrorResponseHandler` read through `readResponseWithSizeLimit`, with `DEFAULT_MAX_DOWNLOAD_SIZE = 2 * 1024 * 1024 * 1024`. Exceeding it throws `DownloadError`, which is not an `APICallError` and is never retried.
- `createEventSourceResponseHandler` parses the stream with no size or count limit.

## Goals / Non-Goals

**Goals:**

- One fixed buffered-read backstop that matches upstream, and no client-side stream bounds.
- Keep the existing error classification, precedence and cleanup paths, so only limit-driven behavior changes.

**Non-Goals:**

- Reshaping the decoders (#375) or changing the SSE framing logic beyond removing the counters.
- Adding a body-limiting transport or any other replacement knob.

## Decisions

1. **Backstop constant plus a per-provider unexported seam.** Add an unexported `maxBufferedResponseBytes int64 = 2 << 30`. `Provider` replaces `limits Limits` with an unexported `bufferedReadLimit int64`, which `newProvider` always sets to the constant. `readJSON(ctx, resp, limit)` and `readGatewayError` keep their `limit` parameter, and discovery, unary and error paths pass `p.bufferedReadLimit`. Tests lower the field on their own provider instance, so the exact-size and one-byte-over cases keep running through `ListModels`, `DoGenerate` and `readGatewayError`. That keeps the existing precedence and cleanup tests end to end.
   - *Alternative:* call `readJSON` directly with a small limit. Rejected: it drops the public-path coverage of `DoGenerate` precedence, discovery atomicity and error-body handling.
   - *Alternative:* a mutable package variable. Rejected: it adds global mutable state and is unsafe with parallel tests.
   - *Alternative:* test with a real 2 GiB body. Rejected by the acceptance criteria.

2. **Keep the backstop failure as the current non-retryable protocol error** (`grafana: response byte limit exceeded`, wrapped by the existing path messages). Keep the `DoGenerate` precedence: context error, then backstop, then read error. Upstream's `DownloadError` is also terminal and never retried, so this is a parity-preserving Go adaptation and needs no `knownGaps` entry.

3. **No `Content-Length` early rejection.** Upstream rejects early when the declared length exceeds the limit. The incremental `io.LimitReader(limit+1)` check gives the same observable result (an error and no truncation). The only difference is that a server declaring an oversized body fails before it is read. Given a 2 GiB backstop and server-owned bounds, the extra branch isn't worth it. This is recorded as a parity-preserving adaptation.
   - *Alternative:* mirror the early check. It costs a few lines plus a test, and it would make the backstop observable through the public path. Revisit if reviewers want strict mechanism parity.

4. **Error bodies share the backstop (64 KiB becomes 2 GiB).** This matches `createJsonErrorResponseHandler`. Retained `Data`/`ResponseBody` behavior is unchanged.

5. **`eventReader` keeps only framing state.** It keeps the BOM skip, CR/CRLF/LF normalization, `data` field handling and `skipLF`. It drops `total`, `count`, the per-event counter, the `Limits` field and the `io.LimitReader` wrapper. `errStreamLimit` goes away, so every non-EOF read error becomes the existing retryable `grafana: stream read failed` part with status 200. `consumeStream` loses its `limits` parameter.

6. **Raw usage keeps shape checks only.** Remove `maxRawUsageBytes` and the size comparison. Keep the present-but-empty and non-object rejection and the pre-decode UTF-8/JSON validation of the enclosing document.

7. **Public surface is pinned by name, not by a substring.** `public_surface_test.go` asserts that there is no exported `Limits` type or `DefaultLimits` function, and no `Limits` field on `TokenExchangeConfig`, `ServiceAccountConfig` or `AccessTokenConfig`. A blanket `Limit` substring check would wrongly flag `GatewayRateLimit`.

8. **Large-stream acceptance uses generated readers, not prebuilt buffers.** There are two cases:
   - more than 100,000 small text-delta events;
   - 65 text-delta events above 1 MiB each, which also exceeds 64 MiB cumulatively.

   Both stream from a generating `io.Reader`, which keeps memory and `-race` runtime modest. Each case asserts the delivered part count and that no `PartError` occurs.

9. **Docs.** In `docs/providers/grafana-gateway.md`:
   - the raw-output note keeps only the Gateway frame limit;
   - "Handle large catalogs" drops the Go `DiscoveryBytes` advice but keeps the TypeScript helper note;
   - generated files count toward the Gateway's limits;
   - "Bound work and handle errors" states that the Gateway owns resource bounds and callers use `http.Client` and `context`.

   `doc.go` drops "bounded" from the streaming sentence and states the same ownership. The architecture walkthrough and glossary do not describe client limits, so they need no change.

## Risks / Trade-offs

- [A misbehaving or misconfigured endpoint can make the client buffer up to 2 GiB, or an arbitrarily large SSE event] → The Gateway bounds provider responses (16 MiB) and model duration. Callers can bound time with `context`, and bytes with a body-limiting `http.RoundTripper` on their own `http.Client`. The docs say so.
- [Breaking change for callers that set `Limits`] → Compile-time failure with an obvious fix (delete the field). The PR title carries `!`. Release automation stays inactive until #245.
- [On 32-bit platforms, `2 GiB + 1` exceeds `math.MaxInt`] → `io.LimitReader` takes `int64`, so the check still works. A body that large fails allocation first, which is the same failure upstream's limit exists to make catchable. This is acceptable for a backstop.
- [Overlap with #375 in `stream.go`/`model.go`] → Keep this change to the counters and parameters only, so the later rebase stays mechanical.
- [Ordering against #480] → This change lands first. It only widens what the client accepts, so it is safe with today's server.

## Migration Plan

- Callers remove `Limits` from their config structs and any `DefaultLimits()` use. No wire or server change.
- Rollback: revert the PR. There is no persisted state.
