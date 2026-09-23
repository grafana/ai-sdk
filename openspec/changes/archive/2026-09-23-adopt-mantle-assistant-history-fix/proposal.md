## Why

Bedrock Mantle still pins an OpenAI adapter that encodes reconstructed assistant history as incomplete output messages, which live GPT-OSS checks in [#207](https://github.com/grafana/ai-sdk/issues/207) rejected with `invalid_prompt`. The shared encoder correction and upstream requirements are now merged in [#230](https://github.com/grafana/ai-sdk/pull/230), and its published module is available for independent Bedrock adoption.

## What Changes

- Adopt the merged, publicly downloadable OpenAI module revision in `providers/bedrock/go.mod` and update its checksums. Align the conformance test module's OpenAI requirement so its readonly parity gate resolves the upgraded Bedrock dependency graph.
- Capture requests through `mantle.NewResponses` for unary and streaming continuations, proving reconstructed assistant text uses string content, preserves phase, and omits stale item IDs.
- Preserve stored item references, OpenAI metadata namespacing, Mantle attribution, routing, and authentication.
- Verify the fix outside the workspace and document the Mantle-specific evidence boundary in `test/conformance/PARITY.md`, linking this adoption to #207.
- Repeat the minimal live GPT-OSS recall check when credentials are available; deterministic tests remain credential-free.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-mantle-responses-provider`: Make the existing shared Responses continuation contract explicit for reconstructed assistant history in unary and streaming calls, including standalone published-dependency behavior. The merged `openai-responses-provider` encoding requirements remain unchanged.

## Impact

- Expected implementation files: `providers/bedrock/go.mod`, `providers/bedrock/go.sum`, `providers/bedrock/mantle/provider_test.go`, `test/conformance/go.mod`, `test/conformance/go.sum` (if needed), and `test/conformance/PARITY.md`.
- Selected producer: `github.com/grafana/ai-sdk/providers/openai@v0.0.0-20260923145714-9fb535a02817`, from merged commit `9fb535a028172fd48a73457c655d1389d6e6290c`.
- Parity layer: provider implementation/request conversion and published module adoption, against the registered OpenAI 4.0.71 / Amazon Bedrock 5.0.88 baseline.
- No new public API, local production replacement, duplicate Mantle encoder, baseline upgrade, or fabricated conformance recording. Mantle Chat, reasoning-summary controls (#164), and web-search source includes remain out of scope.
