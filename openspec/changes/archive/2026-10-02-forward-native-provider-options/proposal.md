## Why

Gateway-only namespace and field inventories silently discard native-consumed options and caller history, making remote providers behave differently from their native adapters. Issue [#318](https://github.com/grafana/ai-sdk/issues/318) replaces the request-option portion of superseded #303 with scoped opaque forwarding and concrete, consumption-backed protections.

## What Changes

- Forward ordinary namespace objects unchanged at supported call, message, content-part, function-tool and nested tool-result file-entry scopes, including unknown fields and meaningful empty/nested values.
- Remove `providerwire/v4/provider_option_policy.go`, service option inventories and unused catalog option-policy plumbing; leave interpretation and namespace precedence to native adapters.
- Replace blanket protected-field spelling rules with protections justified by the consuming adapter, namespace, scope and actual precedence. Preserve credential/account/destination, prompt/role/union, tool-ownership and protocol/execution boundaries.
- Preserve native-consumed Anthropic supplied local assistant-call caller history, OpenAI/Azure phase/item/reasoning controls, and compatible scoped extensions without introducing another allowlist. Ordinary tool-result caller remains forwarded to the adapter without claiming consumption by its currently ignoring converter.
- Keep Gateway-owned controls explicitly unsupported before native I/O unless an existing owning feature consumes them; separate outer Gateway authentication from provider call headers.
- Deliver pinned TypeScript and independent Go unary/streaming handler and command tests, synthetic native request assertions, bypass/ordinary-value controls, isolation tests and applicable documentation.
- **BREAKING**: Remove the Gateway catalog's option-policy API and its silent filtering contract. Applications must not rely on filtered options making a fallback request appear eligible; fallback restrictions remain enforced on the preserved request.

## Capabilities

### New Capabilities

- `gateway-native-provider-options`: Opaque direct-route scoped forwarding, consumption-backed protections and native-request/client evidence.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: Remove selected-backend filtering and replace universal protected-field rules with adapter-consumption-backed checks while retaining unsupported codec and host/header boundaries.
- `gateway-file-inputs`: Preserve file/function-tool namespaces without selected-backend filtering and retain native safety checks and deferred sibling capabilities.
- `gateway-ordered-text-fallback`: Remove references to filtering as an eligibility mechanism without broadening the existing fallback capability guard.

## Impact

Affected areas are the AGPL Gateway mapper, handler, catalog, command provider composition, service tests, ProviderWire cross-language harness and `docs/providers/grafana-gateway.md`. Native SDK adapters are the consumption reference, not targets for unrelated fixes; the Apache Go client remains independent of the service module. No new dependencies or baseline upgrade are planned.

The inspected starting point is canonical main `c0299776`; the registered reference is `ee3169b3c4880e2abe4d0d7c781243bb81822ec4` (ai 7.0.116, Gateway 4.0.94, Provider 4.0.18). Reconfirm these before implementation. This proposal uses no stopped #303/#309 code or artifacts.

## Non-goals

No fallback eligibility expansion (#319), response metadata codec or output-derived continuation (#280), diagnostics/carriers/access policy, discovery, new routing/BYOK, operator/consumer capture changes or missing tool activation. Supplied history proves only the request path. #318 has no prerequisite issue; #319 and #280 consume this foundation.
