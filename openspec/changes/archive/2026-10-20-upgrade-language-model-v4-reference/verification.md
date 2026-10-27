# Verification

The exact package manifests match the frozen source reference in the design. All provider recordings and imported input bytes are unchanged. Only the excluded Anthropic non-LanguageModel fixture index changes from evaluation to decision.

## Contract evidence

- Native UI/request/provider-part replay passes for the registered corpus. Snapshot differences follow Anthropic message-start custom parts, native text citations, retrieval-source ordering and OpenAI text input encoding/stored-reference eligibility.
- Repeated provider generation, every committed UI generator and deferred-discovery generation reproduce identical expectations.
- Frontend integration passes 150 tests, including schema-parsed citation/text-input persistence and resume. Gateway contracts/runtime pass 126 tests; production command integration passes 94 tests, including native custom output, source ordering and approval continuation.
- Focused native/root tests cover maximum reasoning, stored text references/deduplication, retained citations, simulation and required empty document titles. The document-title regression failed before its correction. An existing React metadata-history assertion now waits for the observation effect while retaining both initial and updated-state assertions.
- Aggregate parity, all module tests, build, vet, lint, docs lint, workspace/boundary and merged-pin checks pass. Root, Anthropic, OpenAI, Bedrock/Mantle and affected Gateway race suites pass.

Changed capabilities validate strictly and aggregate non-strict validation passes all 108 specifications. Extra broad strict validation reports three pre-existing overlong-description warnings in unchanged Bedrock, compatible termination and ProviderWire streaming specs; these are not suppressed or claimed green.

## Publication boundary

Candidate-source validation uses the repository workspaces. Standalone module diagnostics fail against existing older internal pins, including missing root schema helpers, earlier Gateway contracts and the new citation producer API. No Go module pins or published-module replacements change here. Merge/publish the root producer before independently releasing consumers; this verification does not claim standalone publication readiness.

Synthetic and imported replay evidence does not establish live-provider acceptance, deployed hosting policy, Vercel private-service behavior or the full production-image Gateway corpus. Optional parity packages and unresolved historical issue dispositions belong to the upgrade PR and issues, not this specification.
