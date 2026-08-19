## 1. Compact Contract Fixtures

- [x] 1.1 Replace expanded schema-negative documents with named positive seeds and narrow mutation recipes.
- [x] 1.2 Replace repeated HTTP envelope documents with two curated seeds and narrow mutation recipes.
- [x] 1.3 Update Go contract tests to expand recipes in memory and preserve existing categories and validation-path assertions.

## 2. Shared Response Evidence

- [x] 2.1 Add test-only TypeScript helpers for the curated unary and clean-stream seeds and derived DONE and safe-error projections.
- [x] 2.2 Reuse shared projections in request-capture and response-consumption scenarios.
- [x] 2.3 Remove committed mechanical response variants and update Go projection validation.

## 3. Conformance Transport Seam

- [x] 3.1 Validate selected provider-independent conformance stream inputs against the ProviderWire stream schema.
- [x] 3.2 Render selected inputs as SSE, consume them through the pinned Gateway client, and compare semantic UI chunks with existing expectations.
- [x] 3.3 Record conformance source, direction, authority, claims, and non-claims in the fixture index and parity map.

## 4. Verification and Updates

- [x] 4.1 Make request-capture replacement atomic and expose it through `mise run update-providerwire-v4-artifacts`.
- [x] 4.2 Add the non-mutating `mise run check-providerwire-v4` aggregate and update CI and artifact metadata.
- [x] 4.3 Prove ordinary verification does not modify committed artifacts.

## 5. Validation

- [x] 5.1 Run formatting, focused Go and TypeScript tests, contract, interop, parity-baseline, and OpenSpec validation.
- [x] 5.2 Run parity review and architecture/provenance review, then resolve valid findings.
