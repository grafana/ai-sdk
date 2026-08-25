## 1. Signing service resolution

- [x] 1.1 Add `defaultSigningService` (`bedrock`), `mantleSigningService` (`bedrock-mantle`), and `mantleHostPrefix` (`bedrock-mantle.`) constants in `providers/bedrock/model.go`, replacing the fixed `signingService` constant
- [x] 1.2 Add the `signingService` field to the `model` struct in `providers/bedrock/model.go`
- [x] 1.3 Implement `isMantleEndpoint(endpoint string) bool` (parse URL, match host prefix `bedrock-mantle.`, return false on parse error) and verify via `TestIsMantleEndpoint`
- [x] 1.4 Implement `resolveSigningService()` with precedence explicit override → Mantle-host inference → `bedrock`, and verify via `TestModel_ResolveSigningService`

## 2. Constructor option

- [x] 2.1 Add `WithSigningService(service string) Option` in `providers/bedrock/options.go` and verify via the `WithSigningService` construction-table case in `TestModel_Construction`
- [x] 2.2 Update `WithBaseURL` godoc to note automatic Mantle signing inference

## 3. Wire resolution into signing

- [x] 3.1 Change `signRequest` in `providers/bedrock/signing.go` to sign with `m.resolveSigningService()` instead of the constant
- [x] 3.2 Verify credential-scope service name for default, Mantle-host, and explicit-override cases via `TestSignRequest_SigningService`

## 4. Documentation

- [x] 4.1 Document Mantle signing and its scope limitation in `providers/bedrock/doc.go` and `docs/providers/bedrock.md`

## 5. Validation

- [x] 5.1 Run `go test ./...` from `providers/bedrock/` and confirm pass (including the new signing-service tests)
- [x] 5.2 Run `go test ./test/conformance/...` from the repo root and confirm no fixture changes are required (default service remains `bedrock`)
