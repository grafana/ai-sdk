## Context

The existing conformance suite replays authentic provider responses through real provider implementations and SDK orchestration. TypeScript generation establishes `expected.jsonl`, `expected-requests.jsonl`, and optional object, usage, or unary snapshots; Go replay compares against those committed artifacts. The current corpus has 141 provider cases across Anthropic, OpenAI Responses, OpenAI-compatible, and Bedrock. Discovery, not this count, must define the matrix.

The registered reference is `test/conformance/upstream.yaml`: `ai@7.0.143`, `@ai-sdk/gateway@4.0.116`, and repository commit `b7f6255acaae7724adc0fd37aac3dae3f3466578`. Investigation used that commit's gateway implementation/tests and `prepare-tool-choice` implementation/tests, with no baseline substitution. Coverage belongs to the conformance-harness, provider-implementation, and ProviderWire layers in `PARITY.md`.

Current Gateway command tests already run both clients against the plugin under an in-memory plugin host with synthetic backends. They are focused protocol/service evidence, not provider fixture conformance. The plugin currently supports Anthropic, OpenAI Responses and OpenAI-compatible backends; Bedrock is unsupported. Gaps the new suite exposes are findings, not prerequisites this change must fix.

The gateway runs as the Grafana App Platform plugin `grafana-aigateway-app`. Grafana starts its backend at boot, passes its settings section as `GF_PLUGIN_*` variables, and serves its routes under `/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{ns}/`. The plugin reads its model catalog once at startup and rejects the whole catalog when a provider type is unsupported; Grafana then exits because provisioning cannot find the app.

## Goals / Non-Goals

**Goals:**

- Make every existing provider fixture a compatibility check for each gateway client.
- Exercise the plugin package in real Grafana and the production provider adapters with offline replay.
- Retain the upstream direct-path oracle and all applicable assertions.
- Deliver useful, complete failure evidence while most cases may fail.
- Require both the direct suite and the gateway matrix for merging.

**Non-Goals:**

- Implement missing gateway capabilities, add provider backends, or upgrade dependency pins to improve the score.
- Maintain a separate supported-feature allowlist, expected-failure baseline, or bespoke gateway fixture corpus.
- Rewrite provider captures, regenerate goldens, or silently accept gateway differences.
- Add production mock providers to route the provider-independent `ui/` fixtures.
- Replace authentication, cancellation, privacy, bounds, frontend-hook, or service contract tests with replay conformance.

## Decisions

### 1. One corpus, three execution paths, one oracle

```text
Pinned TS SDK + direct provider -> replay -> committed reference snapshots

Go SDK + direct provider ---------------------> replay [existing required]
TS SDK + @ai-sdk/gateway -> Grafana -> plugin --> replay [new, required]
Go SDK + providers/grafana -> Grafana -> plugin -> replay [new, required]
```

The default gateway run discovers every provider fixture and creates two stable rows keyed by provider/category/scenario/client. New fixtures enter automatically, including new provider directories: missing harness wiring must be visible as a failed setup row rather than omitted discovery. Local scenario/client selectors are allowed, but filtered reports must declare their scope and CI must use the full inventory.

Streaming cases keep `streamText`/`StreamText` and UI conversion on the client, including local tool execution, approvals, multi-step loops, and structured output. Existing unary cases retain their low-level generate operation and unary oracle. Do not replace high-level streaming calls with `doStream` merely to bypass a compatibility failure.

This avoids copying fixtures or comparing only the two gateway clients, which could agree on the same server defect. Direct TypeScript execution remains expectation generation; a new required live TS-direct validation lane is not needed.

### 2. Share execution and comparison without importing the gateway

Refactor existing Go scenario setup and TypeScript generation helpers only enough to accept a selected model/endpoint and collect results independently of writing expectations. Existing generation and recording commands retain ownership of expectation writes. Gateway execution is read-only with respect to fixtures and goldens.

Reuse Go discovery, framing, and comparison infrastructure where practical; use a test-only Go executable or test runner for `providers/grafana` and a TypeScript executor for the pinned public client. Coordinate them through process/HTTP boundaries. Gateway orchestration must not introduce an import, require, or replace of `ai-gateway` into SDK/harness modules. Keep AGPL implementation helpers inside the gateway tree.

Compare provider stream parts at the client model boundary against the direct provider-parts golden. Retain existing normalization rules for request headers, JSON, deterministic IDs, OpenAI source IDs, and adjacent local tool outputs. Apply equivalent comparison semantics to both clients. Do not add blanket metadata removal, reorder arbitrary chunks, or suppress errors for the gateway lane. A necessary new equivalence requires a separate reviewed parity decision.

### 3. One Grafana stack, isolated rows, honest startup reporting

The local task builds the plugin package from the checkout with the repository's plugin build task unless an explicit plugin directory is supplied; CI always builds it. One Grafana container, at the image pinned for the plugin end-to-end tests, loads the package. Plugin settings come from the repository's development `grafana.ini` feature toggles with a conformance plugin section: a generated catalog, `access_mode = self-hosted` and a dummy backend key. Plugin preinstallation and update checks are disabled so the stack does not download plugins at boot. The harness creates a Grafana service account token through the admin API; both clients use it as their only credential.

