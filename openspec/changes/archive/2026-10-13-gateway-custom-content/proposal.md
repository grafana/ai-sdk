# Gateway custom content

## Why

Issue #112 (work package 18) requires registered LanguageModelV4 custom content to cross the Gateway the way it crosses the native adapters. Today the Gateway rejects assistant custom history as `custom-content`, rejects custom tool-result content as `tools`, and turns unary or streamed custom output into an internal error. That breaks OpenAI compaction continuation, caller-supplied custom history (such as Anthropic fallback markers) and any future provider-owned block. The Gateway still refuses enabling Anthropic server-side fallback itself; that refusal is unchanged here. The native adapters have gaps of their own compared with the registered upstream:
- Anthropic drops custom tool-result content, warns on custom assistant kinds upstream ignores, substitutes empty text for fully filtered tool results, and omits `container_upload` output;
- OpenAI-compatible fails, and Bedrock warns, on assistant custom history that upstream skips;
- Bedrock Converse degrades custom tool-result content that upstream rejects. The pinned-target upgrade also needs a generic stream custom codec before it can add Anthropic `anthropic.message_start`.

## What Changes

- Map the four registered custom discriminators end to end, keeping `kind` and the opaque `providerOptions`/`providerMetadata` intact:
  - assistant `custom{kind, providerOptions}` history;
  - tool-result content `custom{providerOptions}`;
  - unary `custom{kind, providerMetadata}` content;
  - stream `custom{kind, providerMetadata}` parts.
- Treat stream custom as an atomic content event. It is allowed after response metadata and before finish. It leaves text/reasoning/tool state unchanged and commits a fallback candidate like any other first part.
- Have the independent Go client (`providers/grafana`) decode unary and stream custom content equivalently to the registered Vercel client.
- Align Anthropic with the registered upstream:
  - a custom tool-result part with `anthropic.type = "tool-reference"` becomes a `tool_reference` block;
  - any other custom tool-result part produces an `other` warning;
  - unknown assistant custom kinds are skipped without a warning;
  - tool results whose entries are all dropped are sent with empty content;
  - unary `container_upload` output becomes `anthropic.container_upload` custom content.
- Skip assistant custom history in OpenAI-compatible (instead of failing) and Bedrock Converse (instead of warning), matching upstream.
- Align OpenAI Responses compaction replay presence with upstream, now that callers can supply it through the Gateway.
- **BREAKING** (Bedrock Converse direct callers): when tools remain active after tool preparation, reject custom tool-result content before any I/O, matching upstream, instead of warning and dropping it. Bedrock Mantle is unchanged.
- Wrap Bedrock Converse's existing pre-I/O conversion errors whose upstream counterpart throws UnsupportedFunctionalityError with the same sentinel (provider-reference files, non-S3 URLs, unsupported media types). Messages are unchanged; warnings stay warnings.
- Add the root sentinel `provider.ErrUnsupportedFunctionality` for deterministic pre-I/O adapter rejection. Core retry never retries it. The default fallback decider advances past it. The Gateway reports it as the fixed non-retryable invalid request only when every attempted failure is that rejection.
- Keep custom options and metadata out of metadata-only operator telemetry. Update the Gateway guide and parity map.

## Capabilities

### New Capabilities

- `gateway-custom-content`: bounded transport of registered custom request, tool-result, unary and stream content through ProviderWire and both clients, including privacy, fallback and evidence boundaries.
- `native-custom-content`: native adapter conversion of custom content not consumed as a specific provider block (Anthropic tool references, filtering and `container_upload`; OpenAI compaction replay presence; OpenAI-compatible and Bedrock assistant skips).

### Modified Capabilities

- `providerwire-v4-unary-runtime`: custom content no longer an unsupported family; unary success admits custom content; custom tool-result content leaves the deferred result scopes; unsupported-functionality rejection classification.
- `providerwire-v4-streaming-runtime`: custom stream parts no longer an unsupported stream family, and are accepted by the stream state machine.
- `gateway-provider-metadata`: custom unary content and stream parts become supported metadata scopes.
- `gateway-unary-function-tools`: custom tool-result content becomes a supported result arm.
- `gateway-file-inputs`: custom content leaves the deferred sibling capabilities.
- `gateway-native-provider-options`: custom part and custom tool-result entry scopes, plus their consumption audit.
- `gateway-ordered-text-fallback`: custom codecs leave the deferred-refusal scenario.
- `grafana-gateway-client`: unary and stream consumption decode custom content.
- `provider-v4-content-model`: Anthropic assistant custom parts follow `native-custom-content` instead of the blanket unsupported warning.
- `bedrock-provider`: custom tool-result content is rejected before I/O when tools are active; existing pre-I/O unsupported conversion errors wrap the sentinel.
- `api-call-error`: unsupported-functionality sentinel and its default fallback decision.

## Impact

- Root `provider` package (new exported sentinel); `fallback` tests only, since the default decider already advances on unknown errors.
- `providers/anthropic`, `providers/bedrock`, `providers/openai`, `providers/openai-compatible` and `providers/grafana` (Apache-2.0).
- No adapter served by the Gateway command produces the new rejection yet: the command does not configure Bedrock (#118). Gateway classification is forward-compatible and proven with fake models.
- `ai-gateway/providerwire/v4` request, unary, stream and error mapping plus the `ai-gateway/test/providerwire-v4` workspace (AGPL-3.0-only).
- `docs/providers/grafana-gateway.md`, `docs/providers/bedrock.md` and `test/conformance/PARITY.md`.
- The change ships in one PR. Source CI builds the workspace candidate. Existing merged module pins remain until a later publication bump. No standalone-publication readiness is claimed.
- Registered baseline: ai 7.0.127, provider 4.0.21, gateway 4.0.103, anthropic 4.0.71, amazon-bedrock 5.0.105, openai 4.0.83, openai-compatible 3.0.62 at `eb77f09e`. The target-only `anthropic.message_start` emitter and server-side fallback beta belong to the follow-up upgrade PR.
