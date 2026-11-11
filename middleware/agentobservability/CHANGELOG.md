# Changelog

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/middleware/agentobservability/v0.1.0-alpha.1...middleware/agentobservability/v0.1.0-alpha.2) (2026-10-28)


### Bug Fixes

* **ai-gateway:** contain panics in stream worker goroutines ([#471](https://github.com/grafana/ai-sdk/issues/471)) ([e74f87b](https://github.com/grafana/ai-sdk/commit/e74f87bcf30f2a9fccdc900f5d4d6ecdb06c8ec6)), refs [#419](https://github.com/grafana/ai-sdk/issues/419)

## 0.1.0-alpha.1 (2026-10-26)


### ⚠ BREAKING CHANGES

* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425))
* **grafana:** harden authentication headers and BYOK capture ([#367](https://github.com/grafana/ai-sdk/issues/367))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234))

### Features

* **ai-gateway:** preserve normalized URL and document sources ([#246](https://github.com/grafana/ai-sdk/issues/246)) ([5486e79](https://github.com/grafana/ai-sdk/commit/5486e7925a120ec8b443535510d947f673a5ec63))
* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))
* **grafana:** harden authentication headers and BYOK capture ([#367](https://github.com/grafana/ai-sdk/issues/367)) ([bb0cacc](https://github.com/grafana/ai-sdk/commit/bb0cacce07ab19e8018112902ddd0ee3806e00bb))
* **providers:** support Claude Sonnet 5.5 ([#276](https://github.com/grafana/ai-sdk/issues/276)) ([e21f9fe](https://github.com/grafana/ai-sdk/commit/e21f9fe7626ae33e5af7ddbabcc4986dc518dfe9))
* **sdk:** preserve selected file inputs across providers ([#234](https://github.com/grafana/ai-sdk/issues/234)) ([0aa53d8](https://github.com/grafana/ai-sdk/commit/0aa53d8f6dcbb265611897863b29b58684183254))
* support Gateway tool approvals and continuation ([#425](https://github.com/grafana/ai-sdk/issues/425)) ([307551a](https://github.com/grafana/ai-sdk/commit/307551a566076610500bf458c5461b015a9c6e11))


### Bug Fixes

* **agentobservability:** mark inclusive input usage ([#266](https://github.com/grafana/ai-sdk/issues/266)) ([3ab55d8](https://github.com/grafana/ai-sdk/commit/3ab55d88a2f47cdc73fb13f2d0bfe91667359a2b))
* **agentobservability:** record a finished stream before forwarding its finish part ([#438](https://github.com/grafana/ai-sdk/issues/438)) ([c11bbfa](https://github.com/grafana/ai-sdk/commit/c11bbfa6b465ee2b3491540d1521e049800c2e3b))
* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update agento11y bumps (require conformance review) to v0.18.0 ([#55](https://github.com/grafana/ai-sdk/issues/55)) ([047a0a1](https://github.com/grafana/ai-sdk/commit/047a0a1677b042d785348e55816d6b0b0d7574cd))
* **deps:** update agento11y bumps (require conformance review) to v0.19.0 ([#434](https://github.com/grafana/ai-sdk/issues/434)) ([4bbfc54](https://github.com/grafana/ai-sdk/commit/4bbfc541ded440f4a8609a8585bcebf8e5171993))
* **deps:** update go module minor updates ([#338](https://github.com/grafana/ai-sdk/issues/338)) ([79acadf](https://github.com/grafana/ai-sdk/commit/79acadf9b1ee4149a83b4129a2bfac13fb723abb))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#405](https://github.com/grafana/ai-sdk/issues/405)) ([6fd0007](https://github.com/grafana/ai-sdk/commit/6fd00076fac2d5bee797777751494f7f799a044d))
* **deps:** update module github.com/grafana/ai-sdk to v0.1.0-alpha.2 ([#523](https://github.com/grafana/ai-sdk/issues/523)) ([83c01e9](https://github.com/grafana/ai-sdk/commit/83c01e9a1e40ac9a4473349fbd165b5f609a52a9))
* **security/unknown/middleware/agentobservability:** update module golang.org/x/net to v0.60.0 [security] ([#383](https://github.com/grafana/ai-sdk/issues/383)) ([b516a88](https://github.com/grafana/ai-sdk/commit/b516a889ac92a3bfd43c49f3cef281117d73c687))
