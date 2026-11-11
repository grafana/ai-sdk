# Changelog

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/providers/bedrock/v0.1.0-alpha.1...providers/bedrock/v0.1.0-alpha.2) (2026-10-28)


### Bug Fixes

* **bedrock:** omit token limits with high Nova reasoning ([#516](https://github.com/grafana/ai-sdk/issues/516)) ([397ca5f](https://github.com/grafana/ai-sdk/commit/397ca5f53460d313db8eeae7b083d19fa6d488b6))

## 0.1.0-alpha.1 (2026-10-28)


### ⚠ BREAKING CHANGES

* **ai-gateway:** Bedrock Converse now rejects custom tool-result content with an error wrapping `provider.ErrUnsupportedFunctionality` before any HTTP request when tools are active. It used to drop the entry with a warning. Upstream throws for it. Assistant custom history is now skipped without a warning.
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425))
* **gateway:** `provider.ResponseFormat.Name` is now `*string`. Omit it for the native default; use a pointer for an explicit name, including an empty one. Output helpers handle this presence distinction. Published module versions and release tags are unchanged.
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* **ai-gateway:** support custom content ([#423](https://github.com/grafana/ai-sdk/issues/423)) ([0e5f36b](https://github.com/grafana/ai-sdk/commit/0e5f36b036c53ef45635694406333d90bcf07cae))
* **bedrock:** support selective Converse guard content ([#283](https://github.com/grafana/ai-sdk/issues/283)) ([c775b39](https://github.com/grafana/ai-sdk/commit/c775b398b99151027425eb4910a47e3b7debd405)), refs [#223](https://github.com/grafana/ai-sdk/issues/223)
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **gateway:** support structured output with candidate failure details ([#416](https://github.com/grafana/ai-sdk/issues/416)) ([5ef4ed4](https://github.com/grafana/ai-sdk/commit/5ef4ed403107b9b23a9a3bf789c231b8aef7f1d6))
* **gateway:** transport bounded reasoning content and continuation metadata ([#247](https://github.com/grafana/ai-sdk/issues/247)) ([566db5d](https://github.com/grafana/ai-sdk/commit/566db5d9df4f6c2360d7ffa963a7db8d1eb1b763))
* **parity:** advance pinned upstream to ai 7.0.143 ([#458](https://github.com/grafana/ai-sdk/issues/458)) ([f76495c](https://github.com/grafana/ai-sdk/commit/f76495c223918a2e879bc4c80cb0741528c2b565))
* **provider:** align provider-tool contracts and client decoding ([#238](https://github.com/grafana/ai-sdk/issues/238)) ([fe1e4ac](https://github.com/grafana/ai-sdk/commit/fe1e4ac3b44f577a93f22b99636719d72c627c5b))
* **providers:** support Claude Sonnet 5.5 ([#276](https://github.com/grafana/ai-sdk/issues/276)) ([e21f9fe](https://github.com/grafana/ai-sdk/commit/e21f9fe7626ae33e5af7ddbabcc4986dc518dfe9))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425)) ([307551a](https://github.com/grafana/ai-sdk/commit/307551a566076610500bf458c5461b015a9c6e11))


### Bug Fixes

* **bedrock:** adopt published Mantle assistant-history correction ([#232](https://github.com/grafana/ai-sdk/issues/232)) ([256e9db](https://github.com/grafana/ai-sdk/commit/256e9dbef7ee38a898f742b34335b0ee7fe11ce1))
* **bedrock:** align Converse model-family and structured-output routing ([#243](https://github.com/grafana/ai-sdk/issues/243)) ([5625aa9](https://github.com/grafana/ai-sdk/commit/5625aa9ea93017aeb82d051c372616ede99f4274)), refs [#222](https://github.com/grafana/ai-sdk/issues/222)
* **bedrock:** avoid unsupported Mantle web-search source include ([#257](https://github.com/grafana/ai-sdk/issues/257)) ([ecdf797](https://github.com/grafana/ai-sdk/commit/ecdf797e75c57d648470da34fc30bf323190aa57))
* **bedrock:** convert assistant-role tool results like upstream ([#533](https://github.com/grafana/ai-sdk/issues/533)) ([d64f0b8](https://github.com/grafana/ai-sdk/commit/d64f0b8e9933846767de0fc5937fb7efdf310449))
* **bedrock:** discard cache points for omitted source content ([#415](https://github.com/grafana/ai-sdk/issues/415)) ([b1a6d75](https://github.com/grafana/ai-sdk/commit/b1a6d7575cbadecfa0f9f0f70d46ae1274cd250a))
* **bedrock:** reject Converse EOF before messageStop ([#412](https://github.com/grafana/ai-sdk/issues/412)) ([47055de](https://github.com/grafana/ai-sdk/commit/47055dee685ae30cfd656d61912e51361d3a07f2))
* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#338](https://github.com/grafana/ai-sdk/issues/338)) ([79acadf](https://github.com/grafana/ai-sdk/commit/79acadf9b1ee4149a83b4129a2bfac13fb723abb))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#405](https://github.com/grafana/ai-sdk/issues/405)) ([6fd0007](https://github.com/grafana/ai-sdk/commit/6fd00076fac2d5bee797777751494f7f799a044d))
* **deps:** update go module patch updates ([#432](https://github.com/grafana/ai-sdk/issues/432)) ([cf2e708](https://github.com/grafana/ai-sdk/commit/cf2e708a9e1f013975e775d89303692996eee71c))
* **deps:** update go module patch updates ([#528](https://github.com/grafana/ai-sdk/issues/528)) ([c3b7179](https://github.com/grafana/ai-sdk/commit/c3b7179b090411bd459e669cfb05bc34fce12834))
* **deps:** update module github.com/grafana/ai-sdk/providers/openai to v0.1.0-alpha.1 ([#512](https://github.com/grafana/ai-sdk/issues/512)) ([c36c490](https://github.com/grafana/ai-sdk/commit/c36c49080af71b4562da3a0a6c0e68f47b5e118f))
* **openai:** merge replayed reasoning summaries sharing one item ID ([#578](https://github.com/grafana/ai-sdk/issues/578)) ([f86d936](https://github.com/grafana/ai-sdk/commit/f86d936bf72d3d01359622f2672efaae2f68307b))
* **openai:** preserve reasoning summary overrides on Mantle Luna ([#311](https://github.com/grafana/ai-sdk/issues/311)) ([c029977](https://github.com/grafana/ai-sdk/commit/c029977691f75fd1cc1095e2d2d586ef4ac434a4))
* **providers:** align provider stream parts with upstream ([#397](https://github.com/grafana/ai-sdk/issues/397)) ([17030fc](https://github.com/grafana/ai-sdk/commit/17030fc1ac1b4cf4d2038380dfcd14084ec65568))
