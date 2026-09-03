# Contributing to the Grafana AI SDK for Go

Thanks for your interest in contributing. We welcome everyone who wants to
contribute in a healthy and constructive manner, and we ask all participants to
follow our [Code of Conduct](CODE_OF_CONDUCT.md).

This SDK follows the design of [Vercel's AI SDK](https://github.com/vercel/ai)
and stays wire-compatible with its TypeScript frontend hooks. That single
constraint shapes most of this guide. Two conventions are unusual enough that
it's worth reading them before you write code:

- **[Upstream parity](#upstream-parity-with-the-vercel-ai-sdk)** — correct
  behavior is defined by a registered upstream baseline, not by what looks most
  natural in Go.
- **[Spec-driven development](#spec-driven-development-with-openspec)** —
  non-trivial changes are planned as an OpenSpec change, and CI checks it.

For build/test commands and code style, see [AGENTS.md](AGENTS.md). It is written
for AI coding agents but is the most complete reference for humans too.

## Table of contents

- [Getting help](#getting-help)
- [Before you contribute](#before-you-contribute)
- [Ways to contribute](#ways-to-contribute)
- [Development setup](#development-setup)
- [Upstream parity with the Vercel AI SDK](#upstream-parity-with-the-vercel-ai-sdk)
- [Spec-driven development with OpenSpec](#spec-driven-development-with-openspec)
- [Testing](#testing)
- [Documentation: where things go](#documentation-where-things-go)
- [Submitting a pull request](#submitting-a-pull-request)
- [Dependency management](#dependency-management)
- [Reporting security issues](#reporting-security-issues)
- [License](#license)

## Getting help

Before opening an issue or a pull request, check whether your question has
already been discussed:

- [Open issues](https://github.com/grafana/ai-sdk/issues) and
  [pull requests](https://github.com/grafana/ai-sdk/pulls)
- [Grafana Labs Slack](https://slack.grafana.com/)
- [community.grafana.com](https://community.grafana.com/)

For "what are the options / what's the signature?" questions, the API reference
on [pkg.go.dev](https://pkg.go.dev/github.com/grafana/ai-sdk) is generated from
the source and cannot drift.

## Before you contribute

- Read the [Code of Conduct](CODE_OF_CONDUCT.md). All contributors are expected
  to follow it.
- Search [existing issues](https://github.com/grafana/ai-sdk/issues) so you
  don't duplicate work already in flight.
- **Sign your commits.** All Grafana Labs repositories
  [require signed commits](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches#require-signed-commits).
  Unsigned pull requests are rejected, including those authored by agents. See
  [about commit signature verification](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification).
- For a substantial change, open an issue first. Design discussion is much
  cheaper than a rewrite, especially where upstream parity is involved.

## Ways to contribute

- **Report a bug.** Open an [issue](https://github.com/grafana/ai-sdk/issues/new)
  with a reproduction. If the bug is a behavioral difference from upstream, say
  which upstream package and version you compared against — that turns the
  report directly into a conformance fixture.
- **Request a feature.** Open an issue describing the use case. If upstream
  already has the feature, link to its implementation.
- **Fix a bug or implement a feature.** Follow
  [spec-driven development](#spec-driven-development-with-openspec) for anything
  beyond a small, self-contained fix.
- **Add or improve a provider.** Provider modules live under
  [`providers/`](providers) as separate Go modules. See
  [docs/providers/](docs/providers) and the
  [parity rules for the provider layers](#the-coverage-map-and-the-layers).
- **Add or improve middleware.** See [`middleware/`](middleware) and
  [Writing middleware](docs/middleware/writing-middleware.md).
- **Improve the docs.** See
  [Documentation: where things go](#documentation-where-things-go).
- **Add a runnable example.** Self-contained Go modules under
  [`examples/`](examples).

## Development setup

### Prerequisites

Tooling is pinned by [mise](https://mise.jdx.dev/) in [`mise.toml`](mise.toml) —
Go, Node, pnpm, Python, `golangci-lint`, the OpenSpec CLI, and
`markdownlint-cli2`. You do not need to install those yourself.

```bash
git clone https://github.com/grafana/ai-sdk.git
cd ai-sdk

mise trust   # trust the project config on first checkout
mise deps    # install workspace dependencies (the test/ pnpm workspace)
```

Run `mise tasks` to list every task.

### Module layout

The repository is a set of Go modules, not one. Provider modules, middleware
integrations, and the test harnesses have their own `go.mod`, so `go test ./...`
at the root only covers the root module.

```text
aisdk/                  root module — orchestration (StreamText, UIMessage, SSE, tools)
  provider/             provider interface and types (no dependency on the root)
  output/               structured output
  fallback/             model failover
  registry/             provider registry
  gateway/              provider-neutral model catalog
  middleware/           in-tree middleware; integrations are their own modules
ai-gateway/             separate Gateway service module
providers/<name>/       one Go module per provider (anthropic, bedrock, openai, ...)
docs/                   concepts, guides, providers, middleware, best practices
examples/               outcome-oriented programs, one self-contained module each
test/                   shared integration, CLI, conformance, and TypeScript tooling
openspec/               specs and change proposals
```

### Everyday commands

```bash
mise run fmt            # gofmt -w . across modules
mise run vet            # go vet across modules
mise run lint           # golangci-lint across modules
mise run lint-docs      # structural + markdown style lint for docs
mise run build          # build all modules, including examples
mise run test           # all Go tests across all modules
mise run test-short     # skip integration/E2E tests
mise run check          # fmt + vet + lint + docs + tests
mise run verify-ai-gateway-boundary
                        # verify the one-way Gateway dependency boundary
```

To run a single test, invoke `go test` in the right module directory:

```bash
go test -run TestStreamTextSingleStep ./...
cd providers/anthropic && go test -run TestBuildParams_SystemMessage ./...
```

## Upstream parity with the Vercel AI SDK

The Vercel AI SDK is the canonical reference for behavior, naming, and protocol
details. A change is "correct" here when it matches upstream behavior at the
registered baseline — not when it merely compiles and passes a hand-written
test.

Parity means matching **behavior, semantics, and wire format**. It does not mean
transliterating TypeScript. Use Go idioms — interfaces, channels, `error`
returns, functional options — and keep the observable behavior identical.

### The registered baseline

[`test/conformance/upstream.yaml`](test/conformance/upstream.yaml) pins the exact
upstream package versions this port is verified against, along with known gaps
and documented deviations. Always compare against those versions. Do not compare
local Go code against upstream `main`, the latest docs, or an arbitrary version
unless the task is explicitly a baseline upgrade — and if you can't find the
matching upstream source, say so rather than silently substituting a different
version.

### The coverage map and the layers

[`test/conformance/PARITY.md`](test/conformance/PARITY.md) is the coverage map.
It records, per capability, whether parity is `automated`, `manual`, a
`documented-deviation`, or a `gap`. Start there: it tells you what proof your
change needs, because the required evidence differs by layer.

| Layer | Typical proof |
|---|---|
| Core orchestration (`StreamText`, tools, output) | UI chunk snapshots (`expected.jsonl`) |
| Provider contract (`provider.LanguageModel`) | Shape report plus source comparison |
| Provider implementation (Anthropic, Bedrock, ...) | Provider request snapshots (`expected-requests.jsonl`) |
| Frontend interop | Hook-level integration tests against the real upstream client |
| Conformance harness | Regenerated fixtures |

Work is **parity-sensitive** when it touches stream parts, UI chunks, SSE
framing, provider messages, provider request conversion, tool orchestration,
output behavior, provider options, frontend interop, or conformance fixtures.

### Conformance-first TDD

When a bug can be reproduced by replaying recorded provider chunks or by
asserting the provider request, add or update the conformance fixture **first**,
watch the Go replay fail, then fix the implementation. For a new
parity-sensitive feature, record or import the upstream behavior alongside the
implementation so the fixture becomes the regression contract for the next
baseline upgrade.

Hand-written Go tests are still the right tool for local invariants, error
paths, and helpers that don't cross a provider or UI wire boundary.

### Classifying a difference

Every difference you observe against upstream must be classified as one of:

1. **Parity-preserving Go adaptation** — same behavior, idiomatic Go shape. Fine.
2. **Intentional deviation** — must be recorded in `upstream.yaml` or
   `PARITY.md` with a rationale.
3. **Implementation bug** — fix it, with a fixture where possible.
4. **Coverage gap** — record it in `PARITY.md`.

Silently diverging is the one option that is not available.

### Parity commands

```bash
mise run validate-parity-baseline   # metadata-only check of upstream.yaml
mise run parity-coverage            # validate the PARITY.md coverage map
mise run parity-provider-shape      # report provider API shape drift
mise run parity-check               # full check when changing committed parity behavior
mise run test-conformance           # replay conformance fixtures
mise run generate-conformance       # regenerate expected fixtures
```

Run `mise run parity-check` when you change committed parity behavior, and
`mise run validate-parity-baseline` for metadata-only changes. If your change
alters wire-format or provider-boundary behavior, consider whether
`expected.jsonl`, `expected-requests.jsonl`, or `expected-object.json` need to be
regenerated.

Baseline upgrades — bumping upstream `ai` or `@ai-sdk/*` versions — must move the
manifest, the conformance dependency pins, the generated snapshots, and the
lockfiles together. The selected package set must satisfy the
`minimumReleaseAge` gate in `test/pnpm-workspace.yaml`; do not bypass it.

In CI, every retained parity job is a required status check:
`parity-baseline`, `conformance-test`, and `integration-test` all block the
merge, as documented in `PARITY.md`. A fixture regeneration therefore has to
land in the same pull request as the behavior change it covers.

## Spec-driven development with OpenSpec

This repository is spec-driven. Behavior lives in specs under
[`openspec/specs/`](openspec/specs), and a change to behavior starts as a
proposal rather than as a diff. The workflow is defined by
[`openspec/config.yaml`](openspec/config.yaml) (schema: `spec-driven`) and driven
by the OpenSpec CLI, which mise installs for you.

The point is that intent is reviewable separately from implementation. For a port
with a strict external contract, that matters: reviewers can disagree with what
you're building before you've built it.

### The lifecycle

A change lives in `openspec/changes/<change-name>/` and builds up a set of
artifacts:

| Artifact | Answers |
|---|---|
| `proposal.md` | What and why, including non-goals |
| `design.md` | How, including the upstream comparison and any deviation |
| `specs/<capability>/spec.md` | The delta against the current specs |
| `tasks.md` | The implementation checklist |

Once the tasks are done, the delta specs are merged into `openspec/specs/` and
the change is archived under `openspec/changes/archive/`.

### Driving it

If you use an AI coding agent, the repository ships a command for each step:

```text
/opsx-explore    think through the problem, read-only
/opsx-propose    create the change and generate its artifacts
/opsx-apply      implement the tasks
/opsx-verify     check the implementation against the artifacts
/opsx-sync       merge delta specs into the main specs
/opsx-archive    archive the completed change
```

The equivalent CLI surface, if you'd rather drive it yourself:

```bash
openspec list                                # active changes
openspec status --change "<name>"            # artifact and task status
openspec validate "<name>" --type change     # validate artifacts
```

### What CI enforces

The `OpenSpec` workflow runs on pull requests that touch `openspec/`:

- **At most one active change** per pull request.
- **Active changes must be archived before merge.** A pull request that leaves an
  unarchived change fails. Archive it and push.
- Artifacts must validate.

Pull requests that touch no OpenSpec files are not blocked — CI posts a note
suggesting the workflow. Use judgment: a typo fix, a dependency bump, or a
one-line bug fix does not need a change proposal. A new capability, a behavior
change, a new provider, or anything parity-sensitive does.

## Testing

Tests live in the same package as the code they test (white-box), use `testify`'s
`require` for preconditions and `assert` for value checks, and are table-driven
with subtests named `TestFunctionName_Scenario`. Mocks are hand-written; there is
no mocking framework. See [AGENTS.md](AGENTS.md) for the full conventions.

```bash
mise run test              # all Go tests, all modules
mise run test-short        # skip integration/E2E
mise run test-integration  # cross-language integration tests (Go server, Vitest client)
mise run test-conformance  # replay upstream-recorded fixtures
```

Wire-format work needs a cross-language test. If you add or change a
`UIMessageChunk` type, change `PipeUIMessageStreamToResponse` or
`WriteTextStream`, add a streaming mode or response format, modify SSE headers,
or add a provider feature that produces new stream parts, add a scenario under
`test/integration/testserver/` plus a matching Vitest test. See
[`test/README.md`](test/README.md).

Bug fixes should come with a regression test. Where the bug crosses a provider or
UI wire boundary, that regression test should be a conformance fixture.

## Documentation: where things go

Documentation lives in **three** surfaces, each with one job. Keeping content in
the right place is what stops the README from re-bloating and prevents the docs
from drifting out of sync with the code.

| Surface | Owns | Examples |
|---|---|---|
| **godoc** (`doc.go` / per-symbol comments) | API reference | Function signatures, option lists, struct fields, type semantics |
| **Root `README.md`** | Landing page | Pitch, install, ONE quick start, a router into `docs/` |
| **`docs/`** | Concepts, guides, narrative | "Why it works this way", "how do I do X", per-provider setup, best practices |

### The drift boundary

When you're about to document something, ask which question you're answering:

```text
"What are the options / what's the signature?"   → godoc   (pkg.go.dev)
"Why does it work this way / how do I do X?"      → docs/
"Convince me + get me running once"               → README
```

Concretely:

- **Do not** reproduce exhaustive option tables or symbol signatures in `docs/`.
  Link to the symbol on
  [pkg.go.dev](https://pkg.go.dev/github.com/grafana/ai-sdk) instead. godoc is
  generated from the source of truth and cannot drift.
- **Keep the root README a landing page.** New feature deep-dives go in `docs/`
  (narrative) and godoc (reference), with a link from the README's
  "Documentation" router if they warrant top-level discoverability.
- **Centralize user-facing docs in `docs/`.** Don't add per-module `README.md`
  files for user-facing setup or behavior — fold that content into the relevant
  `docs/` page (e.g. a provider goes in `docs/providers/`, a middleware in
  `docs/middleware/`). `doc.go` files stay co-located as the godoc API reference.
  Contributor/tooling READMEs that are not user-facing (e.g. under `test/`) may
  stay where they are.

### docs/ structure

```text
docs/
├── README.md          index / map of the docs
├── getting-started/   installation + the two onboarding paths
├── concepts/          the mental model (architecture, messages, wire, providers)
├── guides/            task-oriented application how-tos
├── providers/         per-provider setup + writing your own
├── middleware/        overview + per-middleware integrations
└── best-practices/    production, error handling, security
```

Conventions:

- Plain GitHub-rendered markdown — no frontmatter, no site-generator syntax.
- Navigation is the `docs/README.md` index plus a per-page
  `← Prev · Up · Next →` footer.
- New pages must be linked from `docs/README.md`.
- Diagrams use mermaid (GitHub-rendered) or ASCII.
- Stub pages use a consistent template (title, intro, a visible **Status: stub**
  note, "what this will cover", "see also", nav footer) so the structure reads
  as intentional.

### Linting the docs

```bash
mise run lint-docs
```

Two layers run:

- **Structural** (`scripts/lint-docs.py`, no dependencies) enforces the rules
  above: dead relative links, dead `#anchor` links, missing nav footers, pages
  missing from the index, and the "no reference tables in `docs/`" contract.
- **Style** (`markdownlint-cli2`, managed by mise; config in
  `.markdownlint-cli2.jsonc`) covers prose/formatting.

Both run in CI (the `docs-lint` job) and as part of `mise run check`.

### Runnable examples

Complete, runnable programs live under [`examples/`](examples) as self-contained
Go modules (with `replace` directives to the local SDK and providers). Guides
link to them rather than embedding large programs.

Examples must `go build` and include deterministic credential-free behavioral
tests. New example modules are picked up automatically by `mise run
build-examples` and `mise run test-examples`, both wired into blocking CI.
Examples that call a real provider need credentials to *run*, but must always
compile and test without them.

## Submitting a pull request

Before you mark a pull request ready for review:

```bash
mise trust          # trust the project config on first checkout
mise deps           # install workspace dependencies
mise run check      # fmt + vet + lint + lint-docs + test
mise run build      # verify modules and examples compile
```

Checklist:

1. **Commits are signed.** See [Before you contribute](#before-you-contribute).
2. **Title uses a conventional commit prefix** (`feat:`, `fix:`, `docs:`,
   `refactor:`, `test:`, `chore:`, `ci:`, `build:`, `perf:`, `style:`). Write it
   for a reader with no other context.
3. **Description** explains what changed, why, and how you validated it. For
   parity-sensitive work, name the upstream package and version you compared
   against.
4. **Tests** are added or updated — a regression test for a bug fix, a
   conformance fixture where the behavior crosses a wire boundary.
5. **Parity artifacts** are updated if the change is parity-sensitive:
   `upstream.yaml`, `PARITY.md`, and any regenerated fixtures.
6. **OpenSpec change is archived** if the pull request touches `openspec/`.
7. **Docs** are updated for any user-visible change, in the right surface.
8. **Branch is synced** with `main`.

Reviewers are assigned automatically from
[`.github/CODEOWNERS`](.github/CODEOWNERS). Community pull requests need a
maintainer to trigger CI.

## Dependency management

The repository uses Go modules across several module roots, plus a pnpm
workspace under `test/` for TypeScript-side harnesses.

`ai-gateway/` is intentionally absent from the root `go.work`. Gateway code may
import explicitly pinned SDK modules, but no module outside `ai-gateway/` may
import or require `github.com/grafana/ai-sdk/ai-gateway`.

```bash
mise run tidy        # go mod tidy across all modules
```

Commit `go.mod` and `go.sum` changes together. When adding a dependency to a
provider or middleware module, run tidy from that module's directory.

The `test/` pnpm workspace is supply-chain hardened: `blockExoticSubdeps`,
`strictDepBuilds`, and a `minimumReleaseAge` gate in
`test/pnpm-workspace.yaml`. Do not bypass those settings to land a version bump.
Dependency updates are otherwise automated via Renovate
([`renovate.json`](renovate.json)).

## Reporting security issues

Do not report security vulnerabilities in public issues. Follow the
[security policy](https://github.com/grafana/ai-sdk/security/policy) for
coordinated disclosure.

## License

Contributions outside [`ai-gateway/`](ai-gateway/) are licensed under
[Apache-2.0](LICENSE). Contributions under `ai-gateway/` are licensed under
[AGPL-3.0-only](ai-gateway/LICENSE).
