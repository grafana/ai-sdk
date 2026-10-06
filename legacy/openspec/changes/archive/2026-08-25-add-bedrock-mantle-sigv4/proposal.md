## Why

Bedrock Mantle is a separate AWS service (`bedrock-mantle.<region>.api.aws`) whose SigV4 signatures must be scoped to the `bedrock-mantle` service name, not `bedrock`. The Bedrock provider hard-codes the signing service to `bedrock`, so any SigV4 request against a Mantle endpoint fails authentication with a signature-scope mismatch. Callers deploying against Mantle (IAM/IRSA, no static bearer token) have no working SigV4 path today.

## What Changes

- Add a `WithSigningService(service string)` functional option that overrides the AWS service name used in the SigV4 credential scope.
- Resolve the signing service per request: an explicit `WithSigningService` value wins; otherwise infer `bedrock-mantle` when the endpoint host is a Mantle host (`bedrock-mantle.<region>.api.aws`); otherwise default to `bedrock`.
- Sign requests using the resolved service name instead of the fixed `bedrock` constant.
- Default behavior is unchanged (`bedrock`), so existing wire and parity behavior is preserved. Bearer-token authentication is unaffected by the signing service.
- Non-goal: routing Converse-shaped requests to Mantle's OpenAI-/Anthropic-compatible API surfaces. This change covers the authentication layer only.

## Capabilities

### New Capabilities

### Modified Capabilities
- `bedrock-provider`: The AWS authentication requirement changes so the SigV4 signing service is resolved (explicit override → Mantle-host inference → `bedrock` default) rather than always `bedrock`. The constructor-and-options requirement adds the `WithSigningService` option.

## Impact

- `providers/bedrock/model.go`: new `signingService` field, `resolveSigningService()`, `isMantleEndpoint()`, and `bedrock`/`bedrock-mantle`/host-prefix constants.
- `providers/bedrock/options.go`: new `WithSigningService` option; `WithBaseURL` doc notes Mantle inference.
- `providers/bedrock/signing.go`: `signRequest` signs with `m.resolveSigningService()`.
- `providers/bedrock/doc.go`, `docs/providers/bedrock.md`: document Mantle signing and its scope limitation.
- Additive, Go-specific option; upstream `@ai-sdk/amazon-bedrock` does not model a configurable signing service. No committed conformance fixtures change.
