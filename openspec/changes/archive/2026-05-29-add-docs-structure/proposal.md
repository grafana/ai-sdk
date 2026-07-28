## Why

The root `README.md` has grown to ~712 lines and carries six jobs at once:
project pitch, install, quick start, full API reference, per-provider details,
and operational guidance. As features and providers keep landing, this single
file becomes harder to navigate and increasingly duplicates content that godoc
already owns, creating drift risk. The project needs a dedicated, structured
`docs/` tree so the README can return to being a clean, focused landing page.

## What Changes

- Introduce a `docs/` directory with a deliberate taxonomy: `getting-started/`,
  `concepts/`, `guides/`, `providers/`, and `best-practices/`, plus a
  `docs/README.md` index that maps the whole tree.
- Establish an explicit content-ownership contract between three doc systems:
  - **godoc** (`doc.go` / per-symbol) owns API reference — signatures, option
    lists, field-level detail. Authoritative, lives with code.
  - **root README.md** owns the landing page — pitch, install, ONE quick start,
    and a "where to go next" router into `docs/`.
  - **`docs/`** owns everything that needs room: concepts, task guides,
    per-provider narrative, best practices.
- **BREAKING (docs only):** Remove the deep reference content from the root
  README (Core APIs option tables, Tools deep-dive, Tool approval, Messages,
  HTTP helpers, provider detail, retry/timeout, registry, stop conditions,
  prepare-step). This content moves to `docs/` (narrative) and godoc (reference).
- Runnable example **code** lives under a top-level `/examples` Go directory
  (compilable / `go build`-verified); docs link to it rather than embedding
  large drift-prone programs.
- GitHub-native delivery: plain markdown, relative links, mermaid diagrams, and
  hand-rolled nav (index + prev/up/next footers). No site generator, no
  frontmatter — but written so a future static site can adopt it.
- Phased authoring: establish the full skeleton, write `getting-started/` and
  `concepts/` to completion first, then stub remaining sections with a
  consistent template.

## Capabilities

### New Capabilities
- `docs-structure`: Defines the canonical `docs/` directory taxonomy, the
  README/godoc/docs content-ownership contract, GitHub-native navigation
  conventions, the relationship to runnable `/examples`, and the authoring
  template/phasing that keep the documentation coherent as the project grows.

### Modified Capabilities
<!-- None. No SDK behavioral specs change; this is documentation-only. -->

## Impact

- **New**: `docs/` tree and `docs/README.md` index.
- **Modified**: root `README.md` (trimmed to landing-page role with router into
  `docs/`).
- **Possibly new**: top-level `/examples` Go directory for runnable samples
  (created/populated incrementally as guides reference them).
- **Unchanged**: package-local READMEs (`providers/grafana/README.md`,
  `test/README.md`) and all `doc.go` files stay where they are, co-located with
  code per Go convention; `docs/providers/*.md` link to them rather than moving
  them.
- **No code/behavior changes**: this is documentation-only; no Go source, APIs,
  or wire format are affected.
