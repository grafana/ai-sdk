## Context

Go `LoadConfig` at `465b024` (`test/conformance/runner.go`) called `yaml.Unmarshal(data, &cfg)`. TypeScript `loadConfig` (`test/conformance/tools/common.mts`) returned `parseYaml(raw) as Config`. The generation tool (`generate.mts`), the recording tool (`record.mts`) and the Go runner and Bedrock runner all go through these two functions, so making them strict covers every path that produces or consumes a parity snapshot.

This is conformance harness work in the `PARITY.md` sense: it changes how evidence is checked, not SDK behavior, so it has no upstream counterpart to compare against.

## Goals / Non-Goals

**Goals:** Reject undeclared keys at the top level and in every structured nested value, with the key's path; keep payload maps open; make it impossible for the two loaders to accept different key sets without a failing test.

**Non-Goals:** Validating value types or enum values beyond what each loader already does; changing fixture semantics; the separate fail-closed issues #33 and #35.

## Decisions

### 1. KnownFields plus a strict decode inside the custom unmarshaler

`yaml.Decoder.KnownFields(true)` covers every struct reached by ordinary decoding, including map values such as `tools.<name>`. It does not reach code inside `MessageConfig.UnmarshalYAML`: yaml.v3 hands the unmarshaler a `*yaml.Node`, and `Node.Decode` builds a new decoder without the setting. `decodeKnownFields` re-encodes the node and decodes it with a strict decoder, wrapping errors with the node's line in the fixture because the inner decoder's line numbers are relative to the node. Running the invalid fixtures with `KnownFields` alone left exactly the message and message-part typos accepted, which is what this decision closes.

### 2. Key lists checked by the compiler in TypeScript

`keyList<T>(keys: Record<keyof T, true>)` returns a key list and makes the compiler reject a missing or extra key, so each list cannot drift from its interface. Two levels have no local interface, UI messages and model-output content items, which reuse upstream types; their lists name exactly the keys Go replay reads, so a key replay would drop is rejected instead of silently ignored.

### 3. One shared file proves alignment in both directions

A test in each language requires `all-keys.yaml` to load under that language's strict loader and requires every key that language declares to appear in the file at its level. Together they mean neither side can declare a key the other rejects. Removing the new Go `allowSystemInMessages` field makes the Go test fail with `field allowSystemInMessages not found in type conformance.Config`, the drift the issue's scope would otherwise have left in place.

### 4. Payload values stay open by type

Go leaves `map[string]any`, `any` and `[]map[string]any` values unchecked because `KnownFields` only applies to struct fields. The TypeScript walk only checks the structured levels it names. `all-keys.yaml` puts made-up keys inside provider options, schemas, tool inputs, provider tool args and UI message parts, and both loaders accept them.

## Risks / Trade-offs

A fixture that relied on a silently ignored key now fails to load. All 139 current fixtures load under both loaders, so none did. Go's strict error for a nested message reports two positions, the message's line in the fixture and the field's line within that message.
