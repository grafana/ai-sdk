# Decode native compatible alternatives

An explicitly opted-in CallOptions.ChoiceCount sends n in one native request. The compatible decoder preserves all indexed alternatives, independently accumulating interleaved text/reasoning/tools. A global choices-finish carries usage once only after every choice finishes. Refusal and logprob metadata remain per choice. Defaults retain pinned @ai-sdk/openai-compatible 3.0.82 first-choice behavior; the opt-in is the registered intentional native-multiple-choices extension.

Synthetic focused fixtures cover decoder behavior and malformed indices; these are not provider recordings.
