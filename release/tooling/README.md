# Release proposal preview

This tool builds proposals using locked release-please 17.6.0, matching the
pinned GitHub Action. It does not create pull requests, tags, or releases.
The HTTP transport permits GitHub reads and GraphQL queries only.

From the repository root:

```bash
npm ci --prefix release/tooling --ignore-scripts
npm test --prefix release/tooling
GH_TOKEN="$(gh auth token)" node release/tooling/preview.mjs . \
  > /tmp/ai-sdk-root-preview.json
```

The optional argument is an exact component path from
`release-please-config.json`: `.` selects root, `providers/openai` selects
OpenAI, and omitting it previews every configured component. Unknown paths fail.
Credentials are passed through the environment rather than command arguments.

The preview reads configuration, release state, and history from GitHub `main`.
Its JSON output includes the source SHA, proposed branches, versions, PR bodies,
and original/generated file contents. Diagnostics go to stderr. If `main`
advances during preparation, the command fails; discard that run and retry.
Individual requests have a 30-second timeout.

This is proposal evidence, not publication approval. It does not run library
readiness checks or prove that the release App can create signed commits and
trigger required checks. Release automation remains disabled.

The exported `prepareCandidates` function uses the pinned manifest API's
component filter. The pinned Action's `path` input does **not** filter manifest
releases; do not use that input to limit an App write test to root.
Any future write path must retain its own authorization checks.

## Commit parsing

Release-please parses the full squash commit message. A valid Conventional
Commit title can still be discarded when a copied dependency changelog in the
body fails parsing. Review parser diagnostics before creating release PRs.

Release-please supports a `BEGIN_COMMIT_OVERRIDE` / `END_COMMIT_OVERRIDE`
section in the merged PR body to correct the release message without rewriting
Git history. This tool does not add overrides or silently fall back to titles:
doing so could lose breaking-change notes or other release metadata.
The tests exercise the supported override against the pinned parser.

Dependency resolution for this isolated tool is captured in `package-lock.json`.
Use `npm ci --ignore-scripts`, not an unpinned installation or lifecycle scripts.

For the opt-in App write test and its approval boundary, see the
[controlled PR exercise](APP-PR-EXERCISE.md). The preview command remains read-only.

For read-only decisions about already merged library candidates, see
[merged library authorization](AUTHORIZATION.md). It verifies exact revision evidence
and selected readiness without creating tags or releases.

For the disabled writer and partial-failure recovery contract, see
[library publication integration](PUBLICATION.md). Live activation remains separate.
