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

## Verification

The local HTTP regression failed with UND_ERR_SOCKET before the fix. All 28
release-tooling tests pass after covering both the read-only API and artifact
requests. Independent review found no issues. Normal live CLI authorization
from ec64c59c156b393621686ad51fa793a1942470b5 approved root PR #490 at squash
c8e497f3ca45571ef57c3556ebde32a836e8b918 without a preload. Source run39846721747,
readiness run39844872856 and artifact12454303002 were verified. No publication
was performed. Draft PR #492 contains this fix.
