## Context

The authenticated Gateway command from #151 composes only direct Anthropic models over one hardened model HTTP client. `providers/openai` implements the OpenAI Responses API and exposes both `NewResponses`, which builds its client through `openai-go`'s `NewClient`, and `NewResponsesWithClient` for integrations that own transport and authentication.

The registered baseline is `@ai-sdk/openai@4.0.41` and `@ai-sdk/gateway@4.0.52` at Vercel commit `d76eb85a`. `openai-go/v3` v3.48.0 `NewClient` prepends defaults read from `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_ORG_ID`, `OPENAI_PROJECT_ID` and `OPENAI_CUSTOM_HEADERS`; the option that disables them lives in the SDK's internal `requestconfig` package and cannot be passed from another module. Upstream has no Gateway command equivalent, so this change is a Grafana host composition over the registered public Gateway client.

This behavior is implemented at this change's head.

## Goals / Non-Goals

**Goals:**

- Let operators declare named OpenAI Responses provider instances and route public models and aliases to them.
- Keep explicit Gateway configuration authoritative over ambient `OPENAI_*` SDK environment state.
- Reuse the shared hardened model transport, startup endpoint validation and privacy guarantees.

**Non-Goals:**

- OpenAI Chat Completions, Azure OpenAI, OpenAI-compatible, Bedrock and Vertex provider configuration.
- Per-provider limits or renaming the `anthropic.response-*` flags.
- Reasoning content, provider options such as `store`, or other capability families beyond the text-only runtime.
- Changing `providers/openai` construction or its parity with upstream.

## Decisions

### Assemble the Responses service from explicit options

The Gateway builds `responses.NewResponseService` with `WithAPIKey`, `WithBaseURL`, `WithHTTPClient` and `WithMaxRetries(0)`, wraps it in `openaisdk.Client{Responses: ...}`, and passes that to `providers/openai.NewResponsesWithClient`. This never calls `NewClient`, so no environment default is read. Passing request options through `NewResponses` would still merge environment defaults first; an explicit base URL would override `OPENAI_BASE_URL`, but organization, project and custom headers would still be sent. `providers/openai` cannot disable the defaults itself without the SDK exposing its internal opt-out.

### Default the base URL in the Gateway

`NewResponseService` sets no default base URL, and an empty `WithBaseURL` produces an unusable URL, so the Gateway supplies `https://api.openai.com/v1` when configuration omits `baseURL`. The default is an HTTPS constant, so production endpoint validation needs no special case.

### Share the hardened model client and limits

OpenAI Responses models use the same `*http.Client` and `anthropic.response-*` limits as Anthropic. Those limits apply to any model upstream in the same way, and per-type flags would not solve per-instance tuning. The flag help text now states that it covers all provider types.

### Name the type `openai`

The type matches the `providers/openai` module, which is Responses-only, and upstream `@ai-sdk/openai`, whose default language model is Responses.

## Risks / Trade-offs

- Reasoning models fail: OpenAI Responses returns a reasoning item for reasoning models, `providers/openai` converts every such item into a reasoning part even without a summary, and the text-only runtime rejects it (#111). Unary calls fail after the backend has generated the response; streams end with the retryable terminal internal error.
- Requests omit `store` because provider options are not accepted, so OpenAI's server-side retention default applies to Gateway traffic under the configured account.
- An upstream redirect or empty stream surfaces as a terminal internal error, so operators cannot tell it apart from a Gateway failure.
- Fake Responses servers modelled on a recorded stream establish deterministic command, transport and privacy evidence only; live OpenAI inference was not run.
