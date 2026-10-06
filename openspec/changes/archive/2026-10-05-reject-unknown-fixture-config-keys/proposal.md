## Why

[Issue #34](https://github.com/grafana/ai-sdk/issues/34) reports that both conformance fixture loaders accept unknown YAML keys. Go `LoadConfig` called `yaml.Unmarshal`, which drops undeclared struct fields, and TypeScript `loadConfig` returned `parseYaml(raw) as Config`, which checks nothing at runtime. A fixture with `needsAproval: true` loaded in both with approval off, so the generated snapshot and the Go replay agreed on behavior the fixture never exercised: false parity evidence.

The two key sets had also drifted. The TypeScript tools read `allowSystemInMessages`, which one recorded fixture sets, and Go had no field for it, so Go replay dropped the key without a trace.

## What Changes

- Go `LoadConfig` decodes with `KnownFields(true)`, and `MessageConfig.UnmarshalYAML` decodes through a new `decodeKnownFields` helper, because a yaml.v3 node's `Decode` inside a custom unmarshaler ignores the calling decoder's `KnownFields`.
- Go `Config` declares `allowSystemInMessages`, with a comment that Go replay accepts system messages without the opt-in.
- TypeScript adds `parseConfig`, which `loadConfig` calls, and checks every structured level against key lists that the compiler ties to their interfaces.
- `testdata/fixture-config/all-keys.yaml` lists every accepted key at every level, and `testdata/fixture-config/invalid/` holds ten single-typo fixtures. Tests on both sides require all-keys to load, require every declared key to appear in it, and require every invalid fixture to fail naming its key.
- The conformance README documents the rule and how to add a key.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conformance-testing`: Add the requirement that both loaders reject undeclared fixture keys with their path while payload maps stay open, and that the two key sets stay aligned.

## Impact

Test tooling only: `test/conformance/runner.go`, `test/conformance/tools/common.mts`, their tests, shared test data and the conformance README. No SDK, provider, Gateway, baseline or snapshot change. All 139 existing fixtures load under both strict loaders unchanged.
