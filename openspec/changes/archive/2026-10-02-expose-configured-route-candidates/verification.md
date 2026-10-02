# Implementation verification

## Contract and scope

Compared the implementation with #324, the approved #321 decision, all change
requirements, and the registered Gateway discovery implementation/tests at
`ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. Source versions match Gateway 4.0.94,
ai 7.0.116, provider 4.0.18 and provider-utils 5.0.49; no baseline upgrade occurred.
Configured facts are an intentional Grafana extension. Stock TS normalization
still strips them; Go ListModels and the shipped companion retain them explicitly.

Independent synthetic catalog/HTTP tests cover immutable facts, canonical/alias
agreement, ordering, malformed and duplicate inputs, cardinality, UTF-8 strings,
encoded bytes, cleanup and scoped listing/resolution. Real-command witnesses
exercise raw HTTP, Go, pinned stock TS and the companion for direct/mixed fallback
routes, with zero native inference during discovery. JWT rejection and the dummy
CAP-edge denial/credential-stripping tests remain green. Allocation preflight and
bounded fuzzing passed (89,909 fuzz executions). Review corrections cover cleanup
error replacement, trailing-line public IDs, Unicode whitespace and limiting only
referenced discovery facts rather than unrelated provider configuration.

No endpoint/package, unary/SSE codec, request routing, BYOK/account construction,
operator capture policy, dependency pin or provider recording was added/changed.
No conformance expectation regeneration was needed for this catalog-only feature.

## Passed checks

- Focused catalog/config/discovery/service/startup tests and full Grafana client tests.
- `mise run test-ai-gateway`, `mise run test-providerwire-v4`, `mise run test-ai-gateway-command` (41 command tests).
- Grafana client race/vet; Gateway catalog and command-internal race tests.
- `mise run vet`, `mise run build`, `mise run lint`, `mise run lint-docs`.
- Changed Go formatting and `git diff --check`.
- `mise run validate-parity-baseline`, `mise run parity-check`.
- `mise run verify-gateway-workspace`, `mise run verify-ai-gateway-boundary`, `mise run verify-merged-pins`.
- `MODULE=providers/grafana mise run verify-published-module` with standalone readonly published dependencies.
- `mise run test-ai-gateway-image-source`.
- `openspec validate expose-configured-route-candidates --strict --no-interactive`.

The normal parity run skips its advisory provider-shape report because the npm
source layout is unavailable to that reporter. This is not full provider-interface
proof; the relevant exact-commit discovery source/tests were reviewed separately.

## Review loop

Completed two rounds: initial forked oracle plus independent correctness/parity,
cleanup and evidence reviewers, followed by fresh correctness and cleanup reviews.
The parent verified findings, reproduced failures and remained the sole writer.
The second round found no actionable bugs, parity drift or worthwhile cleanup.
No material design/API changes, rejected substantive findings or owner decisions
remain. Reviewers inspected source and logs; the parent executed the checks.

Corrections:

- Initially rejected escaped unpaired surrogates in recognized discovery strings.
  The approved client-policy simplification below supersedes that stricter rule
  with standard JSON decoding; generic runtime codecs remain unchanged.
- Replace retained TS chunk references with immediately copied, owned, bounded
  storage. A recycled-buffer stream reproduced valid JSON corruption before the
  fix; fragmented, empty and split-multibyte chunk regressions now pass.
- Independently test aggregate alias expansion at 1,024/1,025 public rows in
  config and HTTP projection, without violating the per-route alias ceiling.

After fixes, full Grafana race/vet, Gateway tests, ProviderWire typecheck/schema/
client checks, all 41 command tests, lint/docs and standalone readonly client
validation passed. Final parity, module and strict OpenSpec checks were refreshed.
Review reports: `/tmp/gw324-review/r1-*.md` and `/tmp/gw324-review/r2-*.md`;
workflow receipt: `/tmp/pi-subagents-uid-1000/async-subagent-runs/de78cb46-a1bf-42f0-8502-dabaf31ec6f6/workflow-receipt.json`.

## PR feedback cleanup

Merged the eight added Go test files into existing config, discovery, process,
catalog and client suites. Candidate copying now extends the existing static and
registry tests; shared config fixtures and existing settings-default assertions
replace redundant setup/coverage. The separate TS suite tests the new companion.

Replaced discovery's handwritten string/route encoder and bounded-buffer state
with typed JSON projection and per-row `encoding/json`. Raw collection/string
preflight and exact encoded-size checks still precede alias expansion; assembly
independently checks the final byte budget before HTTP success. Standard JSON
escaping preserves string values, including line separators, and its actual bytes
count toward the limit. Moved discovery limits/startup validation into the handler.
Rewrote both new guide sections around model selection, provider choices and
large-catalog handling; evidence limitations remain in this record and PARITY.md.

After cleanup, Gateway tests, Grafana race/vet, catalog/config/discovery/process
race checks, ProviderWire typecheck/schema/client tests, all 41 command tests,
parity, lint/docs/build/vet and all 83 strict main-spec validations passed.
Bounded discovery fuzzing passed with 99,444 executions. Logs:
`/tmp/gw324-cleanup-{go,parity,quality}.log`.

## Approved client-policy simplification

Removed duplicated Go/TS row, alias, candidate and string-size policy ceilings.
Config loading and server projection retain their existing ceilings, startup
validation, raw preflight and final encoded-byte protection. Clients retain
independent bounded reads, atomic structural/route consistency checks, credential
exclusions and resource cleanup. Server and client byte defaults are unchanged.

Deleted the Go raw-string scanner and its duplicate object parsing; discovery
now uses the existing selected-field decoder. Standard JSON semantics apply:
Go normalizes escaped lone UTF-16 surrogates to U+FFFD, while TS retains the
standard decoded UTF-16 value. This accepted Go adaptation does not establish
lossless cross-client identity agreement for those escapes. Semantic validation
still rejects duplicate tuples after normalization. Valid Unicode pairs, genuine
U+FFFD, ignored additives and last-member behavior remain covered.

Updated independent Go/TS tests first and confirmed failures before the changes.
Both clients now accept documents beyond each server policy ceiling within their
byte budgets; byte overflow still fails atomically. Main specs, archived artifacts,
client guidance and PARITY.md reflect the approved contract. The upstream baseline
and effective 16-candidate server ceiling remain unchanged.

Passed full Grafana race/vet, Gateway tests, ProviderWire typecheck/schema/client
checks, all 41 command tests, parity, vet/build/lint/docs, module/boundary/pin checks
and all 83 strict main-spec validations. Logs:
`/tmp/gw324-client-simplify-{go,parity,quality}.log`. Provider-shape reporting retains
the previously documented advisory skip. Checks on the preceding `71ae78a5` PR
revision passed; image validation/publication/deployment were skipped by CI.

## Spec synchronization and archive

Synced five added configured-discovery requirements and nine modified requirements
across four existing capabilities. Compared each merged requirement with its delta
and verified unrelated requirements remained unchanged. Strict validation passed
for all 83 main specs. Archived the completed 22/22-task change on 2026-10-02;
no active changes remain.

## Delivery and evidence boundaries

The server default remains independently configurable at 1 MiB, Go defaults to
4 MiB, and the TS helper defaults to/maxes out at 4 MiB. A larger client allowance
does not enlarge server capacity; configured projection must fit before readiness.

The current command exposes one static catalog to accepted identities. Scoped
fakes and the dummy Cloud edge do not prove customer-account construction,
deployed CAP enforcement or BYOK tenant isolation. Custom fetch implementations
are caller-owned and must honor standard redirect/abort behavior.

Validation ran on `nrbrd/gw-routing` above the approved parent `7b0b35cf`, with
#327 open from `nrbrd/gw-contract` to main. Delivery targets that open parent;
the child merges after it, then rebases/retargets main after parent merge.
Full revision-clean multi-platform image publication/deployment smoke is the
existing push-only CI gate, not established by the local checks above.
