# Tasks

The root middleware denial option is delivered by the `record-middleware-denials` change, which this change builds on.

## 1. Public contract and source vocabulary

- [x] 1.1 Create `ai-gateway/pkg/extension` with the types, phase interfaces, sentinels and `Source.Keys()`, and a v0 `doc.go`; verify with the external-module build test and `extensionhost` tests.
- [x] 1.2 Make `execution.Source` an alias of `extension.Source`, with the two existing constants re-declared from it; verify existing `execution` and `service` suites pass unchanged.
- [x] 1.3 Add `Options.Extensions`, `Options.Mandatory` and `DefaultExtensions()` to `pkg/gateway`; pass `DefaultExtensions()` from `pkg/main.go`; extend the external-module build test with a distribution that defines an extension; verify `go test ./pkg/gateway/...` without `-short`.

## 2. Runtime: entry, classification, policy check

- [x] 2.1 Build the attempt capture before composition and mark entry in an `entered` model under the composed layers; verify runtime tests for a pre-model refusal (no attempt, no overview) and an unchanged single-candidate view without a composer.
- [x] 2.2 Treat `ErrDenied` and `ErrUnavailable` after entry as a selected synthesized attempt; verify with attempt-view and runtime tests.
- [x] 2.3 Add `CategoryUnavailable` at the end of the list and classify both sentinels first, including a status-less API error cause; verify `classify_test.go` tables.
- [x] 2.4 Export the host policy check and option cloning shared by translation and extensions; verify translation tests and `TestPolicyAllows`.
- [x] 2.5 Add `safeUnavailable`, map both categories in ProviderWire with no native summary on in-band extension errors, and extend the test-time schemas; verify `TestExtensionOutcomeDocuments`, `TestPublicError_FixedBytes` and the client-consumption tests in `test/providerwire-v4`.

## 3. Operator configuration

- [x] 3.1 Parse the `extensions` setting in `config` (bound, strict shape, entries, `<secret>Env`); verify `TestParseExtensions` and `TestParseExtensions_Refusals`.
- [x] 3.2 Apply entries to definitions (unknown, tenant, undeclared secrets, schema validation with location, mandatory) and resolve secrets after other validation; verify `TestNewPlan`, `TestNewPlan_Refusals` and `TestPlan_ResolveSecrets`.

## 4. Extension host

- [x] 4.1 Validate definitions and mandatory names; verify `TestValidateDefinitions` and `TestApplies`.
- [x] 4.2 Construct instances with scoped dependencies, refuse instances with no phase, contain factory panics, close built instances on failure, and build per-source ordered lists; verify `TestNew_Construction` and `TestHost_Compose`.
- [x] 4.3 Run request phases, the per-transform policy check, failure policies, panic containment and per-call state; verify `TestPipeline_PhaseOrder`, `TestPipeline_GatesStopAtFirstDenial`, `TestPipeline_TransformOrder`, `TestPipeline_HostPolicyAfterEachTransform`, `TestPipeline_FailurePolicies`, `TestPipeline_PanicContainment` and `TestPipeline_PartialImplementationAndPerCallState` under `-race`.
- [x] 4.4 Run unary and stream response phases on normalized parts with validation and abort; verify `TestPipeline_NormalizedParts` and `TestPipeline_ResponseAbort` under `-race`.
- [x] 4.5 Dispatch Observe asynchronously, bounded, tracked and drained by `Close`, with dropped observations counted; verify `TestPipeline_Observe`, `TestPipeline_ObservationIsOffTheResponsePath` and `TestHost_CloseRespectsTheDeadline`.
- [x] 4.6 Add the caller type to `auth.Caller` and the service request-info accessor; verify `TestAccessPolicy_Decide` and `TestPipeline_RequestInfo`.

## 5. Wiring and observation

- [x] 5.1 Validate definitions before the idle check, plan before secrets, resolve extension secrets last, add them to the redactor, and close the host in the final flush; verify `TestNew_ExtensionStartup`.
- [x] 5.2 Compose `observers(pipeline(...))`, set `IsDenied` on the logical observers and allow `denied` in the log policy; verify `TestExtensionPipeline_EverySelectionGetsThePipelineOnce` and `TestExtensionPipeline_DenialIsADistinctObservedOutcome`.
- [x] 5.3 Prove the plugin-protocol behavior: admit denial with zero provider calls, output denial after fallback with its overview, in-band stream abort, transform header refusal and extension secret redaction; verify `TestGateway_Extensions`.

## 6. Documentation

- [x] 6.1 Write `docs/guides/gateway-extensions.md`, add the glossary terms, update the architecture walkthrough and the README settings table; verify `mise run lint-docs`.

## 7. Integration checks

- [ ] 7.1 Run `mise run fmt`, `vet`, `lint`, `test`, `build`, `test-ai-gateway`, `test-integration`, the boundary, requirements and release checks, `lint-docs` and `openspec validate add-gateway-extensions --strict`; verify all pass, with CI providing e2e.

## Workflow follow-up

- Archive the change after review, and verify the archived specs.
