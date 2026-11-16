# Preserve Chat output metadata

The Chat ingress needs refusal and logprobs after native normalization. Add an
opt-in constructor setting that retains validated first-choice fields in the
resolved provider namespace. Streaming accumulates them at finish, avoiding
custom content in history. Native accounting remains in Usage.Raw.

Malformed optional metadata produces a warning or recoverable stream error,
without discarding ordinary content. Default compatible behavior is unchanged.
This is an intentional Go extension with synthetic HTTP and frontend tests;
no live recordings or baseline changes are claimed.
