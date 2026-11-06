# Chat custom tool protocol

Extend function-only Chat ingress/output to custom text and grammar tools using
the existing common provider custom-tool representation. Preserve JSON-string
final inputs versus raw text deltas deliberately, including history without current
declarations. Retain shared execution lifecycle validation and tighten compatible
native guards against custom payload overrides. No grammar engine is emulated.
