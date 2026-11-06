# Compatibility verification and restack evidence

## Historical prototype evidence — 2026-10-27

The original change recorded passing package tests for `d6dffb5a / 94e16d24`, covering
writers without socket deadlines and known Anthropic start metadata. OpenSpec 1.14.3 validated and archived that scoped change.
Those results apply to the prototype commits, not automatically to the
2026-10-28 runtime-based source. The revised design and delta supersede the
prototype's ownership assumptions; old evidence is not silently relabeled.

## Runtime restack — 2026-10-28

The base now includes #555 execution, #579 attempt visibility and #581 host
contract. Runtime.Call/Generate/Stream, per-attempt Translator and host-error
mapping replace protocol-private execution, candidate wrappers and classifiers.
Source inspection and strict proposal/delta validation cover this restated
scope. Isolated sequential replay of the three Chat changes successfully
archives/syncs each cut and validates each resulting spec independently.

Fresh integrated Go/TypeScript, stream, route and manifest evidence is recorded
in `../2026-10-28-register-chat-adapter-on-platform/verification.md`. That later
change's protocol-hop evidence is not claimed as behavior delivered by this
individual review cut. The review cut itself contains only its scoped protocol
and runtime behavior, not production route registration.

#483 shared provider-lifecycle validation remains pending. Chat explicitly
refuses a second native response segment without successful termination; the
#583 ProviderWire draft does not grant permission to truncate Chat. No live
provider, deployed Grafana, full parity or release-readiness claim follows from
these archives. No recorded provider input was changed for these specs.
