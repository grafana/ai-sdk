# Changelog

## [0.1.0-alpha.4](https://github.com/grafana/ai-sdk/compare/providers/anthropic/v0.1.0-alpha.3...providers/anthropic/v0.1.0-alpha.4) (2026-10-30)


### Features

* **parity:** advance pinned upstream to ai 7.0.153 ([#610](https://github.com/grafana/ai-sdk/issues/610)) ([fd3f743](https://github.com/grafana/ai-sdk/commit/fd3f743a213e3f049e7ffbfe65025f58a182867b))


### Bug Fixes

* **anthropic:** retain tool-search errors during assistant-history replay ([#560](https://github.com/grafana/ai-sdk/issues/560)) ([e55060c](https://github.com/grafana/ai-sdk/commit/e55060c7732364b08908115f465703caac13dcef))

## [0.1.0-alpha.3](https://github.com/grafana/ai-sdk/compare/providers/anthropic/v0.1.0-alpha.2...providers/anthropic/v0.1.0-alpha.3) (2026-10-28)


### Features

* **anthropic:** replay uploaded-file references inside tool results ([#566](https://github.com/grafana/ai-sdk/issues/566)) ([e3b02f0](https://github.com/grafana/ai-sdk/commit/e3b02f020fe746e291dbf7ce8f0beb942098f7b7))

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/providers/anthropic/v0.1.0-alpha.1...providers/anthropic/v0.1.0-alpha.2) (2026-10-28)


### Features

* **parity:** advance pinned upstream to ai 7.0.143 ([#458](https://github.com/grafana/ai-sdk/issues/458)) ([f76495c](https://github.com/grafana/ai-sdk/commit/f76495c223918a2e879bc4c80cb0741528c2b565))


### Bug Fixes

* **ai-gateway:** contain panics in stream worker goroutines ([#471](https://github.com/grafana/ai-sdk/issues/471)) ([e74f87b](https://github.com/grafana/ai-sdk/commit/e74f87bcf30f2a9fccdc900f5d4d6ecdb06c8ec6)), refs [#419](https://github.com/grafana/ai-sdk/issues/419)
* **anthropic:** align tool-result cache control, file warnings and MCP routing ([#530](https://github.com/grafana/ai-sdk/issues/530)) ([4d2516d](https://github.com/grafana/ai-sdk/commit/4d2516d275b04178357b15eb685fa03d4d36e8f2))
* **anthropic:** move client tool_use blocks to the end of reasoning segments ([#517](https://github.com/grafana/ai-sdk/issues/517)) ([8c138af](https://github.com/grafana/ai-sdk/commit/8c138af2150d6bc17648b6339ee65bb851a95c7d))

## 0.1.0-alpha.1 (2026-10-26)


### ⚠ BREAKING CHANGES

* **ai-gateway:** Bedrock Converse now rejects custom tool-result content with an error wrapping `provider.ErrUnsupportedFunctionality` before any HTTP request when tools are active. It used to drop the entry with a warning. Upstream throws for it. Assistant custom history is now skipped without a warning.
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425))
* **gateway:** `provider.ResponseFormat.Name` is now `*string`. Omit it for the native default; use a pointer for an explicit name, including an empty one. Output helpers handle this presence distinction. Published module versions and release tags are unchanged.
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* **ai-gateway:** support custom content ([#423](https://github.com/grafana/ai-sdk/issues/423)) ([0e5f36b](https://github.com/grafana/ai-sdk/commit/0e5f36b036c53ef45635694406333d90bcf07cae))
* **anthropic:** expose opt-in safeguards and verdict metadata ([#288](https://github.com/grafana/ai-sdk/issues/288)) ([86fc7d0](https://github.com/grafana/ai-sdk/commit/86fc7d0dc2a7841d16026be875c2f69cefc78b08))
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **gateway:** support structured output with candidate failure details ([#416](https://github.com/grafana/ai-sdk/issues/416)) ([5ef4ed4](https://github.com/grafana/ai-sdk/commit/5ef4ed403107b9b23a9a3bf789c231b8aef7f1d6))
* **gateway:** transport bounded reasoning content and continuation metadata ([#247](https://github.com/grafana/ai-sdk/issues/247)) ([566db5d](https://github.com/grafana/ai-sdk/commit/566db5d9df4f6c2360d7ffa963a7db8d1eb1b763))
* **provider:** align provider-tool contracts and client decoding ([#238](https://github.com/grafana/ai-sdk/issues/238)) ([fe1e4ac](https://github.com/grafana/ai-sdk/commit/fe1e4ac3b44f577a93f22b99636719d72c627c5b))
* **providers:** support Claude Sonnet 5.5 ([#276](https://github.com/grafana/ai-sdk/issues/276)) ([e21f9fe](https://github.com/grafana/ai-sdk/commit/e21f9fe7626ae33e5af7ddbabcc4986dc518dfe9))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425)) ([307551a](https://github.com/grafana/ai-sdk/commit/307551a566076610500bf458c5461b015a9c6e11))


### Bug Fixes

* **anthropic:** align dated model capabilities and web tools ([#253](https://github.com/grafana/ai-sdk/issues/253)) ([118d99f](https://github.com/grafana/ai-sdk/commit/118d99fc13d51d54bf632412e385ef507dceecd7))
* **anthropic:** count cache tokens from executor iterations ([#413](https://github.com/grafana/ai-sdk/issues/413)) ([5081cfb](https://github.com/grafana/ai-sdk/commit/5081cfb2cd63ae8b53740e2ba504309947fedb48))
* **anthropic:** use native structured output on Vertex ([#302](https://github.com/grafana/ai-sdk/issues/302)) ([1a68d30](https://github.com/grafana/ai-sdk/commit/1a68d30c5d3822c2760688866cacf7582c8b7c19))
* **conformance:** preserve query parameters in request snapshots ([#307](https://github.com/grafana/ai-sdk/issues/307)) ([9bf50c7](https://github.com/grafana/ai-sdk/commit/9bf50c70144771f6a6eaa54af1ccda106e358292))
* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#338](https://github.com/grafana/ai-sdk/issues/338)) ([79acadf](https://github.com/grafana/ai-sdk/commit/79acadf9b1ee4149a83b4129a2bfac13fb723abb))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#405](https://github.com/grafana/ai-sdk/issues/405)) ([6fd0007](https://github.com/grafana/ai-sdk/commit/6fd00076fac2d5bee797777751494f7f799a044d))
* **deps:** update go module patch updates ([#528](https://github.com/grafana/ai-sdk/issues/528)) ([c3b7179](https://github.com/grafana/ai-sdk/commit/c3b7179b090411bd459e669cfb05bc34fce12834))
* **parity:** advance frozen reference to AI SDK 7.0.107 ([#230](https://github.com/grafana/ai-sdk/issues/230)) ([9fb535a](https://github.com/grafana/ai-sdk/commit/9fb535a028172fd48a73457c655d1389d6e6290c))
* **parity:** align continuation with AI SDK 7.0.127 ([#365](https://github.com/grafana/ai-sdk/issues/365)) ([28e08a4](https://github.com/grafana/ai-sdk/commit/28e08a459341bf0e50de3980cb534dc59d488096))
* **providers:** align provider stream parts with upstream ([#397](https://github.com/grafana/ai-sdk/issues/397)) ([17030fc](https://github.com/grafana/ai-sdk/commit/17030fc1ac1b4cf4d2038380dfcd14084ec65568))
* **providers:** preserve call headers and SDK transport metadata ([#306](https://github.com/grafana/ai-sdk/issues/306)) ([4ebcc14](https://github.com/grafana/ai-sdk/commit/4ebcc14943a9ad67121e0938c5dc08822204a919))
* **providers:** preserve native unary source titles through simulated streams ([#340](https://github.com/grafana/ai-sdk/issues/340)) ([465b024](https://github.com/grafana/ai-sdk/commit/465b0247092d8ee7f56e8a701022ead2f31c948b))
* **security/unknown/providers/anthropic:** update module golang.org/x/net to v0.60.0 [security] ([#384](https://github.com/grafana/ai-sdk/issues/384)) ([79be97b](https://github.com/grafana/ai-sdk/commit/79be97b02b9e02ec1e8800d1e3553acc23cabb2b))
* sum all usage counters and send tool_choice none ([#374](https://github.com/grafana/ai-sdk/issues/374)) ([ddaa841](https://github.com/grafana/ai-sdk/commit/ddaa841aeb854d86df41ccd5ce33e53b3ffea705))
