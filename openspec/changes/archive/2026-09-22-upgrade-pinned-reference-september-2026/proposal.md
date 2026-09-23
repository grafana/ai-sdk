## Why

The registered August reference no longer describes current upstream behavior. Upgrade to the approved, mature September package set while distinguishing a verified reference from full behavioral parity.

## What Changes

- Apply the frozen nine-package target selected on 2026-09-22, including `ai@7.0.107`, `@ai-sdk/provider@4.0.17`, and `@ai-sdk/gateway@4.0.87`.
- Preserve its per-package source evidence alongside the change; update all four consumers, lockfile, expectations, and reviewed witnesses together.
- Assess supported core, provider, frontend, Gateway, and harness behavior against exact target implementation/tests, including older differences.
- Correct required-check and supported-integration blockers; register nonblocking work through deduplicated, labeled issues and coverage records.

Non-goals: implement the entire parity backlog, adopt unsupported upstream product families, change the selected target, or bypass published-module ordering.

## Capabilities

### New Capabilities

- `openai-compatible-stream-metadata`: specify placeholder metadata handling for the existing adapter.

### Modified Capabilities

- `upstream-parity-governance`: retain the selected target's per-package source record with the transition.
- `stream-text-lifecycle`: keep text/reasoning identifiers unique across steps and gate local tool dispatch/continuation after failed or incomplete output.
- `openai-responses-provider`: easy-input assistant history, apply-patch finish reasons, internal parallel wrappers and recoverable malformed stream events.
- `provider-executed-tool-roundtrip`: preserve Anthropic caller metadata and web-search error continuation.
- `grafana-gateway-client`: preserve bounded validated unary warnings.

## Impact

Parity manifests and lockfile, generated expectations, coverage records, and Gateway attestation are in scope. Go compatibility changes require exact upstream evidence and regression proof. Public module consumers must use already published prerequisites. The owner approved publishing the OpenAI correction here and separately repinning Mantle afterward under #207; the existing Mantle defect is not claimed fixed by this producer change.
