## 1. Host-safe ProviderWire errors

- [x] 1.1 Add `ai-gateway/providerwire/v4` tests for exact fixed authentication, permission, and internal documents plus invalid-category internal fallback without runtime schema or byte-limit behavior.
- [x] 1.2 Implement the non-fallible narrow typed host-safe error writer by reusing package-owned fixed documents without accepting configuration, messages, causes, or dynamic encoding.
- [x] 1.3 Add compile-time/API tests proving the host writer exposes no private DTO, arbitrary status, code, type, retryability, or byte-limit control.

## 2. Direct Anthropic environment isolation

- [x] 2.1 Add direct-provider poisoned-environment tests for conflicting `ANTHROPIC_BASE_URL`, API key/auth token, explicit and fallback profiles, federation/organization/identity-token sources, and custom headers across unary and streaming calls.
- [x] 2.2 Correct `providers/anthropic.New` to pass `option.WithoutEnvironmentDefaults()` and `option.WithAPIKey(apiKey)` directly to `anthropic.NewClient`, preserving the separate Vertex path and explicit request options.
