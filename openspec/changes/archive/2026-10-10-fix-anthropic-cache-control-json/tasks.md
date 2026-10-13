# Tasks

## 1. Cache-control serialization

- [x] 1.1 Add JSON and native request regression tests covering no TTL, 5m and 1h, system/message/part/tool locations, unary and streaming conversion; confirm failure before the fix.
- [x] 1.2 Implement canonical helper serialization and compatible typed decoding, document the wire behavior in godoc, and verify the regression tests pass.

## 2. Integration verification

- [x] 2.1 Run Anthropic tests with race detection, provider/root wire tests, parity-check components, formatting/lint checks, and strict OpenSpec validation; record any existing or environmental limitations.

Validation passed with Go 1.27.1, Node 24.18.1, pnpm 12.8.2, OpenSpec 1.14.1, and golangci-lint 2.14.0. Inspected mise task equivalents ran directly without changing trust settings. All modules and examples built, passed short tests and vet; all tracked modules passed lint. Root and Anthropic race tests, baseline validation and 109 tooling tests, conformance inventory, provider shape, typecheck, recorded conformance replays, and Go/TypeScript ProviderWire V4 contracts passed. TypeScript contracts required a canonical temporary path on macOS to keep Go and Node module locations consistent. A separate local synthetic HTTP witness preserved caching through the public Grafana client, Gateway V4 decoder, and Anthropic native transport in all 12 location/TTL combinations. No live provider acceptance or new recorded fixture is claimed.

## Workflow follow-up

- Sync and archive the completed change, then validate the archived specification.
- Prepare a signed commit and review packet before publication.
