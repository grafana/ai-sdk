## 1. Map provider options and call headers

- [x] 1.1 Map call, message and text-part provider options to `provider.RawProviderOption` in `ai-gateway/providerwire/v4/request.go`, rejecting a namespace that is not a JSON object.
- [x] 1.2 Map body-carried call headers, preserving key case, dropping protocol names, and rejecting names that differ only in case.
- [x] 1.3 Retire the `provider-options` and `body-headers` documents and register `reserved provider option namespace`, `protected call header` and `protected provider option` in `schema/error.json`.

## 2. Refuse what the host owns

- [x] 2.1 Reject the reserved `grafana` namespace before resolution, and reject an openai-compatible `providerName` resolving to it in `internal/config/file.go`.
- [x] 2.2 Refuse credential-bearing body headers through `IsProtectedCallHeader`.
- [x] 2.3 Refuse protected provider-option fields after folding case and separators.

## 3. Forward only what the selected backend reads

- [x] 3.1 Add `catalog.ProviderOptionPolicy` to `ResolvedModel`, `StaticEntry` and `RegistryRoute`, copied defensively; the zero value forwards none.
- [x] 3.2 Apply the policy after resolution at call, message and part level in `providerwire/v4/provider_option_policy.go`.
- [x] 3.3 Set Anthropic and openai-compatible policies in `internal/service/catalog.go` from `internal/service/provider_options.go`.

## 4. Verify

- [x] 4.1 Cover opaque value survival, malformed and reserved namespaces, header case and duplicates, protected headers, and protected fields at call, message, system and part level, including case and separator variants.
- [x] 4.2 Drive the inbound Cloud edge with a valid stack assertion and a negative control, and assert every refused name is a protected call header.
- [x] 4.3 Cover selected-backend forwarding, field removal with byte preservation, and the zero-value policy in `provider_options_test.go`.
- [x] 4.4 Fail on unclassified typed Anthropic option fields, and check the openai-compatible policy against the real provider.
- [x] 4.5 Assert exact refusal documents in `gateway-command.test.ts` and update the golden replay expectations.
- [x] 4.6 Run `mise run test-ai-gateway`, `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, `mise run verify-ai-gateway-boundary` and `mise run lint-docs`.
- [x] 4.7 Update the unary runtime specification, the Cloud authentication specification and the `PARITY.md` rows.
