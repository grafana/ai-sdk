## 1. Mantle Responses package

- [x] 1.1 Add the official Bedrock client and stacked OpenAI provider dependencies, then verify the Bedrock module resolves with `GOWORK=off`
- [x] 1.2 Add the Responses constructor, config aliases, and provider identity delegation, then verify construction and model identity tests pass
- [x] 1.3 Verify generic `/v1/responses`, every documented `/openai/v1/responses` exception, and custom routing preserve native model IDs and never emit Converse paths

## 2. Authentication and behavior

- [x] 2.1 Add bearer-mode tests covering explicit rollback credentials and authorization headers
- [x] 2.2 Add SigV4 tests covering `bedrock-mantle` credential scope, region, payload hashing, and session tokens
- [x] 2.3 Add generate and stream tests verifying `bedrock-mantle.responses` attribution with OpenAI-namespaced continuation metadata
- [x] 2.4 Verify invalid or conflicting routing/authentication configuration fails closed through the official client

## 3. Documentation and parity

- [x] 3.1 Document Mantle Responses setup, model-aware `/v1` and `/openai/v1` routing, dual authentication, and the remaining Chat gap; verify documentation lint passes
- [x] 3.2 Narrow the Mantle parity records to Responses mixed coverage and a Chat/default-provider gap; verify baseline validation passes
- [x] 3.3 Archive and strictly validate the OpenSpec change

## 4. Validation

- [x] 4.1 Run Bedrock and OpenAI focused tests, vet, lint, module verification, formatting, and diff checks
- [x] 4.2 Confirm the branch contains only the follow-up diff from PR #154 and record the temporary stacked dependency pin in the PR description
