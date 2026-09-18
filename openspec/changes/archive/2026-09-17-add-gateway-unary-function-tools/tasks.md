## 1. Baseline and failing contracts

- [x] 1.1 Verify integrated WP7/WP8 heads and exact registered upstream; classify provider-contract, provider-implementation and Gateway-client coverage in PARITY, keeping prerequisite landing acceptance separate from local implementation completion.
- [x] 1.2 Add exact-client unary tool request/response cases and a two-request continuation through the real handler; document the actual base's tools-unsupported behavior without claiming a historical red-test run.
- [x] 1.3 Add negative mode, role, inactive-arm, required-empty, JSON-null, Strict-presence and reserved function-option tests.

## 2. Apache prerequisites

- [x] 2.1 Audit existing provider.Tool, ToolResultOutput and Go client projectTool/projectOutput; fix only demonstrated domain/validation gaps while preserving Strict *bool.
- [x] 2.2 Add deterministic Anthropic native-request cases for schema/examples/strict false/choice/result continuation; update affected converters only, and inspect Bedrock/OpenAI regressions if shared domain changes.
- [x] 2.3 Extend existing Go unary response and request mapping where differential cases fail; retain bounded reads and Vercel-owned warnings/request/response normalization.
- [x] 2.4 Extend reusable normalized tool observation only where required and verify standalone Apache tests.
- [x] 2.5 Commit Apache prerequisites separately and obtain an approved immutable proxy-resolvable pin before dependent Gateway release validation; use no committed replacement.

## 3. Gateway unary support

- [x] 3.1 Add typed unary function DTO mapping after complete schema validation; gate streaming until WP12 and preserve fixed unsupported-family errors.
- [x] 3.2 Map text/json/error-text/error-json/text-only-content results and assistant calls with exact selected-arm semantics; reject deferred result options and approvals/media.
- [x] 3.3 Extend private unary DTOs and all preflight/final bounds for tool calls; reject providerExecuted true and dynamic true before HTTP 200, retain the supported-union guard for provider results/preliminary behavior, and verify both clients receive the fixed error with zero local execution. Test false/absent markers separately; update local output schemas and raw privacy tests.
- [x] 3.4 Preserve one logical middleware chain and metadata-only filtering; add actual exported-payload/log/metric assertions for two independent generations.
- [x] 3.5 Integrate WP9 fallback-route rejection when present, including tool history without new definitions and zero physical invocation.

## 4. Acceptance and handoff

- [x] 4.1 Pass authenticated exact Vercel and Go unary round trips with captured native requests, local deterministic execution, result continuation and final text.
- [x] 4.2 Run affected module tests, Gateway tests with GOWORK=off committed pins, ProviderWire contract checks and mise run parity-check; preserve fixture provenance.
- [x] 4.3 Update narrative Gateway scope/docs and PARITY with supported unary arms, metadata-only tool observation and deferred WP12/13/14/15/18/21 boundaries.
- [x] 4.4 Extend existing internal rollout smoke only if this capability is being enabled after WP10; otherwise record the activation handoff.
- [x] 4.5 Validate this OpenSpec change strictly, run git diff --check and report evidence without marking WP12 complete.

Apply handoff: direct-route loops and full command/safe-JWKS deterministic native
unary continuation pass in both clients. Published root prerequisite `07aacebe97a2`
and Anthropic/client prerequisite `e9128cc3b35a` are available; Gateway consumes
the immutable root and Anthropic pins, and both Apache provider modules consume
the immutable root pin. Native request tests prove selected empty schema `{}` and
selected empty tool-result content `[]` remain present while absent values keep
their established behavior. Hosted module-resolution validation passed at
`4df4ed4`.

Acceptance reconciliation, 2026-09-17:

- Exact WP7 `e0e6c01` and WP8 `a2177d9` integration heads are ancestors of the
  reviewed WP11 head. This verifies the implementation baseline, not an external
  approval or merged status. Task 1.1 now states that distinction explicitly.
  Restacking onto the accepted completed WP9 stack remains a landing dependency.
- Task 1.2's historical red-run wording was reconciled to auditable evidence:
  base `d588ab9` rejects nonempty tools/choice in `providerwire/v4/request.go` and
  rejects non-text output in `response.go`. The current real-handler two-client
  and authenticated native continuation tests pass; no earlier red execution is
  retrospectively claimed.
- `gateway-command.test.ts` exercises unary definitions, all four choices,
  call-only history and full continuation on the real authenticated fallback
  route, using both clients plus exact raw error checks. Neither physical
  candidate is invoked. Direct-route native loops export four distinct canonical
  generations with nonzero usage and normalized finishes; an additional tool
  request failure has a safe error generation. Exported payloads, logs and metrics
  omit tool content and private identities. Existing reusable observation mapping
  suffices; no extra Apache observation API was needed.
- Standalone root provider, Anthropic, Grafana and reusable observation race
  tests, isolated Gateway/service race tests with immutable pins, the full
  command suite and `mise run parity-check` pass. Parity uses an exact detached
  checkout of registered commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` via
  `AI_SDK_UPSTREAM_ROOT`; no provider fixture input was created or modified.

WP11's local capability acceptance is complete. Its archive does not assert
that WP9 has landed, that the resulting restacked head has passed hosted checks,
or that production activation is authorized. Those handoff conditions remain
separate; production activation is deferred to WP10.
