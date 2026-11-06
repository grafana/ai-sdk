# Preserve Chat output metadata

The Chat ingress cannot recover refusal, logprobs and detailed usage after native
normalization. Add a local opt-in extension that preserves validated typed fields
through the existing provider metadata/custom-part contract. Default output stays
aligned with pinned @ai-sdk/openai-compatible 3.0.82; this is an intentional Go
extension needed for the Chat protocol codec. No common provider union changes,
raw forwarding, transport bypass, new recorded fixtures or baseline pin changes.
