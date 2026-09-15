## Context

The authenticated Gateway command from #151 composes only direct Anthropic models. Its model configuration is strict YAML, its outbound package owns one hardened model HTTP client with redirect rejection, a response-header timeout and a cumulative response byte bound, and the ProviderWire V4 runtime is text-only.

The registered baseline is `@ai-sdk/openai-compatible@3.0.30` and `@ai-sdk/gateway@4.0.52` at Vercel commit `d76eb85a`. The upstream compatible chat model sends `stream_options: { include_usage: true }` only when the provider is created with `includeUsage`; the Go `providers/openai-compatible` module mirrors that with `WithIncludeUsage`, which defaults to off. Upstream has no Gateway command equivalent, so this change is a Grafana host composition over the registered public Gateway client.

This behavior is implemented at this change's head.

## Goals / Non-Goals

**Goals:**

- Let operators declare named OpenAI-compatible provider instances and route public models and aliases to them.
- Reuse the existing hardened model transport and startup endpoint validation without new flags.
- Carry backend token usage through ProviderWire finish parts.
- Prove privacy, sanitized failures, redirect rejection and cancellation through the real command.

**Non-Goals:**

- OpenAI Responses, Bedrock and Vertex provider configuration (#117 to #119).
- Per-provider or per-instance outbound limits, or renaming the `anthropic.response-*` flags.
- Reasoning, tools, files or other capability families beyond the text-only runtime (#105 to #116).
- A keyless authentication mode for local backends.
- Changing the Go `providers/openai-compatible` defaults or its parity with upstream.

## Decisions

### Share the hardened model client

Compatible models use the same `*http.Client` as Anthropic models. The limits that client enforces apply to any model upstream in the same way, and a per-type flag pair would not solve the realistic tuning need, which is per instance (a slow local backend next to a hosted one). The existing flag names stay, and their help text states that they govern both provider types.

### Require `baseURL` for compatible providers

A compatible backend has no canonical endpoint, and `providers/openai-compatible` falls back to `https://api.openai.com/v1` when no base URL is set, which would send the configured key to OpenAI. Configuration validation rejects a missing `baseURL`, and catalog construction checks it again because `BuildCatalog` accepts resolved providers directly.

### Always request streaming usage

The Gateway enables `WithIncludeUsage(true)` for every compatible model because ProviderWire finish parts carry usage and callers rely on it for accounting. Without it, streamed usage through the Gateway was empty. Backends that ignore the option still finish with empty usage. This is a host composition choice, not a change to the SDK default.

### Accept `providerName` only for compatible providers

`providers/openai-compatible` uses the name as its provider identity. It never reaches Gateway clients, because discovery publishes provider `grafana`, ProviderWire rejects provider options and response metadata omits provider identity. Other provider types reject the key so a setting is never silently ignored under strict configuration.

## Risks / Trade-offs

- Reasoning models fail: compatible backends that emit reasoning parts end streams with the text-only runtime's terminal internal error until reasoning content lands (#111).
- A backend that rejects `stream_options` fails every streaming request; OpenAI, vLLM and Ollama accept it.
- One set of limits covers every provider instance in a Gateway, and the shared flags keep `anthropic.*` names; a provider-neutral rename is cheapest before deployment assets land.
- Keyless backends need a placeholder `apiKeyEnv` value, which is sent upstream as a bearer token.
- Fake compatible servers establish deterministic command, transport and privacy evidence only; they are not provider conformance provenance.
