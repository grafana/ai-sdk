# Validation

## Current scope and reference

#370 activates #373's compact execution overview. This follow-up starts from #370 `956a20e4e054cfe7ec1114dacb99162210b938a6`, with #373 `6bf43a9118495b89e45e08f9e59066727dd4657a` unchanged. The remaining stack is #373 → #370 → #369 after #367/#368 merged. Tail integration must replay all commits through #369 `80cd4da89e8149ea6cdf9df26cc292bc306567d4` from exact parent `956a20e4`, preserving its static-key authentication work.

Registered reference remains `eb77f09e3c06c28e860d92e0de941b143c2eecec`: ai7.0.127, Gateway4.0.103, Anthropic4.0.71, OpenAI4.0.83, Provider4.0.21 and Utils5.0.53. Exact matching upstream source was supplied for parity checks. Client carrier preservation is upstream behavior; server summary protection is a Gateway-owned security policy, not private-service parity.

## Configured-key protection evidence

- A real-command red regression changed the expected configured-key echo to `[REDACTED]` and failed against the previous implementation, including successful fallback and classified HTTP failures. The complete updated command suite passes.
- Immutable startup policy covers every resolved provider key, including unused accounts, and the enabled exporter credential without resolving it twice. The process secret-resolution tests remain green. Static Gateway access keys are explicitly excluded by the owner; no raw static-key retention or authentication change is introduced.
- Summary tests cover empty/duplicate/overlapping configured values, JSON escaping, string/numeric code distinction, marker collision, source/configuration nonmutation and BYOK-only echo preservation. Configured ownership wins on a value collision; arbitrary transformed-key detection is not claimed.
- Handler tests cover direct/exhausted errors, successful fallback unary/finish and configured/plain committed errors with unchanged statuses, counts and finish behavior. Low-level TypeScript/Go clients verify protected current summaries without invented catalog attribution, while generated text and opaque metadata remain untouched.
- Snapshot/current policy is applied after pure projection and before encoding. SDK observer, source errors, classification, lifecycle and transport fit code are unchanged. Optional enrichment can be omitted but no unredacted Gateway-produced summary is used as fallback.

## Checks

All listed checks pass:

- Go1.26.8 readonly root and Gateway full race suites and vet; Gateway golangci-lint: zero issues. Standalone readonly Grafana-client and SDK-only integration-harness races pass.
- Focused summary/invocation/selection/capture races repeated ten times under Go1.27.2.
- Current pinned tooling: Go1.27.2, Node24, pnpm12.8.2 and OpenSpec1.14.1; no tool pins changed.
- `mise run parity-check` with exact registered source: provider shapes, recorded conformance, 109 baseline checks, 138 schema tests and 114 runtime/client tests.
- `mise run test-integration`: 25 frontend files / 140 tests plus Gateway command coverage.
- `mise run test-ai-gateway-command`: 75 tests, no skips.
- Gateway boundary, SDK isolation, workspace, merged-pin, docs and whitespace checks.
- OpenSpec all-strict: 99/99 while the reused change was reopened; 98/98 after returning it to its existing archive.

Workspace checksum additions produced by local tooling were removed; no module, schema, provider recording, lockfile or minimum-Go changes are included. No live-provider credential echo or deployed-service proof is claimed.

## Historical evidence and limits

The earlier implementation/review phases remain recorded in completed tasks and Git history. Their broad credential-source filtering and later unconditional echo-preservation policies are superseded by this narrow configured-provider/exporter summary policy. The previous independent review loop stopped at its round cap; this follow-up has parent validation, not a fresh independent clean review.

The owner requested reuse of this change rather than a new proposal. Its current capability deltas preserve existing scenarios and document the narrowed policy. Summary protection is not generic output DLP or stream-fragment scanning. Opaque metadata, ordinary text/tool output and arbitrarily encoded values remain outside it. Request-account observation gaps remain #317; raw diagnostics remain #323; coherent current read/fit limits remain pending #394. No merge, deployment or static-auth activation is authorized by this work.
