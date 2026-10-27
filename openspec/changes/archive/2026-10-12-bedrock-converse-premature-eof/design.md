## Context

The decoder recognizes complete binary frames, but only the Converse consumer knows whether messageStop arrived. Registered @ai-sdk/amazon-bedrock@5.0.105 at eb77f09e3c06c28e860d92e0de941b143c2eecec initializes other/undefined and unconditionally emits finish in flush. Its matching stream tests cover stop and metadata. This fix is an intentional deviation recorded in upstream.yaml.

## Decisions

When decoding returns nil without finishKnown, wrap io.ErrUnexpectedEOF and reuse the existing retryable transport-error branch. Exception frames return a distinct sentinel; cancellation suppresses the new error. A real empty or unmapped stop reason remains valid. Requiring metadata or inferring failure from other/empty raw would reject legitimate terminal events.

## Risks and validation

Retryability does not authorize replay after partial output; core and fallback commitment controls remain unchanged. Synthetic HTTP/frame tests cover missing stop, incomplete tools, valid stop, metadata, cancellation and corrupt frames. Schema-parsed frontend tests preserve partial text/tool input and one error, followed by the UI protocol's error finish. No live-provider evidence is claimed.

## Completion

This archive documents the reviewed implementation and its already synchronized spec; it does not claim the proposal preceded the October 6 implementation. Candidate-source race, integration and required parity checks pass, with canonical TMPDIR=/private/tmp for macOS temporary-workspace harnesses.
