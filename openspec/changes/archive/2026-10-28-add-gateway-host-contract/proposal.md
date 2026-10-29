# Proposal

## Why

After #481, every adapter shares the execution runtime. But adding a second adapter still means editing dispatch, access routing, discovery and catalog code. #248 shows this: it wires itself into the router and wraps catalog candidates with Chat defaults. This change puts a host contract between the plugin and its adapters. An adapter then needs one route-table entry plus its own code (#482, part of #394).

## What Changes

- **One access-aware selector.** It replaces `NewConfiguredSelector` and `NewBYOKSelector`, so one runtime and one ProviderWire handler serve both access modes:
  - configured access resolves through the catalog and refuses `gateway` controls, as today;
  - BYOK access decodes accounts from the controls;
  - any other access fails with the account-access error, with zero catalog and provider calls.
  - **Removed:** the per-access handler split (`routeHandlers`, `byAccess`) and the second runtime.
- **Route table.** Each entry is `{method, path, label, handler, host-error writer}`:
  - every entry gets the access middleware and the telemetry label;
  - the production table must match the committed manifest;
  - undeclared paths and methods do no work;
  - the two-route manifest is unchanged.
- **Host errors.** The host defines a closed set of host-error categories: permission, internal and BYOK discovery unsupported. ProviderWire writes them with byte-identical documents. The unused authentication category is removed.
- **Discovery.** The host owns listing: panic safety, the access decision and internal failures. The ProviderWire discovery document moves from the command into `providerwire/v4` as that adapter's projection.
- **Per-attempt native option translation:**
  - an adapter may supply a translator for each request;
  - the host wraps every configured candidate and every BYOK account once, at construction, directly outside native option validation and inside fallback;
  - translated options are re-checked by host policy;
  - a panicking translator, or one whose output fails host policy, fails that attempt as internal, does not fall back, and makes zero provider requests;
  - with no translator, options pass through unchanged;
  - BYOK's Anthropic option validation also runs on each account candidate, after translation. The request-level check before fallback stays, so a refused request still makes no account attempt.

  This replaces adapter-specific catalog wrappers such as #248's Chat defaults.
- **Test adapter on a route.** The minimal adapter from #481 becomes a test-only HTTP adapter. Tests register it on an extra route, where it gets access refusal in its own format, its own label, the shared runtime and translation.
- **Docs.** Godoc for the adapter contract. The architecture walkthrough and glossary cover the selector, the route table, host errors and option translation.

### Revised #482 decisions

- "It sits behind the single body bound": #521 removed the Gateway body bound, and the platform's route message size now bounds every request. Route entries add no body bound. For every entry, including the test adapter's, a refused caller's body is not read. "The BYOK bound" is the 8-account bound, which still maps to the fixed BYOK 400.
- For the reserved namespace in translated options, the issue means `gateway`, the only namespace the host reserves since #481.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-host`: route-table composition checked against the manifest, closed host-error categories, host-owned discovery listing with adapter projection, test-adapter composition evidence and the adapter-contract godoc.
- `gateway-execution-runtime`: one access-aware selector per runtime, per-attempt native option translation with a host-policy re-check, and the test-only adapter served over HTTP.
- `gateway-request-byok`: Anthropic option validation also runs on each account candidate after translation. A refused request still fails before any account attempt, with no overview.
- `providerwire-v4-unary-runtime`: the host error writer implements the host contract's categories, without the removed authentication category.

## Impact

- **Code:**
  - `ai-gateway/internal/execution`: translator, target and candidate wrapper, `Call` accepting a translator, and classification of translation failures;
  - a new `ai-gateway/internal/host` package for the contract: host-error categories, the writer interface and the discovery projection;
  - `providerwire/v4`: host-error writer and discovery projection;
  - `cmd/grafana-ai-gateway/internal/{gateway,service,discovery,byok}`: route table, unified selector, host discovery and per-account wrapping;
  - the test adapter moves from `execution_test` to a test-only package.
- **Go API:** `v4.HostErrorWriter`, `v4.HostErrorCategory` and `HostErrorAuthentication` change or are removed. `execution.Runtime.Call` gains an optional translator. Nothing outside the `ai-gateway` module consumes them.
- **Root module:** `fallback` exports its default decider, so Gateway chains can stop on translation failures (design D5). That is an additive Go extension with no upstream counterpart.
- **Wire:** no document changes, and the configured-discovery and host-error documents stay byte-identical. A BYOK Anthropic request that native validation refuses still fails before any account attempt, with the same document and no overview.
- **Parity:** host composition and option translation are Gateway extensions, with no counterpart in `@ai-sdk/gateway` at the pinned version in `test/conformance/upstream.yaml`. Fixtures, recordings and `knownGaps` are unchanged.
- **Delivery:** branch `nrbrd/gateway-host-contract`, based on `nrbrd/gateway-execution-runtime` (#555, open). Draft PR title: `refactor(ai-gateway): register adapters through a host contract`.
- **Out of scope:** real adapters (#248, #427), new manifest routes, lifecycle validation (#483), inbound credential translation for native SDKs, and publishing BYOK overviews (#420).
