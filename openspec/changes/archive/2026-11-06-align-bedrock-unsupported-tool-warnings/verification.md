## Evidence

- Registered reference: Bedrock 5.0.130 and Anthropic 4.0.94 at upstream `028a025398782f4938303f48fbeee45c79af544b`; source and tests inspected. The issue's older 5.0.125 reference has the same relevant behavior.
- Exact warning regression tests failed before the production change. Ten preparation cases cover model families, multiple unknown tools, web filtering, known tools, none, required and named choices.
- Eight synthetic HTTP cases assert exact unary/stream warnings and unchanged tool request filtering. No provider recording or imported fixture was added.
- The installed upstream package was independently executed with synthetic fetch responses for seven cases; complete warning lists matched the Go expectations.
- Full Bedrock module tests, race detector and golangci-lint 2.15.0 passed.
- Direct constituents of `parity-check` passed: baseline validation and 115 baseline tests; fixture coverage; provider-shape report; ProviderWire Go contract, TypeScript typecheck, 154 schema tests and 147 client tests; conformance typecheck and all conformance replay modules.
- Documentation structure and markdown lint passed. Existing Gateway architecture and glossary remain accurate; this adapter diagnostic fix changes no Gateway boundary.
- OpenSpec 1.16.0 change validation passed before archive. No conformance snapshots changed.

## Limits

The issue is warnings-only. Existing request differences for unknown provider tools combined with explicit choices remain: for example, upstream adds `additionalModelRequestFields.tool_choice: {type: "auto"}` for a required unknown tool on Sonnet 5.5, while Go omits it. This change does not claim request parity for that combination or live-provider acceptance. The named `mise` task was not run because this worktree was not trusted; its parity constituents ran directly using the installed pinned toolchain and registered upstream source.

The synchronized capability validates normally. Strict capability validation reports two inherited long-requirement warnings at requirements 14 and 15 (Nova token limits and option merge); both requirement blocks are byte-identical to the base. The new requirement introduces no validation warning.
