## Context

The repository currently has five self-contained example modules: complete text generation, terminal streaming, a tools loop, structured output, and a React-compatible chat server. The first two largely duplicate onboarding material, the tools example is disconnected from the frontend path where progressive tool state matters most, and every module repeats the same provider bootstrap and dependency graph. CI builds examples but does not execute tests in their modules.

The registered upstream baseline is `ai@7.0.37` and `@ai-sdk/react@4.0.40`; both tags resolve to upstream commit `3c98985d44482f36704acd1a76f351f329028b5f`. At that baseline, upstream keeps atomic recipes in docs or a shared script workspace and uses compact application examples, notably `examples/next-agent`, to demonstrate agents, tools, streaming, and React together. This change follows that information architecture with Go-idiomatic modules and tests.

## Goals / Non-Goals

**Goals:**

- Reduce the runnable collection to two recognizable application outcomes.
- Demonstrate tools and agent orchestration through the React-compatible chat boundary.
- Preserve structured extraction as an independent non-conversational workflow.
- Make each example's behavior deterministic and credential-free under test.
- Run every example module's tests in blocking CI.
- Verify agent tool state through the pinned upstream React hook.

**Non-Goals:**

- Include a React project or add Node dependencies under `examples/`.
- Add live provider calls to CI.
- Turn examples into exhaustive coverage of providers, output modes, middleware, reliability controls, approval, or gateway infrastructure.
- Change exported SDK APIs, orchestration semantics, UI chunks, or SSE framing.
- Preserve old example paths after documentation links are migrated.

## Decisions

### Keep two self-contained modules organized by application outcome

`examples/agent-chat` will replace `chat-server` and `tools-agent`. It will construct a reusable `ToolLoopAgent` with a deterministic typed weather tool and expose a `useChat`-compatible handler. `examples/structured-extraction` will retain the alert-triage workflow under an outcome-oriented name.

Separate modules preserve copyability and independent `go run .` behavior. With only two modules, their dependency metadata is an acceptable trade-off. A single examples module was rejected because it optimizes lockfile count at the cost of making each application less self-contained.

### Keep atomic generation and raw stream consumption in onboarding docs

The root README and installation guide remain the first complete `GenerateText` path. The backend guide remains the focused `FullStream` recipe. Separate runnable directories for those calls are removed because they do not cross an integration boundary or add application context.

A command with modes such as `generate`, `stream`, `tools`, and `object` was rejected: it reduces top-level directory count while moving the same conceptual choices behind application-specific CLI scaffolding.

### Isolate provider construction from testable application logic

Each example will keep Anthropic credential lookup and model construction in `main`, while application constructors accept `provider.LanguageModel`. Tests in the same module will supply small scripted models that emit provider stream parts. The tests exercise the same agent, handler, tool executor, schema, and rendering paths used at runtime without credentials or network access.

No reusable public mock-model package will be introduced. The fakes remain local because they encode each example's scenario rather than a general SDK testing API.

### Test behavior at two boundaries

Example-module Go tests will cover:

- `agent-chat`: invalid request handling, the scripted tool-call/tool-result/final-answer loop, emitted UI-message SSE chunks, and stream termination;
- `structured-extraction`: response-format configuration, typed valid output, validation failure, and rendered result behavior.

A `test-examples` mise task will discover every `go.mod` below `examples/` and run `go test -short ./...`. The blocking CI test job will invoke it through `test-short`, while `build-examples` continues to verify all commands compile.

The existing cross-language integration harness will add an agent-tool scenario consumed by the pinned `@ai-sdk/react` `useChat`. It will assert progressive tool state and final text. This scenario validates the same public composition as `agent-chat`; it intentionally does not import the example module, avoiding a dependency from the conformance/integration harness onto user-facing sample code.

### Keep the example backend Go-only

The existing full-stack guide continues to own the minimal React client and Vite setup. `agent-chat` is named as a backend rather than claiming to be a complete full-stack project. Adding a second package-manager surface under `examples/` was rejected because the repository already has a pinned cross-language integration workspace and the user-approved direction is Go-only.

## Risks / Trade-offs

- [The combined agent chat is less minimal than the old chat handler] → Keep one deterministic tool, a small agent constructor, and a thin handler; route deeper concepts to guides.
- [Removing the raw streaming command reduces copyable terminal code] → Keep the focused guide snippet and retain SDK-level stream tests; examples are no longer the atomic recipe surface.
- [Scripted provider streams may accidentally test implementation details] → Assert application-visible behavior, request options needed by the scenario, and protocol chunks rather than internal goroutine timing.
- [The React integration scenario can drift from the example] → Keep both flows intentionally small and assert the same tool name, input, output, and final answer.
- [Live provider behavior is not exercised in CI] → Continue build verification and document a manual credentialed run; deterministic CI owns application correctness while provider conformance owns provider behavior.
- [Old deep links break] → Update every in-repository reference in the same change; backward-compatible directories or redirects are out of scope.

## Migration Plan

1. Add the new modules and their tests while the old examples still exist.
2. Add `test-examples` and cross-language agent-tool integration coverage.
3. Remove the superseded modules.
4. Update README, docs, contributor guidance, and example navigation.
5. Run example tests/builds, integration tests, docs lint, short tests, and the full focused validation set.

Rollback is a source revert; no runtime state or external deployment migration is involved.

## Open Questions

None. The user approved the Go-only recommendation and deterministic blocking CI coverage.
