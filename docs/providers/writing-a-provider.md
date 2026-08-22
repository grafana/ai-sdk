# Writing a provider

Write a provider when an LLM service cannot be represented safely by an
existing provider or a small OpenAI-compatible request transform. A provider
adapts the vendor's messages, tools, results, errors, and stream events to the
common `provider.LanguageModel` contract.

This is extension work for SDK and platform authors. Application developers
should start with the [provider overview](overview.md).

## Implement both call paths

A model implements `provider.LanguageModel`, including streaming and
non-streaming calls plus provider/model identity. See the
[interface reference](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#LanguageModel)
for the exact methods.

Keep the adapter focused on provider concerns:

- convert `provider.CallOptions` into the vendor request;
- map vendor output into `GenerateResult` or ordered `StreamPart` values;
- preserve usage, warnings, finish reason, response metadata, and provider
  metadata;
- translate provider failures into `provider.APICallError` with correct retry
  classification;
- honor context cancellation;
- report unsupported generic settings as warnings when the call can proceed.

Do not import the root `aisdk` package from a provider. The `provider` package is
the transport-independent leaf contract.

## Preserve stream behavior

Emit parts in the order required by the common protocol. A streaming result must
close its channel, preserve errors, and stop promptly when the context is
cancelled. Forward provider chunks as they arrive. Use an explicit simulated
streaming wrapper for providers that return complete responses.

Tools, reasoning, files, sources, provider-executed tools, and structured output
all have provider-level representations. Implement only features the vendor
actually supports and return warnings for portable settings that must be
dropped.

## Package the provider separately

Provider integrations should be separate Go modules so vendor SDKs do not enter
the core dependency graph. Follow the existing `providers/<name>` modules for
constructor, option, test, and documentation conventions.

## Test the boundary

Test request conversion and response conversion independently. Include:

- representative message and tool requests;
- streaming event sequences;
- malformed and non-2xx errors;
- cancellation;
- provider-specific options;
- unsupported-setting warnings.

Then add conformance fixtures against the registered upstream package versions.
The baseline is recorded in `test/conformance/upstream.yaml`; do not compare
against upstream `main` silently. See the
[conformance test guide](../../test/conformance/README.md).

## Reference

- [`provider` package](https://pkg.go.dev/github.com/grafana/ai-sdk/provider)
- [How a request runs](../concepts/architecture.md)
- [Legacy ProviderWire retirement](../guides/provider-wire-retirement.md)

---

← [OpenAI-compatible](openai-compatible.md) · [Docs index](../README.md) · [Gateway model catalog →](../guides/gateway-model-catalog.md)
