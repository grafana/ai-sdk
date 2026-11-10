# Opt-in native choice domain

The pinned @ai-sdk/openai-compatible 3.0.82 source at 028a025398782f4938303f48fbeee45c79af544b selects choices[0] for generate and stream. Supporting native Chat n>1 therefore requires an explicit intentional Go extension, not a parity claim or hidden metadata projection.

ChoiceCount, GenerateResult.Choices and StreamPart.ChoiceIndex are Go-only fields. PartChoicesFinish explicitly closes all indexed choices with aggregate usage. The runtime owns independent validators and shared prompt history policy for each choice. Default and ProviderWire consumers cannot opt in accidentally, and unsolicited alternative output is rejected.
