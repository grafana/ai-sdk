## 1. Provider integration seam

- [x] 1.1 Add a preconfigured-client Responses constructor and verify existing direct OpenAI construction remains unchanged
- [x] 1.2 Add provider identity configuration and verify `Provider()` changes while response metadata retains the OpenAI/Azure continuation namespace
- [x] 1.3 Verify configured client endpoint, authentication, headers, retries, and transport reach model calls
- [x] 1.4 Add a two-call regression test verifying custom provider identity preserves stored item references
- [x] 1.5 Ignore empty provider-name overrides and verify the default identity is preserved

## 2. Parity and validation

- [x] 2.1 Record the dedicated upstream Bedrock Mantle provider as an explicit parity gap and verify baseline validation passes
- [x] 2.2 Update godoc and OpenSpec to describe the provider-integration boundary and verify documentation checks pass
- [x] 2.3 Run focused tests, vet, lint, formatting, and diff checks
