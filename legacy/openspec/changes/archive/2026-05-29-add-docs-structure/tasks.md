## 1. Scaffold the docs/ skeleton

- [x] 1.1 Create the `docs/` directory with subfolders `getting-started/`,
  `concepts/`, `guides/`, `providers/`, `best-practices/`
- [x] 1.2 Define a shared stub-page template (title, intro, visible status note,
  "see also", `← Prev · Up · Next →` footer)
- [x] 1.3 Create stub pages for all planned files:
  getting-started/{installation,full-stack-chat,backend-only}.md;
  concepts/{architecture,messages,wire-protocol,providers}.md;
  guides/{tools,tool-approval,structured-output,agent-loops,streaming-http,retry-and-timeout,fallback-and-registry,middleware}.md;
  providers/{overview,anthropic,grafana-cloud,writing-a-provider}.md;
  best-practices/{production,error-handling,security}.md
- [x] 1.4 Write `docs/README.md` index linking every page, grouped by section

## 2. Write getting-started/ (complete)

- [x] 2.1 `installation.md` — go get for root + provider modules, version notes
- [x] 2.2 `full-stack-chat.md` — Go backend + `@ai-sdk/react` headline path,
  links to a runnable program under `/examples`
- [x] 2.3 `backend-only.md` — `GenerateObject` / structured-output path

## 3. Write concepts/ (complete)

- [x] 3.1 `architecture.md` — 3-layer event model + request-flow mermaid
- [x] 3.2 `messages.md` — `UIMessage` vs `ModelMessage`, conversion
- [x] 3.3 `wire-protocol.md` — `UIMessageChunk` / SSE compatibility
- [x] 3.4 `providers.md` — the `LanguageModel` interface and abstraction
- [x] 3.5 Ensure all concept pages link reference symbols to pkg.go.dev (no
  reproduced signatures/option tables)

## 4. Trim the root README to landing-page role

- [x] 4.1 Remove relocated reference sections (Core APIs/options table, Tools
  deep-dive, Tool approval, Messages, HTTP helpers, provider detail, retry,
  registry, stop conditions, prepare-step)
- [x] 4.2 Keep/trim: pitch, "Why", feature bullets, install, ONE quick start
  (full-stack chat), one architecture mermaid
- [x] 4.3 Add a "Where to go next" router section linking to `docs/README.md`
  and key sections
- [x] 4.4 Verify README is ~120–150 lines and contains no duplicated reference
  content

## 5. Establish the examples linkage

- [x] 5.1 Create the top-level `/examples` directory (and a brief
  `/examples/README.md` if helpful)
- [x] 5.2 Add the full-stack chat example program referenced by
  getting-started, ensuring it passes `go build`
- [x] 5.3 Link the example from the relevant getting-started/guide pages

## 6. Verify the ownership contract and navigation

- [x] 6.1 Confirm no `docs/` page reproduces exhaustive option tables or symbol
  signatures (links to pkg.go.dev instead)
- [x] 6.2 Confirm package-local READMEs (`providers/grafana/README.md`,
  `test/README.md`) and all `doc.go` files are unmoved; `docs/providers/*`
  link to them
- [x] 6.3 Link-check all `docs/` and README relative links
- [x] 6.4 Verify every page renders on GitHub (plain markdown, mermaid ok) and
  has a working nav footer; index lists every page
- [x] 6.5 Run `openspec validate add-docs-structure` and resolve issues
