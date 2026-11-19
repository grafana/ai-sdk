## 1. Client request contract

- [ ] 1.1 Add white-box, testify-based transport tests for exact catalog discovery, unary, and streaming routes. Exercise a shared client/model with the same catalog name across two stacks, multiple catalogs in one stack, and a call without a catalog. Assert that the exchange gets the correct namespace and subject and that the Gateway receives only the resulting access token. Assert that a catalog error does not fall back to a namespace route.
- [ ] 1.2 Cover a configured namespace (default and explicit match), mismatch and missing call namespace, and wildcard behavior (service and delegated user). Assert zero exchange and Gateway requests on invalid input, including on both direct credential flows. Check that namespaced token-exchange base URLs are still refused and catalog-prefixed direct calls still work.
- [ ] 1.3 Cover name boundaries and case preservation, including 253 bytes, a non-DNS valid name, and invalid empty, overlong, path, percent-encoded, query/fragment, dot-segment, whitespace, and non-ASCII values. Snapshot the emitted model request with a literal JSON expectation and check the method, path, streaming and model headers, and content negotiation.
- [ ] 1.4 Add the context helper and implement validation and prefix construction in the common request path. Keep namespace resolution, authentication headers, exchange/cache behavior, and request/response encoding unchanged.

## 2. Documentation

- [ ] 2.1 Update package godoc and the Go provider guide. In the authentication guide, show an app backend composing `WithNamespace`, `WithOnBehalfOf`, and `WithCatalog`, and using that context for both `ListModels` and generation. Contrast with the service-account and caller-managed access-token catalog base URL. Explain that Grafana's catalog route access requirements still apply.
- [ ] 2.2 Qualify the fixed-prefix guidance in the Gateway README and glossary. Check the architecture walkthrough for drift; the server-side route and access flow does not change.

## 3. Verification

- [ ] 3.1 Run the Grafana provider module tests and `mise run test-providerwire-v4` against the registered baseline. Keep the existing stock-client goldens and fixed-prefix catalog differential unchanged unless a wire contract regression is found.
- [ ] 3.2 Verify a representative non-DNS case-preserving name by using it in the existing Grafana catalog-route E2E coverage. Take platform E2E evidence from the pinned Grafana CI job, not a local Docker run. Report client transport evidence separately from platform authorization evidence.
