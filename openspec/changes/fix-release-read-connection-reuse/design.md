# Design

## Context

PR #490 live verification found the final PR reread failing after synchronous
selected-library checks. A diagnostic Connection: close preload passed the same
complete authorization. Pinned release-please 17.6.0 accepts the existing fetch
adapter; no library source change is needed. Runtime upstream remains ai@7.0.127.

## Decisions

Set Connection: close in readOnlyFetch after its existing host/method policy,
and in both artifact-download GETs. The authenticated artifact redirect otherwise
leaves a reusable API connection in the same global pool before validation.
Preserve credentials and caller headers, timeout composition and redirect rejection.
Do not add retry behavior, including to write transports. A worker-hosted local
HTTP server closes its socket while the test client is synchronously blocked;
the next real fetch must succeed with a distinct connection. All requests still
pass the canonical-host policy before the test transport maps them to localhost.

## Tradeoffs

Each API read incurs a connection setup. This small release tool values reliable
final rechecks over connection reuse; failures still block authorization.
