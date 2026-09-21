## Context

The existing conformance suite replays authentic provider responses through real provider implementations and SDK orchestration. TypeScript generation establishes `expected.jsonl`, `expected-requests.jsonl`, and optional object, usage, or unary snapshots; Go replay compares against those committed artifacts. The current corpus has 136 provider cases across Anthropic, OpenAI Responses, OpenAI-compatible, and Bedrock. Discovery, not this count, must define the matrix.

The registered reference is `test/conformance/upstream.yaml`: `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, and repository commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. Investigation used that commit's gateway implementation/tests and `prepare-tool-choice` implementation/tests, with no baseline substitution. Coverage belongs to the conformance-harness, provider-implementation, and ProviderWire layers in `PARITY.md`.

Current Gateway command tests already run both clients against the real binary using synthetic backends. They are focused protocol/service evidence, not provider fixture conformance. The production gateway currently supports Anthropic and OpenAI-compatible backends and text-only responses. It rejects nonempty provider options and tool choices, including the automatic choice emitted by upstream high-level calls without tools. The Go client also has text-only response handling. These are findings the new suite must expose, not prerequisites this change must fix.

The production Dockerfile builds a separate AGPL module using pinned published SDK/provider versions with `GOWORK=off`. A checkout's direct Go tests can therefore exercise newer providers than its gateway image. Preserve this distinction.

## Goals / Non-Goals

**Goals:**

- Make every existing provider fixture a compatibility check for each gateway client.
- Exercise the actual image and production provider adapters with offline replay.
- Retain the upstream direct-path oracle and all applicable assertions.
- Deliver useful, complete failure evidence while most cases may fail.
- Keep the direct suite required and the new gateway check initially advisory.

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
TS SDK + @ai-sdk/gateway -> gateway image ------> replay [new advisory]
Go SDK + providers/grafana -> gateway image ----> replay [new advisory]
```

The default gateway run discovers every provider fixture and creates two stable rows keyed by provider/category/scenario/client. New fixtures enter automatically, including new provider directories: missing harness wiring must be visible as a failed setup row rather than omitted discovery. Local scenario/client selectors are allowed, but filtered reports must declare their scope and CI must use the full inventory.

Streaming cases keep `streamText`/`StreamText` and UI conversion on the client, including local tool execution, approvals, multi-step loops, and structured output. Existing unary cases retain their low-level generate operation and unary oracle. Do not replace high-level streaming calls with `doStream` merely to bypass a compatibility failure.

This avoids copying fixtures or comparing only the two gateway clients, which could agree on the same server defect. Direct TypeScript execution remains expectation generation; a new required live TS-direct validation lane is not needed.

### 2. Share execution and comparison without importing the gateway

Refactor existing Go scenario setup and TypeScript generation helpers only enough to accept a selected model/endpoint and collect results independently of writing expectations. Existing generation and recording commands retain ownership of expectation writes. Gateway execution is read-only with respect to fixtures and goldens.

Reuse Go discovery, framing, and comparison infrastructure where practical; use a test-only Go executable or test runner for `providers/grafana` and a TypeScript executor for the pinned public client. Coordinate them through process/HTTP boundaries. Gateway orchestration must not introduce an import, require, or replace of `ai-gateway` into SDK/harness modules. Keep AGPL implementation helpers inside the gateway tree.

Retain existing normalization rules for request headers, JSON, deterministic IDs, OpenAI source IDs, and adjacent local tool outputs. Apply equivalent comparison semantics to both clients. Do not add blanket metadata removal, reorder arbitrary chunks, or suppress errors for the gateway lane. A necessary new equivalence requires a separate reviewed parity decision.

### 3. Build once, isolate attempts, and report blocked execution honestly

The local task builds the production image once by default; an explicit image override permits reproduction against a selected artifact. CI builds from its checkout rather than relying on a published image that may not exist for the PR. Record the image ID/source revision, gateway module pins, client revisions/versions, and upstream baseline in the report.

Start the gateway with test-only credentials and its existing development/auth configuration, mounted read-only model configuration, and replay endpoints reachable from the container. Use a Docker internal network on a local Linux daemon, reach container addresses directly from the host without publishing ports, bind replay/JWKS listeners to the bridge address, and avoid ambient provider credentials or real provider access. Generate an ephemeral signing key and serve test JWKS in memory: production unsafe-auth mode requires a loopback listener and is unsuitable for container networking. Preserve provider-native model IDs, request paths, and framing. Public gateway model IDs can differ but must map explicitly to the fixture's backend model.

Use bounded execution with isolated fixture/client replay state and configuration; sequential attempts are the initial default. Each client starts at the first recorded response and gets fresh tool mock state. Do not share a multi-step response counter between clients. Container reuse is only an optimization if equivalent isolation is demonstrated.

