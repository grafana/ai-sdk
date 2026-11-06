# Native Chat request controls

Map Chat scalar fields into compatible native fields or typed Responses options.
Reject controls without native equivalents using the shared unsupported-functionality
category. The pinned Responses normalizer preserves logprobs only with a positive
alternative count; request one when Chat asks for selected-token-only probabilities,
then trim alternatives in the projection helper. No model generation semantics change.

This change supplies request mapping and a projection helper. The parent stack
must wire the helper into unary and streaming output before claiming complete
client-visible logprob support. Existing native provider warnings retain model
capability details. The helper validates typed metadata and never forwards raw output.
