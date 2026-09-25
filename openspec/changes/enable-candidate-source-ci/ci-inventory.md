# Required-path inventory at implementation start

| Required job / task | Current Go selection | Candidate-source action |
| --- | --- | --- |
| `ci`: `fmt-check` | source formatting, no resolution | retain |
| `ci`: `vet`, `build`, `test-short`, `lint` | root/providers/middleware use root `go.work`; Gateway uses `GOWORK=off`; examples use root workspace | select absolute `go.gateway.work` for Gateway; keep root workspace for SDK and examples |
| `docs-lint` | no Go compilation | retain |
| `module-resolution`: `test-module-policy`, `verify-ai-gateway-boundary`, `verify-merged-pins` | structural checks and public-proxy declared/selected pin ancestry | retain blocking, separate from standalone compilation |
| `module-resolution`: `verify-module-resolution` | all published modules, public proxy, fresh cache, readonly `GOWORK=off` build/test | independent visible PR diagnostic, not required or a publisher dependency |
| `module-resolution`: `verify-sdk-gateway-isolation` | root + Grafana copied without Gateway, then `GOWORK=off` build/test | use copied root workspace and candidate Grafana source |
| `parity-baseline`: `validate-parity-baseline`, `parity-coverage`, `parity-provider-shape` | pinned upstream fixture/metadata checks | retain |
| `parity-baseline`: `test-providerwire-v4` | Gateway Go V4 test `GOWORK=off`; TS runtime server defaults off; Go client differential defaults off; `test:client` includes semantic mutation controls | explicit Gateway workspace and candidate Grafana client; temporary workspace for copied mutants; keep pinned TS comparator |
| `integration-test`: `test-integration` | SDK testserver has local root replacement; Gateway command and runtime default off; command's Go client defaults off | keep SDK-only server, select candidate Gateway and Grafana binaries |
| `conformance-test`: `test-conformance` | `GOWORK=off` but `test/conformance/go.mod` replaces local SDK/provider modules | retain; assert actual candidate selection, not standalone evidence |
| `image-validation` | Dockerfile builds Gateway standalone `GOWORK=off`, multiarch + native smoke | artifact-only job, gated on eligible canonical main/tag push |
| `publish-ai-gateway-image` | eligible canonical main/tag push needs source jobs and image-validation; Dockerfile standalone | require exact-revision standalone Gateway module gate and image validation |
| `deploy-ai-gateway` | canonical main push needs successful publisher | retain same-revision publication dependency |

`test/integration/global-setup.ts` builds the SDK-only testserver; `test/integration/testserver/go.mod` replaces the root locally. Gateway `gateway-command.test.ts` and `runtime-integration.test.ts` read `GATEWAY_TEST_GOWORK` but default off; `go-client-capture.ts` forces off except its separate high-level probe. `go-client-request-mutations.test.ts` copies the client outside the workspace and requires semantic `AssertionError` red controls. Example modules use root `go.work`; published provider/middleware module commands inherit it. The root workspace does not include Gateway. `test/conformance/go.mod` has local replacements, even though its task sets `GOWORK=off`.