Rows share the stack but no state. Each row gets its own replay server on the Docker bridge, its own provider instance and its own public model in the generated catalog, so both clients start at the first recorded response with fresh tool state and no reset is needed. Replay is reached through `host.docker.internal`; Grafana is published on an ephemeral loopback port. The harness waits for Grafana health and for the catalog route to list every admitted model before running rows sequentially.

Unsupported providers are admitted progressively. If the plugin exits at startup with an explicit unsupported-type reason for a configured instance, every row of that provider fails at `provider-setup` with the logged line, and the stack boots again without it. This costs at most one extra boot per unsupported type and needs no allowlist. Any other startup exit is an unclassified harness failure for every row, with container state, bounded logs and the attempted catalog. Do not route OpenAI Responses to OpenAI-compatible or invent a fallback provider. Missing test wiring is a harness error, not evidence of a gateway rejection.

Readiness, startup, scenario execution, shutdown, and child processes have deadlines. Clean up the container, listeners, processes, and staging files on success, failure, timeout, or cancellation, including after an ambiguous container creation failure. After a failed row the harness checks that the stack is alive; if it stopped, remaining rows are reported as not executed.

Real Grafana was chosen over the in-memory plugin test host because it exercises the production host path: Grafana's authentication, App Platform routing and SSE delivery. A spike confirmed service account tokens reach the route, SSE arrives incrementally, and startup reasons are visible in Grafana's logs.

### 4. Preserve assertions and make red results actionable

For each row collect actual UI chunks, backend requests, applicable object/usage/unary results, client errors, and bounded gateway logs. Compare against every expectation artifact applicable to the fixture. No backend request is itself evidence when the client/server rejected the call; still report the missing expected requests rather than hiding that assertion behind the first output failure.

Aggregate failures across rows and collect all available assertion diffs within a row. Gateway-specific privacy or metadata changes remain discrepancies, even when intentional, until separately reviewed. Classify findings using the existing parity categories; classification does not turn a red result green.

Write a machine-readable report and a concise CI summary with inventory totals, executed/passed/failed/not-executed counts, provider/client breakdowns, failure stage, and artifact links. Reconcile the results against the discovered two-client inventory. Missing/duplicate rows and an empty default inventory are harness failures. Exit nonzero for any failed or unexecuted row, infrastructure error, or incomplete report.

### 5. Required for merging

Add `mise run test-conformance-gateway` and a separate CI job. The existing required `conformance-test` status aggregates the direct and gateway jobs and succeeds only when both succeed. The direct job keeps its own status. Gateway conformance is not a prerequisite of plugin package validation or retention. The job uses the Grafana image pinned for the plugin end-to-end tests, enforced by a workflow test. No job or step uses `continue-on-error`; summary and artifact steps run after failures.

## Risks / Trade-offs

- **Many failures share one early blocker** -> Preserve all rows and group summaries by stage/cause; do not claim coverage of downstream assertions that were never reached.
- **Provider configuration evolves during rebases** -> Keep configuration adapters small and separate from discovery. Unsupported setup stays red; adapting new production configuration is normal harness maintenance, not a capability skip list.
- **Shared stack** -> A plugin crash would affect later rows; the harness checks liveness after failures and reports remaining rows as not executed. Logs are shared, so each row keeps the plugin log window for its own run.
- **An initially all-red suite could hide a broken comparator** -> Add focused harness tests and negative controls for discovery, comparisons, timeouts, reporting, and continuation. Synthetic harness data stays in unit tests, never provider provenance directories.
- **Grafana nightly drift** -> The image is pinned by digest and shared with the plugin end-to-end tests; bumping it is an explicit change.
- **Intentional gateway security policy prevents exact parity** -> Keep the mismatch visible and defer any exception to an explicit contract decision. Do not weaken privacy to make tests green.
- **Network configuration differs across development environments** -> Linux with a local Docker daemon is required; replay binds to the bridge gateway and Grafana reaches it through `host-gateway`.
- **Older main specs contain stale fixture-provenance wording** -> Follow current `AGENTS.md` provenance rules: no fabricated or derived provider captures. This change adds gateway requirements without broad unrelated spec cleanup.

## Migration Plan

1. Add tested execution/reporting infrastructure without changing direct-suite behavior or snapshots.
2. Run the entire gateway matrix locally and verify result completeness rather than parity success.
3. Add the gateway job and aggregate it into the required `conformance-test` status.
4. Document reproduction, reading reports, and current limits in the conformance guide and coverage map.
5. Merge once the matrix passes. Rollback consists of removing the gateway job from the aggregate status; direct conformance remains intact.

## Open Questions

- Whether the gateway's `providerMetadata.gateway.execution` extension on finish parts becomes a reviewed equivalence in provider-parts comparisons.
