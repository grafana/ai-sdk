## 1. Mapping

- [x] 1.1 Confirm the module already pins agento11y v0.19.0; no dependency change is needed.
- [x] 1.2 Cover inclusive input totals, including a reported zero, in usage mapping tests.
- [x] 1.3 Normalize Bedrock and Anthropic Vertex names at the Agent Observability and OTel boundaries, and use agento11y's registry names on the preflight span.

## 2. Hook compatibility

- [x] 2.1 Encode hook test responses with the v0.19.0 server wire shape.
- [x] 2.2 Treat empty HTTP `transformed_input:{}` as no transform after the dependency normalizes it to nil.
- [x] 2.3 Reject every non-nil decoded `TransformedInput` with `ErrHookTransformFailed` before provider invocation, regardless of `FailOpen`.
- [x] 2.4 Cover valid assistant, system-prompt, tool-message, tool-list, and user changes, unchanged replacements, and decoder-dropped malformed parts in generate and stream tests.
- [x] 2.5 Verify allow without a transform and deny remain unchanged, and rejected transforms bypass recording in the standard stack.
- [x] 2.6 Keep prompt, tool, and tool-choice reconstruction in one inactive helper, and replay the hook fixture through it.

## 3. OTel coverage

- [x] 3.1 Cover unary generation content, controls, identity, usage, context, and parentage.
- [x] 3.2 Cover streaming context, lifetime, pass-through, errors, metadata-only capture, and cancellation.
- [x] 3.3 Prove client shutdown invokes the configured flusher before tracer-provider shutdown.
- [x] 3.4 Preserve requested identity, provided-only context, and filtering on success, immediate errors, and filter panics.

## 4. Contracts and documentation

- [x] 4.1 Document application-owned exporters, sampling, flushing, shutdown, and delivery limits.
- [x] 4.2 Update the Agent Observability middleware specification and archive its change artifacts.
- [x] 4.3 Compare the generation span with `@ai-sdk/otel@1.0.153` and record field-level differences in the specification. Leave `@ai-sdk/otel` unregistered.
- [x] 4.4 Document full transform rejection, decoder limitations, the nil normalization exception, and inactive reconstruction rules.
- [x] 4.5 Document the stored provider rename for every export protocol.

## 5. Validation

- [x] 5.1 Run middleware, conformance, parity, vet, lint, documentation, formatting, and full test checks.
- [x] 5.2 Review the final diff and remove unrelated workspace changes.
- [x] 5.3 After implementation, rerun hook tests and conformance checks against the revised rejection contract.
