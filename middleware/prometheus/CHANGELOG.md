# Changelog

## [0.1.0-alpha.3](https://github.com/grafana/ai-sdk/compare/middleware/prometheus/v0.1.0-alpha.2...middleware/prometheus/v0.1.0-alpha.3) (2026-10-30)


### ⚠ BREAKING CHANGES

* **middleware:** A stream recording is no longer enqueued before its first `finish` is forwarded. Consumers that close or shut down the shared agento11y client after reading a `finish` must drain the stream to EOF or wait for `RecordingOptions.OnRecordComplete`. A consumer that stops at `finish` and cancels still records a successful generation. Stream usage in logs, metrics and Agent Observability now comes from the last `finish`: a finish reporting a lower counter than an earlier part of the same response wins, and a multi-finish stream reports the last finish's usage instead of the greatest value across the stream.

### Features

* **middleware:** record every response segment of one stream ([#614](https://github.com/grafana/ai-sdk/issues/614)) ([03c01b2](https://github.com/grafana/ai-sdk/commit/03c01b2a81c1a16b052de171fe86a292dd69424b))
* **parity:** advance pinned upstream to ai 7.0.153 ([#610](https://github.com/grafana/ai-sdk/issues/610)) ([fd3f743](https://github.com/grafana/ai-sdk/commit/fd3f743a213e3f049e7ffbfe65025f58a182867b))

## [0.1.0-alpha.2](https://github.com/grafana/ai-sdk/compare/middleware/prometheus/v0.1.0-alpha.1...middleware/prometheus/v0.1.0-alpha.2) (2026-10-28)


### Bug Fixes

* **ai-gateway:** contain panics in stream worker goroutines ([#471](https://github.com/grafana/ai-sdk/issues/471)) ([e74f87b](https://github.com/grafana/ai-sdk/commit/e74f87bcf30f2a9fccdc900f5d4d6ecdb06c8ec6)), refs [#419](https://github.com/grafana/ai-sdk/issues/419)

## 0.1.0-alpha.1 (2026-10-26)


### Features

* **build:** add merged-pin and Gateway source foundations ([#249](https://github.com/grafana/ai-sdk/issues/249)) ([d41abb4](https://github.com/grafana/ai-sdk/commit/d41abb420ba241dac60adf517f2ca6e2751b19c6))


### Bug Fixes

* **deps:** require Go 1.27.3 for x509 security fixes ([#475](https://github.com/grafana/ai-sdk/issues/475)) ([f0df9ab](https://github.com/grafana/ai-sdk/commit/f0df9ab9ff3aba1d11c04acaeb385a7754e58fab))
* **deps:** update go module minor updates ([#338](https://github.com/grafana/ai-sdk/issues/338)) ([79acadf](https://github.com/grafana/ai-sdk/commit/79acadf9b1ee4149a83b4129a2bfac13fb723abb))
* **deps:** update go module minor updates ([#408](https://github.com/grafana/ai-sdk/issues/408)) ([1ce44d9](https://github.com/grafana/ai-sdk/commit/1ce44d9e07e646c178c5797289a7830d2664a571))
* **deps:** update go module minor updates ([#56](https://github.com/grafana/ai-sdk/issues/56)) ([68d3913](https://github.com/grafana/ai-sdk/commit/68d3913a085886f5f62a2163163b9286a1edbaac))
* **deps:** update go module patch updates ([#432](https://github.com/grafana/ai-sdk/issues/432)) ([cf2e708](https://github.com/grafana/ai-sdk/commit/cf2e708a9e1f013975e775d89303692996eee71c))
* **deps:** update module github.com/grafana/ai-sdk to v0.1.0-alpha.2 ([#523](https://github.com/grafana/ai-sdk/issues/523)) ([83c01e9](https://github.com/grafana/ai-sdk/commit/83c01e9a1e40ac9a4473349fbd165b5f609a52a9))
