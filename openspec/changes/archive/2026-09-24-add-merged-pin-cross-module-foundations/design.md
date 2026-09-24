## Context

The repository has eleven published Go modules (root, Gateway, providers, middleware), a root `go.work` for SDK modules and local-only test/example modules, and a standalone public-proxy gate (`mise run verify-module-resolution`). `scripts/verify-ai-gateway-boundary.sh` currently couples structural AGPL/Apache checks with root, Grafana client, and Gateway standalone builds/tests. Gateway command integration normally builds with `GOWORK=off`; other test builds select root-workspace or isolated behavior inconsistently. Some committed internal pseudo-version requirements are branch-only. The registered Vercel AI SDK baseline is `test/conformance/upstream.yaml`; this change affects build governance, not upstream behavior.

## Goals / Non-Goals

**Goals:** Verify published internal pins are merged, provide explicit candidate-source Gateway integration, separate structural from standalone checks, and select one published module for independent validation without changing existing required checks.

**Non-Goals:** Make `main` independently consumable at every revision, switch required source PR checks, decide release-readiness or deployment authorization, create tags, publish artifacts, or change Vercel parity behavior. #245 owns the gate transition and prerequisite-freshness policy.

## Decisions

### Canonical ancestry, not downloadability

Discover published module roots via tracked `go.mod` files and their declared module paths, respecting nested module boundaries; exclude deliberate `examples/` and `test/` local-only modules. For each published module inspect both its explicit `require` entries and its selected `go list -m -json all` graph under `GOWORK=off` with readonly manifests. Check internal module paths against the set of declared published modules, not a loose path prefix. Resolve selected tags or pseudo-versions through Go's module/VCS metadata to their full immutable Git commit; reject unknown formats, inconsistent resolutions, and failures. Resolve against an explicitly fetched `refs/heads/main` anchor from the fixed canonical `https://github.com/grafana/ai-sdk.git` URL in an isolated Git repository, then use commit ancestry, not local `origin/main`, PR HEAD, GitHub's synthetic merge ref, or proxy availability. Fetch enough history to establish ancestry (including shallow CI clones) or fail closed. Keep network/Git verification and local Git-history fixtures separable so deterministic tests cover merged versions/tags, branch-only commits, unmerged tags, unverifiable revisions, and false ancestry via synthetic merge refs. A merged pseudo-version is valid even without a tag. No trust in fork-controlled refs or input-provided remote addresses.

Alternative: verify only `go mod download` succeeds or use the checkout's `origin/main`. Both can admit unmerged commits, especially in fork and PR-merge contexts.

### Explicit integration workspace

Add a separately named checked-in `.work` file with Gateway and local root SDK/provider/middleware modules as explicit `use` entries. Keep root `go.work` unchanged and Gateway-free. Set `GOWORK` to an absolute path only in dedicated source-integration commands. Audit the real build sites: the Gateway command build in `gateway-command.test.ts`, Gateway runtime `testserver` build in `runtime-integration.test.ts`, Go capture helpers in `go-client-capture.ts`, and `test/integration/global-setup.ts` / its Go testserver. The Gateway command and Gateway-dependent testserver builds must support explicit candidate-source integration; retain independently callable pinned-dependency variants, including client capture paths intended to prove standalone behavior. The ordinary root integration testserver already uses root-workspace SDK source and should not acquire Gateway access without a demonstrated dependency. Use `GOWORK=off` explicitly for isolated tests, image and release/production builds. Never set integration `GOWORK` as a repository-wide default. Assert workspace resolution with `go list -m -json` and use a controlled candidate-source change in deterministic build/behavior tests to distinguish candidate source from the pinned dependency, not just workspace-file membership.

Alternative: add Gateway to root `go.work` or rely on an ambient `GOWORK` value. Both silently alter production-oriented builds and invalidate the existing SDK-only boundary.

### Structural checks and standalone checks remain distinct

Refactor `verify-ai-gateway-boundary.sh` into a small policy check: verify the license files and Gateway module identity, keep Gateway and replacements out of root `go.work`, reject Gateway references in tracked non-Gateway Go files and module manifests (including nested modules), and check root SDK and Grafana client module graphs. Treat `ai-gateway/` as the AGPL directory boundary; the shared module inventory validates tracked module paths. Do not build/test again inside the structural check. The existing all-published-modules gate already builds/tests the SDK root and Grafana client with `GOWORK=off`, a fresh public-proxy cache, readonly manifests, and no published-module replacements. A separate copy of the repository without Gateway source duplicates that evidence and couples root tests to checkout layout, so do not add one. Add a module-root/path-selectable standalone entry point using the same gate. Unknown or local-only selections fail. The existing `module-resolution` CI job continues calling the all-module and boundary gates, plus the ancestry guard after migration. Do not weaken image validation or publication/deployment dependencies.

Alternative: reimplement standalone checks per module or use the source integration workspace for release validation. Those would duplicate policy and hide incompatible requirements.

## Risks / Trade-offs

- [Canonical Git and Go proxy unavailable, incomplete/shallow history, or mismatched version metadata] → Fail closed with an actionable module/version/anchor diagnostic; do not fall back to fork or synthetic refs.
- [A pin's required behavior is only available on a branch] → Stop migration and escalate; do not repin by timestamp/title or create an unmerged prerequisite commit.
- [Workspace integration accidentally contaminates release builds] → Explicit `GOWORK` per entry point, integration-vs-standalone resolution tests, production Dockerfile `GOWORK=off`, and retained standalone CI.
- [Nested modules or test placeholders classified incorrectly] → Reuse tracked module inventory, scan all non-Gateway source paths and manifests, and keep published-module replacement checks in standalone validation. The policy check is not a legal audit for copied code or third-party licenses.
- [Pin ancestry alone mistaken for compatibility] → Keep standalone public-proxy checks and record old/new pin provenance plus behavior-specific test results.

## Migration Plan

1. Add deterministic checker/selector/workspace and boundary tests without changing required checks; inventory every real published internal pin and selected graph.
2. For each unmerged pin, identify an already-merged revision with the needed behavior, record the module path, old/new versions and full commits, behavior justification and standalone test results in the implementation PR for review; update requirements/sums and run standalone readonly public-proxy tests. Block and ask maintainers if no equivalent merged revision exists.
3. Enable the merged-pin guard in the existing required CI job only after the migrated tree passes; retain all existing all-module, parity, integration, boundary, image, and deployment checks. Validate fork PR and shallow-checkout behavior.
4. Roll back the tooling/pin change as one normal PR if it causes unexpected failure; do not bypass the current standalone checks or temporarily relax publication gates.

## Open Questions

None blocking the proposal. The identified Gateway-owned cross-language testserver and command build paths need candidate-source selection, while pinned-dependency probes and the SDK-only integration testserver retain their separate evidence roles.
