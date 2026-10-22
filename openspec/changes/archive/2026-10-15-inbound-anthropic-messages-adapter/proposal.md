# Proposal

## Why

The Gateway currently exposes its ProviderWire API but cannot serve clients built against Anthropic's Messages API. An inbound adapter gives official Anthropic SDKs a useful, testable protocol endpoint while reusing configured Gateway identity, routing, execution, and telemetry.

## What Changes

- Add an authenticated `POST /v1/messages` route with unary and incremental SSE responses.
- Map a documented initial subset of Anthropic system/messages, text, client function tools and tool results onto Gateway provider calls.
- Return Anthropic-shaped errors, stop reasons, usage, and message lifecycle events without claiming complete Anthropic or Claude Code compatibility.
- Add command-level synthetic provider tests driven by pinned official Go and TypeScript Anthropic SDK clients, and document supported behavior and gaps.

## Capabilities

### New Capabilities

- `gateway-anthropic-messages`: Authenticated inbound Anthropic Messages protocol adaptation over configured Gateway model execution.

### Modified Capabilities

- None.

## Impact

The adapter and command composition live in the AGPL Gateway module under `ai-gateway/`. Runtime dependencies remain unchanged; official SDK versions are pinned in integration test manifests. It uses existing auth middleware, model catalog/resolver, provider execution and telemetry. Shared transport resource policy remains owned by the current Gateway policy and #394; this change adds no adapter-local body or output budgets. Request checks are limited to ordinary JSON decoding and information needed for safe canonical mapping, consistent with #395.

This implementation uses the command host present on current main for local SDK acceptance tests. The retired Gateway image deployment is not a rollout path. Deployment requires integration with the App Platform plugin host in #428, including its route manifest, delegated identity and plugin-host SDK tests.
