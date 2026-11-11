# Changelog

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/providers/grafana/v0.1.0-alpha.1...providers/grafana/v0.1.0-alpha.2) (2026-10-28)


### ⚠ BREAKING CHANGES

* **ai-gateway:** delegate request limits to the platform and bound attempts once ([#532](https://github.com/grafana/ai-sdk/issues/532))
* **grafana:** remove client response limits ([#514](https://github.com/grafana/ai-sdk/issues/514))
* **ai-gateway:** run the AI Gateway as a Grafana App Platform plugin ([#428](https://github.com/grafana/ai-sdk/issues/428))

### Features

* **ai-gateway:** run the AI Gateway as a Grafana App Platform plugin ([#428](https://github.com/grafana/ai-sdk/issues/428)) ([1cc2e65](https://github.com/grafana/ai-sdk/commit/1cc2e65c264c830b5ddbd06379b471188b305873))
* **parity:** advance pinned upstream to ai 7.0.143 ([#458](https://github.com/grafana/ai-sdk/issues/458)) ([f76495c](https://github.com/grafana/ai-sdk/commit/f76495c223918a2e879bc4c80cb0741528c2b565))


### Bug Fixes

* **core:** align provider-executed approval handling with upstream ([#567](https://github.com/grafana/ai-sdk/issues/567)) ([8973575](https://github.com/grafana/ai-sdk/commit/89735754faeba9f7fbcc4a0e44c0aa74fc874ef6))


### Code Refactoring

* **ai-gateway:** delegate request limits to the platform and bound attempts once ([#532](https://github.com/grafana/ai-sdk/issues/532)) ([2250fcf](https://github.com/grafana/ai-sdk/commit/2250fcff7964ab174cd796196a15578cde978a27))
* **grafana:** remove client response limits ([#514](https://github.com/grafana/ai-sdk/issues/514)) ([4b17628](https://github.com/grafana/ai-sdk/commit/4b176287e0c58a87892ad60177604cf22b61a1ad))

## 0.1.0-alpha.1 (2026-10-26)


### ⚠ BREAKING CHANGES

* **ai-gateway:** Bedrock Converse now rejects custom tool-result content with an error wrapping `provider.ErrUnsupportedFunctionality` before any HTTP request when tools are active. It used to drop the entry with a warning. Upstream throws for it. Assistant custom history is now skipped without a warning.
* **ai-gateway:** isolate private JWT and Cloud BYOK access ([#369](https://github.com/grafana/ai-sdk/issues/369))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425))
* **gateway:** `provider.ResponseFormat.Name` is now `*string`. Omit it for the native default; use a pointer for an explicit name, including an empty one. Output helpers handle this presence distinction. Published module versions and release tags are unchanged.
* **ai-gateway:** add request-scoped BYOK execution ([#368](https://github.com/grafana/ai-sdk/issues/368))
* **grafana:** harden authentication headers and BYOK capture ([#367](https://github.com/grafana/ai-sdk/issues/367))
* **gateway:** preserve native response values ([#325](https://github.com/grafana/ai-sdk/issues/325))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* **ai-gateway:** add request-scoped BYOK execution ([#368](https://github.com/grafana/ai-sdk/issues/368)) ([5c9b4fa](https://github.com/grafana/ai-sdk/commit/5c9b4facb0a1ec31beb5dde5903dda8e6f416e8c))
* **ai-gateway:** expose authorized configured route candidates ([#329](https://github.com/grafana/ai-sdk/issues/329)) ([4934134](https://github.com/grafana/ai-sdk/commit/4934134825f1a5771b5d06e30fbccd652f7a1fcc))
* **ai-gateway:** expose compact execution overviews and failures ([#370](https://github.com/grafana/ai-sdk/issues/370)) ([2066055](https://github.com/grafana/ai-sdk/commit/2066055d56f3a74823d92364be02c396c19d1fbc))
* **ai-gateway:** isolate private JWT and Cloud BYOK access ([#369](https://github.com/grafana/ai-sdk/issues/369)) ([612a077](https://github.com/grafana/ai-sdk/commit/612a077dc089f506bac85b28301898d93f865a62))
* **ai-gateway:** preserve normalized URL and document sources ([#246](https://github.com/grafana/ai-sdk/issues/246)) ([5486e79](https://github.com/grafana/ai-sdk/commit/5486e7925a120ec8b443535510d947f673a5ec63))
* **ai-gateway:** preserve opaque provider metadata ([#332](https://github.com/grafana/ai-sdk/issues/332)) ([1be616c](https://github.com/grafana/ai-sdk/commit/1be616c38bebff6a2618f2fffe5d8e319fd57ab3))
* **ai-gateway:** return bounded provider raw usage ([#291](https://github.com/grafana/ai-sdk/issues/291)) ([a2c3768](https://github.com/grafana/ai-sdk/commit/a2c376839d17d2344638d5d0878dd8ed1614482b)), closes [#281](https://github.com/grafana/ai-sdk/issues/281)
* **ai-gateway:** support custom content ([#423](https://github.com/grafana/ai-sdk/issues/423)) ([0e5f36b](https://github.com/grafana/ai-sdk/commit/0e5f36b036c53ef45635694406333d90bcf07cae))
* **ai-gateway:** support generated file output ([#426](https://github.com/grafana/ai-sdk/issues/426)) ([e58ed8e](https://github.com/grafana/ai-sdk/commit/e58ed8e0c592f28c547cb5c8d7c582ff2c03141e))
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **gateway:** support structured output with candidate failure details ([#416](https://github.com/grafana/ai-sdk/issues/416)) ([5ef4ed4](https://github.com/grafana/ai-sdk/commit/5ef4ed403107b9b23a9a3bf789c231b8aef7f1d6))
* **gateway:** transport bounded reasoning content and continuation metadata ([#247](https://github.com/grafana/ai-sdk/issues/247)) ([566db5d](https://github.com/grafana/ai-sdk/commit/566db5d9df4f6c2360d7ffa963a7db8d1eb1b763))
* **grafana:** authenticate Cloud gateway with stack CAP credentials ([#233](https://github.com/grafana/ai-sdk/issues/233)) ([ff2a385](https://github.com/grafana/ai-sdk/commit/ff2a385c0c6ef062a7a8a4b8cdc6efe3f9e3c8c0))
* **grafana:** harden authentication headers and BYOK capture ([#367](https://github.com/grafana/ai-sdk/issues/367)) ([bb0cacc](https://github.com/grafana/ai-sdk/commit/bb0cacce07ab19e8018112902ddd0ee3806e00bb))
* **provider:** align provider-tool contracts and client decoding ([#238](https://github.com/grafana/ai-sdk/issues/238)) ([fe1e4ac](https://github.com/grafana/ai-sdk/commit/fe1e4ac3b44f577a93f22b99636719d72c627c5b))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425)) ([307551a](https://github.com/grafana/ai-sdk/commit/307551a566076610500bf458c5461b015a9c6e11))


### Bug Fixes

* align text-only tool choice across core and gateway ([#203](https://github.com/grafana/ai-sdk/issues/203)) ([1aa0127](https://github.com/grafana/ai-sdk/commit/1aa0127563defd8f85f569ce9720f0b7896b3e24))
* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#338](https://github.com/grafana/ai-sdk/issues/338)) ([79acadf](https://github.com/grafana/ai-sdk/commit/79acadf9b1ee4149a83b4129a2bfac13fb723abb))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#405](https://github.com/grafana/ai-sdk/issues/405)) ([6fd0007](https://github.com/grafana/ai-sdk/commit/6fd00076fac2d5bee797777751494f7f799a044d))
* **deps:** update go module patch updates ([#432](https://github.com/grafana/ai-sdk/issues/432)) ([cf2e708](https://github.com/grafana/ai-sdk/commit/cf2e708a9e1f013975e775d89303692996eee71c))
* **deps:** update module github.com/grafana/ai-sdk to v0.1.0-alpha.2 ([#523](https://github.com/grafana/ai-sdk/issues/523)) ([83c01e9](https://github.com/grafana/ai-sdk/commit/83c01e9a1e40ac9a4473349fbd165b5f609a52a9))
* **gateway:** preserve native response values ([#325](https://github.com/grafana/ai-sdk/issues/325)) ([f1019c8](https://github.com/grafana/ai-sdk/commit/f1019c8f347f6d969f19581b9f105dce0054b7d7))
* **grafana:** preserve retryability of post-header transport failures ([#252](https://github.com/grafana/ai-sdk/issues/252)) ([dd43c6d](https://github.com/grafana/ai-sdk/commit/dd43c6dff058adac9c5c56d6e2f5c3ec9700d5db))
* **grafana:** retain additive Gateway error data ([#372](https://github.com/grafana/ai-sdk/issues/372)) ([6fc1a38](https://github.com/grafana/ai-sdk/commit/6fc1a38dbd1185a27547ca9ffb7a00d794e9b81e))
* **parity:** advance frozen reference to AI SDK 7.0.107 ([#230](https://github.com/grafana/ai-sdk/issues/230)) ([9fb535a](https://github.com/grafana/ai-sdk/commit/9fb535a028172fd48a73457c655d1389d6e6290c))
* **security/unknown/providers/grafana:** update module golang.org/x/net to v0.60.0 [security] ([#385](https://github.com/grafana/ai-sdk/issues/385)) ([9573bc9](https://github.com/grafana/ai-sdk/commit/9573bc9294abfb6b1b2bc153cce11c74d485d08a))
