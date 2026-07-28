## Context

The project currently documents itself through three overlapping surfaces:

- **godoc** — `doc.go` package overviews plus per-symbol comments. Authoritative
  API reference, rendered on pkg.go.dev, co-located with code.
- **root `README.md`** — ~712 lines doing pitch + install + quick start + full
  API reference + provider detail + ops guidance.
- **package-local READMEs** — `providers/grafana/README.md`, `test/README.md`,
  correctly co-located with their modules.

The README is the pain point: it duplicates reference content that godoc already
owns (e.g. the `StreamText` options table mirrors `options.go`), and it mixes
audiences (a newcomer wanting one working example vs. someone looking up the
exact shape of `ToolApprovalDecision`). As features and providers grow, this
file grows unboundedly and drifts from godoc.

This change introduces a `docs/` tree and a clear ownership contract so each
surface has one job. Delivery is GitHub-native (no site generator), per the
explore-phase decisions.

## Goals / Non-Goals

**Goals:**

- A discoverable `docs/` taxonomy that scales as features and providers are
  added without reorganization.
- A single, enforceable rule for where any piece of content belongs
  (README vs godoc vs docs/) to prevent re-growth and drift.
- A clean root README focused on landing + routing.
- GitHub-readable today; migratable to a static site later without rewrites.
- Honest, compilable example code that cannot silently rot.

**Non-Goals:**

- No static site generator, frontmatter, or sidebar config in this change.
- No changes to SDK behavior, APIs, wire format, or any Go source other than
  README/docs and (optionally) new `/examples` programs.
- Not moving package-local READMEs or `doc.go` files — Go convention keeps them
  with code.
- Not writing every guide to completion in this change; later sections are
  stubbed with a template (see phasing).

## Decisions

### 1. Five-folder taxonomy under `docs/`

```
docs/
├── README.md              docs index (the map); linked prominently from root README
├── getting-started/       installation, full-stack-chat, backend-only
├── concepts/              architecture, messages, wire-protocol, providers
├── guides/                tools, tool-approval, structured-output, agent-loops,
│                          streaming-http, retry-and-timeout,
│                          fallback-and-registry, middleware
├── providers/             overview, anthropic, grafana-cloud, writing-a-provider
└── best-practices/        production, error-handling, security
```

Rationale: this mirrors the upstream Vercel AI SDK mental model
(Foundations → Guides → Providers → ...), which TS users crossing to Go already
recognize. `concepts/` is treated as first-class because the wire-compatible
3-layer event model is the thing newcomers most need explained, and the current
README buries it under "Architecture".

The user's four requested doc *types* map cleanly onto folders:

| Requested type            | Folder(s)                               |
|---------------------------|-----------------------------------------|
| usage docs                | `getting-started/` + `guides/`          |
| best practices            | `best-practices/`                       |
| common scenarios/examples | `/examples` (code) linked from `guides/`|
| (mental model — implied)  | `concepts/`                             |

Alternatives considered:
- *Flat docs/ folder* — rejected; doesn't scale, no audience separation.
- *Folder-per-provider mirroring providers/ in code* — kept only inside
  `docs/providers/`; the rest is task/concept oriented, which ages better than
  mirroring package layout.

### 2. The README ↔ godoc ↔ docs ownership contract (the drift boundary)

```
"What are the options / what's the signature?"   → godoc   (pkg.go.dev)
"Why does it work this way / how do I do X?"      → docs/
"Convince me + get me running once"               → README
```

Concretely:
- **README target ~120–150 lines**: pitch + "Why" + trimmed feature bullets +
  install + ONE quick start (full-stack chat) + "Where to go next" router +
  one architecture mermaid with a link into `concepts/`.
- **Removed from README → relocated**: Core APIs option tables, Tools
  deep-dive, Tool approval, Messages, HTTP helpers, provider detail, retry,
  registry, stop conditions, prepare-step. Narrative goes to `docs/`; reference
  (option lists, signatures, fields) goes to godoc.
- **Guides describe options narratively and link to pkg.go.dev** — they do not
  reproduce exhaustive option tables (decision: options table moves to godoc).

Rationale: a single boundary rule is what prevents the README from re-bloating.
Reference duplicated as prose always drifts; godoc is generated from the source
of truth.

Alternative considered: keep a curated `guides/options.md` cheat-sheet table.
Rejected for this change to avoid re-introducing the duplication we're removing;
can revisit if godoc proves insufficient for scanning.

### 3. Example code lives in `/examples`, docs link to it

Full runnable programs go under a top-level `/examples` Go directory so they are
`go build`-verifiable and cannot silently rot. Guides contain short
illustrative snippets inline but link to the complete program for anything
non-trivial. This also keeps `docs/` free of large code blocks.

Consequence: no `examples/` folder inside `docs/` — it would only be prose
pointing at `/examples`, so we fold the pointers directly into the relevant
guides.

### 4. GitHub-native navigation

No site generator. Navigation is hand-rolled and survives a future migration:
- `docs/README.md` is the master index (grouped link list).
- Each doc ends with a `← Prev · Up · Next →` footer.
- Root README links prominently to `docs/README.md`.
- Diagrams use mermaid (GitHub renders it) and ASCII where richer.
- Plain markdown only — no frontmatter — so files render on GitHub now and can
  gain frontmatter later if a site is adopted.

### 5. Package-local READMEs and doc.go stay put

`providers/grafana/README.md`, `test/README.md`, and all `doc.go` files remain
co-located with code (Go/pkg.go.dev convention). `docs/providers/grafana-cloud.md`
is the narrative guide that links to `providers/grafana/README.md` for
auth/wire minutiae rather than copying it.

### 6. Authoring phasing

1. Create the full skeleton (all folders + `docs/README.md` index + stub files
   with a shared template: title, intro, "see also", nav footer).
2. Write `getting-started/` and `concepts/` to completion (onboarding + mental
   model are highest leverage).
3. Trim the root README to its landing-page role and wire the router links.
4. Fill `guides/`, `providers/`, `best-practices/` incrementally (out of scope
   to complete all here; stubs are acceptable end-state for this change).

## Risks / Trade-offs

- **Drift between docs/ and godoc** → Mitigated by the explicit ownership
  contract (Decision 2): docs never reproduce signatures/option tables; they
  link to pkg.go.dev.
- **Stub files look unfinished on GitHub** → Mitigated by a consistent stub
  template with a visible "Status: stub / coming soon" note and working nav, so
  the structure reads as intentional rather than broken.
- **Hand-rolled prev/next footers go stale when files are added/reordered** →
  Mitigated by keeping `docs/README.md` as the single source of ordering; the
  footers are secondary. Acceptable maintenance cost for a no-build setup.
- **README link rot after the big trim** → Mitigated by a link-check pass and
  by routing through the `docs/README.md` index rather than deep-linking
  everywhere from the root README.
- **`/examples` programs require real API keys to run** → They must `go build`
  in CI even if they need credentials to *run*; build-only verification keeps
  them honest without secrets.

## Open Questions

- Should `/examples` programs be wired into CI as `go build`-only checks now, or
  deferred until the first example is added?
- Do we want a top-level `CONTRIBUTING`-style note on the docs ownership
  contract so future PRs don't re-bloat the README, or is the docs-structure
  spec enough?
