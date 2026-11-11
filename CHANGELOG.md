# Changelog

## [0.1.0-alpha.5](https://github.com/grafana/ai-sdk/compare/v0.1.0-alpha.4...v0.1.0-alpha.5) (2026-10-28)


### Code Refactoring

* **ai-gateway:** register adapters through a host contract ([#581](https://github.com/grafana/ai-sdk/issues/581)) ([3c3730d](https://github.com/grafana/ai-sdk/commit/3c3730d335cc4537395b60d6134062572a4a0e85))

## [0.1.0-alpha.4](https://github.com/grafana/ai-sdk/compare/v0.1.0-alpha.3...v0.1.0-alpha.4) (2026-10-28)


### ⚠ BREAKING CHANGES

* **ai-gateway:** delegate request limits to the platform and bound attempts once ([#532](https://github.com/grafana/ai-sdk/issues/532))
* **ai-gateway:** run the AI Gateway as a Grafana App Platform plugin ([#428](https://github.com/grafana/ai-sdk/issues/428))
* **core:** project all steps and response history on results and OnFinish ([#531](https://github.com/grafana/ai-sdk/issues/531))

### Features

* **ai-gateway:** run the AI Gateway as a Grafana App Platform plugin ([#428](https://github.com/grafana/ai-sdk/issues/428)) ([1cc2e65](https://github.com/grafana/ai-sdk/commit/1cc2e65c264c830b5ddbd06379b471188b305873))
* **core:** project all steps and response history on results and OnFinish ([#531](https://github.com/grafana/ai-sdk/issues/531)) ([25de8d4](https://github.com/grafana/ai-sdk/commit/25de8d467233c2fc3007dbe3fa5712f2bc9eb0de))
* **parity:** advance pinned upstream to ai 7.0.143 ([#458](https://github.com/grafana/ai-sdk/issues/458)) ([f76495c](https://github.com/grafana/ai-sdk/commit/f76495c223918a2e879bc4c80cb0741528c2b565))


### Bug Fixes

* **core:** align provider-executed approval handling with upstream ([#567](https://github.com/grafana/ai-sdk/issues/567)) ([8973575](https://github.com/grafana/ai-sdk/commit/89735754faeba9f7fbcc4a0e44c0aa74fc874ef6))


### Code Refactoring

* **ai-gateway:** delegate request limits to the platform and bound attempts once ([#532](https://github.com/grafana/ai-sdk/issues/532)) ([2250fcf](https://github.com/grafana/ai-sdk/commit/2250fcff7964ab174cd796196a15578cde978a27))

## [0.1.0-alpha.3](https://github.com/grafana/ai-sdk/compare/v0.1.0-alpha.2...v0.1.0-alpha.3) (2026-10-27)


### ⚠ BREAKING CHANGES

* **output:** support bounded array outputs ([#526](https://github.com/grafana/ai-sdk/issues/526))

### Features

* **output:** support bounded array outputs ([#526](https://github.com/grafana/ai-sdk/issues/526)) ([904e766](https://github.com/grafana/ai-sdk/commit/904e766556fc8256de8e0fbd6c3e6eaeabac93ee))


### Bug Fixes

* **agent:** pass the runtime context to approved tools on resume ([#484](https://github.com/grafana/ai-sdk/issues/484)) ([842e854](https://github.com/grafana/ai-sdk/commit/842e854d177c3e90133565f98d92b253753e97b3))
* **ai-gateway:** contain panics in stream worker goroutines ([#471](https://github.com/grafana/ai-sdk/issues/471)) ([e74f87b](https://github.com/grafana/ai-sdk/commit/e74f87bcf30f2a9fccdc900f5d4d6ecdb06c8ec6)), refs [#419](https://github.com/grafana/ai-sdk/issues/419)
* **deps:** update npm non-major dependencies ([#536](https://github.com/grafana/ai-sdk/issues/536)) ([2b4f4a0](https://github.com/grafana/ai-sdk/commit/2b4f4a0d5fd21fcac272f4f6c0611db08bc6781a))

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/v0.1.0-alpha.1...v0.1.0-alpha.2) (2026-10-22)


### ⚠ BREAKING CHANGES

* **ai-gateway:** Bedrock Converse now rejects custom tool-result content with an error wrapping `provider.ErrUnsupportedFunctionality` before any HTTP request when tools are active. It used to drop the entry with a warning. Upstream throws for it. Assistant custom history is now skipped without a warning.
* **ai-gateway:** isolate private JWT and Cloud BYOK access ([#369](https://github.com/grafana/ai-sdk/issues/369))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425))
* **gateway:** `provider.ResponseFormat.Name` is now `*string`. Omit it for the native default; use a pointer for an explicit name, including an empty one. Output helpers handle this presence distinction. Published module versions and release tags are unchanged.
* **ai-gateway:** add request-scoped BYOK execution ([#368](https://github.com/grafana/ai-sdk/issues/368))
* **ui:** retain tool state across persistence and resume ([#310](https://github.com/grafana/ai-sdk/issues/310))
* **gateway:** preserve native response values ([#325](https://github.com/grafana/ai-sdk/issues/325))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* add shared fallback capture and compact Gateway execution overviews ([#373](https://github.com/grafana/ai-sdk/issues/373)) ([2cf33c3](https://github.com/grafana/ai-sdk/commit/2cf33c36db73e52bca3afde512ae03643250b00b))
* **ai-gateway:** add request-scoped BYOK execution ([#368](https://github.com/grafana/ai-sdk/issues/368)) ([5c9b4fa](https://github.com/grafana/ai-sdk/commit/5c9b4facb0a1ec31beb5dde5903dda8e6f416e8c))
* **ai-gateway:** expose compact execution overviews and failures ([#370](https://github.com/grafana/ai-sdk/issues/370)) ([2066055](https://github.com/grafana/ai-sdk/commit/2066055d56f3a74823d92364be02c396c19d1fbc))
* **ai-gateway:** isolate private JWT and Cloud BYOK access ([#369](https://github.com/grafana/ai-sdk/issues/369)) ([612a077](https://github.com/grafana/ai-sdk/commit/612a077dc089f506bac85b28301898d93f865a62))
* **ai-gateway:** preserve normalized URL and document sources ([#246](https://github.com/grafana/ai-sdk/issues/246)) ([5486e79](https://github.com/grafana/ai-sdk/commit/5486e7925a120ec8b443535510d947f673a5ec63))
* **ai-gateway:** preserve opaque provider metadata ([#332](https://github.com/grafana/ai-sdk/issues/332)) ([1be616c](https://github.com/grafana/ai-sdk/commit/1be616c38bebff6a2618f2fffe5d8e319fd57ab3))
* **ai-gateway:** support custom content ([#423](https://github.com/grafana/ai-sdk/issues/423)) ([0e5f36b](https://github.com/grafana/ai-sdk/commit/0e5f36b036c53ef45635694406333d90bcf07cae))
* **ai-gateway:** support generated file output ([#426](https://github.com/grafana/ai-sdk/issues/426)) ([e58ed8e](https://github.com/grafana/ai-sdk/commit/e58ed8e0c592f28c547cb5c8d7c582ff2c03141e))
* **ai-gateway:** support mapped capabilities through fallback ([#328](https://github.com/grafana/ai-sdk/issues/328)) ([d863b86](https://github.com/grafana/ai-sdk/commit/d863b86eed8208fab909fa9c53a0a74e5b8f5827))
* **anthropic:** expose opt-in safeguards and verdict metadata ([#288](https://github.com/grafana/ai-sdk/issues/288)) ([86fc7d0](https://github.com/grafana/ai-sdk/commit/86fc7d0dc2a7841d16026be875c2f69cefc78b08))
* **azure:** add reusable Foundry Claude provider ([#354](https://github.com/grafana/ai-sdk/issues/354)) ([9c4110d](https://github.com/grafana/ai-sdk/commit/9c4110d3b09a0644dd86a6b83fa64641481f4635))
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **core:** add caller-aware tool routing and streamed results ([#290](https://github.com/grafana/ai-sdk/issues/290)) ([9571d90](https://github.com/grafana/ai-sdk/commit/9571d905bafe6a07a1c85e1550de8df73de7c169))
* **core:** discover deferred local tools across model steps ([#308](https://github.com/grafana/ai-sdk/issues/308)) ([19d618d](https://github.com/grafana/ai-sdk/commit/19d618db192a15be6596173e33f3398be3969fcc))
* **core:** stop on any named tool call ([#347](https://github.com/grafana/ai-sdk/issues/347)) ([018d506](https://github.com/grafana/ai-sdk/commit/018d506ebffc9972e098f0f362244fbf45e72e5f))
* **fallback:** support bounded complete-stream failover ([#424](https://github.com/grafana/ai-sdk/issues/424)) ([9a9b8f2](https://github.com/grafana/ai-sdk/commit/9a9b8f20081d1b4727a39d1a3ee722fbaaadb1b8))
* **gateway:** support structured output with candidate failure details ([#416](https://github.com/grafana/ai-sdk/issues/416)) ([5ef4ed4](https://github.com/grafana/ai-sdk/commit/5ef4ed403107b9b23a9a3bf789c231b8aef7f1d6))
* **gateway:** transport bounded reasoning content and continuation metadata ([#247](https://github.com/grafana/ai-sdk/issues/247)) ([566db5d](https://github.com/grafana/ai-sdk/commit/566db5d9df4f6c2360d7ffa963a7db8d1eb1b763))
* **openai:** support capability-gated async configuration ([#289](https://github.com/grafana/ai-sdk/issues/289)) ([4fe5c41](https://github.com/grafana/ai-sdk/commit/4fe5c418fb1a34d1b97649831a2415506a760ad7)), closes [#220](https://github.com/grafana/ai-sdk/issues/220)
* **output:** repair invalid structured output on demand ([#286](https://github.com/grafana/ai-sdk/issues/286)) ([8845029](https://github.com/grafana/ai-sdk/commit/88450296f3b4c196ee2d5763a651fc9c8a444d7e)), closes [#260](https://github.com/grafana/ai-sdk/issues/260)
* **parity:** add frozen-target upgrade workflow ([#205](https://github.com/grafana/ai-sdk/issues/205)) ([f30e989](https://github.com/grafana/ai-sdk/commit/f30e9899007a1b56fb963bb4cf1345213ceb30ed))
* **provider:** align provider-tool contracts and client decoding ([#238](https://github.com/grafana/ai-sdk/issues/238)) ([fe1e4ac](https://github.com/grafana/ai-sdk/commit/fe1e4ac3b44f577a93f22b99636719d72c627c5b))
* **providers:** support Claude Sonnet 5.5 ([#276](https://github.com/grafana/ai-sdk/issues/276)) ([e21f9fe](https://github.com/grafana/ai-sdk/commit/e21f9fe7626ae33e5af7ddbabcc4986dc518dfe9))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425)) ([307551a](https://github.com/grafana/ai-sdk/commit/307551a566076610500bf458c5461b015a9c6e11))


### Bug Fixes

* **agent:** honor per-call tool approval secret and text repair ([51330b5](https://github.com/grafana/ai-sdk/commit/51330b5ad49cd2cc85537bc62b876f9d148585db))
* **agentobservability:** mark inclusive input usage ([#266](https://github.com/grafana/ai-sdk/issues/266)) ([3ab55d8](https://github.com/grafana/ai-sdk/commit/3ab55d88a2f47cdc73fb13f2d0bfe91667359a2b))
* align text-only tool choice across core and gateway ([#203](https://github.com/grafana/ai-sdk/issues/203)) ([1aa0127](https://github.com/grafana/ai-sdk/commit/1aa0127563defd8f85f569ce9720f0b7896b3e24))
* **bedrock:** align Converse model-family and structured-output routing ([#243](https://github.com/grafana/ai-sdk/issues/243)) ([5625aa9](https://github.com/grafana/ai-sdk/commit/5625aa9ea93017aeb82d051c372616ede99f4274)), closes [#222](https://github.com/grafana/ai-sdk/issues/222)
* **core:** reject responses violating effective tool choice ([#305](https://github.com/grafana/ai-sdk/issues/305)) ([c6e9965](https://github.com/grafana/ai-sdk/commit/c6e996586eddf0846e9317d872f34239bc4976d7))
* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#405](https://github.com/grafana/ai-sdk/issues/405)) ([6fd0007](https://github.com/grafana/ai-sdk/commit/6fd00076fac2d5bee797777751494f7f799a044d))
* **deps:** update npm non-major dependencies ([27591bb](https://github.com/grafana/ai-sdk/commit/27591bba04e1047b7d8d9fa5cbf9b07bee317bdd))
* **deps:** update npm non-major dependencies ([#271](https://github.com/grafana/ai-sdk/issues/271)) ([9090c47](https://github.com/grafana/ai-sdk/commit/9090c472c72f686bcf5294cf0c9c24a58a11f3f0))
* **gateway:** preserve native response values ([#325](https://github.com/grafana/ai-sdk/issues/325)) ([f1019c8](https://github.com/grafana/ai-sdk/commit/f1019c8f347f6d969f19581b9f105dce0054b7d7))
* **middleware:** preserve generated content in simulated streams ([#241](https://github.com/grafana/ai-sdk/issues/241)) ([cf61a18](https://github.com/grafana/ai-sdk/commit/cf61a18d956f7339d438e75e9795c9ab5dae0f8a))
* **middleware:** preserve overlapping reasoning and text stream blocks ([#284](https://github.com/grafana/ai-sdk/issues/284)) ([f8a3a84](https://github.com/grafana/ai-sdk/commit/f8a3a847532a2c4efd2d41cf97b9218010b9056e))
* **output:** align wrapped parsing and partial choice presence with upstream ([#339](https://github.com/grafana/ai-sdk/issues/339)) ([115a303](https://github.com/grafana/ai-sdk/commit/115a3032458dfd0e02a5b2995f39b88bb8f21def))
* **output:** parse complete generated output on non-stop finishes ([#285](https://github.com/grafana/ai-sdk/issues/285)) ([12aa5e6](https://github.com/grafana/ai-sdk/commit/12aa5e61ee0abbe65bf401147b6c588e81f060e7)), closes [#226](https://github.com/grafana/ai-sdk/issues/226)
* **parity:** advance frozen reference to AI SDK 7.0.107 ([#230](https://github.com/grafana/ai-sdk/issues/230)) ([9fb535a](https://github.com/grafana/ai-sdk/commit/9fb535a028172fd48a73457c655d1389d6e6290c))
* **parity:** align continuation with AI SDK 7.0.127 ([#365](https://github.com/grafana/ai-sdk/issues/365)) ([28e08a4](https://github.com/grafana/ai-sdk/commit/28e08a459341bf0e50de3980cb534dc59d488096))
* **providers:** align provider stream parts with upstream ([#397](https://github.com/grafana/ai-sdk/issues/397)) ([17030fc](https://github.com/grafana/ai-sdk/commit/17030fc1ac1b4cf4d2038380dfcd14084ec65568))
* **providers:** preserve call headers and SDK transport metadata ([#306](https://github.com/grafana/ai-sdk/issues/306)) ([4ebcc14](https://github.com/grafana/ai-sdk/commit/4ebcc14943a9ad67121e0938c5dc08822204a919))
* **security/medium/ai-gateway/test/providerwire-v4:** update dependency ajv to v8.18.0 [security] ([#163](https://github.com/grafana/ai-sdk/issues/163)) ([c0ab5b8](https://github.com/grafana/ai-sdk/commit/c0ab5b88a7c6e6886c28848d0a9c9d06a00b8d76))
* **streamtext:** keep step tool results in call order for mixed steps ([#348](https://github.com/grafana/ai-sdk/issues/348)) ([69bdfa0](https://github.com/grafana/ai-sdk/commit/69bdfa02503c96256c0f23594ad396542de3c374))
* **streamtext:** let cancellation finish a stream nobody is reading ([#345](https://github.com/grafana/ai-sdk/issues/345)) ([f14efec](https://github.com/grafana/ai-sdk/commit/f14efecc00b3ac7369ce8f31a6b13c9ee4391904))
* sum all usage counters and send tool_choice none ([#374](https://github.com/grafana/ai-sdk/issues/374)) ([ddaa841](https://github.com/grafana/ai-sdk/commit/ddaa841aeb854d86df41ccd5ce33e53b3ffea705))
* **ui:** retain tool state across persistence and resume ([#310](https://github.com/grafana/ai-sdk/issues/310)) ([66d429f](https://github.com/grafana/ai-sdk/commit/66d429ff0db08b0c0a0bac12aea6c53a8b3f8949))

## 0.1.0-alpha.1 (2026-07-28)

Initial alpha release of the Grafana AI SDK for Go.