Missing provider support can fail before the gateway listens. Isolate provider setup so an unsupported provider cannot stop other providers. Record every affected row as failed, attach the actual startup/configuration evidence, and state that client invocation did not occur. Use provider-setup attribution only when the gateway exposes an explicit provider-configuration rejection. The current image sanitizes startup errors to `process_failure`; these opaque exits remain unclassified harness failures with Docker state, logs, and attempted configuration, rather than inferred provider incompatibilities. Do not route OpenAI Responses to OpenAI-compatible or invent a fallback provider. Missing test wiring is a harness error, not evidence of a gateway rejection.

Readiness, startup, scenario execution, shutdown, and child processes have deadlines. Clean up containers, listeners, processes, and temporary files on success, failure, timeout, or cancellation. Global infrastructure failures mark remaining rows as not executed due to a harness failure; they cannot appear as passing, skipped for capability, or ordinary compatibility failures.

### 4. Preserve assertions and make red results actionable

For each row collect actual UI chunks, backend requests, applicable object/usage/unary results, client errors, and bounded gateway logs. Compare against every expectation artifact applicable to the fixture. No backend request is itself evidence when the client/server rejected the call; still report the missing expected requests rather than hiding that assertion behind the first output failure.

Aggregate failures across rows and collect all available assertion diffs within a row. Gateway-specific privacy or metadata changes remain discrepancies, even when intentional, until separately reviewed. Classify findings using the existing parity categories; classification does not turn a red result green.

Write a machine-readable report and a concise CI summary with inventory totals, executed/passed/failed/not-executed counts, provider/client breakdowns, failure stage, and artifact links. Reconcile the results against the discovered two-client inventory. Missing/duplicate rows and an empty default inventory are harness failures. Exit nonzero for any failed or unexecuted row, infrastructure error, or incomplete report.

### 5. Advisory means optional, not successful

Add `mise run test-conformance-gateway` and a separate CI job. Leave `mise run test-conformance`, the required direct CI check, and the existing required parity commands independent of Docker/gateway failures. Do not make the advisory job a dependency of image publication or deployment.

The new job reports failure normally, without job/step-level `continue-on-error` or ignored final exit codes that turn the check green. Summary/artifact steps run after failures. Repository rules must leave the new check non-required; required-check configuration is separate from workflow YAML and must be verified by the maintainer.

Once the complete matrix genuinely passes, a later policy change can make it required and decide whether to gate publication. This change does not impose a pass-rate threshold or make future green status an acceptance criterion for the harness work.

## Risks / Trade-offs

- **Many failures share one early blocker** -> Preserve all rows and group summaries by stage/cause; do not claim coverage of downstream assertions that were never reached.
- **Provider configuration evolves during rebases** -> Keep configuration adapters small and separate from discovery. Unsupported setup stays red; adapting new production configuration is normal harness maintenance, not a capability skip list.
- **Image dependencies lag checkout providers** -> Report both version sets and preserve the production build boundary rather than replacing dependencies with local code.
- **An initially all-red suite could hide a broken comparator** -> Add focused harness tests and negative controls for discovery, comparisons, timeouts, reporting, and continuation. Synthetic harness data stays in unit tests, never provider provenance directories.
- **Container startup increases runtime** -> Build once, bound concurrency and deadlines, collect timings, and optimize reuse only after isolation tests exist.
- **Intentional gateway security policy prevents exact parity** -> Keep the mismatch visible and defer any exception to an explicit contract decision. Do not weaken privacy to make tests green.
- **Network configuration differs across development environments** -> Document the supported Docker topology and ensure readiness distinguishes connectivity failures from provider incompatibility.
- **Older main specs contain stale fixture-provenance wording** -> Follow current `AGENTS.md` provenance rules: no fabricated or derived provider captures. This change adds gateway requirements without broad unrelated spec cleanup.

## Migration Plan

1. Add tested execution/reporting infrastructure without changing direct-suite behavior or snapshots.
2. Run the entire gateway matrix locally, retain the initial failure artifacts, and verify result completeness rather than requiring parity success.
3. Add the independent advisory job and confirm it is absent from required checks and publication/deployment dependencies.
4. Document reproduction, reading reports, and the future promotion process in the conformance guide and coverage map.
5. Rebase or merge the harness while feature work progressively resolves actual failures. Rollback consists of removing/disabling the new advisory job/task; direct conformance remains intact.

## Open Questions

No product-scope decision blocks implementation. Runtime measurements will determine whether container reuse is worthwhile. Repository maintainers must verify external required-check policy before merging; workflow code alone cannot guarantee that policy.
