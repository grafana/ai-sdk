# Changelog

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/providers/openai/v0.1.0-alpha.1...providers/openai/v0.1.0-alpha.2) (2026-10-28)


### Features

* **openai:** align GPT-6 reasoning controls and effort updates with upstream ([#565](https://github.com/grafana/ai-sdk/issues/565)) ([590ecbb](https://github.com/grafana/ai-sdk/commit/590ecbb6f28215cbad0b0b6abc159faecca2b7a4))
* **parity:** advance pinned upstream to ai 7.0.143 ([#458](https://github.com/grafana/ai-sdk/issues/458)) ([f76495c](https://github.com/grafana/ai-sdk/commit/f76495c223918a2e879bc4c80cb0741528c2b565))


### Bug Fixes

* **ai-gateway:** contain panics in stream worker goroutines ([#471](https://github.com/grafana/ai-sdk/issues/471)) ([e74f87b](https://github.com/grafana/ai-sdk/commit/e74f87bcf30f2a9fccdc900f5d4d6ecdb06c8ec6)), refs [#419](https://github.com/grafana/ai-sdk/issues/419)
* **core:** align provider-executed approval handling with upstream ([#567](https://github.com/grafana/ai-sdk/issues/567)) ([8973575](https://github.com/grafana/ai-sdk/commit/89735754faeba9f7fbcc4a0e44c0aa74fc874ef6))
* **openai:** merge replayed reasoning summaries sharing one item ID ([#578](https://github.com/grafana/ai-sdk/issues/578)) ([f86d936](https://github.com/grafana/ai-sdk/commit/f86d936bf72d3d01359622f2672efaae2f68307b))
* **openai:** normalize lookaround patterns and recursive schema references ([#515](https://github.com/grafana/ai-sdk/issues/515)) ([7b81ec1](https://github.com/grafana/ai-sdk/commit/7b81ec1c75f87f76a25a8c6e683a9be8da7300b7))

## 0.1.0-alpha.1 (2026-10-26)


### ⚠ BREAKING CHANGES

* **ai-gateway:** Bedrock Converse now rejects custom tool-result content with an error wrapping `provider.ErrUnsupportedFunctionality` before any HTTP request when tools are active. It used to drop the entry with a warning. Upstream throws for it. Assistant custom history is now skipped without a warning.
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425))
* **gateway:** `provider.ResponseFormat.Name` is now `*string`. Omit it for the native default; use a pointer for an explicit name, including an empty one. Output helpers handle this presence distinction. Published module versions and release tags are unchanged.
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* **ai-gateway:** support custom content ([#423](https://github.com/grafana/ai-sdk/issues/423)) ([0e5f36b](https://github.com/grafana/ai-sdk/commit/0e5f36b036c53ef45635694406333d90bcf07cae))
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **gateway:** support structured output with candidate failure details ([#416](https://github.com/grafana/ai-sdk/issues/416)) ([5ef4ed4](https://github.com/grafana/ai-sdk/commit/5ef4ed403107b9b23a9a3bf789c231b8aef7f1d6))
* **gateway:** transport bounded reasoning content and continuation metadata ([#247](https://github.com/grafana/ai-sdk/issues/247)) ([566db5d](https://github.com/grafana/ai-sdk/commit/566db5d9df4f6c2360d7ffa963a7db8d1eb1b763))
* **openai:** support capability-gated async configuration ([#289](https://github.com/grafana/ai-sdk/issues/289)) ([4fe5c41](https://github.com/grafana/ai-sdk/commit/4fe5c418fb1a34d1b97649831a2415506a760ad7)), closes [#220](https://github.com/grafana/ai-sdk/issues/220)
* **provider:** align provider-tool contracts and client decoding ([#238](https://github.com/grafana/ai-sdk/issues/238)) ([fe1e4ac](https://github.com/grafana/ai-sdk/commit/fe1e4ac3b44f577a93f22b99636719d72c627c5b))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425)) ([307551a](https://github.com/grafana/ai-sdk/commit/307551a566076610500bf458c5461b015a9c6e11))


### Bug Fixes

* **bedrock:** avoid unsupported Mantle web-search source include ([#257](https://github.com/grafana/ai-sdk/issues/257)) ([ecdf797](https://github.com/grafana/ai-sdk/commit/ecdf797e75c57d648470da34fc30bf323190aa57))
* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#338](https://github.com/grafana/ai-sdk/issues/338)) ([79acadf](https://github.com/grafana/ai-sdk/commit/79acadf9b1ee4149a83b4129a2bfac13fb723abb))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#405](https://github.com/grafana/ai-sdk/issues/405)) ([6fd0007](https://github.com/grafana/ai-sdk/commit/6fd00076fac2d5bee797777751494f7f799a044d))
* **deps:** update go module patch updates ([#432](https://github.com/grafana/ai-sdk/issues/432)) ([cf2e708](https://github.com/grafana/ai-sdk/commit/cf2e708a9e1f013975e775d89303692996eee71c))
* **deps:** update module github.com/grafana/ai-sdk to v0.1.0-alpha.2 ([#523](https://github.com/grafana/ai-sdk/issues/523)) ([83c01e9](https://github.com/grafana/ai-sdk/commit/83c01e9a1e40ac9a4473349fbd165b5f609a52a9))
* **openai:** correct forced shell and tool-search choices ([#304](https://github.com/grafana/ai-sdk/issues/304)) ([ede541b](https://github.com/grafana/ai-sdk/commit/ede541b32b61da71c98dcee2e99abd85029b7586))
* **openai:** normalize Responses request schemas ([#250](https://github.com/grafana/ai-sdk/issues/250)) ([f246094](https://github.com/grafana/ai-sdk/commit/f2460946f6a1635e118785a1726c6ee0ea497edd))
* **openai:** preserve multipart function results and cache breakpoints ([#242](https://github.com/grafana/ai-sdk/issues/242)) ([8521891](https://github.com/grafana/ai-sdk/commit/8521891ea0110d6f09eae89dd5eeb85d417cf374)), closes [#221](https://github.com/grafana/ai-sdk/issues/221)
* **openai:** preserve reasoning summary overrides on Mantle Luna ([#311](https://github.com/grafana/ai-sdk/issues/311)) ([c029977](https://github.com/grafana/ai-sdk/commit/c029977691f75fd1cc1095e2d2d586ef4ac434a4))
* **openai:** resolve allowedTools against declared tool kinds ([#282](https://github.com/grafana/ai-sdk/issues/282)) ([177272c](https://github.com/grafana/ai-sdk/commit/177272c27200dea10247d64851808897a90f6921))
* **parity:** advance frozen reference to AI SDK 7.0.107 ([#230](https://github.com/grafana/ai-sdk/issues/230)) ([9fb535a](https://github.com/grafana/ai-sdk/commit/9fb535a028172fd48a73457c655d1389d6e6290c))
* **providers:** align provider stream parts with upstream ([#397](https://github.com/grafana/ai-sdk/issues/397)) ([17030fc](https://github.com/grafana/ai-sdk/commit/17030fc1ac1b4cf4d2038380dfcd14084ec65568))
* **providers:** preserve call headers and SDK transport metadata ([#306](https://github.com/grafana/ai-sdk/issues/306)) ([4ebcc14](https://github.com/grafana/ai-sdk/commit/4ebcc14943a9ad67121e0938c5dc08822204a919))
* **providers:** preserve native unary source titles through simulated streams ([#340](https://github.com/grafana/ai-sdk/issues/340)) ([465b024](https://github.com/grafana/ai-sdk/commit/465b0247092d8ee7f56e8a701022ead2f31c948b))
