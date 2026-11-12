# Tasks

## 1. Egress policy and client

- [x] 1.1 Add `outbound.EgressPolicy`, `ParseEgressList` and the address classification (refused ranges, embedded IPv4, never-dialable, precedence, hostname and wildcard matching); verify with the `classify`, parse and precedence table tests.
- [x] 1.2 Add `execution.ErrEgressRefused` and `outbound.NewEgressClient` (no proxy, host check in `DialContext`, `Control` address check, sanitized refusal and DNS errors, `https`-only `RoundTrip`, shared bounds, redirect refusal, own pool) over a shared transport constructor; verify with dial, rebinding, mixed-record, numeric-host, redirect, `http` and error-text tests using the in-memory resolver and loopback TLS servers.
- [x] 1.3 Add `policy.CheckHost` and the host-form check for static use; verify with literal, numeric-form, zone and deny-hostname tests.

## 2. Settings

- [x] 2.1 Parse `stack_catalog_egress_allow` and `stack_catalog_egress_deny` into `config.Settings`; verify with settings tests for unset, valid, invalid and oversized lists, with errors that name the setting without its value.

## 3. Stack-catalog checks

- [x] 3.1 Accept `openai-compatible` and the endpoint and compatible fields in `stackcatalog.checkPolicy` and `convert`. Add `baseURL` form, `https` and static egress checks with the policy as a `Parse` parameter. Verify with `Parse` table tests and field-path messages that don't echo the URL.
- [x] 3.2 Give `stackcatalog.Validator` the policy and wire it through `Gateway` and `managedKinds`; verify with validator tests for an accepted custom endpoint, a deny-listed host and an allow-listed private literal.

## 4. Runtime

- [x] 4.1 Pass `BaseURL`, `ProviderName` and `SupportsStructuredOutputs` into stack candidates. Choose the egress client when `BaseURL` is set, through `StackCatalogOptions.EgressClient`, and pass the policy to `parseStackCatalog`. Verify with `service` tests for client choice, the compatible fields and a stricter policy that maps to catalog configuration.
- [x] 4.2 Share the provider-slug rule between `newCandidate`, `stackIndex.resolve` and `StackCatalogLister`; verify with discovery and `only`/`order` tests for a compatible `providerName`.
- [x] 4.3 Add `CategoryEgressRefused` to `ClassifyProvider` and `ClassifyStreamPart`, and its fixed ProviderWire document. Verify with classification tests through the `openai-compatible`, Anthropic SDK and OpenAI SDK error paths, and with unary and stream-setup document bytes.
- [x] 4.4 Construct the egress client in `gateway.newRouteHandler`; verify with harness tests: a custom endpoint reached through the test host with an allow entry, the refused-then-fallback overview without addresses, pool isolation, and an operator catalog on loopback HTTP that is unchanged.

## 5. Cross-language and end-to-end evidence

- [x] 5.1 Add a `test/providerwire-v4` scenario: both clients receive the egress document, as not retryable, for a catalog whose endpoint is a refused loopback listener, which gets zero requests; verify with `mise run test-providerwire-v4`.
- [x] 5.2 Add the e2e infrastructure: certificate SANs, a TLS listener and per-host counts on `fake-anthropic`, the `egress-refused` and `egress-denied` aliases, and the allow and deny settings in `docker-compose.e2e.yaml`. Verify with `docker compose -f docker-compose.yaml -f docker-compose.e2e.yaml config`.
- [x] 5.3 Add the Playwright specs for custom endpoint success, refusal, deny precedence, admission refusals and acceptance of a compatible catalog, and update `catalog.spec.ts`; verify in CI's Playwright job.

## 6. Documentation

- [x] 6.1 Update `ai-gateway/README.md`: stack catalog fields and types, the custom endpoint guidance, the two settings, the egress document and the no-proxy limit. Check the architecture walkthrough and glossary for outbound drift. Verify with `mise run lint-docs`.

## 7. Integration checks

- [x] 7.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-providerwire-v4`, `test-integration`, `verify-ai-gateway-boundary`, `lint-docs` and `openspec validate add-stack-catalog-egress-policy --strict`, with `GOFLAGS=-buildvcs=false`.
