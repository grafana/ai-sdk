## 1. Client request contract

- [x] 1.1 Add white-box, testify-based transport tests for exact catalog discovery, unary, and streaming routes. Exercise a shared client/model with the same catalog name across two stacks, multiple catalogs in one stack, and a call without a catalog. Assert that the exchange gets the correct namespace and subject and that the Gateway receives only the resulting access token. Assert that a catalog error does not fall back to a namespace route.
- [x] 1.2 Cover a configured namespace (default and explicit match), mismatch and missing call namespace, and wildcard behavior (service and delegated user). Assert zero exchange and Gateway requests on invalid input, including on both direct credential flows. Check that namespaced token-exchange base URLs are still refused and catalog-prefixed direct calls still work.
- [x] 1.3 Cover rejection of empty names, dot-segments and slash-containing names. Verify that other characters are escaped as one segment without client-side resource-name validation. Snapshot the emitted model request with a literal JSON expectation and check the method, path, streaming and model headers, and content negotiation.
- [x] 1.4 Add the context helper and implement validation and prefix construction in the common request path. Keep namespace resolution, authentication headers, exchange/cache behavior, and request/response encoding unchanged.

## 2. Documentation

- [x] 2.1 Update package godoc and the Go provider guide. In the authentication guide, show an app backend composing `WithNamespace`, `WithOnBehalfOf`, and `WithCatalog`, and using that context for both `ListModels` and generation. Contrast with the service-account and caller-managed access-token catalog base URL. Explain that Grafana's catalog route access requirements still apply.
- [x] 2.2 Qualify the fixed-prefix guidance in the Gateway README and glossary. Check the architecture walkthrough for drift; the server-side route and access flow does not change.

## 3. Verification

- [x] 3.1 Run the Grafana provider module tests and `mise run test-providerwire-v4` against the registered baseline. Separately run the fixed-prefix stack-catalog cases of `gateway-command.test.ts` (`mise run test-ai-gateway-command` or a focused run). Keep the existing goldens and cases unchanged unless a wire contract regression is found.
- [x] 3.2 Run available local validation: Grafana provider checks, repository checks, parity checks, and doc lint. Where sandboxed Git blocks wrapper tasks, run the independent local checks directly. Leave the module boundary check, upstream checkout-dependent coverage and shape checks, and pinned Grafana plugin E2E to CI; the Go transport tests mock token exchange rather than checking live platform authorization.
