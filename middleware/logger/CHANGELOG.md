# Changelog

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/middleware/logger/v0.1.0-alpha.1...middleware/logger/v0.1.0-alpha.2) (2026-10-28)


### Bug Fixes

* **ai-gateway:** contain panics in stream worker goroutines ([#471](https://github.com/grafana/ai-sdk/issues/471)) ([e74f87b](https://github.com/grafana/ai-sdk/commit/e74f87bcf30f2a9fccdc900f5d4d6ecdb06c8ec6)), refs [#419](https://github.com/grafana/ai-sdk/issues/419)

## 0.1.0-alpha.1 (2026-10-26)


### ⚠ BREAKING CHANGES

* **grafana:** harden authentication headers and BYOK capture ([#367](https://github.com/grafana/ai-sdk/issues/367))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* **ai-gateway:** preserve normalized URL and document sources ([#246](https://github.com/grafana/ai-sdk/issues/246)) ([5486e79](https://github.com/grafana/ai-sdk/commit/5486e7925a120ec8b443535510d947f673a5ec63))
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **grafana:** harden authentication headers and BYOK capture ([#367](https://github.com/grafana/ai-sdk/issues/367)) ([bb0cacc](https://github.com/grafana/ai-sdk/commit/bb0cacce07ab19e8018112902ddd0ee3806e00bb))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))


### Bug Fixes

* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update module github.com/grafana/ai-sdk to v0.1.0-alpha.2 ([#523](https://github.com/grafana/ai-sdk/issues/523)) ([83c01e9](https://github.com/grafana/ai-sdk/commit/83c01e9a1e40ac9a4473349fbd165b5f609a52a9))
