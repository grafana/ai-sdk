# Forced tool-choice request goldens

These are synthetic HTTP request projections captured from the registered
`@ai-sdk/openai@4.0.72` npm package, not provider recordings or imported provider
response fixtures. They establish request encoding, not live OpenAI acceptance.

Reference:
- Package tag: `@ai-sdk/openai@4.0.72`, commit
  `fe37eb0f67c1028c4bed0f4640ae17c57145a818`.
- Registered aggregate commit: `4e8c387622ee1bb0d55841664416d38754d5c9a3`.
- Exact source: `packages/openai/src/responses/openai-responses-prepare-tools.ts`,
  ordinary forced-choice branch (lines 539–561). The source/tests and package
  manifest at the package tag match the registered aggregate commit.

From the repository root:

```sh
mise deps
node providers/openai/testdata/forced_tool_choice/capture.mjs
```

`capture.mjs` resolves the provider from `test/conformance/tools`' pinned Node
installation and asserts the package version. It calls low-level `doGenerate`
and `doStream` with model `gpt-4o`, user text `hi`, one provider tool with empty
args, and a forced named choice. The declared tool names are `terminal` for
`openai.shell`, `localTerminal` for `openai.local_shell`, and `discover` for
`openai.tool_search`. Each is selected by both its canonical name and configured
alias, in both modes (twelve cases). The fetch override captures actual encoded
requests and returns minimal synthetic success responses without network access.

Each golden retains the complete `tools` and `tool_choice` fields, and `stream`
when present. No names, discriminators, array order, or choice fields are
normalized. JSON object key order is immaterial. Unrelated fields such as model,
prompt and provider default options, and all headers/credentials, are excluded
by this projection; they are outside this issue's regression contract. Generate
requests omit `stream`; streaming requests contain `stream: true`.

Regenerate twice and compare files to verify deterministic capture. The Go
`TestForcedToolChoice_RequestSnapshots` uses the same inputs through actual
`DoGenerate`/`DoStream` encoding and compares this projection semantically.
Expectations must not be generated from the Go serializer. Existing conformance
provider response inputs and auto-choice expectations remain untouched.
