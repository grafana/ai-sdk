## 1. Baseline and failing contracts

- [ ] 1.1 Verify accepted WP7/WP8 heads and exact registered upstream; classify provider-contract, provider-implementation and Gateway-client coverage in PARITY.
- [ ] 1.2 Add exact-client unary tool request/response cases and a two-request continuation through the real handler; prove current tools-unsupported failure.
- [x] 1.3 Add negative mode, role, inactive-arm, required-empty, JSON-null, Strict-presence and reserved function-option tests.

## 2. Apache prerequisites

- [x] 2.1 Audit existing provider.Tool, ToolResultOutput and Go client projectTool/projectOutput; fix only demonstrated domain/validation gaps while preserving Strict *bool.
- [x] 2.2 Add deterministic Anthropic native-request cases for schema/examples/strict false/choice/result continuation; update affected converters only, and inspect Bedrock/OpenAI regressions if shared domain changes.
- [x] 2.3 Extend existing Go unary response and request mapping where differential cases fail; retain bounded reads and Vercel-owned warnings/request/response normalization.
- [ ] 2.4 Extend reusable normalized tool observation only where required and verify standalone Apache tests.
- [ ] 2.5 Commit Apache prerequisites separately and obtain an approved immutable proxy-resolvable pin before dependent Gateway release validation; use no committed replacement.

## 3. Gateway unary support

- [x] 3.1 Add typed unary function DTO mapping after complete schema validation; gate streaming until WP12 and preserve fixed unsupported-family errors.
- [x] 3.2 Map text/json/error-text/error-json/text-only-content results and assistant calls with exact selected-arm semantics; reject deferred result options and approvals/media.
- [x] 3.3 Extend private unary DTOs and all preflight/final bounds for tool calls; reject providerExecuted true and dynamic true before HTTP 200, retain the supported-union guard for provider results/preliminary behavior, and verify both clients receive the fixed error with zero local execution. Test false/absent markers separately; update local output schemas and raw privacy tests.
- [x] 3.4 Preserve one logical middleware chain and metadata-only filtering; add actual exported-payload/log/metric assertions for two independent generations.
- [ ] 3.5 Integrate WP9 fallback-route rejection when present, including tool history without new definitions and zero physical invocation.

## 4. Acceptance and handoff

- [x] 4.1 Pass authenticated exact Vercel and Go unary round trips with captured native requests, local deterministic execution, result continuation and final text.
- [ ] 4.2 Run affected module tests, Gateway tests with GOWORK=off committed pins, ProviderWire contract checks and mise run parity-check; preserve fixture provenance.
- [x] 4.3 Update narrative Gateway scope/docs and PARITY with supported unary arms, metadata-only tool observation and deferred WP12/13/14/15/18/21 boundaries.
- [x] 4.4 Extend existing internal rollout smoke only if this capability is being enabled after WP10; otherwise record the activation handoff.
- [x] 4.5 Validate this OpenSpec change strictly, run git diff --check and report evidence without marking WP12 complete.

Apply handoff: direct-route loops and full command/safe-JWKS deterministic native
unary continuation now pass in both clients. Immutable publication pins and WP9
effect guard remain incomplete; production activation is deferred. See the dated apply report; unchecked
compound tasks may have partial evidence and are not whole-package completion.
