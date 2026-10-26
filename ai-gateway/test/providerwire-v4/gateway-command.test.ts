import assert from "node:assert/strict";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, request as httpRequest, type IncomingMessage, type ServerResponse } from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import nodeProcess from "node:process";
import { after, before, describe, it } from "node:test";
import { createGateway, GatewayInvalidRequestError } from "@ai-sdk/gateway";
import type { JSONValue, LanguageModelV4, LanguageModelV4CallOptions, LanguageModelV4StreamPart } from "@ai-sdk/provider";
import { generateText, isStepCount, jsonSchema, Output, streamText, tool } from "ai";
import { buildGoClientCapture, buildGoStreamTextCapture, captureGoClient } from "./go-client-capture";
import packageManifest from "./package.json" with { type: "json" };
import { fetchConfiguredModels } from "../../examples/configured-discovery";
import { nativeApprovalReply } from "./native-approvals";
import { nativeMetadataReply } from "./native-metadata";
import { nativeProxyEnvironment, startNativeProviderProxy } from "./native-provider-proxy";

const BYOK_MODEL = "anthropic/request-only-native";
const BYOK_OPTIONS = { gateway: { byok: { anthropic: [{ apiKey: "request-only-key" }] } } };

const AI_GATEWAY_ROOT = resolve(import.meta.dirname, "../..");
const COMMAND_DIR = resolve(AI_GATEWAY_ROOT, "cmd/grafana-ai-gateway");
const HOST_DIR = resolve(AI_GATEWAY_ROOT, "test/platformhost/cmd/plugin-host");
const READY_TIMEOUT_MS = 30_000;
const TEST_TOKEN = "glsa_integration_service_account_token";
const TEST_NAMESPACE = "stacks-27038";
const INTERNAL_SERVICE = "integration-internal-service";

let buildDirectory: string;
let binaryPath: string;
let hostBinaryPath: string;
let nativeProxyBinaryPath: string;
let goClientBinaryPath: string;
let goStreamTextBinaryPath: string;

before(() => {
  buildDirectory = mkdtempSync(join(tmpdir(), "grafana-ai-gateway-build-"));
  binaryPath = join(buildDirectory, "grafana-ai-gateway");
  nativeProxyBinaryPath = join(buildDirectory, "grafana-ai-gateway-native-proxy");
  hostBinaryPath = join(buildDirectory, "plugin-host");
  for (const [binary, tags, directory] of [[binaryPath, [], COMMAND_DIR], [nativeProxyBinaryPath, ["-tags=gatewaytest"], COMMAND_DIR], [hostBinaryPath, [], HOST_DIR]] as const) {
    execFileSync("go", ["build", ...tags, "-o", binary, "."], {
      cwd: directory,
      stdio: "pipe",
      env: {
        ...nodeProcess.env,
        GOWORK: nodeProcess.env.GATEWAY_TEST_GOWORK ?? "off",
        GOFLAGS: `${nodeProcess.env.GOFLAGS ? `${nodeProcess.env.GOFLAGS} ` : ""}-mod=readonly`,
      },
    });
  }
  goClientBinaryPath = buildGoClientCapture(buildDirectory);
  goStreamTextBinaryPath = buildGoStreamTextCapture(buildDirectory);
});

after(() => {
  rmSync(buildDirectory, { recursive: true, force: true });
});

describe("structured output candidate attribution", () => {
  it("preserves actual failures and selection through both clients on configured heterogeneous routes", async () => {
    const primary = await FakeAnthropic.start();
    const secondary = await FakeOpenAI.start();
    primary.failureCode = "schema_rejected";
    secondary.failureCode = 7123;
    secondary.structuredOutput = true;
    const config = `providers:
  structured-primary:
    type: anthropic
    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY
    baseURL: ${primary.url}
  structured-secondary:
    type: openai
    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY
    baseURL: ${secondary.url}/v1
models:
  structured-route:
    name: Structured output
    aliases: [structured-alias]
    primary:
      provider: structured-primary
      model: backend-private
    fallback:
      - provider: structured-secondary
        model: backend-private
`;
    let gateway: GatewayProcess | undefined;
    try {
      gateway = await GatewayProcess.start(binaryPath, "", [], {}, "access-token", config);
      const options: LanguageModelV4CallOptions = {
        prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }],
        maxOutputTokens: 64,
        responseFormat: { type: "json", name: "", schema: { type: "object", properties: { value: { type: "string" } } } },
      };
      for (const scenario of [
        { name: "noneligible", primary: 400, secondaryFails: false, secondaryCalls: 0, status: 424 },
        { name: "exhausted", primary: 503, secondaryFails: true, secondaryCalls: 1, status: 502 },
        { name: "secondary-selected", primary: 503, secondaryFails: false, secondaryCalls: 1, status: undefined },
      ]) {
        primary.failureStatus = scenario.primary;
        secondary.failWithSecret = scenario.secondaryFails;
        for (const mode of ["generate", "stream"] as const) {
          for (const language of ["go", "typescript"] as const) {
            const counts = [primary.requests.length, secondary.requests.length];
            let metadata: any;
            if (language === "go") {
              const result = await captureGoClient(goClientBinaryPath, {
                baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN,
                modelID: "structured-alias", mode, options,
              });
              assert.equal(result.error?.statusCode, scenario.status);
              metadata = scenario.status === undefined
                ? mode === "generate" ? result.result.providerMetadata : result.partMetadata.at(-1)
                : result.error.apiError.data.providerMetadata;
            } else {
              const model = gateway.client()("structured-alias");
              if (scenario.status !== undefined) {
                await assert.rejects(async () => mode === "generate" ? await model.doGenerate(options) : await model.doStream(options), (error: any) => {
                  assert.equal(error.statusCode, scenario.status);
                  metadata = error.cause.data.providerMetadata;
                  return true;
                });
              } else if (mode === "generate") {
                metadata = (await model.doGenerate(options)).providerMetadata;
              } else {
                const parts = await collectGatewayStream((await model.doStream(options)).stream);
                metadata = (parts.at(-1) as any).providerMetadata;
              }
            }
            if (scenario.status !== undefined) {
              if (language === "go") {
                const high = await captureGoClient(goClientBinaryPath, {
                  baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN,
                  modelID: "structured-alias", mode: `structured-${mode}`, options,
                });
                assert.equal(high.error?.statusCode, scenario.status);
                assert.deepEqual(high.error.apiError.data.providerMetadata, metadata);
              } else {
                const highOptions: { model: LanguageModelV4; prompt: string; maxOutputTokens: number; maxRetries: number; output: ReturnType<typeof Output.json> } = { model: gateway.client()("structured-alias"), prompt: "normal-stream", maxOutputTokens: 64, maxRetries: 0, output: Output.json() };
                if (mode === "generate") {
                  await assert.rejects(() => generateText(highOptions), (error: any) => {
                    assert.deepEqual(error.cause.data.providerMetadata, metadata);
                    return true;
                  });
                } else {
                  const failures: any[] = [];
                  await streamText({ ...highOptions, onError: ({ error }) => { failures.push(error); } }).consumeStream();
                  assert.equal(failures.length, 1);
                  assert.deepEqual(failures[0].cause.data.providerMetadata, metadata);
                }
              }
            }
            if (scenario.status === undefined) {
              if (language === "go") {
                const high = await captureGoClient(goClientBinaryPath, {
                  baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN,
                  modelID: "structured-alias", mode: `structured-${mode}`, options,
                });
                assert.equal(high.error, undefined);
                assert.equal(high.text, '{"value":"ok"}');
                assert.deepEqual(high.providerMetadata, metadata);
              } else {
                const highOptions: { model: LanguageModelV4; prompt: string; maxOutputTokens: number; maxRetries: number; output: ReturnType<typeof Output.json> } = { model: gateway.client()("structured-alias"), prompt: "normal-stream", maxOutputTokens: 64, maxRetries: 0, output: Output.json() };
                if (mode === "generate") {
                  const high = await generateText(highOptions);
                  assert.deepEqual(high.output, { value: "ok" });
                  assert.deepEqual(high.providerMetadata, metadata);
                } else {
                  const high = streamText(highOptions);
                  await high.consumeStream();
                  assert.deepEqual(await high.output, { value: "ok" });
                  assert.deepEqual(await high.providerMetadata, metadata);
                }
              }
            }
            const execution = metadata?.gateway?.execution;
            assert.ok(execution, `${scenario.name} ${language} ${mode}: execution evidence missing`);
            assert.equal(execution.requestedModelId, "structured-alias");
            assert.equal(execution.canonicalModelId, "structured-route");
            assert.equal(execution.attempts.length, 1 + scenario.secondaryCalls);
            assert.equal(execution.attempts[0].provider, "anthropic");
            assert.equal(execution.attempts[0].modelId, "backend-private");
            assert.equal(execution.attempts[0].outcome, "failed");
            assert.equal(execution.attempts[0].providerInstance, "structured-primary");
            assert.equal(execution.attempts[0].error.statusCode, scenario.primary);
            assert.equal(execution.attempts[0].error.type, "api_error");
            assert.equal(execution.attempts[0].error.code, "schema_rejected");
            assert.equal(execution.attempts[0].error.message, "provider-secret-response [REDACTED] backend-private");
            if (scenario.secondaryCalls) {
              assert.equal(execution.attempts[1].provider, "openai");
              assert.equal(execution.attempts[1].modelId, "backend-private");
              assert.equal(execution.attempts[1].outcome, scenario.secondaryFails ? "failed" : "selected");
              assert.equal(execution.attempts[1].providerInstance, "structured-secondary");
              if (scenario.secondaryFails) {
                assert.equal(execution.attempts[1].error.statusCode, 502);
                assert.equal(execution.attempts[1].error.type, "server_error");
                assert.equal(execution.attempts[1].error.code, 7123);
              }
              else assert.equal(execution.attempts[1].error, undefined);
            }
            const calls = 2;
            assert.deepEqual([primary.requests.length - counts[0], secondary.requests.length - counts[1]], [calls, scenario.secondaryCalls * calls]);
            const evidence = JSON.stringify(execution);
            for (const excluded of [primary.url, secondary.url, "x-private-provider", "backend-private-request", "responseHeaders", "responseBody", "isRetryable", "willFallback"]) {
              assert.ok(!evidence.includes(excluded), `unapproved detail ${excluded}`);
            }
          }
        }
      }
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => primary.stop(), () => secondary.stop());
    }
  });
});

describe("output-derived metadata continuation", () => {
  for (const family of ["anthropic", "openai", "compatible"] as const) {
    for (const fallback of [false, true]) {
      for (const store of family === "openai" ? [false, true] : [false]) {
        it(`${family} ${fallback ? "fallback" : "direct"} store=${store} preserves actual output in separate unary and streaming client paths`, async () => {
          const anthropic = await FakeAnthropic.start();
          const openai = await FakeOpenAI.start();
          const compatible = await FakeCompatible.start();
          const candidates = { anthropic, openai, compatible };
          candidates[family].metadata = true;
          const loser = family === "anthropic" ? "openai" : "anthropic";
          if (fallback) {
            if (loser === "anthropic") anthropic.failureStatus = 503;
            else openai.failWithSecret = true;
          }
          const config = `providers:
  anthropic:
    type: anthropic
    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY
    baseURL: ${anthropic.url}
  openai:
    type: openai
    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY
    baseURL: ${openai.url}/v1
  compatible:
    type: openai-compatible
    apiKeyEnv: GATEWAY_TEST_COMPATIBLE_KEY
    baseURL: ${compatible.url}/v1
    providerName: google
models:
  metadata:
    name: Metadata
    primary:
      provider: ${fallback ? loser : family}
      model: backend-private
${fallback ? `    fallback:\n      - provider: ${family}\n        model: backend-private\n` : ""}`;
          let gateway: GatewayProcess | undefined;
          try {
            gateway = await GatewayProcess.start(binaryPath, "", [], {}, "access-token", config);
            const model = gateway.client()("metadata");
            for (const path of ["ts-unary", "ts-stream", "go-unary", "go-stream"] as const) {
              const before = Object.fromEntries(Object.entries(candidates).map(([key, fake]) => [key, fake.requests.length]));
              let executions = 0;
              if (path.startsWith("ts")) {
                const options = { model, prompt: "metadata", maxOutputTokens: 64, providerOptions: { openai: { store } }, stopWhen: isStepCount(2), maxRetries: 0, tools: { weather: tool({ inputSchema: jsonSchema<{ city: string }>({ type: "object", properties: { city: { type: "string" } }, required: ["city"] }), execute: async ({ city }) => { assert.equal(city, "Rio"); executions++; return "sunny"; } }) } };
                if (path === "ts-unary") await generateText(options);
                else await streamText(options).consumeStream();
              } else {
                const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "metadata", mode: path === "go-unary" ? "metadata-unary-replay" : "metadata-stream-replay", options: { prompt: [{ role: "user", content: [{ type: "text", text: "metadata" }] }], maxOutputTokens: 64, providerOptions: { openai: { store } }, tools: [{ type: "function", name: "weather", inputSchema: { type: "object", properties: { city: { type: "string" } } } }] } });
                assert.equal(go.error, undefined);
                executions = go.executions;
              }
              assert.equal(executions, 1, path);
              for (const [key, fake] of Object.entries(candidates)) {
                const requests = fake.requests.slice(before[key]);
                assert.equal(requests.length, key === family || (fallback && key === loser) ? 2 : 0, `${path}: ${key}`);
                for (const request of requests) assert.equal(request.body.stream ?? false, path.endsWith("stream"), `${path}: native selector`);
              }
              const replay = candidates[family].requests.at(-1)!.body;
              if (family === "anthropic") {
                const blocks = (replay.messages as any[]).filter(message => message.role === "assistant").flatMap(message => message.content);
                assert.deepEqual(blocks.find(block => block.type === "tool_use").caller, { type: "direct" });
                assert.equal(blocks.find(block => block.type === "thinking").signature, "returned-signature");
                assert.equal(blocks.find(block => block.type === "redacted_thinking").data, "returned-redacted");
              } else if (family === "openai") {
                const input = replay.input as any[];
                if (store) {
                  assert.ok(input.some(item => item.type === "item_reference" && item.id === "msg-metadata"));
                  assert.ok(input.some(item => item.type === "item_reference" && item.id === "rs-metadata"));
                } else {
                  assert.equal(input.find(item => item.role === "assistant").phase, "commentary");
                  assert.equal(input.find(item => item.type === "reasoning").encrypted_content, "returned-encrypted");
                }
              } else {
                const call = (replay.messages as any[]).find(message => message.tool_calls)?.tool_calls[0];
                assert.equal(call?.extra_content?.google?.thought_signature, "returned-thought-signature");
              }
            }
            for (const fake of Object.values(candidates)) assert.deepEqual(fake.violations, []);
          } finally {
            await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => anthropic.stop(), () => openai.stop(), () => compatible.stop());
          }
        });
      }
    }
  }
});

describe("metadata observation policies", () => {
  it("keeps caller metadata and independent consumer observation unchanged with operator capture off or metadata-only", async () => {
    const observer = await FakeAgentObservability.start();
    const seen: unknown[] = [];
    try {
      for (const enabled of [false, true]) {
        const args = enabled ? ["--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`, "--no-agento11y.tls"] : [];
        const [fake, gateway] = await startGateway(args, { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" });
        fake.metadata = true;
        try {
          const options: LanguageModelV4CallOptions = { prompt: [{ role: "user", content: [{ type: "text", text: "metadata observation" }] }], maxOutputTokens: 64 };
          const unary = await gateway.client()("assistant").doGenerate(options);
          const stream = await collectGatewayStream((await gateway.client()("assistant").doStream(options)).stream);
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode: "metadata-observe", options });
          assert.equal(go.error, undefined);
          assert.deepEqual(go.observedUnary, [unary.providerMetadata ?? null, ...unary.content.map(part => part.providerMetadata ?? null)]);
          assert.deepEqual(go.observedStream, go.returnedStream);
          assert.deepEqual(go.returnedStream, stream.map(part => "providerMetadata" in part ? part.providerMetadata ?? null : null));
          seen.push({ content: unary.content, metadata: unary.providerMetadata, stream, observedUnary: go.observedUnary, observedStream: go.observedStream });
          const caller = JSON.stringify(seen.at(-1));
          assert.ok(caller.includes("returned-signature"));
          assert.ok(caller.includes("returned-redacted"));
          for (const credential of [TEST_TOKEN, "integration-anthropic-key", "integration-agento11y-key"]) assert.ok(!caller.includes(credential));
          assert.deepEqual(fake.violations, []);
        } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
      }
      assert.deepEqual(seen[0], seen[1]);
      await observer.waitForGenerations(4);
      const exported = JSON.stringify(observer.generations);
      for (const value of ["returned-signature", "returned-redacted", "metadata observation", "integration-anthropic-key"]) assert.ok(!exported.includes(value));
      assert.deepEqual(observer.violations, []);
    } finally { await observer.stop(); }
  });
});

describe("native option forwarding through the authenticated command", () => {
  for (const family of ["anthropic", "openai", "compatible"] as const) {
    it(`uses ${family} defaults when clients omit output-token limits`, async () => {
      const [fake, gateway] = family === "anthropic" ? await startGateway() : family === "openai" ? await startOpenAIGateway() : await startCompatibleGateway();
      const modelID = family === "anthropic" ? "assistant" : family;
      const options: LanguageModelV4CallOptions = { prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }] };
      try {
        const model = gateway.client()(modelID);
        for (const mode of ["generate", "stream"] as const) {
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID, mode, options });
          await assertNativeOptionClientResults(go, model, options, mode);
          assert.deepEqual(fake.requests.at(-2)!.body, fake.requests.at(-1)!.body);
        }
        const result = await generateText({ model, prompt: "normal-stream", maxRetries: 0, abortSignal: AbortSignal.timeout(10_000) });
        assert.equal(result.finishReason, "stop");
        assert.equal(result.text, family === "anthropic" ? "hello from fake Anthropic" : family === "openai" ? "hello from fake openai" : "hello from fake compatible");
        assert.equal(fake.requests.length, 5);
        assert.ok(singleHeader(fake.requests.at(-1)!.headers["user-agent"])!.split(/\s/).includes(`ai/${packageManifest.dependencies.ai}`));
        for (const request of fake.requests) {
          if (family === "anthropic") assert.equal(request.body.max_tokens, 4096);
          else assert.equal(Object.hasOwn(request.body, family === "openai" ? "max_output_tokens" : "max_tokens"), false);
        }
        assert.deepEqual(fake.violations, []);
      } finally {
        await settleCleanup(() => gateway.stop(), () => fake.stop());
      }
    });

    it(`preserves ${family} consumed scopes and ordinary values in both clients and modes`, async () => {
      const [fake, gateway] = family === "anthropic" ? await startGateway() : family === "openai" ? await startOpenAIGateway() : await startCompatibleGateway();
      const modelID = family === "anthropic" ? "assistant" : family;
      const nested = { null: null, false: false, zero: 0, empty: "", array: [], object: {} };
      const options: LanguageModelV4CallOptions = family === "anthropic" ? {
        maxOutputTokens: 64,
        tools: [{ type: "function", name: "lookup", inputSchema: {}, providerOptions: { anthropic: { allowedCallers: [], eagerInputStreaming: false } } }],
        providerOptions: { anthropic: { container: { id: "supplied-container" }, unknown: nested, model: "ignored" }, other: { role: "ordinary" } },
        prompt: [
          { role: "assistant", content: [{ type: "tool-call", toolCallId: "call", toolName: "lookup", input: {}, providerOptions: { anthropic: { caller: { type: "direct" } } } }] },
          { role: "tool", content: [{ type: "tool-result", toolCallId: "call", toolName: "lookup", output: { type: "text", value: "done normal-stream" }, providerOptions: { anthropic: { caller: { type: "direct" } } } }] },
        ],
      } : family === "openai" ? {
        maxOutputTokens: 64,
        providerOptions: { azure: { store: false, model: "ignored", reasoningEffort: "low" }, irrelevant: nested },
        prompt: [{ role: "assistant", content: [
          { type: "text", text: "answer", providerOptions: { azure: { phase: "final_answer", itemId: "msg_supplied" } } },
          { type: "reasoning", text: "thought", providerOptions: { azure: { itemId: "rs_supplied", reasoningEncryptedContent: "supplied-encrypted" } } },
        ] }],
      } : {
        maxOutputTokens: 64,
        providerOptions: { "compatible-backend": { user: "raw", extension: { ignored: true } }, openaiCompatible: { user: "fixed" }, compatibleBackend: { user: "camel", extension: nested, headers: { Authorization: "ordinary-body-value" }, messages: [], tools: [], stream: false }, anthropic: { model: "ignored" } },
        prompt: [{ role: "user", providerOptions: { openaiCompatible: { priority: "high" } }, content: [
          { type: "text", text: "normal-stream", providerOptions: { openaiCompatible: { sentiment: "positive", nested } } },
          { type: "text", text: "two" },
        ] }],
      };
      const original = JSON.stringify(options);
      try {
        for (const mode of ["generate", "stream"] as const) {
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID, mode, options });
          await assertNativeOptionClientResults(go, gateway.client()(modelID), options, mode);
          const goBody = fake.requests.at(-2)!.body;
          const tsBody = fake.requests.at(-1)!.body;
          assert.deepEqual(tsBody, goBody);
          assert.equal(tsBody.model, "backend-private");
          if (family === "anthropic") {
            assert.deepEqual(tsBody.container, { id: "supplied-container" });
            const definitions = tsBody.tools as Array<Record<string, unknown>>;
            assert.deepEqual(definitions[0]!.allowed_callers, []);
            assert.equal(definitions[0]!.eager_input_streaming, undefined);
            const messages = tsBody.messages as Array<{ content: Array<Record<string, unknown>> }>;
            assert.deepEqual(messages[0]!.content[0]!.caller, { type: "direct" });
            assert.equal(messages[1]!.content[0]!.caller, undefined);
          } else if (family === "openai") {
            const input = tsBody.input as Array<Record<string, unknown>>;
            assert.equal(input[0]!.phase, "final_answer");
            assert.equal(input[1]!.id, "rs_supplied");
            assert.equal(input[1]!.encrypted_content, "supplied-encrypted");
          } else {
            assert.equal(tsBody.user, "camel");
            assert.deepEqual(tsBody.extension, nested);
            const messages = tsBody.messages as Array<{ priority: string; content: Array<Record<string, unknown>> }>;
            assert.equal(messages[0]!.priority, "high");
            assert.equal(messages[0]!.content[0]!.sentiment, "positive");
            assert.deepEqual(messages[0]!.content[0]!.nested, nested);
          }
          assert.equal(JSON.stringify(options), original);
        }
        assert.equal(fake.requests.length, 4);
        assert.deepEqual(fake.violations, []);
      } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
    });
  }

  for (const family of ["anthropic", "openai"] as const) {
    for (const configuredFallback of [false, true]) {
      it(`forwards ${family} hosted MCP settings and history with fallback=${configuredFallback}`, async () => {
        const observer = await FakeAgentObservability.start();
        let primary: FakeAnthropic | FakeOpenAI | undefined;
        let secondary: FakeAnthropic | FakeOpenAI | undefined;
        let gateway: GatewayProcess | undefined;
        const serverURL = "https://mcp.example.test/tools";
        const serverName = "private-mcp-server";
        const remoteToken = "private-mcp-authorization";
        const remoteHeader = "private-mcp-header";
        try {
          primary = family === "anthropic" ? await FakeAnthropic.start() : await FakeOpenAI.start();
          if (configuredFallback) {
            secondary = family === "anthropic" ? await FakeAnthropic.start() : await FakeOpenAI.start();
            if (primary instanceof FakeAnthropic) primary.failureStatus = 503;
            else primary.failWithSecret = true;
          }
          const config = family === "anthropic"
            ? secondary ? anthropicFallbackConfig(primary.url, secondary.url) : anthropicConfig(primary.url)
            : openAIConfig(primary.url, secondary?.url);
          gateway = await GatewayProcess.start(binaryPath, "", [
            "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
            "--no-agento11y.tls",
          ], { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" }, "access-token", config);
          const prompt: LanguageModelV4CallOptions["prompt"] = [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }];
          const options: LanguageModelV4CallOptions = family === "anthropic"
            ? {
              prompt: [...prompt, { role: "assistant", content: [
                { type: "tool-call", toolCallId: "private-mcp-call", toolName: "lookup", input: {}, providerExecuted: true, providerOptions: { anthropic: { type: "mcp-tool-use", serverName } } },
                { type: "tool-result", toolCallId: "private-mcp-call", toolName: "lookup", output: { type: "json", value: "private-mcp-result" } },
              ] }],
              providerOptions: { anthropic: { mcpServers: [{ type: "url", name: serverName, url: serverURL, authorizationToken: remoteToken, toolConfiguration: { enabled: true, allowedTools: ["lookup"] } }] } },
            }
            : {
              prompt,
              tools: [{ type: "provider", id: "openai.mcp", name: "remote", args: { serverLabel: serverName, serverUrl: serverURL, authorization: remoteToken, headers: { "X-MCP-Key": remoteHeader }, allowedTools: ["lookup"] } }],
            };
          const original = JSON.stringify(options);
          const modelID = family === "anthropic" ? "assistant" : "openai";
          const candidates = secondary ? [primary, secondary] : [primary];
          for (const mode of ["generate", "stream"] as const) {
            const before = candidates.map(candidate => candidate.requests.length);
            const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID, mode, options });
            await assertNativeOptionClientResults(go, gateway.client()(modelID), options, mode);
            for (const [index, candidate] of candidates.entries()) {
              const requests = candidate.requests.slice(before[index]);
              assert.equal(requests.length, 2);
              assert.deepEqual(requests[0]!.body, requests[1]!.body);
              for (const { body, apiKey, headers } of requests) {
                assert.equal(body.model, "backend-private");
                assert.equal(body.stream ?? false, mode === "stream");
                assert.equal(apiKey, family === "anthropic" ? "integration-anthropic-key" : "Bearer integration-openai-key");
                assert.equal(headers["x-mcp-key"], undefined);
                if (family === "anthropic") {
                  assert.deepEqual(body.mcp_servers, [{ type: "url", name: serverName, url: serverURL, authorization_token: remoteToken, tool_configuration: { enabled: true, allowed_tools: ["lookup"] } }]);
                  const assistant = (body.messages as Array<{ role: string; content: unknown[] }>).find(message => message.role === "assistant");
                  assert.deepEqual(assistant?.content, [
                    { type: "mcp_tool_use", id: "private-mcp-call", name: "lookup", input: {}, server_name: serverName },
                    { type: "mcp_tool_result", tool_use_id: "private-mcp-call", content: "private-mcp-result", is_error: false },
                  ]);
                } else {
                  assert.deepEqual(body.tools, [{ type: "mcp", server_label: serverName, server_url: serverURL, authorization: remoteToken, headers: { "X-MCP-Key": remoteHeader }, allowed_tools: ["lookup"], require_approval: "never" }]);
                }
              }
              assert.deepEqual(candidate.violations, []);
            }
            assert.equal(JSON.stringify(options), original);
          }
          await observer.waitForGenerations(4);
          const metrics = await gateway.metrics();
          await gateway.stop();
          const captured = JSON.stringify({ metrics, logs: gateway.stderr, generations: observer.generations });
          for (const value of [serverURL, serverName, remoteToken, remoteHeader, "private-mcp-call", "private-mcp-result"]) assert.ok(!captured.includes(value));
          assert.deepEqual(observer.violations, []);
        } finally {
          await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), ...(secondary ? [() => secondary!.stop()] : []), ...(primary ? [() => primary!.stop()] : []), () => observer.stop());
        }
      });
    }
  }

  it("preserves OpenAI-first and Azure-fallback item-reference semantics", async () => {
    const [fake, gateway] = await startOpenAIGateway();
    try {
      for (const namespace of ["openai", "azure"] as const) {
        const options: LanguageModelV4CallOptions = {
          providerOptions: namespace === "openai" ? { openai: { store: true }, azure: { store: false } } : { azure: { store: true } },
          prompt: [{ role: "assistant", content: [{ type: "text", text: "history", providerOptions: { openai: { itemId: "msg_openai" }, azure: { itemId: "msg_azure" } } }] }],
        };
        for (const mode of ["generate", "stream"] as const) {
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "openai", mode, options });
          await assertNativeOptionClientResults(go, gateway.client()("openai"), options, mode);
          assert.deepEqual(fake.requests.at(-1)!.body, fake.requests.at(-2)!.body);
          assert.deepEqual(fake.requests.at(-1)!.body.input, [{ type: "item_reference", id: `msg_${namespace}` }]);
          assert.equal(fake.requests.at(-1)!.body.store, true);
        }
      }
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("rejects concrete bypasses but not ignored scope values in both clients and modes", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const attempts: LanguageModelV4CallOptions[] = [
        { prompt: [], providerOptions: { compatibleBackend: { model: "other" } } },
        { prompt: [], providerOptions: { compatibleBackend: { response_format: { type: "json_object" } } } },
        { prompt: [{ role: "system", content: "system", providerOptions: { openaiCompatible: { role: "tool" } } }] },
        { prompt: [{ role: "assistant", content: [{ type: "tool-call", toolCallId: "call", toolName: "lookup", input: {}, providerOptions: { openaiCompatible: { function: { name: "other", arguments: "{}" } } } }] }] },
      ];
      for (const mode of ["generate", "stream"] as const) {
        for (const options of attempts) {
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "compatible", mode, options });
          assert.equal(go.error?.statusCode, 400);
          const model = gateway.client()("compatible");
          await assert.rejects(async () => mode === "generate" ? await model.doGenerate(options) : await model.doStream(options), (error: unknown) => GatewayInvalidRequestError.isInstance(error));
        }
      }
      assert.equal(fake.requests.length, 0);
      const options: LanguageModelV4CallOptions = { prompt: [{ role: "assistant", content: [{ type: "text", text: "normal-stream", providerOptions: { openaiCompatible: { role: "tool", type: "ignored" } } }] }], providerOptions: { CompatibleBackend: { model: "ignored" } } };
      for (const mode of ["generate", "stream"] as const) {
        const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "compatible", mode, options });
        await assertNativeOptionClientResults(go, gateway.client()("compatible"), options, mode);
      }
      assert.equal(fake.requests.length, 4);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("protects Anthropic execution controls without rejecting safe contextual history", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const attempts: LanguageModelV4CallOptions[] = [
        { prompt: [], providerOptions: { anthropic: { container: { skills: [{ type: "anthropic", skillId: "skill" }] } } } },
        { prompt: [], providerOptions: { anthropic: { fallbacks: "default" } } },
      ];
      for (const mode of ["generate", "stream"] as const) {
        for (const options of attempts) {
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options });
          assert.equal(go.error?.statusCode, 400);
          const model = gateway.client()("assistant");
          await assert.rejects(async () => mode === "generate" ? await model.doGenerate(options) : await model.doStream(options), (error: unknown) => GatewayInvalidRequestError.isInstance(error));
        }
      }
      assert.equal(fake.requests.length, 0);
      const options: LanguageModelV4CallOptions = { prompt: [{ role: "assistant", content: [{ type: "text", text: "normal-stream", providerOptions: { anthropic: { type: "compaction" } } }] }], providerOptions: { anthropic: { mcpServers: [], container: null, fallbacks: null }, irrelevant: { type: "mcp-tool-use" } } };
      for (const mode of ["generate", "stream"] as const) {
        const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options });
        await assertNativeOptionClientResults(go, gateway.client()("assistant"), options, mode);
      }
      assert.equal(fake.requests.length, 4);
      for (const { body } of fake.requests) {
        const messages = body.messages as Array<{ content: Array<Record<string, unknown>> }>;
        assert.equal(messages[0]!.content[0]!.type, "compaction");
      }
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("keeps concurrent scoped extensions isolated and caller options immutable", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const inputs: LanguageModelV4CallOptions[] = Array.from({ length: 12 }, (_, index) => ({
        prompt: [{ role: "user", content: [{ type: "text", text: `request-${index} normal-stream`, providerOptions: { openaiCompatible: { scoped_marker: index } } }] }],
        providerOptions: { compatibleBackend: { call_marker: index } },
      }));
      const original = JSON.stringify(inputs);
      await Promise.all(inputs.map(async (options, index) => {
        if (index % 2 === 0) {
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "compatible", mode: index % 4 === 0 ? "generate" : "stream", options });
          assert.equal(go.error, undefined);
          if (index % 4 !== 0) {
            assert.equal(go.canceled, false);
            assertNativeOptionStream(go.parts);
          }
        } else {
          const model = gateway.client()("compatible");
          if (index % 4 === 1) await model.doGenerate(options);
          else assertNativeOptionStream(await collectGatewayStream((await model.doStream(options)).stream));
        }
      }));
      assert.equal(JSON.stringify(inputs), original);
      assert.equal(fake.requests.length, inputs.length);
      for (const { body } of fake.requests) {
        const message = (body.messages as Array<Record<string, unknown>>)[0]!;
        assert.equal(message.content, `request-${body.call_marker} normal-stream`);
        assert.equal(message.scoped_marker, body.call_marker);
      }
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });
});

async function assertNativeOptionClientResults(go: Awaited<ReturnType<typeof captureGoClient>>, model: LanguageModelV4, options: LanguageModelV4CallOptions, mode: "generate" | "stream") {
  assert.equal(go.error, undefined);
  if (mode === "generate") {
    const result = await model.doGenerate(options);
    assert.equal(result.finishReason.unified, "stop");
    assert.deepEqual(go.result.content, result.content);
    assert.deepEqual(go.result.finishReason, result.finishReason);
    assert.deepEqual(go.result.usage, result.usage);
    assert.deepEqual(go.result.providerMetadata, result.providerMetadata);
    return result.providerMetadata;
  } else {
    assert.equal(go.canceled, false);
    const parts = await collectGatewayStream((await model.doStream(options)).stream);
    assert.deepEqual(assertNativeOptionStream(go.parts), assertNativeOptionStream(parts));
    const finish = parts.at(-1) as Extract<LanguageModelV4StreamPart, { type: "finish" }>;
    assert.deepEqual(go.parts.at(-1).providerMetadata, finish.providerMetadata);
    return finish.providerMetadata;
  }
}

function assertNativeOptionStream(parts: Array<{ type: string; delta?: string }>) {
  assert.ok(parts.every(part => part.type !== "error"));
  const finishes = parts.filter(part => part.type === "finish");
  assert.equal(finishes.length, 1);
  assert.equal(parts.at(-1)?.type, "finish");
  const finish = finishes[0] as Extract<LanguageModelV4StreamPart, { type: "finish" }>;
  assert.equal(finish.finishReason.unified, "stop");
  return {
    text: parts.filter(part => part.type === "text-delta").map(part => part.delta).join(""),
    finishReason: finish.finishReason,
    usage: finish.usage,
  };
}

describe("mapped configured fallback", () => {
  it("consumes native namespaces in a heterogeneous configured order in both clients and modes", async () => {
    const anthropic = await FakeAnthropic.start();
    const openai = await FakeOpenAI.start();
    const compatible = await FakeCompatible.start();
    let gateway: GatewayProcess | undefined;
    const config = `providers:
  anthropic:
    type: anthropic
    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY
    baseURL: ${anthropic.url}
  openai:
    type: openai
    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY
    baseURL: ${openai.url}/v1
  compatible:
    type: openai-compatible
    apiKeyEnv: GATEWAY_TEST_COMPATIBLE_KEY
    baseURL: ${compatible.url}/v1
    providerName: compatible-backend
models:
  mapped:
    name: Mapped
    primary:
      provider: anthropic
      model: backend-private
    fallback:
      - provider: openai
        model: backend-private
      - provider: compatible
        model: backend-private
`;
    const nested = { null: null, false: false, zero: 0, empty: "", array: [], object: {} };
    const options: LanguageModelV4CallOptions = {
      prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream", providerOptions: { openaiCompatible: { sentiment: "positive" }, anthropic: { cacheControl: { type: "ephemeral" } } } }] }],
      maxOutputTokens: 64,
      headers: { "x-ordinary": "native-header" },
      tools: [{ type: "function", name: "lookup", inputSchema: {}, providerOptions: { anthropic: { allowedCallers: [] } } }],
      providerOptions: { anthropic: { container: { id: "native-container" } }, openai: { store: false }, azure: { store: true }, compatibleBackend: { user: "camel", extension: nested }, "compatible-backend": { user: "raw" }, irrelevant: nested },
    };
    const original = JSON.stringify(options);
    try {
      gateway = await GatewayProcess.start(binaryPath, "", [], {}, "access-token", config);
      for (const winner of [0, 1, 2, 3]) {
        anthropic.failureStatus = winner > 0 ? 503 : undefined;
        openai.failWithSecret = winner > 1;
        compatible.failWithSecret = winner > 2;
        for (const mode of ["generate", "stream"] as const) {
          const counts = [anthropic.requests.length, openai.requests.length, compatible.requests.length];
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "mapped", mode, options });
          let metadata: any;
          if (winner < 3) metadata = await assertNativeOptionClientResults(go, gateway.client()("mapped"), options, mode);
          else {
            let failure: any;
            try {
              const model = gateway.client()("mapped");
              if (mode === "generate") await model.doGenerate(options);
              else await collectGatewayStream((await model.doStream(options)).stream);
            } catch (error) { failure = error; }
            assert.equal(failure?.statusCode, 502);
            assert.equal(go.error?.statusCode, 502);
            assert.equal(go.error.isRetryable, failure.isRetryable);
            metadata = failure.cause.data.providerMetadata;
            assert.deepEqual(go.error.apiError.data.providerMetadata, metadata);
          }
          const execution = metadata.gateway.execution;
          assert.deepEqual([execution.requestedModelId, execution.canonicalModelId], ["mapped", "mapped"]);
          assert.deepEqual(execution.attempts.map((attempt: any) => [attempt.providerInstance, attempt.provider, attempt.modelId]), [
            ["anthropic", "anthropic", "backend-private"],
            ["openai", "openai", "backend-private"],
            ["compatible", "compatible-backend", "backend-private"],
          ].slice(0, Math.min(winner + 1, 3)));
          assert.deepEqual(execution.attempts.map((attempt: any) => attempt.outcome), winner < 3 ? [...Array(winner).fill("failed"), "selected"] : Array(3).fill("failed"));
          for (const [index, attempt] of execution.attempts.entries()) {
            if (index === winner) assert.equal(attempt.error, undefined);
            else {
              assert.equal(attempt.error.statusCode, index === 0 ? 503 : 502);
              assert.equal(attempt.error.type, index === 0 ? "api_error" : "server_error");
              assert.equal(attempt.error.message, index === 0 ? "provider-secret-response [REDACTED] backend-private" : "provider-secret-response");
            }
          }
          if (winner === 0) assertConsumerSecretsAbsent([go, metadata]);
          const candidates = [anthropic, openai, compatible];
          assert.deepEqual(candidates.map((candidate, index) => candidate.requests.length - counts[index]!), candidates.map((_, index) => index <= winner ? 2 : 0));
          for (const candidate of candidates.slice(0, winner + 1)) {
            assert.deepEqual(candidate.requests.at(-1)!.body, candidate.requests.at(-2)!.body);
            assert.equal(candidate.requests.at(-1)!.headers["x-ordinary"], "native-header");
            assert.equal(candidate.requests.at(-1)!.body.model, "backend-private");
            assert.equal(candidate.requests.at(-1)!.body.irrelevant, undefined);
          }
          const anthropicBody = anthropic.requests.at(-1)!.body;
          assert.deepEqual(anthropicBody.container, { id: "native-container" });
          assert.equal(anthropicBody.extension, undefined);
          assert.deepEqual((anthropicBody.tools as any[])[0].allowed_callers, []);
          assert.deepEqual((anthropicBody.messages as any[])[0].content[0].cache_control, { type: "ephemeral" });
          if (winner >= 1) {
            const body = openai.requests.at(-1)!.body;
            assert.equal(body.store, false);
            assert.equal(body.container, undefined);
            assert.equal(body.extension, undefined);
          }
          if (winner >= 2) {
            const body = compatible.requests.at(-1)!.body;
            assert.equal(body.user, "camel");
            assert.deepEqual(body.extension, nested);
            assert.equal((body.messages as any[])[0].sentiment, "positive");
            assert.equal(body.container, undefined);
            assert.equal(body.store, undefined);
          }
          assert.equal(JSON.stringify(options), original);
        }
      }
      for (const candidate of [anthropic, openai, compatible]) assert.deepEqual(candidate.violations, []);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => anthropic.stop(), () => openai.stop(), () => compatible.stop());
    }
  });
});

describe("authenticated Anthropic Gateway command", () => {
  for (const mode of ["access-token", "cloud-gateway"] as const) {
    it(`enforces ${mode} discovery policy without inference`, async () => {
      const fake = await FakeAnthropic.start();
      let gateway: GatewayProcess | undefined;
      try {
        const config = `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${fake.url}\n  openai-backup:\n    type: openai\n    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY\n    baseURL: ${fake.url}\nmodels:\n  grafana/assistant:\n    name: sk-ordinary-display\n    primary:\n      provider: anthropic-primary\n      model: configured-primary\n    fallback:\n      - provider: openai-backup\n        model: configured-backup\n    aliases:\n      - assistant\n`;
        gateway = await GatewayProcess.start(binaryPath, fake.url, [], {}, mode, config);
        const baseURL = gateway.baseURL;
        const headers: Record<string, string> = { Authorization: `Bearer ${TEST_TOKEN}` };
        const raw = await fetch(`${baseURL}/config`, { headers });
        if (mode === "cloud-gateway") {
          assert.equal(raw.status, 400);
          assert.equal((await raw.json()).error.message, "catalog discovery is unsupported for BYOK");
          await assert.rejects(gateway.client().getAvailableModels(), GatewayInvalidRequestError.isInstance);
          const go = await captureGoClient(goClientBinaryPath, { baseURL, serviceAccountToken: TEST_TOKEN, mode: "discovery" });
          assert.equal(go.error?.statusCode, 400);
          assert.equal(fake.requests.length, 0);
          return;
        }
        assert.equal(raw.status, 200);
        const document = await raw.json();
        assert.deepEqual(document.models.map((row: { id: string }) => row.id), ["grafana/assistant"]);
        const configured = { aliases: ["assistant"], primary: { providerInstance: "anthropic-primary", provider: "anthropic", providerModelId: "configured-primary" }, fallbacks: [{ providerInstance: "openai-backup", provider: "openai", providerModelId: "configured-backup" }] };
        assert.deepEqual(document.models.map((row: { gateway: unknown }) => row.gateway), [configured]);
        const stock = await createGateway({ baseURL, apiKey: TEST_TOKEN }).getAvailableModels();
        assert.deepEqual(stock.models.map(row => row.id), ["grafana/assistant"]);
        assert.equal("gateway" in stock.models[0], false);
        const companion = await fetchConfiguredModels({ baseURL, headers });
        assert.deepEqual(companion, document);
        const go = await captureGoClient(goClientBinaryPath, { baseURL, serviceAccountToken: TEST_TOKEN, mode: "discovery" });
        assert.equal(go.error, undefined);
        assert.deepEqual(go.models, document.models);
        assertDiscoverySecretsAbsent([document, companion, go], fake, ["integration-openai-key", "GATEWAY_TEST_OPENAI_KEY"]);
        assert.equal(fake.requests.length, 0);
        assert.deepEqual(fake.violations, []);
      } finally { await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => fake.stop()); }
    });
  }
  it("serves the complete configured catalog above the former response cap without inference", async () => {
    const fake = await FakeAnthropic.start();
    let gateway: GatewayProcess | undefined;
    try {
      const candidates = Array.from({ length: 16 }, (_, i) => ({ providerInstance: "anthropic-primary", provider: "anthropic", providerModelId: `${i}-` + '"'.repeat(512) }));
      const routes = Array.from({ length: 65 }, (_, i) => ({ id: `public-${String(i).padStart(3, "0")}`, aliases: [`alias-${i}`] }));
      const config = `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${fake.url}\nmodels:\n` + routes.map(route => `  ${route.id}:\n    name: Model\n    primary:\n      provider: anthropic-primary\n      model: '${candidates[0].providerModelId}'\n    fallback:\n${candidates.slice(1).map(candidate => `      - provider: anthropic-primary\n        model: '${candidate.providerModelId}'\n`).join("")}    aliases: [${route.aliases[0]}]\n`).join("");
      gateway = await GatewayProcess.start(binaryPath, fake.url, [], {}, "access-token", config);
      const baseURL = gateway.baseURL;
      const headers = { Authorization: `Bearer ${TEST_TOKEN}` };
      const response = await fetch(`${baseURL}/config`, { headers });
      assert.equal(response.status, 200);
      const body = await response.text();
      assert.ok(Buffer.byteLength(body) > 1_048_576);
      const document = JSON.parse(body);
      assert.equal(document.models.length, 65);
      assert.deepEqual(document.models.map((model: { id: string }) => model.id), routes.map(route => route.id));
      for (const [i, model] of document.models.entries()) assert.deepEqual(model.gateway, { aliases: routes[i].aliases, primary: candidates[0], fallbacks: candidates.slice(1) });
      assert.deepEqual(await fetchConfiguredModels({ baseURL, headers }), document);
      const go = await captureGoClient(goClientBinaryPath, { baseURL, serviceAccountToken: TEST_TOKEN, mode: "discovery" });
      assert.equal(go.error, undefined);
      assert.deepEqual(go.models, document.models);
      assert.equal(fake.requests.length, 0);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => fake.stop()); }
  });

  for (const family of ["anthropic", "openai"] as const) {
    it(`replays assembled ${family} reasoning through authenticated native requests in both clients`, async () => {
      const [fake,gateway] = family === "anthropic" ? await startGateway() : await startOpenAIGateway();
      const id=family === "anthropic" ? "assistant" : "openai";
      try {
        const first=streamText({model:gateway.client()(id),prompt:"wp17-reasoning",maxOutputTokens:64});
        const response=await first.response;
        assert.equal(await first.reasoningText,"private thought");
        const metadata=(await first.reasoning)[0].providerMetadata;
        assert.deepEqual(metadata,family === "anthropic" ? {anthropic:{signature:"end-signature"}} : {openai:{itemId:"rs_wp17",reasoningEncryptedContent:"final-encrypted"}});
        const second=streamText({model:gateway.client()(id),messages:[{role:"user",content:"wp17-reasoning"},...response.messages],maxOutputTokens:64});
        await second.consumeStream();
        assert.equal(fake.requests.length,2);
        const field=family === "anthropic" ? "messages" : "input";
        const native=fake.requests[1].body[field];
        assert.ok(JSON.stringify(native).includes(family === "anthropic" ? "end-signature" : "rs_wp17"));
        const go=await captureGoClient(goStreamTextBinaryPath,{baseURL:gateway.baseURL,serviceAccountToken:TEST_TOKEN,modelID:id,mode:"reasoning-replay",options:{prompt:[{role:"user",content:[{type:"text",text:"wp17-reasoning"}]}],maxOutputTokens:64}});
        assert.equal(go.error,undefined);
        assert.equal(fake.requests.length,4);
        assert.deepEqual(fake.requests[3].body[field],native);
        assert.deepEqual(fake.violations,[]);
      } finally {await gateway.stop();await fake.stop();}
    });
  }
  it("preserves authenticated Vercel and Go mapped behavior across ordered fallback", async () => {
    const token = TEST_TOKEN;
    const primary = await FakeAnthropic.start();
    const secondary = await FakeAnthropic.start();
    let gateway: GatewayProcess | undefined;
    try {
      const yaml = anthropicFallbackConfig(primary.url, secondary.url, "backend-private");
      gateway = await GatewayProcess.start(binaryPath, primary.url, [], {}, "access-token", yaml);
      const base = { baseURL: gateway.baseURL, serviceAccountToken: token, modelID: "assistant" };
      const discovery = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery" });
      assert.equal(discovery.error, undefined);
      const client = gateway.client(token);
      const models = await client.getAvailableModels();
      assert.deepEqual(models.models.map(model => model.id).sort(), discovery.models.map((model: { id: string }) => model.id).sort());
      const denied = await captureGoClient(goClientBinaryPath, { ...base, serviceAccountToken: "glsa_invalid", mode: "generate", options: { prompt: [] } });
      assert.equal(denied.error?.statusCode, 401);
      assert.equal(primary.requests.length, 0);
      assert.equal(secondary.requests.length, 0);
      const call = { role: "assistant" as const, content: [{ type: "tool-call" as const, toolCallId: "private-call", toolName: "private-tool", input: { secret: "private-input" } }] };
      const result = { role: "tool" as const, content: [{ type: "tool-result" as const, toolCallId: "private-call", toolName: "private-tool", output: { type: "text" as const, value: "private-result" } }] };
      const mappedRequests: LanguageModelV4CallOptions[] = [
        { prompt: [], tools: [{ type: "function", name: "private-tool", inputSchema: { type: "object" } }] },
        { prompt: [], tools: [{ type: "provider", id: "anthropic.code_execution_20260120", name: "private-tool", args: {} }] },
        { prompt: [], providerOptions: { anthropic: { mcpServers: [{ type: "url", name: "echo", url: "https://mcp.example.test", authorizationToken: "fallback-mcp-secret" }] } } },
        ...(["none", "required"] as const).map(type => ({ prompt: [], toolChoice: { type } })),
        { prompt: [], toolChoice: { type: "tool", toolName: "private-tool" } },
        { prompt: [call] },
        { prompt: [call, result] },
        { prompt: [{ role: "user", content: [{ type: "file", data: { type: "text", text: "private-file" }, mediaType: "text/plain" }] }] },
        { prompt: [{ role: "user", content: [{ type: "text", text: "hello" }], providerOptions: { anthropic: { cacheControl: { type: "ephemeral" } } } }] },
        { prompt: [{ role: "user", content: [{ type: "text", text: "hello" }], providerOptions: { vendor: { flag: false } } }] },
      ];
      primary.failureStatus = 503;
      mappedRequests.push(
        { prompt: [{ role: "assistant", content: [{ type: "reasoning", text: "thought", providerOptions: { anthropic: { signature: "supplied-signature" } } }] }], reasoning: "high" },
        { prompt: [{ role: "assistant", content: [{ type: "reasoning-file", data: { type: "data", data: "" }, mediaType: "image/png" }] }] },
        { prompt: [{ role: "user", content: [{ type: "file", data: { type: "data", data: "" }, mediaType: "image/png", filename: "" }] }] },
        { prompt: [], headers: { "x-ordinary": "mapped-header" }, providerOptions: { anthropic: { container: { id: "supplied-container" } }, irrelevant: { nested: { null: null, zero: 0, empty: "", array: [] } } } },
      );
      for (const mode of ["generate", "stream"] as const) {
        for (const input of mappedRequests) {
          const options: LanguageModelV4CallOptions = { ...input, maxOutputTokens: 64, prompt: [...input.prompt, { role: "user", content: [{ type: "text", text: "normal-stream" }] }] };
          const original = JSON.stringify(options);
          const counts: [number, number] = [primary.requests.length, secondary.requests.length];
          const go = await captureGoClient(goClientBinaryPath, { ...base, mode, options });
          await assertNativeOptionClientResults(go, client("assistant"), options, mode);
          assert.deepEqual([primary.requests.length - counts[0], secondary.requests.length - counts[1]], [2, 2]);
          assert.deepEqual(primary.requests.at(-1)!.body, secondary.requests.at(-1)!.body);
          assert.deepEqual(secondary.requests.at(-1)!.body, secondary.requests.at(-2)!.body);
          assert.equal(JSON.stringify(options), original);
          if (options.headers) assert.equal(secondary.requests.at(-1)!.headers["x-ordinary"], "mapped-header");
        }
      }
      secondary.failureStatus = undefined;
      secondary.functionTools = true;
      let executions = 0;
      const executeWeather = (input: unknown): string => {
        assert.deepEqual(input, { city: "Rio" });
        executions++;
        return "sunny";
      };
      for (const mode of ["generate", "stream"] as const) {
        for (const implementation of ["go", "vercel"] as const) {
          const counts: [number, number] = [primary.requests.length, secondary.requests.length];
          const invoke = async (options: LanguageModelV4CallOptions): Promise<any[]> => {
            if (implementation === "go") {
              const response = await captureGoClient(goClientBinaryPath, { ...base, mode, options });
              assert.equal(response.error, undefined);
              return mode === "generate" ? response.result.content : response.parts;
            }
            const model = client("assistant");
            return mode === "generate" ? (await model.doGenerate(options)).content : await collectGatewayStream((await model.doStream(options)).stream);
          };
          const options: LanguageModelV4CallOptions = { maxOutputTokens: 64, tools: [{ type: "function", name: "weather", inputSchema: { type: "object", properties: { city: { type: "string" } }, required: ["city"] } }], toolChoice: { type: "tool", toolName: "weather" }, prompt: [{ role: "user", content: [{ type: "text", text: "weather" }] }] };
          const first = await invoke(options);
          const calls = first.filter(part => part.type === "tool-call");
          assert.equal(calls.length, 1);
          const call = calls[0];
          assert.equal(call.toolName, "weather");
          const toolResult = executeWeather(JSON.parse(call.input));
          const final = await invoke({ ...options, toolChoice: { type: "auto" }, prompt: [...options.prompt,
            { role: "assistant", content: [{ type: "tool-call", toolCallId: call.toolCallId, toolName: call.toolName, input: JSON.parse(call.input) }] },
            { role: "tool", content: [{ type: "tool-result", toolCallId: call.toolCallId, toolName: call.toolName, output: { type: "text", value: toolResult } }] }],
          });
          assert.equal(final.filter(part => part.type === (mode === "generate" ? "text" : "text-delta")).map(part => part.text ?? part.delta).join(""), "It is sunny.");
          assert.deepEqual([primary.requests.length - counts[0], secondary.requests.length - counts[1]], [2, 2], "continuation must restart at primary without executing the local tool again");
          assert.deepEqual(primary.requests.at(-1)!.body, secondary.requests.at(-1)!.body);
          assert.deepEqual((secondary.requests.at(-1)!.body.messages as any[])[2].content, [{ type: "tool_result", tool_use_id: call.toolCallId, content: [{ type: "text", text: "sunny" }] }]);
        }
      }
      assert.equal(executions, 4);
      secondary.functionTools = false;
      const textRequests: LanguageModelV4CallOptions[] = [
        { prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }], providerOptions: { anthropic: {} } }] },
        { prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }], providerOptions: { vendor: {} } }] },
      ];
      for (const mode of ["generate", "stream"] as const) {
        for (const options of textRequests) {
          const counts: [number, number] = [primary.requests.length, secondary.requests.length];
          const go = await captureGoClient(goClientBinaryPath, { ...base, mode, options });
          assert.equal(go.error, undefined);
          const model = client("assistant");
          const result = mode === "generate" ? await model.doGenerate(options) : await collectGatewayStream((await model.doStream(options)).stream);
          assert.ok(JSON.stringify(result).includes("hello from fake Anthropic"));
          assert.deepEqual([primary.requests.length - counts[0], secondary.requests.length - counts[1]], [2, 2]);
          assert.deepEqual(primary.requests.at(-1)?.body, secondary.requests.at(-1)?.body);
        }
      }
      for (const row of [
        { primary: undefined, secondary: undefined, count: 0, status: undefined },
        { primary: 503, secondary: undefined, count: 1, status: undefined },
        { primary: 400, secondary: undefined, count: 0, status: 424 },
        { primary: 503, secondary: 503, count: 1, status: 503 },
      ]) {
        primary.failureStatus = row.primary;
        secondary.failureStatus = row.secondary;
        for (const [mode, toolChoice] of [
          ["generate", undefined], ["stream", undefined],
          ["generate", { type: "auto" }], ["stream", { type: "auto" }],
        ] as const) {
          const options: LanguageModelV4CallOptions = { prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }], maxOutputTokens: 32, toolChoice, tools: [{ type: "function", name: "lookup", inputSchema: {} }], reasoning: "high", providerOptions: { vendor: { flag: false } } };
          const requestCounts: [number, number] = [primary.requests.length, secondary.requests.length];
          const go = await captureGoClient(goClientBinaryPath, { ...base, mode, options });
          let result: unknown;
          let failure: any;
          try {
            const model = client("assistant");
            result = mode === "generate" ? await model.doGenerate(options) : await collectGatewayStream((await model.doStream(options)).stream);
          } catch (error) { failure = error; }
          assert.equal(go.error?.statusCode, row.status);
          assert.equal(failure?.statusCode, row.status);
          if (row.status === undefined) {
            assert.ok(JSON.stringify(result).includes("hello from fake Anthropic"));
            assert.ok(JSON.stringify(go).includes("hello from fake Anthropic"));
          } else { assert.equal(go.error.isRetryable, failure.isRetryable); }
          assert.equal(primary.requests.length - requestCounts[0], 2, "every client invocation restarts at primary");
          assert.equal(secondary.requests.length - requestCounts[1], row.count * 2);
          if (row.count) {
            assert.deepEqual(primary.requests.at(-1)?.body, secondary.requests.at(-1)?.body);
          }
          const tsMetadata = row.status === undefined ? (mode === "generate" ? (result as any).providerMetadata : (result as any[]).at(-1).providerMetadata) : failure.cause.data.providerMetadata;
          const goMetadata = row.status === undefined ? (mode === "generate" ? go.result.providerMetadata : go.parts.at(-1).providerMetadata) : go.error.apiError.data.providerMetadata;
          assert.deepEqual(goMetadata.gateway.execution, tsMetadata.gateway.execution);
          assert.equal(tsMetadata.gateway.execution.attempts.length, row.count + 1);
          assert.deepEqual(tsMetadata.gateway.execution.attempts.map((attempt: any) => attempt.outcome), row.status === undefined ? [...(row.count ? ["failed"] : []), "selected"] : Array(row.count + 1).fill("failed"));
          if (row.primary) {
            assert.equal(tsMetadata.gateway.execution.attempts[0].error.statusCode, row.primary);
            assert.equal(tsMetadata.gateway.execution.attempts[0].error.message, "provider-secret-response [REDACTED] backend-private");
            assert.ok(!JSON.stringify([result, failure, go]).includes(token));
          } else assertConsumerSecretsAbsent([result, failure, go], [token]);
          assertDiscoverySecretsAbsent([discovery, models], primary, [secondary.url, token]);
        }
      }
      assert.deepEqual(primary.violations, []);
      assert.deepEqual(secondary.violations, []);
      const metrics = await gateway.metrics();
      await gateway.stop();
      const logicalLogs = gateway.stderr.split("\n").filter(line => !line.includes('"event":"gateway_physical_attempt"')).join("\n");
      assertPrivateValuesAbsent([logicalLogs, metrics], primary, [secondary.url, "anthropic-secondary", token]);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => primary.stop(), () => secondary.stop());
    }
  });

  it("authenticates service accounts and continues native function calls for both clients and modes", async () => {
    const observer = await FakeAgentObservability.start();
    const invalidToken = "glsa_unknown_service_account_token";
    let resources: [FakeAnthropic, GatewayProcess] | undefined;
    try {
      resources = await startGateway([
        "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls",
      ], { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" }, "claude-sonnet-4-6");
      const [fake, gateway] = resources;
      fake.functionTools = true;
      const client = gateway.client();
      const tools = [{ type: "function" as const, name: "weather", inputSchema: { type: "object" as const, properties: { city: { type: "string" as const } }, required: ["city"] }, strict: false, inputExamples: [{ input: { city: "Rio" } }] }];
      const prompt = [{ role: "user" as const, content: [{ type: "text" as const, text: "Weather in Rio?" }] }];
      const toolChoice = { type: "tool" as const, toolName: "weather" };
      const options = { prompt, tools, toolChoice, maxOutputTokens: 64 };
      await assert.rejects(async () => gateway.client(invalidToken)("assistant").doGenerate(options), (error: any) => {
        assert.equal(error.statusCode, 401);
        return true;
      });
      assert.equal(fake.requests.length, 0, "an unknown credential must not reach the provider");
      let executions = 0;
      for (const mode of ["generate", "stream"] as const) {
        for (const implementation of ["vercel", "go"] as const) {
          const invoke = async (requestOptions: any): Promise<any[]> => {
            if (implementation === "go") {
              const result = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options: requestOptions });
              assert.equal(result.error, undefined);
              return mode === "generate" ? result.result.content : result.parts;
            }
            return mode === "generate" ? (await client("assistant").doGenerate(requestOptions)).content : await collectGatewayStream((await client("assistant").doStream(requestOptions)).stream);
          };
          const first = await invoke(options);
          const call = first.find(part => part.type === "tool-call");
          assert.ok(call, `${implementation} ${mode}: ${JSON.stringify(first)}`);
          assert.equal(call.toolCallId, "call-weather");
          assert.equal(call.toolName, "weather");
          assert.deepEqual(JSON.parse(call.input), { city: "Rio" });
          if (mode === "stream") {
            assert.deepEqual(first.filter(part => part.type.startsWith("tool-")).map(part => part.type), ["tool-input-start", "tool-input-delta", "tool-input-delta", "tool-input-end", "tool-call"]);
            assert.deepEqual(first.filter(part => part.type === "tool-input-delta").map(part => part.delta), ['{"city":', '"Rio"}']);
          }
          executions++;
          const final = await invoke({ ...options, prompt: [...prompt,
            { role: "assistant", content: [{ type: "tool-call", toolCallId: call.toolCallId, toolName: call.toolName, input: JSON.parse(call.input) }] },
            { role: "tool", content: [{ type: "tool-result", toolCallId: call.toolCallId, toolName: call.toolName, output: { type: "text", value: "sunny" } }] }], toolChoice: { type: "auto" } });
          assert.equal(final.filter(part => part.type === (mode === "generate" ? "text" : "text-delta")).map(part => part.text ?? part.delta).join(""), "It is sunny.");
          const native = fake.requests.at(-2)!.body as any;
          assert.deepEqual(native.tools[0], { name: "weather", input_schema: tools[0].inputSchema, strict: false, input_examples: [{ city: "Rio" }], ...(mode === "stream" ? { eager_input_streaming: true } : {}) });
          assert.equal(native.tool_choice.name, "weather");
          assert.equal(native.stream === true, mode === "stream");
          const continuation = fake.requests.at(-1)!.body as any;
          assert.deepEqual(continuation.messages[1].content, [{ type: "tool_use", id: "call-weather", name: "weather", input: { city: "Rio" } }]);
          assert.deepEqual(continuation.messages[2].content, [{ type: "tool_result", tool_use_id: "call-weather", content: [{ type: "text", text: "sunny" }] }]);
        }
      }
      assert.equal(executions, 4);
      assert.equal(fake.requests.length, 8);
      assert.deepEqual(fake.violations, []);
      fake.failureStatus = 500;
      await assert.rejects(async () => client("assistant").doGenerate(options), (error: any) => error.statusCode === 502);
      await observer.waitForGenerations(9);
      const metrics = await gateway.metrics();
      await gateway.stop();
      assert.equal(observer.generations.length, 9, "each unary or streaming invocation finalizes one independent generation");
      assert.deepEqual(observer.violations, []);
      const completed = observer.generations.slice(0, 8);
      const unary = completed.slice(0, 4);
      const streaming = completed.slice(4);
      assert.ok(unary.every(generation => generation.call_error === undefined));
      assert.deepEqual(unary.map(generation => generation.stop_reason).sort(), ["stop", "stop", "tool-calls", "tool-calls"]);
      assert.deepEqual(streaming.map(generation => generation.stop_reason).sort(), ["stop", "stop", "tool-calls", "tool-calls"]);
      // Streaming output is complete before request-context finalization. Until
      // that lifecycle is decoupled, the exporter may additionally classify the
      // completed stream as timed out; public output and usage remain canonical.
      assert.ok(streaming.every(generation => generation.call_error === undefined || generation.call_error === "timeout"));
      assert.equal(new Set(observer.generations.map(generation => generation.id)).size, 9);
      for (const generation of observer.generations) {
        assert.deepEqual(generation.model, { provider: "grafana", name: "grafana/assistant" });
        assert.equal(typeof (generation.metadata as Record<string, unknown>)["gateway.correlation_id"], "string");
      }
      for (const generation of completed) {
        assert.equal((generation.usage as Record<string, unknown>).input_tokens, "2");
        assert.equal((generation.usage as Record<string, unknown>).output_tokens, "3");
      }
      assert.equal(observer.generations.filter(generation => generation.call_error === "server_error").length, 1);
      assertPrivateValuesAbsent([observer.generations, gateway.stderr, metrics], fake, [
        observer.url, invalidToken, "integration-agento11y-key", "call-weather", "weather", "Rio", "sunny", "claude-sonnet-4-6",
      ]);
    } finally {
      await settleCleanup(...(resources ? [() => resources![1].stop(), () => resources![0].stop()] : []), () => observer.stop());
    }
  });

  for (const mcpEnabled of [false, true]) it(`routes authenticated Anthropic provider tools ${mcpEnabled ? "with MCP" : "without MCP"} and continuation through both clients`, async () => {
    const observer = await FakeAgentObservability.start();
    let resources: [FakeAnthropic, GatewayProcess] | undefined;
    try {
      resources = await startGateway([
        "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls",
      ], { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" }, "claude-sonnet-4-6");
      const [fake, gateway] = resources;
      fake.providerTools = true;
      const token = "mcp-private-token";
      const url = "https://mcp.example.test/tools";
      const providerOptions = mcpEnabled ? { anthropic: { mcpServers: [{ type: "url", name: "echo", url, authorizationToken: token, toolConfiguration: { enabled: false, allowedTools: [] } }] } } : undefined;
      const tools = [{ type: "provider" as const, id: "anthropic.code_execution_20260120" as const, name: "python", args: {} }];
      const prompt = [{ role: "user" as const, content: [{ type: "text" as const, text: "Use tools" }] }];
      const client = gateway.client();
      for (const mode of ["generate", "stream"] as const) {
        for (const implementation of ["vercel", "go"] as const) {
          const invoke = async (messages: LanguageModelV4CallOptions["prompt"]): Promise<any[]> => {
            const options = { prompt: messages, tools, providerOptions: { anthropic: { disableParallelToolUse: true, ...(mcpEnabled ? providerOptions?.anthropic : {}) } } };
            if (implementation === "go") {
              const result = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options });
              assert.equal(result.error, undefined);
              return mode === "generate" ? result.result.content : result.parts;
            }
            return mode === "generate" ? (await client("assistant").doGenerate(options)).content : await collectGatewayStream((await client("assistant").doStream(options)).stream);
          };
          const first = await invoke(prompt);
          const calls = first.filter(part => part.type === "tool-call");
          const results = first.filter(part => part.type === "tool-result");
          assert.deepEqual(calls.map(part => part.toolName), mcpEnabled ? ["python", "echo"] : ["python"]);
          assert.deepEqual(results.map(part => part.toolName), mcpEnabled ? ["python", "echo"] : ["python"]);
          assert.ok(calls.every(part => part.providerExecuted === true));
          assert.deepEqual(calls[0].providerMetadata, { anthropic: { caller: { type: "direct" } } });
          if (mcpEnabled) {
            assert.equal(calls[1].dynamic, true);
            assert.deepEqual(calls[1].providerMetadata, { anthropic: { type: "mcp-tool-use", serverName: "echo" } });
            assert.deepEqual(results[1].providerMetadata, calls[1].providerMetadata);
          }
          const nativeRequest = fake.requests.at(-1)!;
          const native = nativeRequest.body as any;
          assert.equal(singleHeader(nativeRequest.headers["anthropic-beta"])?.includes("mcp-client-2025-04-04") ?? false, mcpEnabled);
          assert.deepEqual(native.tools, [{ type: "code_execution_20260120", name: "code_execution" }]);
          assert.equal(native.max_tokens, 128000);
          assert.equal(native.tool_choice?.disable_parallel_tool_use, true);
          assert.deepEqual(native.mcp_servers, mcpEnabled ? [{ type: "url", name: "echo", url, authorization_token: token, tool_configuration: { enabled: false, allowed_tools: [] } }] : undefined);
          const history = calls.flatMap((call, index) => [
            { type: "tool-call" as const, toolCallId: call.toolCallId, toolName: call.toolName, input: JSON.parse(call.input), providerExecuted: true, ...(call.providerMetadata && { providerOptions: call.providerMetadata }) },
            { type: "tool-result" as const, toolCallId: results[index].toolCallId, toolName: results[index].toolName, output: { type: "json" as const, value: results[index].result }, ...(results[index].providerMetadata && { providerOptions: results[index].providerMetadata }) },
          ]);
          const final = await invoke([...prompt, { role: "assistant", content: history }]);
          assert.equal(final.filter(part => part.type === (mode === "generate" ? "text" : "text-delta")).map(part => part.text ?? part.delta).join(""), "Hosted tools finished.");
          const continuation = fake.requests.at(-1)!.body as any;
          assert.deepEqual(continuation.messages[1].content.map((part: any) => part.type), mcpEnabled ? ["server_tool_use", "code_execution_tool_result", "mcp_tool_use", "mcp_tool_result"] : ["server_tool_use", "code_execution_tool_result"]);
          assert.equal(continuation.messages[1].content[0].name, "code_execution");
          assert.deepEqual(continuation.messages[1].content[0].caller, { type: "direct" });
          assert.equal(continuation.tool_choice?.disable_parallel_tool_use, true);
          if (mcpEnabled) assert.equal(continuation.messages[1].content[2].server_name, "echo");
          assert.ok(!JSON.stringify([first, final]).includes(token));
          assert.ok(!JSON.stringify([first, final]).includes(url));
        }
      }
      assert.equal(fake.requests.length, 8);
      if (mcpEnabled) {
        for (const invalidOptions of [
          { prompt, tools, providerOptions: { anthropic: { mcpServers: [{ type: "url", name: "echo", url: "http://mcp.example.test", authorizationToken: "invalid-private-token" }] } }, maxOutputTokens: 64 },
          { prompt: [...prompt, { role: "assistant" as const, content: [{ type: "tool-call" as const, toolCallId: "call-mcp", toolName: "echo", input: {}, providerExecuted: true, providerOptions: { anthropic: { type: "mcp-tool-use", serverName: "not-configured" } } }] }], tools, providerOptions, maxOutputTokens: 64 },
        ]) {
          for (const mode of ["generate", "stream"] as const) {
            const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options: invalidOptions });
            assert.equal(go.error?.statusCode, 400);
            assert.equal(go.error?.code, "invalid_request");
            assert.equal(go.parts, undefined);
            await assert.rejects(async () => {
              if (mode === "generate") await client("assistant").doGenerate(invalidOptions);
              else await client("assistant").doStream(invalidOptions);
            }, (error: any) => error.statusCode === 400 && error.message === "invalid request");
            assert.equal(fake.requests.length, 8, "invalid MCP settings must fail before native invocation");
          }
        }
      }
      assert.deepEqual(fake.violations, []);
      const expectedGenerations = mcpEnabled ? 16 : 8;
      await observer.waitForGenerations(expectedGenerations);
      assert.equal(observer.generations.length, expectedGenerations);
      for (const generation of observer.generations) {
        assert.deepEqual(generation.model, { provider: "grafana", name: "grafana/assistant" });
      }
      const metrics = await gateway.metrics();
      await gateway.stop();
      assertPrivateValuesAbsent([gateway.stderr, metrics, observer.generations], fake, [token, url, "call-code", "call-mcp", "python", "echo", "code_execution", "integration-agento11y-key"]);
    } finally {
      await settleCleanup(...(resources ? [() => resources![1].stop(), () => resources![0].stop()] : []), () => observer.stop());
    }
  });

  it("preserves non-direct caller IDs through both clients and native continuation", async () => {
    const [fake, gateway] = await startGateway([], {}, "claude-sonnet-4-6");
    fake.providerToolCaller = true;
    try {
      const tools = [{ type: "function" as const, name: "execute", inputSchema: { type: "object" as const } }];
      const prompt = [{ role: "user" as const, content: [{ type: "text" as const, text: "Use a tool" }] }];
      for (const mode of ["generate", "stream"] as const) {
        for (const implementation of ["vercel", "go"] as const) {
          const invoke = async (messages: LanguageModelV4CallOptions["prompt"]): Promise<any[]> => {
            const options = { prompt: messages, tools };
            if (implementation === "go") {
              const captured = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options });
              assert.equal(captured.error, undefined);
              return mode === "generate" ? captured.result.content : captured.parts;
            }
            return mode === "generate" ? (await gateway.client()("assistant").doGenerate(options)).content : await collectGatewayStream((await gateway.client()("assistant").doStream(options)).stream);
          };
          const first = await invoke(prompt);
          const call = first.find(part => part.type === "tool-call");
          assert.ok(call);
          assert.deepEqual(call.providerMetadata, { anthropic: { caller: { type: "code_execution_20260120", toolId: "call-parent" } } });
          const historyCall = { type: "tool-call" as const, toolCallId: call.toolCallId, toolName: call.toolName, input: JSON.parse(call.input), providerOptions: call.providerMetadata };
          const historyResult = { type: "tool-result" as const, toolCallId: call.toolCallId, toolName: call.toolName, output: { type: "text" as const, value: "done" } };
          const final = await invoke([...prompt, { role: "assistant", content: [historyCall] }, { role: "tool", content: [historyResult] }]);
          assert.equal(final.filter(part => part.type === (mode === "generate" ? "text" : "text-delta")).map(part => part.text ?? part.delta).join(""), "Hosted tools finished.");
          const continuation = fake.requests.at(-1)!.body as any;
          assert.deepEqual(continuation.messages[1].content[0].caller, { type: "code_execution_20260120", tool_id: "call-parent" });
          assert.equal(continuation.messages[2].content[0].type, "tool_result");
        }
      }
      assert.equal(fake.requests.length, 8);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  for (const failExecution of [false, true]) {
    it(`preserves automatic streaming native history after ${failExecution ? "failed" : "successful"} local execution`, async () => {
      const [fake, gateway] = await startGateway([], {}, "claude-sonnet-4-6");
      fake.functionTools = true;
      try {
        const executions: Array<{ city: string; toolCallId: string }> = [];
        const executionError = new Error("weather unavailable");
        const errors: unknown[] = [];
        const result = streamText({
          model: gateway.client()("assistant"),
          prompt: "Weather in Rio?",
          tools: {
            weather: tool({
              inputSchema: jsonSchema<{ city: string }>({ type: "object", properties: { city: { type: "string" } }, required: ["city"] }),
              execute: async ({ city }, { toolCallId }) => {
                executions.push({ city, toolCallId });
                if (failExecution) throw executionError;
                return "sunny";
              },
            }),
          },
          prepareStep: ({ stepNumber }) => ({ toolChoice: stepNumber === 0 ? "required" : "none" }),
          stopWhen: isStepCount(2),
          maxRetries: 0,
          abortSignal: AbortSignal.timeout(10_000),
          onError: ({ error }) => { errors.push(error); },
        });
        const parts = [];
        for await (const part of result.fullStream) parts.push(part);
        assert.deepEqual(errors, []);
        assert.deepEqual(executions, [{ city: "Rio", toolCallId: "call-weather" }]);
        assert.equal(await result.text, "It is sunny.");
        assert.equal(await result.finishReason, "stop");
        assert.deepEqual((await result.steps).map(step => step.finishReason), ["tool-calls", "stop"]);
        if (failExecution) {
          const error = parts.find(part => part.type === "tool-error");
          assert.equal(error?.toolCallId, "call-weather");
          assert.equal(error?.error, executionError);
        } else {
          assert.ok(parts.some(part => part.type === "tool-result" && part.toolCallId === "call-weather" && part.output === "sunny"));
        }
        assert.equal(fake.requests.length, 2);
        assert.ok(fake.requests.every(request => request.body.stream === true));
        assert.deepEqual(fake.requests[0]!.body.tool_choice, { type: "any" });
        const continuation = fake.requests[1]!.body;
        assert.deepEqual(continuation.tools, fake.requests[0]!.body.tools);
        assert.deepEqual(continuation.tool_choice, { type: "none" });
        assert.deepEqual(continuation.messages, [
          { role: "user", content: [{ type: "text", text: "Weather in Rio?" }] },
          { role: "assistant", content: [{ type: "tool_use", id: "call-weather", name: "weather", input: { city: "Rio" } }] },
          { role: "user", content: [{ type: "tool_result", tool_use_id: "call-weather", ...(failExecution ? { is_error: true } : {}), content: [{ type: "text", text: failExecution ? String(executionError) : "sunny" }] }] },
        ]);
        assert.deepEqual(fake.violations, []);
      } finally {
        await settleCleanup(() => gateway.stop(), () => fake.stop());
      }
    });
  }

  it("protects configured keys from unused accounts and the exporter in native summaries", async () => {
    const fake = await FakeAnthropic.start();
    const observer = await FakeAgentObservability.start();
    let gateway: GatewayProcess | undefined;
    try {
      const yaml = anthropicConfig(fake.url).replace("models:", "  unused:\n    type: openai\n    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY\nmodels:");
      gateway = await GatewayProcess.start(binaryPath, fake.url, [
        "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls",
      ], { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" }, "access-token", yaml);
      fake.failureStatus = 503;
      const options: LanguageModelV4CallOptions = { prompt: [{ role: "user", content: [{ type: "text", text: "configured-key-summary" }] }], maxOutputTokens: 32 };
      for (const mode of ["generate", "stream"] as const) {
        const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant", mode, options });
        let ts: any;
        try {
          const model = gateway.client()("assistant");
          await (mode === "generate" ? model.doGenerate(options) : model.doStream(options));
        } catch (error) { ts = error; }
        assert.ok(ts);
        const summary = ts.cause.data.providerMetadata.gateway.execution.attempts[0].error;
        assert.deepEqual(summary, { message: "[REDACTED] [REDACTED] [REDACTED] own-byok-key", type: "api_error", statusCode: 503 });
        assert.deepEqual(go.error.apiError.data.providerMetadata.gateway.execution.attempts[0].error, summary);
        assert.equal(ts.statusCode, 503);
        assert.equal(go.error.isRetryable, ts.isRetryable);
      }
      assert.equal(fake.requests.length, 4);
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => fake.stop(), () => observer.stop());
    }
  });

  it("maps the reachable Go public error matrix for unary and stream setup", async () => {
    const [fake, gateway] = await startGateway();
    const rows = [
      { upstream: 400, status: 424, category: "failed_dependency", code: "failed_dependency", retryable: false },
      { upstream: 429, status: 429, category: "rate_limit_exceeded", code: "rate_limit_exceeded", retryable: true },
      { upstream: 500, status: 502, category: "internal_server_error", code: "upstream_error", retryable: true },
      { upstream: 503, status: 503, category: "internal_server_error", code: "overloaded", retryable: true },
      { upstream: 408, status: 504, category: "internal_server_error", code: "timeout", retryable: true },
    ];
    try {
      for (const row of rows) {
        fake.failureStatus = row.upstream;
        for (const mode of ["generate", "stream"] as const) {
          const options = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "public-error-matrix" }] }], maxOutputTokens: 32 };
          const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, mode, modelID: "assistant", options });
          assert.deepEqual({ status: go.error?.statusCode, category: go.error?.category, code: go.error?.code, retryable: go.error?.isRetryable }, { status: row.status, category: row.category, code: row.code, retryable: row.retryable });
          let ts: any;
          try {
            const model = gateway.client()("assistant");
            await (mode === "generate" ? model.doGenerate(options) : model.doStream(options));
          } catch (error) { ts = error; }
          assert.ok(ts);
          assert.deepEqual({ status: go.error.statusCode, category: go.error.category, retryable: go.error.isRetryable }, { status: ts.statusCode, category: ts.type, retryable: ts.isRetryable });
          const overview = ts.cause.data.providerMetadata.gateway.execution;
          assert.deepEqual(go.error.apiError.data.providerMetadata.gateway.execution, overview);
          assert.deepEqual(overview.attempts[0].error, { message: "provider-secret-response [REDACTED] backend-private", type: "api_error", statusCode: row.upstream });
          assert.ok(!JSON.stringify([go, ts]).includes(TEST_TOKEN));
        }
      }
      assert.equal(fake.requests.length, rows.length * 4, "neither client nor service should retry model calls");
      const metrics = await gateway.metrics();
      await gateway.stop();
      assertPrivateValuesAbsent([gateway.stderr, metrics], fake);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("serves Go discovery, canonical/alias unary, streaming, cancellation, and public errors", async () => {
    const [fake, gateway] = await startGateway();
    const base = { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN };
    try {
      const discovery = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery" });
      assert.equal(discovery.error, undefined);
      assert.deepEqual(discovery.models.map((model: { id: string }) => model.id), ["grafana/assistant"]);
      for (const modelID of ["assistant", "grafana/assistant"]) {
        const result = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", modelID, options: { prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }], maxOutputTokens: 32, temperature: 0.2 } });
        assert.equal(result.error, undefined);
        assert.deepEqual(result.result.content, [{ type: "text", text: "hello from fake Anthropic" }]);
        assert.equal(result.result.response.modelId, undefined);
        assertConsumerSecretsAbsent(result);
        assert.deepEqual(result.result.response.body.response, { id: "msg_test", modelId: "backend-private" });
      }
      const stream = await captureGoClient(goClientBinaryPath, { ...base, mode: "stream", modelID: "assistant", options: { prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }], maxOutputTokens: 32 } });
      assert.equal(stream.error, undefined);
      assert.equal(stream.parts[0].type, "stream-start");
      assert.equal(stream.parts.at(-1).type, "finish");
      assert.ok(stream.parts.filter((part: { type: string }) => part.type === "text-delta").map((part: { delta: string }) => part.delta).join("").includes("hello from fake Anthropic stream"));
      const abort = await captureGoClient(goClientBinaryPath, { ...base, mode: "stream", modelID: "assistant", abortAfterParts: 1, options: { prompt: [{ role: "user", content: [{ type: "text", text: "silent-abort" }] }], maxOutputTokens: 32 } });
      assert.equal(abort.error, undefined); assert.equal(abort.parts[0].type, "stream-start");
      assert.ok(abort.parts.every((part: { type: string }) => part.type !== "error"));
      await fake.waitForCancellation("silent-abort");
      const missing = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", modelID: "missing", options: { prompt: [] } });
      assert.equal(missing.error.category, "model_not_found"); assert.equal(missing.error.statusCode, 404);
      const invalid = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", modelID: "assistant", options: { prompt: [], headers: { authorization: "Bearer caller-controlled" } } });
      assert.equal(invalid.error.message, "grafana: reserved authentication header"); assert.equal(invalid.error.statusCode, undefined);
      const unauthorized = await captureGoClient(goClientBinaryPath, { ...base, serviceAccountToken: "glsa_invalid-token", mode: "discovery" });
      assert.equal(unauthorized.error.statusCode, 401);
      assert.deepEqual(fake.violations, []); assert.equal(await gateway.ready(), true);
      const metrics = await gateway.metrics();
      await gateway.stop();
      assertDiscoverySecretsAbsent(discovery, fake);
      assertConsumerSecretsAbsent([stream, abort]);
      assert.equal(stream.parts.find((part: any) => part.type === "response-metadata").modelId, "backend-private");
      assertPrivateValuesAbsent([missing, invalid, unauthorized, metrics, gateway.stderr], fake, ["glsa_invalid-token"]);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("returns unary outputs above the former response limits to both clients", async () => {
    const [fake, gateway] = await startGateway();
    const options = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "large-output" }] }], maxOutputTokens: 32 };
    try {
      const vercel = await gateway.client()("assistant").doGenerate(options);
      assert.deepEqual(vercel.content, [{ type: "text", text: largeText }]);
      assert.equal((vercel.usage.raw as { native_padding?: string }).native_padding, largeUsagePadding);
      const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, mode: "generate", modelID: "assistant", options });
      assert.equal(go.error, undefined);
      assert.deepEqual(go.result.content, [{ type: "text", text: largeText }]);
      assert.equal(go.result.usage.raw.native_padding, largeUsagePadding);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("streams frames, raw events and raw usage above the former frame limit to both clients", async () => {
    const [fake, gateway] = await startGateway();
    const options = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "large-output" }] }], maxOutputTokens: 32, includeRawChunks: true };
    const assertLargeStream = (parts: Array<Record<string, any>>) => {
      assert.equal(parts.filter((part) => part.type === "text-delta").map((part) => part.delta).join(""), largeDelta);
      assert.ok(parts.some((part) => part.type === "raw" && JSON.stringify(part.rawValue).includes(largeDelta)));
      const finish = parts.find((part) => part.type === "finish");
      assert.equal(finish?.usage.raw.native_padding, largeUsagePadding);
      assert.ok(parts.every((part) => part.type !== "error"));
    };
    try {
      const result = await gateway.client()("assistant").doStream(options);
      assertLargeStream(await collectGatewayStream(result.stream));
      const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, mode: "stream", modelID: "assistant", options });
      assert.equal(go.error, undefined);
      assertLargeStream(go.parts);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("streams a provider response above 16 MiB in total, with every event within the bound, to both clients", async () => {
    const [fake, gateway] = await startGateway();
    const options = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "long-output" }] }], maxOutputTokens: 32 };
    const expected = longDelta.repeat(longDeltaCount);
    const assertLongStream = (parts: Array<Record<string, any>>) => {
      const text = parts.filter((part) => part.type === "text-delta").map((part) => part.delta).join("");
      assert.equal(text.length, expected.length);
      assert.ok(text === expected, "streamed text differs from the provider deltas");
      assert.equal(parts.find((part) => part.type === "finish")?.finishReason.unified, "stop");
      assert.ok(parts.every((part) => part.type !== "error"));
    };
    try {
      const result = await gateway.client()("assistant").doStream(options);
      assertLongStream(await collectGatewayStream(result.stream));
      const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, mode: "stream", modelID: "assistant", options });
      assert.equal(go.error, undefined);
      assertLongStream(go.parts);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("maps file inputs and file-result continuation through authenticated native calls", async () => {
    const [fake, gateway] = await startGateway();
    const client = gateway.client();
    const base = { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant" };
    try {
      const prompt: LanguageModelV4CallOptions["prompt"] = [{
        role: "user",
        content: [
          { type: "text", text: "normal-stream" },
          { type: "file", data: { type: "data", data: "AQID" }, mediaType: "image/png", filename: "" },
          { type: "file", data: { type: "text", text: "" }, mediaType: "text/plain", filename: "" },
          { type: "file", data: { type: "url", url: new URL("https://example.test/doc.pdf") }, mediaType: "application/pdf" },
          { type: "file", data: { type: "reference", reference: { anthropic: "file-1" } }, mediaType: "application/pdf" },
          { type: "file", data: { type: "text", text: "inline document" }, mediaType: "application/pdf", filename: "notes.txt" },
        ],
      }];
      for (const mode of ["generate", "stream"] as const) {
        const options = { prompt };
        const ts = mode === "generate"
          ? (await client("assistant").doGenerate(options)).content
          : await collectGatewayStream((await client("assistant").doStream(options)).stream);
        const go = await captureGoClient(goClientBinaryPath, { ...base, mode, options: JSON.parse(JSON.stringify(options)) });
        assert.equal(go.error, undefined);
        assert.ok(JSON.stringify(ts).includes("hello from fake Anthropic"));
        assert.ok(JSON.stringify(mode === "generate" ? go.result.content : go.parts).includes("hello from fake Anthropic"));
        const first = fake.requests.at(-2)!.body as any;
        const second = fake.requests.at(-1)!.body as any;
        assert.deepEqual(second, first);
        const content = first.messages[0].content;
        assert.deepEqual(content[1].source, { type: "base64", media_type: "image/png", data: "AQID" });
        assert.deepEqual(content[2].source, { type: "text", media_type: "text/plain", data: "" });
        assert.equal(content[2].title, "");
        assert.deepEqual(content[3].source, { type: "url", url: "https://example.test/doc.pdf" });
        assert.deepEqual(content[4].source, { type: "file", file_id: "file-1" });
        assert.deepEqual(content[5].source, { type: "text", media_type: "text/plain", data: "inline document" });
        assert.equal(content[5].title, "notes.txt");
      }

      fake.functionTools = true;
      const fileResult: LanguageModelV4CallOptions = {
        prompt: [...prompt,
          { role: "assistant", content: [{ type: "tool-call", toolCallId: "call-weather", toolName: "weather", input: { city: "Rio" } }] },
          { role: "tool", content: [{ type: "tool-result", toolCallId: "call-weather", toolName: "weather", output: { type: "content", value: [
            { type: "text", text: "weather result" },
            { type: "file", filename: "", data: { type: "data", data: "BQY=" }, mediaType: "image/png" },
            { type: "file", data: { type: "url", url: new URL("https://example.test/result.png") }, mediaType: "image/png" },
            { type: "file", data: { type: "url", url: new URL("https://example.test/result.pdf") }, mediaType: "application/pdf" },
            { type: "file", data: { type: "data", data: "JVBERi0=" }, mediaType: "application/pdf" },
          ] } }] },
        ],
      };
      for (const mode of ["generate", "stream"] as const) {
        const ts = mode === "generate"
          ? (await client("assistant").doGenerate(fileResult)).content
          : await collectGatewayStream((await client("assistant").doStream(fileResult)).stream);
        const go = await captureGoClient(goClientBinaryPath, { ...base, mode, options: JSON.parse(JSON.stringify(fileResult)) });
        assert.equal(go.error, undefined);
        assert.ok(JSON.stringify(ts).includes("It is sunny."));
        assert.ok(JSON.stringify(mode === "generate" ? go.result.content : go.parts).includes("It is sunny."));
        const first = fake.requests.at(-2)!.body as any;
        const second = fake.requests.at(-1)!.body as any;
        assert.deepEqual(second, first);
        const result = first.messages.at(-1).content[0];
        assert.equal(result.type, "tool_result");
        assert.deepEqual(result.content[1].source, { type: "base64", media_type: "image/png", data: "BQY=" });
        assert.deepEqual(result.content[2].source, { type: "url", url: "https://example.test/result.png" });
        assert.deepEqual(result.content[3].source, { type: "url", url: "https://example.test/result.pdf" });
        assert.deepEqual(result.content[4].source, { type: "base64", media_type: "application/pdf", data: "JVBERi0=" });
      }
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("exchanges an internal access policy token through the Go client before configured discovery", async () => {
    const fake = await FakeAnthropic.start();
    let gateway: GatewayProcess | undefined;
    try {
      gateway = await GatewayProcess.start(binaryPath, fake.url, [], {}, "cloud-gateway");
      const cap = await gateway.credential({ kind: "access-policy", service: INTERNAL_SERVICE });
      const exchangeURL = `${gateway.url}/platformhost/exchange`;
      const result = await captureGoClient(goClientBinaryPath, { mode: "discovery", tokenExchange: { CAPToken: cap, Namespace: TEST_NAMESPACE, Audience: "grafana-aigateway-app", BaseURL: gateway.baseURL, TokenExchangeURL: exchangeURL } });
      assert.equal(result.error, undefined);
      assert.deepEqual(result.models.map((model: { id: string }) => model.id), ["grafana/assistant"]);
      assert.equal(fake.requests.length, 0);
      const metrics = await gateway.metrics();
      assert.match(metrics, /grafana_ai_gateway_access_decisions_total\{decision="configured"\} 1$/m);
      await gateway.stop();
      assertDiscoverySecretsAbsent(result, fake, [cap]);
      assertPrivateValuesAbsent([metrics, gateway.stderr], fake, [cap]);
      assert.ok(gateway.stderr.includes(`"caller_service":"${INTERNAL_SERVICE}"`));
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => fake.stop());
    }
  });

  it("discovers and invokes every canonical and alias model ID", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const rawDiscoveryResponse = await fetch(`${gateway.baseURL}/config`, {
        headers: { Authorization: `Bearer ${TEST_TOKEN}` },
      });
      assert.equal(rawDiscoveryResponse.status, 200);
      const rawDiscovery = await rawDiscoveryResponse.text();
      for (const privateValue of ["GATEWAY_TEST_ANTHROPIC_KEY", "integration-anthropic-key", fake.url]) {
        assert.ok(!rawDiscovery.includes(privateValue));
      }
      assert.deepEqual(JSON.parse(rawDiscovery).models[0].gateway, { aliases: ["assistant"], primary: { providerInstance: "anthropic-primary", provider: "anthropic", providerModelId: "backend-private" }, fallbacks: [] });
      const baseURL = gateway.baseURL;
      const companion = await fetchConfiguredModels({ baseURL, headers: { Authorization: `Bearer ${TEST_TOKEN}` } });
      assert.deepEqual(companion, JSON.parse(rawDiscovery));
      const go = await captureGoClient(goClientBinaryPath, { baseURL, serviceAccountToken: TEST_TOKEN, mode: "discovery" });
      assert.equal(go.error, undefined);
      assert.deepEqual(go.models, companion.models);
      assert.equal(fake.requests.length, 0);
      const client = gateway.client();
      const metadata = await client.getAvailableModels();
      assert.deepEqual(metadata.models.map((model) => model.id), ["grafana/assistant"]);
      assert.equal("gateway" in metadata.models[0], false);
      for (const row of metadata.models) {
        assert.deepEqual(row.specification, {
          specificationVersion: "v4",
          provider: "grafana",
          modelId: row.id,
        });
      }
      for (const id of companion.models.flatMap(row => [row.id, ...(row.gateway?.aliases ?? [])])) {
        const result = await client(id).doGenerate({
          prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }],
          maxOutputTokens: 32,
          temperature: 0.2,
        });
        assert.deepEqual(result.content, [{ type: "text", text: "hello from fake Anthropic" }]);
      }
      const rawUnaryResponse = await rawProviderWireRequest(gateway.baseURL, "unary");
      assert.equal(rawUnaryResponse.status, 200);
      const rawUnary = await rawUnaryResponse.json() as Record<string, unknown>;
      assert.deepEqual(Object.keys(rawUnary).sort(), ["content", "finishReason", "providerMetadata", "response", "usage", "warnings"]);
      assert.deepEqual(rawUnary.response, { id: "msg_test", modelId: "backend-private" });

      assert.equal(fake.requests.length, 3);
      for (const request of fake.requests) {
        assert.equal(request.path, "/v1/messages");
        assert.equal(request.apiKey, "integration-anthropic-key");
        assert.equal(request.body.model, "backend-private");
        assert.equal(request.body.max_tokens, 32);
        assert.equal(request.body.temperature, 0.2);
        assert.deepEqual(request.body.messages, [{ role: "user", content: [{ type: "text", text: "unary" }] }]);
      }
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("streams normal finish and clean EOF while the command remains ready", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const result = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }],
        maxOutputTokens: 32,
      });
      const reader = result.stream.getReader();
      const parts: Array<{ type: string; delta?: string }> = [];
      for (;;) {
        const next = await reader.read();
        if (next.done) break;
        parts.push(next.value);
      }
      assert.equal(parts[0]?.type, "stream-start");
      assert.ok(parts.map((part) => part.type).includes("finish"));
      assert.ok(parts.filter((part) => part.type === "text-delta").map((part) => part.delta).join("")
        .includes("hello from fake Anthropic stream"));
      assert.equal(await gateway.ready(), true);
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("cancels an established Anthropic stream when the client aborts", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const result = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-abort" }] }],
        maxOutputTokens: 32,
      });
      const reader = result.stream.getReader();
      const first = await reader.read();
      assert.equal(first.done, false);
      assert.equal(first.value?.type, "stream-start");
      await reader.cancel("test abort");
      await fake.waitForCancellation("silent-abort");
      assert.equal(await gateway.ready(), true);
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("records one private-safe logical observation across traffic and shutdown", async () => {
    const observer = await FakeAgentObservability.start();
    let resources: [FakeAnthropic, GatewayProcess] | undefined;
    try {
      resources = await startGateway([
        "--agento11y.enabled",
        "--agento11y.protocol=http",
        `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls",
      ], { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" });
      const [fake, gateway] = resources;
      const fileParts = [
        { type: "file", filename: "private-filename", data: { type: "data", data: "cHJpdmF0ZS1ieXRlcw==" }, mediaType: "image/png" },
        { type: "file", data: { type: "url", url: new URL("https://cdn.example.test/private-url?token=private-query") }, mediaType: "application/pdf" },
        { type: "file", data: { type: "reference", reference: { anthropic: "private-reference" } }, mediaType: "application/pdf" },
        { type: "file", filename: "", data: { type: "text", text: "private-document-text" }, mediaType: "text/plain", providerOptions: { anthropic: { title: "private-file-title" } } },
      ] as const;

      const unary = await gateway.client()("assistant").doGenerate({
        prompt: [{ role: "user", content: [{ type: "text", text: "private-unary-input" }, ...fileParts] }],
        maxOutputTokens: 32,
      });
      assert.deepEqual(unary.content, [{ type: "text", text: "hello from fake Anthropic" }]);

      const streamed = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }, ...fileParts] }],
        maxOutputTokens: 32,
      });
      const streamParts = await collectGatewayStream(streamed.stream);
      assert.equal(streamParts[0]?.type, "stream-start");
      assert.deepEqual(streamParts.map((part) => part.type), [
        "stream-start", "response-metadata", "custom", "text-start", "text-delta", "text-end", "finish",
      ]);
      assert.deepEqual(streamParts[2], { type: "custom", kind: "anthropic.message_start", providerMetadata: { anthropic: { id: "msg_test", model: "backend-private", usage: { input_tokens: 2, output_tokens: 0 } } } });
      assert.equal(streamParts.filter((part) => part.type === "text-delta").map((part) => part.delta).join(""), "hello from fake Anthropic stream");

      const providerFailure = await rawProviderWireRequest(gateway.baseURL, "provider-error", "assistant", fileParts);
      assert.equal(providerFailure.status, 502);
      const failureBody = await providerFailure.json();
      assert.deepEqual(failureBody.error, { message: "upstream failure", type: "internal_server_error", param: null, code: "upstream_error" });
      assert.equal(failureBody.providerMetadata.gateway.execution.attempts[0].error.message, "provider-secret-response");

      const aborted = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-abort" }, ...fileParts] }],
        maxOutputTokens: 32,
      });
      const abortReader = aborted.stream.getReader();
      const abortStart = await abortReader.read();
      assert.equal(abortStart.done, false);
      assert.equal(abortStart.value?.type, "stream-start");
      await abortReader.cancel("integration abort");
      await fake.waitForCancellation("silent-abort");

      const shuttingDown = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-shutdown" }, ...fileParts] }],
        maxOutputTokens: 32,
      });
      const shutdownStart = await shuttingDown.stream.getReader().read();
      assert.equal(shutdownStart.done, false);
      assert.equal(shutdownStart.value?.type, "stream-start");

      await observer.waitForGenerations(4);
      const metrics = await gateway.metrics();
      const canonicalLabels = ['provider="grafana"', 'model="grafana/assistant"'];
      assert.equal(sumPrometheusSamples(metrics, "aisdk_model_requests_total", canonicalLabels), 4);
      assert.equal(sumPrometheusSamples(metrics, "aisdk_model_inflight_requests", canonicalLabels), 1);
      assert.ok(metrics.includes('operation="generate"'));
      assert.ok(metrics.includes('operation="stream"'));
      assert.ok(metrics.includes('status="success"'));
      assert.ok(metrics.includes('status="error"'));
      assert.ok(metrics.includes('status="canceled"'));
      assert.ok(!metrics.includes("grafana_ai_gateway_agento11y_export_failures_total{"));

      await gateway.stop();
      await fake.waitForCancellation("silent-shutdown");
      await observer.waitForGenerations(5);
      assert.ok(!gateway.stderr.includes('"@message":"agent observability export failed"'));
      assert.equal(fake.requests.length, 5);
      assert.deepEqual(fake.violations, []);
      assert.deepEqual(observer.violations, []);
      assert.equal(observer.generations.length, 5);
      const allowedMetadata = new Set([
        "gateway.caller_service", "gateway.correlation_id", "gateway.namespace",
        "agento11y.sdk.content_capture_mode", "agento11y.sdk.name", "call_error",
      ]);
      for (const generation of observer.generations) {
        assert.deepEqual(generation.model, { provider: "grafana", name: "grafana/assistant" });
        assert.equal(generation.agent_name ?? "", "");
        assert.equal(generation.user_id ?? "", "");
        const metadata = generation.metadata as Record<string, unknown>;
        assert.equal(metadata["gateway.caller_service"], undefined);
        assert.equal(metadata["gateway.namespace"], "stacks-27038");
        assert.equal(typeof metadata["gateway.correlation_id"], "string");
        assert.ok(Object.keys(metadata).every((key) => allowedMetadata.has(key)));
        assert.ok([undefined, "server_error", "canceled"].includes(metadata.call_error as string | undefined));
      }
      const surfaces = JSON.stringify({ metrics, logs: gateway.stderr, generations: observer.generations });
      for (const privateValue of [
        "integration-anthropic-key", "integration-agento11y-key", "GF_PLUGIN_AGENTO11Y_TOKEN",
        "backend-private", fake.url, observer.url, TEST_TOKEN, "private-unary-input",
        "hello from fake Anthropic", "provider-secret-response", "private-filename", "cHJpdmF0ZS1ieXRlcw==", "private-url", "private-query", "private-reference", "private-document-text", "private-file-title",
      ]) {
        assert.ok(!surfaces.includes(privateValue), `private value leaked: ${privateValue}`);
      }
    } finally {
      await settleCleanup(
        ...(resources == null ? [] : [() => resources![1].stop(), () => resources![0].stop()]),
        () => observer.stop(),
      );
    }
  });

  it("keeps real command traffic fail-open when Agent Observability is unavailable", async () => {
    const unavailablePort = await availablePort();
    const [fake, gateway] = await startGateway([
      "--agento11y.enabled",
      "--agento11y.protocol=http",
      `--agento11y.endpoint=http://127.0.0.1:${unavailablePort}`,
      "--no-agento11y.tls",
    ], { GF_PLUGIN_AGENTO11Y_TOKEN: "outage-agento11y-key" });
    try {
      const response = await rawProviderWireRequest(gateway.baseURL, "outage-private-input");
      assert.equal(response.status, 200);
      const body = await response.json() as Record<string, unknown>;
      assert.deepEqual(Object.keys(body).sort(), ["content", "finishReason", "providerMetadata", "response", "usage", "warnings"]);
      let metrics = "";
      await poll(async () => {
        metrics = await gateway.metrics();
        return metrics.includes('grafana_ai_gateway_agento11y_export_failures_total{class="transport"} 1');
      }, 30_000, "Agent Observability transport failure metric");
      assert.equal(await gateway.ready(), true);
      assert.equal(fake.requests.length, 1);
      for (const privateValue of [
        "outage-agento11y-key", "outage-private-input", "integration-anthropic-key",
        "backend-private", fake.url, TEST_TOKEN,
      ]) {
        assert.ok(!metrics.includes(privateValue));
        assert.ok(!gateway.stderr.includes(privateValue));
      }
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("accepts only platform credential shapes and rejects redirects and private telemetry", async () => {
    const redirectTarget = await FakeAnthropic.start();
    let resources: [FakeAnthropic, GatewayProcess] | undefined;
    try {
      resources = await startGateway();
      const [fake, gateway] = resources;
      const discovery = await fetch(`${gateway.baseURL}/config`, { headers: { Authorization: `Bearer ${TEST_TOKEN}` } });
      assert.equal(discovery.status, 200);
      assertDiscoverySecretsAbsent(await discovery.json(), fake);

      const duplicateHeaders = new Headers();
      duplicateHeaders.append("Authorization", `Bearer ${TEST_TOKEN}`);
      duplicateHeaders.append("X-Access-Token", TEST_TOKEN);
      assert.equal((await fetch(`${gateway.baseURL}/config`, { headers: duplicateHeaders })).status, 401);
      const serviceToken = await gateway.credential({ kind: "service", service: INTERNAL_SERVICE });
      assert.equal((await fetch(`${gateway.baseURL}/config`, { headers: { Authorization: `Bearer ${serviceToken}` } })).status, 401);
      assert.equal((await fetch(`${gateway.baseURL}/config`, { headers: { "X-Access-Token": serviceToken } })).status, 403);
      assert.equal((await fetch(`${gateway.baseURL}/config`)).status, 401);
      assert.equal(fake.requests.length, 0);

      fake.redirectTo = redirectTarget.url;
      let redirectError = "";
      try {
        await gateway.client()("assistant").doGenerate({
          prompt: [{ role: "user", content: [{ type: "text", text: "redirect" }] }],
          maxOutputTokens: 32,
        });
      } catch (error) {
        redirectError = String(error);
      }
      assert.notEqual(redirectError, "");
      assert.equal(redirectTarget.requests.length, 0);
      assert.deepEqual(fake.violations, []);

      const metrics = await gateway.metrics();
      assert.equal(await gateway.ready(), true);
      await gateway.stop();
      const logs = gateway.stderr;
      assert.deepEqual(processLifecycleEvents(logs), ["plugin_started", "plugin_stopped"]);
      for (const privateValue of [
        "integration-anthropic-key",
        "backend-private",
        fake.url,
        TEST_TOKEN,
        "authorization-is-ignored",
      ]) {
        assert.ok(!JSON.stringify(withoutConfiguredExecution(redirectError)).includes(privateValue));
        assert.ok(!metrics.includes(privateValue));
        assert.ok(!logs.includes(privateValue));
      }
    } finally {
      await settleCleanup(
        ...(resources == null ? [] : [() => resources![1].stop(), () => resources![0].stop()]),
        () => redirectTarget.stop(),
      );
    }
  });

  it("fails malformed scalar startup before readiness with a secret-safe reason", async () => {
    const fake = await FakeAnthropic.start();
    try {
      let failure = "";
      try {
        await GatewayProcess.start(binaryPath, fake.url, ["--max-model-duration=20s"]);
      } catch (error) {
        failure = String(error);
      }
      assert.ok(failure.includes("gateway exited unsuccessfully"));
      assert.ok(failure.includes("gateway plugin failed to start"));
      assert.ok(failure.includes("max model duration must be at least"));
      assert.deepEqual(processLifecycleEvents(failure), []);
      assert.notEqual(GatewayProcess.lastFailedDirectory, undefined);
      assert.equal(existsSync(GatewayProcess.lastFailedDirectory!), false);
      for (const privateValue of ["integration-anthropic-key", "backend-private", fake.url, TEST_TOKEN]) {
        assert.ok(!failure.includes(privateValue));
      }
    } finally {
      await settleCleanup(() => fake.stop());
    }
  });

  it("uses only the built command path for service composition", () => {
    const source = readFileSync(import.meta.filename, "utf8");
    const imports = source.split("\n").filter((line) => line.startsWith("import ")).join("\n");
    assert.ok(source.includes('spawn(hostBinaryPath, ["--plugin", binary'));
    assert.doesNotMatch(imports, /cmd\/grafana-ai-gateway\/internal/);
    assert.doesNotMatch(imports, /providerwire|gateway\/catalog/);
  });

});

describe("Cloud BYOK through the platform host (test-signed identities, dummy native trust)", () => {
  it("rejects the native proxy certificate without explicit test trust", async () => {
    const [native, gateway] = await startCloudGateway([], undefined, { GATEWAY_TEST_CA_FILE: "" });
    try {
      await assert.rejects(async () => gateway.client()(BYOK_MODEL).doGenerate(cloudCall("unary")), (error: unknown) => {
        assert.equal((error as { statusCode?: number }).statusCode, 502);
        return true;
      });
      assert.equal(native.requests.length, 0);
    } finally { await settleCleanup(() => gateway.stop(), () => native.stop()); }
  });

  it("rejects invalid explicit test trust without affecting the production binary", async () => {
    const certificate = join(buildDirectory, "invalid-test-ca.pem");
    writeFileSync(certificate, "not a certificate");
    await assert.rejects(startCloudGateway([], undefined, { GATEWAY_TEST_CA_FILE: certificate }), /gateway test: invalid certificate file/);
    const [fake, gateway] = await startGateway([], { GATEWAY_TEST_CA_FILE: certificate, GODEBUG: "x509usefallbackroots=1" });
    try {
      assert.equal(await gateway.ready(), true);
      assert.ok(!gateway.stderr.includes("gateway test: invalid certificate file"));
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });
  for (const collision of [false, true]) {
    it(`preserves Cloud BYOK echoes and protects configured summary keys, collision=${collision}`, async () => {
      const observer = await FakeAgentObservability.start();
      const configuredKey = collision ? "request-only-key" : "integration-anthropic-key";
      let resources: Awaited<ReturnType<typeof startCloudGateway>> | undefined;
      try {
        resources = await startCloudGateway([
          "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
          "--no-agento11y.tls",
        ], undefined, { GATEWAY_TEST_ANTHROPIC_KEY: configuredKey, GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" });
        const [native, gateway] = resources;
        native.streamErrorMessage = `provider-echo request-only-key ${configuredKey} integration-agento11y-key static-gateway-key backend-private`;
        const options = { ...cloudCall("normal-stream"), providerOptions: { gateway: { byok: { anthropic: [{ apiKey: "request-only-key" }, { apiKey: "unused-second-key" }] } } } };
        const expected = { message: `provider-echo ${collision ? "[REDACTED]" : "request-only-key"} [REDACTED] [REDACTED] static-gateway-key backend-private`, type: "api_error", statusCode: 500 };
        const ts = await collectStream((await gateway.client()(BYOK_MODEL).doStream(options)).stream);
        const go = await captureGoClient(goClientBinaryPath, { mode: "stream", modelID: BYOK_MODEL, options, baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN });
        assert.equal(go.error, undefined);
        for (const parts of [ts, go.parts]) {
          const errors = parts.filter((part: { type: string; error?: { data?: { nativeError?: unknown } } }) => part.type === "error" && part.error?.data?.nativeError !== undefined);
          assert.equal(errors.length, 1);
          assert.deepEqual(errors[0].error.data, { nativeError: expected });
          const text = parts.filter((part: { type: string }) => part.type === "text-delta").map((part: { delta: string }) => part.delta).join("");
          assert.equal(text, native.streamErrorMessage);
          assert.equal(errors[0].error.statusCode, 502);
          assert.equal(errors[0].error.retryable ?? errors[0].error.isRetryable, true);
          assert.ok(parts.every((part: { type: string }) => part.type !== "finish"));
          assert.equal(parts.at(-1).type, "error");
          assert.ok(!JSON.stringify(parts).includes('"execution"'));
          assert.ok(!JSON.stringify(parts).includes('"providerInstance"'));
        }
        assert.equal(native.requests.length, 2);
        assert.ok(native.requests.every(request => request.apiKey === "request-only-key"));
        const metrics = await gateway.metrics();
        await gateway.stop();
        assertCloudPrivateValuesAbsent(gateway.stderr + metrics, ["request-only-key", "unused-second-key", "provider-echo", "static-gateway-key", "integration-agento11y-key"]);
        assert.deepEqual(native.violations, []);
      } finally {
        await settleCleanup(...(resources ? [() => resources![1].stop(), () => resources![0].stop()] : []), () => observer.stop());
      }
    });
  }
  it("distinguishes native credential rejection from Gateway authentication without rotating keys or echoing them", async () => {
    const [native, gateway] = await startCloudGateway();
    native.failureStatus = 401;
    try {
      const options = { ...cloudCall("unary"), providerOptions: { gateway: { byok: { anthropic: [{ apiKey: "request-only-key" }, { apiKey: "unused-second-key" }] } } } };
      for (const mode of ["generate", "stream"] as const) {
        const go = await captureGoClient(goClientBinaryPath, {
          mode, modelID: BYOK_MODEL, options,
          baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN,
        });
        assert.equal(go.error?.statusCode, 424);
        assert.equal(go.error?.category, "failed_dependency");
        const expected = { error: { message: "failed dependency", type: "failed_dependency", param: null, code: "failed_dependency" } };
        assert.deepEqual(go.error.apiError.data, expected);
        assertCloudPrivateValuesAbsent(JSON.stringify(go.error), ["request-only-key", "unused-second-key"]);
        const model = gateway.client()(BYOK_MODEL);
        await assert.rejects(async () => mode === "generate" ? model.doGenerate(options) : model.doStream(options), (error: unknown) => {
          const failure = error as { statusCode?: number; message: string; cause?: { data?: unknown } };
          assert.equal(failure.statusCode, 424);
          assert.deepEqual(failure.cause?.data, expected);
          assertCloudPrivateValuesAbsent(JSON.stringify(failure.cause?.data), ["request-only-key", "unused-second-key"]);
          return true;
        });
      }
      assert.equal(native.requests.length, 4);
      assert.ok(native.requests.every(request => request.apiKey === "request-only-key"));
      const metrics = await gateway.metrics();
      assert.match(metrics, /grafana_ai_gateway_access_decisions_total\{decision="byok"\} 4$/m);
      assert.doesNotMatch(metrics, /decision="refused"/);
      await gateway.stop();
      assertCloudPrivateValuesAbsent(gateway.stderr + metrics, ["request-only-key", "unused-second-key"]);
    } finally { await settleCleanup(() => gateway.stop(), () => native.stop()); }
  });

  it("executes request-only OpenAI Responses with both clients and no configured account", async () => {
    const configured = await FakeAnthropic.start();
    const native = await FakeOpenAI.start();
    native.apiKey = "request-only-openai";
    native.backendModel = "request-only-native@revision+" + "x".repeat(140);
    const proxy = await startNativeProviderProxy(native.url, "api.openai.com:443");
    let gateway: GatewayProcess | undefined;
    try {
      gateway = await GatewayProcess.start(nativeProxyBinaryPath, configured.url, [], {
        ...nativeProxyEnvironment(proxy.url),
        OPENAI_API_KEY: "ambient-private-key", OPENAI_BASE_URL: configured.url,
        OPENAI_ORG_ID: "ambient-private-org", OPENAI_PROJECT_ID: "ambient-private-project",
      }, "cloud-gateway");
      gateway.nativeProxy = proxy;
      const modelID = `openai/${native.backendModel}`;
      const options: LanguageModelV4CallOptions = {
        prompt: [
          { role: "user", content: [{ type: "text", text: "previous question" }] },
          { role: "assistant", content: [{ type: "text", text: "previous answer" }] },
          { role: "user", content: [{ type: "text", text: "continue" }] },
        ],
        maxOutputTokens: 32,
        providerOptions: {
          gateway: { byok: { openai: [{ apiKey: "request-only-openai" }, { apiKey: "unused-openai-second" }], anthropic: [{ apiKey: "unused-private-key" }] } },
          openai: { store: false },
        },
      };
      for (const mode of ["generate", "stream"] as const) {
        const go = await captureGoClient(goClientBinaryPath, {
          mode, modelID, options,
          baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN,
        });
        assert.equal(go.error, undefined);
        const model = gateway.client()(modelID);
        if (mode === "generate") {
          const ts = await model.doGenerate(options);
          assert.deepEqual(go.result.content, ts.content);
        } else {
          const ts = await collectStream((await model.doStream(options)).stream);
          assert.ok(ts.some(part => part.type === "finish"));
          assert.equal(go.parts.filter((part: { type: string }) => part.type === "text-delta").map((part: { delta: string }) => part.delta).join(""),
            ts.filter(part => part.type === "text-delta").map(part => part.delta).join(""));
        }
      }
      const reasoningOptions: LanguageModelV4CallOptions = { ...options, prompt: [...options.prompt.slice(0, 2), { role: "user", content: [{ type: "text", text: "wp17-reasoning" }] }] };
      const goReasoning = await captureGoClient(goClientBinaryPath, {
        mode: "stream", modelID, options: reasoningOptions,
        baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN,
      });
      assert.equal(goReasoning.error, undefined);
      const tsReasoning = await collectStream((await gateway.client()(modelID).doStream(reasoningOptions)).stream);
      assert.equal(goReasoning.parts.filter((part: { type: string }) => part.type === "reasoning-delta").map((part: { delta: string }) => part.delta).join(""), "private thought");
      assert.equal(tsReasoning.filter(part => part.type === "reasoning-delta").map(part => part.delta).join(""), "private thought");
      assert.equal(configured.requests.length, 0);
      assert.equal(native.requests.length, 6);
      for (const request of native.requests) {
        assert.equal(request.body.store, false);
        assert.equal(request.headers["openai-organization"], undefined);
        assert.equal(request.headers["openai-project"], undefined);
        assert.ok(JSON.stringify(request.body).includes("previous answer"));
        assert.ok(!JSON.stringify(request.body).includes("byok"));
      }
      assert.deepEqual(native.violations, []);
      await gateway.stop();
      assertCloudPrivateValuesAbsent(gateway.stderr, ["request-only-openai", "unused-private-key", "ambient-private-key", "ambient-private-org", "ambient-private-project"]);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : [() => proxy.stop()]), () => native.stop(), () => configured.stop());
    }
  });

  it("isolates simultaneous configured and BYOK accounts and cancels both populations on shutdown", async () => {
    const configured = await FakeAnthropic.start();
    const [native, gateway] = await startCloudGateway([], configured.url);
    try {
      const internalToken = await gateway.credential({ kind: "service", service: INTERNAL_SERVICE });
      const privateClient = gateway.accessTokenClient(internalToken);
      const cloudClient = gateway.client();
      const [privateResult, cloudResult] = await Promise.all([
        privateClient("assistant").doGenerate({ prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }], maxOutputTokens: 32 }),
        cloudClient(BYOK_MODEL).doGenerate(cloudCall("unary")),
      ]);
      assert.deepEqual(privateResult.content, cloudResult.content);
      assert.equal(configured.requests.length, 1);
      assert.equal(native.requests.length, 1);
      assert.equal(configured.requests[0]!.apiKey, "integration-anthropic-key");
      assert.equal(native.requests[0]!.apiKey, "request-only-key");
      assert.equal(configured.requests[0]!.body.model, "backend-private");
      assert.equal(native.requests[0]!.body.model, "request-only-native");
      assert.deepEqual((await privateClient.getAvailableModels()).models.map(row => row.id), ["grafana/assistant"]);
      await assert.rejects(gateway.client().getAvailableModels(), GatewayInvalidRequestError.isInstance);
      await assert.rejects(async () => privateClient("assistant").doGenerate(cloudCall("unary")), GatewayInvalidRequestError.isInstance);
      for (const id of ["assistant", "grafana/assistant", "unsupported/model"]) {
        await assert.rejects(async () => cloudClient(id).doGenerate(cloudCall("unary")), GatewayInvalidRequestError.isInstance);
      }
      await assert.rejects(async () => cloudClient(BYOK_MODEL).doGenerate({ prompt: [] }), GatewayInvalidRequestError.isInstance);
      const anonymous = await rawHTTPRequest(`${gateway.baseURL}/config`, "GET", [["X-Scope-OrgID", "27038"]]);
      const accessTokenAsBearer = await rawHTTPRequest(`${gateway.baseURL}/config`, "GET", [["Authorization", `Bearer ${internalToken}`]]);
      assert.equal(anonymous.status, 401);
      assert.equal(accessTokenAsBearer.status, 401);
      assert.equal(configured.requests.length, 1);
      assert.equal(native.requests.length, 1);
      const metrics = await gateway.metrics();
      assert.match(metrics, /decision="configured"/);
      assert.match(metrics, /decision="byok"/);
      assert.match(metrics, /model="byok"/);
      assert.ok(!metrics.includes(BYOK_MODEL));
      const streams = await Promise.all([
        privateClient("assistant").doStream({ prompt: [{ role: "user", content: [{ type: "text", text: "silent-shutdown" }] }], maxOutputTokens: 32 }),
        cloudClient(BYOK_MODEL).doStream(cloudCall("silent-shutdown")),
      ]);
      const readers = streams.map(result => result.stream.getReader());
      for (const reader of readers) assert.equal((await withTimeout(reader.read(), 2_000, "first native part")).value?.type, "stream-start");
      const stopping = gateway.stop();
      await Promise.all([configured.waitForCancellation("silent-shutdown"), native.waitForCancellation("silent-shutdown"), stopping]);
      await Promise.all(readers.map(reader => reader.cancel().catch(() => {})));
      assertCloudPrivateValuesAbsent(gateway.stderr + metrics, ["integration-anthropic-key", "request-only-key", internalToken]);
      assert.ok(gateway.stderr.includes(`"ai_sdk.model":"${BYOK_MODEL}"`));
      assert.deepEqual(configured.violations, []);
      assert.deepEqual(native.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => native.stop(), () => configured.stop());
    }
  });

  for (const { client, mode } of [
    { client: "go", mode: "stream" },
    { client: "typescript", mode: "generate" },
    { client: "typescript", mode: "stream" },
  ] as const) {
    for (const maxOutputTokens of [undefined, 32]) {
      it(`preserves ${client} high-level ${mode} calls through the platform host with ${maxOutputTokens === undefined ? "default" : "explicit"} token limits`, async () => {
        const [fake, gateway] = await startCloudGateway();
        const recorder = await RecordingProxy.start(gateway.url);
        try {
          const prompt = "normal-stream";
          let text: string;
          if (client === "go") {
            const result = await captureGoClient(goStreamTextBinaryPath, {
              baseURL: recorder.baseURL(gateway), serviceAccountToken: TEST_TOKEN,
              mode: "stream-text", modelID: BYOK_MODEL,
              options: { prompt: [{ role: "user", content: [{ type: "text", text: prompt }] }], maxOutputTokens, providerOptions: BYOK_OPTIONS },
            });
            assert.equal(result.error, undefined);
            text = result.text;
          } else {
            const options = { model: createGateway({ apiKey: TEST_TOKEN, baseURL: recorder.baseURL(gateway) })(BYOK_MODEL), prompt, maxOutputTokens, maxRetries: 0, providerOptions: BYOK_OPTIONS };
            if (mode === "generate") {
              const result = await generateText(options);
              assert.equal(result.finishReason, "stop");
              text = result.text;
            } else {
              const result = streamText(options);
              text = await result.text;
              assert.equal(await result.finishReason, "stop");
            }
          }
          assert.equal(recorder.received.length, 1);
          const request = recorder.received[0]!;
          const body = JSON.parse(request.body);
          assert.deepEqual(body.toolChoice, { type: "auto" });
          assert.deepEqual(body.prompt, [{ role: "user", content: [{ type: "text", text: prompt }] }]);
          assert.equal(body.maxOutputTokens, maxOutputTokens);
          assert.equal(Object.hasOwn(body, "maxOutputTokens"), maxOutputTokens !== undefined);
          assert.ok(body.tools === undefined || body.tools.length === 0);
          assert.equal(request.headers["ai-language-model-streaming"], String(mode === "stream"));
          assert.equal(request.headers.authorization, `Bearer ${TEST_TOKEN}`);
          assert.equal(fake.requests.length, 1);
          assert.equal(text, mode === "stream" ? "hello from fake Anthropic stream" : "hello from fake Anthropic");
          assert.equal(fake.requests[0]!.body.tool_choice, undefined);
          assert.equal(fake.requests[0]!.body.max_tokens, maxOutputTokens ?? 4096);
          if (client === "typescript" && mode === "generate") {
            assert.equal(body.headers["user-agent"].split(/\s/, 1)[0], `ai/${packageManifest.dependencies.ai}`);
            assert.equal(singleHeader(fake.requests[0]!.headers["user-agent"]), body.headers["user-agent"]);
          }
          assert.equal(fake.requests[0]!.apiKey, "request-only-key");
          assert.deepEqual(fake.violations, []);
          const metrics = await gateway.metrics();
          await gateway.stop();
          assertCloudPrivateValuesAbsent(JSON.stringify([text, metrics, gateway.stderr, fake.requests]), [
            ...CLOUD_PRIVATE_VALUES,
          ]);
        } finally {
          await settleCleanup(() => recorder.stop(), () => gateway.stop(), () => fake.stop());
        }
      });
    }
  }
});

describe("platform caller access in cloud mode", () => {
  it("serves configured models to internal Go callers directly and on behalf of users", async () => {
    const fake = await FakeAnthropic.start();
    let gateway: GatewayProcess | undefined;
    try {
      gateway = await GatewayProcess.start(binaryPath, fake.url, [], {}, "cloud-gateway");
      const cap = await gateway.credential({ kind: "access-policy", service: INTERNAL_SERVICE });
      const tokenExchange = { CAPToken: cap, Namespace: TEST_NAMESPACE, Audience: "grafana-aigateway-app", BaseURL: gateway.baseURL, TokenExchangeURL: `${gateway.url}/platformhost/exchange` };
      const userIDToken = await gateway.credential({ kind: "id-token", identityType: "user", uid: "private-user-uid" });
      const delegated = await gateway.credential({ kind: "delegated", service: "other-app", identityType: "user", uid: "private-user-uid" });
      for (const onBehalfOf of [undefined, userIDToken, delegated]) {
        const base = { tokenExchange, ...(onBehalfOf ? { onBehalfOf } : {}), modelID: "assistant" };
        const discovery = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery" });
        assert.equal(discovery.error, undefined);
        assert.deepEqual(discovery.models.map((model: { id: string }) => model.id), ["grafana/assistant"]);
        const unary = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", options: { prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }], maxOutputTokens: 32 } });
        assert.equal(unary.error, undefined);
        assert.deepEqual(unary.result.content, [{ type: "text", text: "hello from fake Anthropic" }]);
        const stream = await captureGoClient(goClientBinaryPath, { ...base, mode: "stream", options: { prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }], maxOutputTokens: 32 } });
        assert.equal(stream.error, undefined);
        assert.equal(stream.parts.at(-1).type, "finish");
        assertConsumerSecretsAbsent([discovery, unary, stream], [cap, userIDToken, delegated]);
      }
      assert.equal(fake.requests.length, 6);
      assert.ok(fake.requests.every((request) => request.apiKey === "integration-anthropic-key"));
      for (const secret of [cap, userIDToken, delegated, "private-user-uid"]) assert.ok(!JSON.stringify(fake.requests).includes(secret));
      const metrics = await gateway.metrics();
      assert.match(metrics, /grafana_ai_gateway_access_decisions_total\{decision="configured"\} 9$/m);
      assert.doesNotMatch(metrics, /decision="(?:byok|refused)"/);
      await gateway.stop();
      assert.ok(gateway.stderr.includes(`"caller_service":"${INTERNAL_SERVICE}"`));
      assertPrivateValuesAbsent([metrics, gateway.stderr], fake, [cap, userIDToken, delegated, "private-user-uid", "other-app"]);
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => fake.stop());
    }
  });

  it("refuses relayed, anonymous and unauthenticated callers before any work", async () => {
    const fake = await FakeAnthropic.start();
    let gateway: GatewayProcess | undefined;
    try {
      gateway = await GatewayProcess.start(binaryPath, fake.url, [], {}, "cloud-gateway");
      const internal = await gateway.credential({ kind: "service", service: INTERNAL_SERVICE });
      const relayed = await gateway.credential({ kind: "relay", service: "relay-service", subject: internal });
      const anonymous = await gateway.credential({ kind: "session", identityType: "anonymous", uid: "viewer" });
      const unlisted = await gateway.credential({ kind: "access-policy", service: "relay-service" });
      const permission = '{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}';
      const callers: Array<Record<string, string>> = [{ "X-Access-Token": relayed }, { Cookie: `grafana_session=${anonymous}` }];
      for (const headers of callers) {
        const discovery: Response = await fetch(`${gateway.baseURL}/config`, { headers });
        assert.equal(discovery.status, 403);
        assert.equal(await discovery.text(), permission);
        const inference = await rawHTTPRequest(`${gateway.baseURL}/language-model`, "POST", [...Object.entries(headers), ["Content-Type", "application/json"], ["ai-language-model-specification-version", "4"], ["ai-language-model-id", "assistant"], ["ai-language-model-streaming", "false"]], JSON.stringify({ prompt: [] }));
        assert.equal(inference.status, 403);
      }
      await assert.rejects(gateway.accessTokenClient(relayed).getAvailableModels());
      const go = await captureGoClient(goClientBinaryPath, { mode: "discovery", tokenExchange: { CAPToken: unlisted, Namespace: TEST_NAMESPACE, Audience: "grafana-aigateway-app", BaseURL: gateway.baseURL, TokenExchangeURL: `${gateway.url}/platformhost/exchange` } });
      assert.equal(go.error?.statusCode, 403);
      assert.equal((await fetch(`${gateway.baseURL}/config`)).status, 401);
      assert.equal(fake.requests.length, 0);
      const metrics = await gateway.metrics();
      assert.match(metrics, /grafana_ai_gateway_access_decisions_total\{decision="refused"\} 6$/m);
      assert.doesNotMatch(metrics, /decision="(?:configured|byok)"/);
      await gateway.stop();
      assertPrivateValuesAbsent([metrics, gateway.stderr], fake, [internal, relayed, anonymous, unlisted, "relay-service", "viewer"]);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => fake.stop());
    }
  });
});

describe("authenticated OpenAI-compatible Gateway command", () => {
  it("forwards native provider options and refuses host and credential values", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const client = gateway.client();
      await client("compatible").doGenerate({
        prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }],
        maxOutputTokens: 32,
        providerOptions: {
          openaiCompatible: { reasoningEffort: "low", user: "caller-supplied-user" },
        },
        headers: { "X-Contract-Body": "carried" },
      });

      const forwarded = fake.requests.at(-1);
      assert.ok(forwarded, "the backend received the request");
      assert.equal(forwarded.body.reasoning_effort, "low");
      assert.equal(forwarded.body.user, "caller-supplied-user");
      assert.equal(singleHeader(forwarded.headers["x-contract-body"]), "carried");
      assert.equal(singleHeader(forwarded.headers.authorization), "Bearer integration-compatible-key",
        "the gateway's backend credential is the one presented upstream");

      for (const [rejected, message] of [
        [{ providerOptions: { grafana: { tenant: "other" } } }, "reserved provider option namespace"],
        [{ headers: { "x-api-key": "caller-controlled" } }, "protected call header"],
        [{ providerOptions: { compatibleBackend: { model: "someone-elses-model" } } }, "invalid request"],
      ] as const) {
        await assert.rejects(
          async () => await client("compatible").doGenerate({
            prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }],
            maxOutputTokens: 32,
            ...rejected,
          }),
          (error: unknown) => GatewayInvalidRequestError.isInstance(error) && error.message === message,
          `refused as ${message}, not by an unrelated failure`,
        );
      }
      const refusedBodies = JSON.stringify(fake.requests.map((request: { body: unknown }) => request.body));
      for (const refused of ["caller-controlled", "tenant", "someone-elses-model"]) {
        assert.ok(!refusedBodies.includes(refused), "a refused value never reaches the backend");
      }
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("refuses provider options that restructure a validated message, in both modes", async () => {
    // A namespace field that reaches the provider's message or part metadata is
    // spread over the entry it builds, so it can reinstate content the runtime
    // refused to map. Every one of these must be refused before the call.
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const client = gateway.client();
      const structural: Record<string, JSONValue> = {
        content: [{ type: "image_url", image_url: { url: "https://caller.example/x.png" } }],
        role: "tool",
        type: "image_url",
        image_url: { url: "https://caller.example/x.png" },
        tool_calls: [{ id: "call_1", type: "function", function: { name: "f", arguments: "{}" } }],
      };
      for (const [field, value] of Object.entries(structural)) {
        for (const mode of ["generate", "stream"] as const) {
          const options = {
            prompt: [{
              role: "user" as const,
              content: [
                { type: "text" as const, text: "unary", ...(field === "type" || field === "image_url" ? { providerOptions: { openaiCompatible: { [field]: value } } } : {}) },
                { type: "text" as const, text: "tail" },
              ],
              ...(field === "type" || field === "image_url" ? {} : { providerOptions: { openaiCompatible: { [field]: value } } }),
            }],
            maxOutputTokens: 32,
          };
          await assert.rejects(
            async () => mode === "generate"
              ? await client("compatible").doGenerate(options)
              : await collectGatewayStream((await client("compatible").doStream(options)).stream),
            (error: unknown) => GatewayInvalidRequestError.isInstance(error) && error.message === "invalid request",
            `${field} in ${mode} mode is refused at its consuming scope`,
          );
        }
      }
      assert.equal(fake.requests.length, 0, "a refused request never reaches the backend");
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("forwards nested provider options and call headers to the backend, in both modes", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const client = gateway.client();
      for (const mode of ["generate", "stream"] as const) {
        const options = {
          prompt: [
            {
              role: "system" as const,
              content: "be brief",
              providerOptions: { openaiCompatible: { system_marker: `system-${mode}` } },
            },
            {
              // Two parts, because upstream collapses a lone text part into the
              // message and uses the part's metadata for it.
              role: "user" as const,
              content: [
                { type: "text" as const, text: mode === "stream" ? "normal-stream" : "unary", providerOptions: { openaiCompatible: { part_marker: `part-${mode}` } } },
                { type: "text" as const, text: "tail" },
              ],
              providerOptions: { openaiCompatible: { message_marker: `message-${mode}` } },
            },
          ],
          maxOutputTokens: 32,
          providerOptions: { openaiCompatible: { user: `root-${mode}` } },
          headers: { "X-Contract-Body": `header-${mode}` },
        };
        if (mode === "generate") {
          await client("compatible").doGenerate(options);
        } else {
          await collectGatewayStream((await client("compatible").doStream(options)).stream);
        }

        const forwarded = fake.requests.at(-1);
        assert.ok(forwarded, `the backend received the ${mode} request`);
        const body = forwarded.body as { user?: string; stream?: boolean; messages: Array<Record<string, unknown>> };
        assert.equal(body.user, `root-${mode}`, "root options reach the backend");
        assert.equal(singleHeader(forwarded.headers["x-contract-body"]), `header-${mode}`, "call headers reach the backend");
        assert.equal(body.stream ?? false, mode === "stream", "the mode the caller asked for is the mode sent upstream");
        const [system, user] = body.messages;
        assert.equal(system!.system_marker, `system-${mode}`, "system-message options reach the backend");
        assert.equal(user!.message_marker, `message-${mode}`, "message options reach the backend");
        const parts = user!.content as Array<Record<string, unknown>>;
        assert.equal(parts[0]!.part_marker, `part-${mode}`, "text-part options reach the backend");
      }
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("returns reasoning-only paid unary once and replays high-level history in both clients", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const model = gateway.client()("compatible");
      const unary = await generateText({model,prompt:"wp17-reasoning"});
      assert.equal(fake.requests.length,1);
      assert.equal(unary.reasoningText,"private thought");
      const first=streamText({model,prompt:"wp17-reasoning",maxOutputTokens:64});
      const response=await first.response;
      assert.equal(await first.reasoningText,"private thought");
      const second=streamText({model,messages:[{role:"user",content:"wp17-reasoning"},...response.messages],maxOutputTokens:64});
      await second.consumeStream();
      assert.equal(fake.requests.length,3);
      assert.ok(JSON.stringify(fake.requests[2].body.messages).includes("private thought"));
      const go=await captureGoClient(goStreamTextBinaryPath,{baseURL:gateway.baseURL,serviceAccountToken:TEST_TOKEN,modelID:"compatible",mode:"reasoning-replay",options:{prompt:[{role:"user",content:[{type:"text",text:"wp17-reasoning"}]}],maxOutputTokens:64}});
      assert.equal(go.error,undefined);
      assert.equal(fake.requests.length,5);
      assert.deepEqual(fake.requests[4].body.messages,fake.requests[2].body.messages);
      assert.deepEqual(fake.violations,[]);
    } finally { await gateway.stop(); await fake.stop(); }
  });

  it("ignores foreign MCP settings without changing compatible inference or credentials", async () => {
    const observer = await FakeAgentObservability.start();
    let resources: [FakeCompatible, GatewayProcess] | undefined;
    let mcpInvocations = 0;
    const mcp = createServer((request, response) => {
      mcpInvocations++;
      request.resume();
      response.writeHead(503);
      response.end();
    });
    try {
      await new Promise<void>(resolve => mcp.listen(0, "127.0.0.1", resolve));
      const address = mcp.address();
      assert.ok(address != null && typeof address !== "string");
      const mcpURL = `http://127.0.0.1:${address.port}/mcp`;
      const mcpToken = "compatible-foreign-mcp-token";
      resources = await startCompatibleGateway([
        "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls",
      ], { GF_PLUGIN_AGENTO11Y_TOKEN: "integration-agento11y-key" });
      const [fake, gateway] = resources;
      for (const mode of ["generate", "stream"] as const) {
        for (const implementation of ["go", "vercel"] as const) {
          const options: LanguageModelV4CallOptions = {
            prompt: [{ role: "user", content: [{ type: "text", text: mode === "stream" ? "normal-stream" : "unary" }] }],
            maxOutputTokens: 32,
            providerOptions: { anthropic: { mcpServers: [{ type: "url", name: "foreign-mcp", url: mcpURL, authorizationToken: mcpToken }] } },
          };
          const before = fake.requests.length;
          let publicResponse: unknown;
          if (implementation === "go") {
            const go = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "compatible", mode, options });
            assert.equal(go.error, undefined);
            if (mode === "generate") {
              assert.deepEqual(go.result.content, [{ type: "text", text: "hello from fake compatible" }]);
              publicResponse = { content: go.result.content, finishReason: go.result.finishReason, usage: go.result.usage, warnings: go.result.warnings, providerMetadata: go.result.providerMetadata, response: go.result.response };
            } else {
              assert.equal(go.canceled, false);
              assertNativeOptionStream(go.parts);
              assert.equal(go.parts.filter((part: any) => part.type === "text-delta").map((part: any) => part.delta).join(""), "hello from fake compatible stream");
              publicResponse = go.parts;
            }
          } else if (mode === "generate") {
            const result = await gateway.client()("compatible").doGenerate(options);
            assert.deepEqual(result.content, [{ type: "text", text: "hello from fake compatible" }]);
            publicResponse = { content: result.content, finishReason: result.finishReason, usage: result.usage, warnings: result.warnings, providerMetadata: result.providerMetadata, response: result.response };
          } else {
            const parts = await collectGatewayStream((await gateway.client()("compatible").doStream(options)).stream);
            assertNativeOptionStream(parts);
            assert.equal(parts.filter(part => part.type === "text-delta").map(part => part.delta).join(""), "hello from fake compatible stream");
            publicResponse = parts;
          }
          assert.equal(fake.requests.length, before + 1, `${implementation} ${mode} invokes compatible inference once`);
          const native = fake.requests.at(-1)!;
          assert.equal(native.body.model, "backend-private");
          assert.equal(native.body.stream ?? false, mode === "stream");
          assert.equal(singleHeader(native.headers.authorization), "Bearer integration-compatible-key");
          for (const value of [mcpURL, mcpToken, "foreign-mcp", "mcpServers", "mcp_servers", "authorizationToken"]) {
            assert.ok(!JSON.stringify({ native, publicResponse }).includes(value));
          }
          assert.equal(mcpInvocations, 0);
        }
      }
      await observer.waitForGenerations(4);
      const metrics = await gateway.metrics();
      await gateway.stop();
      const captured = JSON.stringify({ metrics, logs: gateway.stderr, generations: observer.generations });
      for (const value of [mcpURL, mcpToken, "foreign-mcp", "mcpServers", "mcp_servers", "authorizationToken"]) {
        assert.ok(!captured.includes(value));
      }
      assert.equal(observer.generations.length, 4);
      assert.deepEqual(observer.violations, []);
      assert.deepEqual(fake.violations, []);
      assert.equal(mcpInvocations, 0);
    } finally {
      await settleCleanup(
        ...(resources ? [() => resources![1].stop(), () => resources![0].stop()] : []),
        () => observer.stop(),
        async () => { mcp.closeAllConnections(); await new Promise<void>((resolve, reject) => mcp.close(error => error ? reject(error) : resolve())); },
      );
    }
  });
  it("discovers, invokes, and streams usage without exposing private configuration", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const rawDiscoveryResponse = await fetch(`${gateway.baseURL}/config`, {
        headers: { Authorization: `Bearer ${TEST_TOKEN}` },
      });
      assert.equal(rawDiscoveryResponse.status, 200);
      const rawDiscovery = await rawDiscoveryResponse.text();
      for (const privateValue of ["GATEWAY_TEST_COMPATIBLE_KEY", "integration-compatible-key", fake.url]) {
        assert.ok(!rawDiscovery.includes(privateValue));
      }
      assert.deepEqual(JSON.parse(rawDiscovery).models[0].gateway, { aliases: ["compatible"], primary: { providerInstance: "compatible-primary", provider: "compatible-backend", providerModelId: "backend-private" }, fallbacks: [] });
      const client = gateway.client();
      const metadata = await client.getAvailableModels();
      assert.deepEqual(metadata.models.map((model) => model.id), ["grafana/compatible"]);
      const result = await client("grafana/compatible").doGenerate({
        prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }],
        maxOutputTokens: 32,
      });
      assert.deepEqual(result.content, [{ type: "text", text: "hello from fake compatible" }]);

      const stream = await client("compatible").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }],
        maxOutputTokens: 32,
      });
      const reader = stream.stream.getReader();
      const parts: Array<{ type: string; delta?: string; usage?: { outputTokens?: { total?: number } } }> = [];
      for (;;) {
        const next = await reader.read();
        if (next.done) break;
        parts.push(next.value);
      }
      assert.equal(parts[0]?.type, "stream-start");
      assert.equal(parts.filter((part) => part.type === "text-delta").map((part) => part.delta).join(""), "hello from fake compatible stream");
      assert.equal(parts.find((part) => part.type === "finish")?.usage?.outputTokens?.total, 6);
      assert.equal(await gateway.ready(), true);

      assert.equal(fake.requests.length, 2);
      for (const request of fake.requests) {
        assert.equal(request.body.max_tokens, 32);
        assert.deepEqual(request.body.messages, [{ role: "user", content: request.body.stream === true ? "normal-stream" : "unary" }]);
      }
      assert.deepEqual(fake.requests.at(-1)?.body.stream_options, { include_usage: true });
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("rejects redirects and exposes only attributed native failure summaries", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    const redirectTarget = await FakeCompatible.start();
    try {
      fake.failWithSecret = true;
      const failed = await rawProviderWireRequest(gateway.baseURL, "unary", "compatible");
      assert.ok(failed.status >= 400);
      const publicError = await failed.text();
      assert.equal(fake.requests.length, 1);

      fake.failWithSecret = false;
      fake.redirectTo = redirectTarget.url;
      const redirected = await rawProviderWireRequest(gateway.baseURL, "unary", "compatible");
      assert.ok(redirected.status >= 400);
      const redirectError = await redirected.text();
      assert.equal(redirectTarget.requests.length, 0);
      assert.deepEqual(fake.violations, []);

      const metrics = await gateway.metrics();
      assert.equal(await gateway.ready(), true);
      await gateway.stop();
      const logs = gateway.stderr;
      assertConsumerSecretsAbsent([publicError, redirectError]);
      for (const privateValue of ["provider-secret-response", "compatible-primary", "compatible-backend", "backend-private", "integration-compatible-key", fake.url, redirectTarget.url, TEST_TOKEN]) {
        assert.ok(!metrics.includes(privateValue));
        assert.ok(!logs.includes(privateValue));
      }
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop(), () => redirectTarget.stop());
    }
  });

  it("cancels an established compatible stream when the client aborts", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const result = await gateway.client()("compatible").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-abort" }] }],
        maxOutputTokens: 32,
      });
      const reader = result.stream.getReader();
      const first = await reader.read();
      assert.equal(first.done, false);
      assert.equal(first.value?.type, "stream-start");
      await reader.cancel("test abort");
      await fake.waitForCancellation("silent-abort");
      assert.equal(await gateway.ready(), true);
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });
});

describe("custom content through the authenticated command", () => {
  const compactionMetadata = { openai: { type: "compaction", itemId: "cmp_1", encryptedContent: "ENC" } };
  const fallbackMetadata = { anthropic: { type: "fallback", from: { model: "claude-a" }, to: { model: "claude-b" } } };
  const customParts = (parts: readonly any[]) => parts.filter((part) => part.type === "custom").map(({ type, kind, providerMetadata }) => ({ type, kind, providerMetadata }));
  const replay = (kind: `${string}.${string}`, metadata: Record<string, unknown>, text: string, providerOptions?: Record<string, unknown>): LanguageModelV4CallOptions => ({
    prompt: [
      { role: "user", content: [{ type: "text", text }] },
      { role: "assistant", content: [{ type: "text", text: "earlier" }, { type: "custom", kind, providerOptions: metadata as any }] },
      { role: "user", content: [{ type: "text", text: "continue" }] },
    ],
    providerOptions: providerOptions as any,
  });
  const lastBody = (fake: { requests: FakeRequest[] }) => fake.requests.at(-1)!.body as any;

  for (const route of ["direct", "fallback"] as const) {
    it(`round-trips OpenAI compaction through both clients on the ${route} route`, async () => {
      const primary = await FakeOpenAI.start();
      const secondary = route === "fallback" ? await FakeOpenAI.start() : undefined;
      primary.failWithSecret = route === "fallback";
      const serving = secondary ?? primary;
      let gateway: GatewayProcess | undefined;
      try {
        gateway = await GatewayProcess.start(binaryPath, "", [], {}, "access-token", openAIConfig(primary.url, secondary?.url));
        const model = gateway.client()("openai");
        const config = { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "openai" };
        const prompt = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "wp112-compaction" }] }] };

        const tsUnary = await model.doGenerate(prompt);
        const tsStream = await collectGatewayStream((await model.doStream(prompt)).stream);
        const goUnary = await captureGoClient(goClientBinaryPath, { ...config, mode: "generate", options: prompt });
        const goStream = await captureGoClient(goClientBinaryPath, { ...config, mode: "stream", options: prompt });
        assert.equal(goUnary.error, undefined);
        assert.equal(goStream.error, undefined);
        const expected = [{ type: "custom", kind: "openai.compaction", providerMetadata: compactionMetadata }];
        for (const parts of [tsUnary.content, tsStream, goUnary.result.content, goStream.parts]) assert.deepEqual(customParts(parts), expected);

        const replayed = new Map<string, unknown>();
        for (const store of [true, false]) {
          const options = replay("openai.compaction", compactionMetadata, "wp112-history", { openai: { store } });
          await model.doGenerate(options);
          const tsInput = lastBody(serving).input;
          const go = await captureGoClient(goClientBinaryPath, { ...config, mode: "generate", options });
          assert.equal(go.error, undefined);
          assert.deepEqual(lastBody(serving).input, tsInput);
          replayed.set(String(store), tsInput);
          const stream = await captureGoClient(goClientBinaryPath, { ...config, mode: "stream", options });
          assert.equal(stream.error, undefined);
          assert.deepEqual(lastBody(serving).input, tsInput);
        }
        assert.deepEqual((replayed.get("true") as any[]).filter((item) => item.type === "item_reference"), [{ type: "item_reference", id: "cmp_1" }]);
        assert.deepEqual((replayed.get("false") as any[]).filter((item) => item.type === "compaction"), [{ type: "compaction", id: "cmp_1", encrypted_content: "ENC" }]);
        assert.deepEqual(primary.violations, []);
        assert.deepEqual(secondary?.violations ?? [], []);
      } finally { await settleCleanup(() => gateway?.stop() ?? Promise.resolve(), () => primary.stop(), () => secondary?.stop() ?? Promise.resolve()); }
    });
  }

  it("skips OpenAI compaction history for OpenAI-compatible candidates, direct and after fallback", async () => {
    for (const route of ["direct", "fallback"] as const) {
      const openai = await FakeOpenAI.start();
      const compatible = await FakeCompatible.start();
      openai.failWithSecret = true;
      let gateway: GatewayProcess | undefined;
      try {
        const config = route === "direct"
          ? compatibleConfig(compatible.url)
          : `providers:\n  openai-primary:\n    type: openai\n    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY\n    baseURL: ${openai.url}/v1\n  compatible-secondary:\n    type: openai-compatible\n    apiKeyEnv: GATEWAY_TEST_COMPATIBLE_KEY\n    baseURL: ${compatible.url}/v1\n    providerName: compatible-backend\nmodels:\n  grafana/compatible:\n    name: Grafana Compatible\n    description: Integration model\n    primary:\n      provider: openai-primary\n      model: backend-private\n    fallback:\n      - provider: compatible-secondary\n        model: backend-private\n    aliases:\n      - compatible\n`;
        gateway = await GatewayProcess.start(binaryPath, "", [], {}, "access-token", config);
        const options = replay("openai.compaction", compactionMetadata, "wp112-history normal-stream");
        const goConfig = { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "compatible" };
        await gateway.client()("compatible").doGenerate(options);
        const tsMessages = lastBody(compatible).messages;
        const go = await captureGoClient(goClientBinaryPath, { ...goConfig, mode: "generate", options });
        assert.equal(go.error, undefined);
        assert.deepEqual(lastBody(compatible).messages, tsMessages);
        await collectGatewayStream((await gateway.client()("compatible").doStream(options)).stream);
        assert.deepEqual(lastBody(compatible).messages, tsMessages);
        assert.deepEqual(tsMessages.map((message: any) => message.role), ["user", "assistant", "user"]);
        assert.equal(tsMessages[1].content, "earlier");
        assert.ok(!JSON.stringify(compatible.requests).includes("ENC"), route);
        assert.deepEqual(compatible.violations, []);
      } finally { await settleCleanup(() => gateway?.stop() ?? Promise.resolve(), () => openai.stop(), () => compatible.stop()); }
    }
  });

  it("returns and replays the Anthropic fallback marker through both clients (format-conversion evidence)", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const model = gateway.client()("assistant");
      const config = { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "assistant" };
      const prompt = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "wp112-fallback-marker" }] }], maxOutputTokens: 64 };
      const tsUnary = await model.doGenerate(prompt);
      const tsStream = await collectGatewayStream((await model.doStream(prompt)).stream);
      const goUnary = await captureGoClient(goClientBinaryPath, { ...config, mode: "generate", options: prompt });
      const goStream = await captureGoClient(goClientBinaryPath, { ...config, mode: "stream", options: prompt });
      assert.equal(goUnary.error, undefined);
      assert.equal(goStream.error, undefined);
      const expected = [{ type: "custom", kind: "anthropic.fallback", providerMetadata: fallbackMetadata }];
      for (const parts of [tsUnary.content, goUnary.result.content]) assert.deepEqual(customParts(parts), expected);
      for (const parts of [tsStream, goStream.parts]) assert.deepEqual(customParts(parts), [
        { type: "custom", kind: "anthropic.message_start", providerMetadata: { anthropic: { id: "msg_fallback", model: "backend-private", usage: { input_tokens: 2, output_tokens: 0 } } } },
        ...expected,
      ]);

      const history = { ...replay("anthropic.fallback", fallbackMetadata, "history"), maxOutputTokens: 64 };
      await model.doGenerate(history);
      const tsMessages = lastBody(fake).messages;
      const go = await captureGoClient(goClientBinaryPath, { ...config, mode: "generate", options: history });
      assert.equal(go.error, undefined);
      assert.deepEqual(lastBody(fake).messages, tsMessages);
      const assistant = tsMessages[1].content;
      assert.deepEqual(assistant.map((part: any) => part.type), ["text", "fallback"]);
      assert.deepEqual(assistant[1], { type: "fallback", from: { model: "claude-a" }, to: { model: "claude-b" } });
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });
});

describe("authenticated OpenAI Responses Gateway command", () => {
  it("preserves native source/display and response identity separately from canonical operator identity", async () => {
    const [fake,gateway]=await startOpenAIGateway();
    fake.sources=true;
    try {
      const options={prompt:[{role:"user" as const,content:[{type:"text" as const,text:"sources"}]}],maxOutputTokens:32};
      const expected=[
        {type:"source",sourceType:"url",url:"https://public.example",title:"Public citation"},
        {type:"source",sourceType:"document",mediaType:"application/octet-stream",title:"native-file-sk-application",filename:"native-file-sk-application",providerMetadata:{openai:{type:"file_path",fileId:"native-file-sk-application",index:0}}},
      ];
      const nativeSources = (parts: any[]) => parts.filter(part => part.type === "source").map(({ id, text, ...part }) => {
        assert.match(id, /^aitxt-[0-9a-f]{24}$/);
        return part;
      });
      const unary=await gateway.client()("openai").doGenerate(options);
      assert.deepEqual(nativeSources(unary.content),expected);
      assert.deepEqual((unary.response?.body as any).response, { id: "resp_test", modelId: "backend-private", timestamp: "1970-01-01T00:00:01Z" });
      assert.equal(unary.response?.id, undefined); assert.equal(unary.response?.modelId, undefined);
      const streamed=await gateway.client()("openai").doStream(options);
      const parts:unknown[]=[];
      const reader=streamed.stream.getReader();
      for (;;) {const next=await reader.read();if(next.done)break;parts.push(next.value);}
      assert.deepEqual(nativeSources(parts),[expected[1],expected[0]]);
      assert.deepEqual(parts.find((part:any)=>part.type==="response-metadata"), { type: "response-metadata", id: "resp_test", modelId: "backend-private", timestamp: new Date("1970-01-01T00:00:01Z") });
      const config={baseURL:gateway.baseURL,serviceAccountToken:TEST_TOKEN,modelID:"openai",options};
      const goUnary=await captureGoClient(goClientBinaryPath,{...config,mode:"generate"});
      assert.equal(goUnary.error,undefined);
      assert.deepEqual(nativeSources(goUnary.result.content),expected);
      assert.deepEqual(goUnary.result.response.body.response, (unary.response?.body as any).response);
      assert.equal(goUnary.result.response.modelId, undefined);
      const goStream=await captureGoClient(goClientBinaryPath,{...config,mode:"stream"});
      assert.equal(goStream.error,undefined);
      assert.deepEqual(nativeSources(goStream.parts),[expected[1],expected[0]]);
      assert.deepEqual(goStream.parts.find((part:any)=>part.type==="response-metadata"), { type: "response-metadata", id: "resp_test", modelId: "backend-private", timestamp: "1970-01-01T00:00:01.000Z" });
      const metrics=await gateway.metrics();
      for (const privateValue of ["native-file-sk-application","Public citation","https://public.example", "backend-private", "resp_test"]) {
        assert.ok(!gateway.stderr.includes(privateValue));
        assert.ok(!metrics.includes(privateValue));
      }
      assert.deepEqual(fake.violations,[]);
    } finally {await settleCleanup(()=>gateway.stop(),()=>fake.stop());}
  });
  it("discovers, invokes, and streams usage without exposing private configuration", async () => {
    const [fake, gateway] = await startOpenAIGateway();
    try {
      const rawDiscoveryResponse = await fetch(`${gateway.baseURL}/config`, {
        headers: { Authorization: `Bearer ${TEST_TOKEN}` },
      });
      assert.equal(rawDiscoveryResponse.status, 200);
      const rawDiscovery = await rawDiscoveryResponse.text();
      for (const privateValue of ["GATEWAY_TEST_OPENAI_KEY", "integration-openai-key", fake.url]) {
        assert.ok(!rawDiscovery.includes(privateValue));
      }
      assert.deepEqual(JSON.parse(rawDiscovery).models[0].gateway, { aliases: ["openai"], primary: { providerInstance: "openai-primary", provider: "openai", providerModelId: "backend-private" }, fallbacks: [] });
      const client = gateway.client();
      const result = await client("grafana/openai").doGenerate({
        prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }],
        maxOutputTokens: 32,
      });
      assert.deepEqual(result.content, [{ type: "text", text: "hello from fake openai", providerMetadata: { openai: { itemId: "msg_test" } } }]);

      const stream = await client("openai").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }],
        maxOutputTokens: 32,
      });
      const reader = stream.stream.getReader();
      const parts: Array<{ type: string; delta?: string; usage?: { outputTokens?: { total?: number } } }> = [];
      for (;;) {
        const next = await reader.read();
        if (next.done) break;
        parts.push(next.value);
      }
      assert.equal(parts.filter((part) => part.type === "text-delta").map((part) => part.delta).join(""), "hello from fake openai stream");
      assert.equal(parts.find((part) => part.type === "finish")?.usage?.outputTokens?.total, 6);

      assert.equal(fake.requests.length, 2);
      for (const request of fake.requests) {
        assert.equal(request.body.max_output_tokens, 32);
      }
      assert.deepEqual(fake.violations, []);
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("exposes attributed native failures and rejects redirects", async () => {
    const [fake, gateway] = await startOpenAIGateway();
    const redirectTarget = await FakeOpenAI.start();
    try {
      fake.failWithSecret = true;
      const failed = await rawProviderWireRequest(gateway.baseURL, "unary", "openai");
      assert.ok(failed.status >= 400);
      const publicError = await failed.text();
      assert.equal(fake.requests.length, 1);

      fake.failWithSecret = false;
      fake.redirectTo = redirectTarget.url;
      const redirected = await rawProviderWireRequest(gateway.baseURL, "unary", "openai");
      assert.ok(redirected.status >= 400);
      const redirectError = await redirected.text();
      assert.equal(redirectTarget.requests.length, 0);
      assert.deepEqual(fake.violations, []);

      const metrics = await gateway.metrics();
      assert.equal(await gateway.ready(), true);
      await gateway.stop();
      const logs = gateway.stderr;
      assertConsumerSecretsAbsent([publicError, redirectError]);
      for (const privateValue of ["provider-secret-response", "openai-primary", "backend-private", "integration-openai-key", fake.url, redirectTarget.url, TEST_TOKEN]) {
        assert.ok(!metrics.includes(privateValue));
        assert.ok(!logs.includes(privateValue));
      }
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop(), () => redirectTarget.stop());
    }
  });
});

const CLOUD_PRIVATE_VALUES = [TEST_TOKEN, INTERNAL_SERVICE];
type HeaderPairs = Array<[string, string]>;
type RawResponse = { status: number; body: string; headers: IncomingMessage["headers"] };

function cloudCall(text: string) {
  return {
    prompt: [{ role: "user" as const, content: [{ type: "text" as const, text }] }],
    maxOutputTokens: 32,
    providerOptions: BYOK_OPTIONS,
    abortSignal: AbortSignal.timeout(5_000),
  };
}

async function collectStream(stream: ReadableStream<{ type: string; delta?: string }>) {
  const reader = stream.getReader();
  const parts: Array<{ type: string; delta?: string }> = [];
  try {
    for (;;) {
      const next = await reader.read();
      if (next.done) return parts;
      parts.push(next.value);
    }
  } finally {
    reader.releaseLock();
  }
}

function assertCloudPrivateValuesAbsent(output: string, values: readonly string[]): void {
  for (const value of values) assert.ok(!output.includes(value), `private value leaked: ${value}`);
}

function rawHTTPRequest(url: string, method: string, headers: HeaderPairs, body = ""): Promise<RawResponse> {
  return new Promise((resolve, reject) => {
    const request = httpRequest(url, {
      method, headers: [["Host", new URL(url).host], ["Connection", "close"], ...headers].flat(),
      signal: AbortSignal.timeout(5_000),
    }, (response) => {
      const chunks: Buffer[] = [];
      response.on("data", (chunk: Buffer) => chunks.push(chunk));
      response.on("error", reject);
      response.on("end", () => resolve({ status: response.statusCode!, headers: response.headers, body: Buffer.concat(chunks).toString() }));
    });
    request.on("error", reject);
    request.end(body);
  });
}

async function startCloudGateway(extraArgs: string[] = [], configuredURL?: string, extraEnv: Record<string, string> = {}): Promise<[FakeAnthropic, GatewayProcess]> {
  const fake = await FakeAnthropic.start();
  fake.backendModel = "request-only-native";
  fake.apiKey = "request-only-key";
  const proxy = await startNativeProviderProxy(fake.url);
  let gateway: GatewayProcess | undefined;
  try {
    gateway = await GatewayProcess.start(nativeProxyBinaryPath, configuredURL ?? fake.url, extraArgs, { ...nativeProxyEnvironment(proxy.url), ...extraEnv }, "cloud-gateway");
    gateway.nativeProxy = proxy;
    return [fake, gateway];
  } catch (error) {
    await settleCleanup(() => fake.stop(), () => proxy.stop(), ...(gateway == null ? [] : [() => gateway!.stop()]));
    throw error;
  }
}

class RecordingProxy {
  readonly received: Array<{ path: string; headers: IncomingMessage["headers"]; body: string }> = [];

  private constructor(private readonly server: ReturnType<typeof createServer>, readonly url: string) {}

  static async start(target: string): Promise<RecordingProxy> {
    let proxy: RecordingProxy;
    const server = createServer((request, response) => {
      const captured = { path: request.url ?? "/", headers: { ...request.headers }, body: "" };
      proxy.received.push(captured);
      const upstream = httpRequest(target + captured.path, { method: request.method, headers: request.headers, signal: AbortSignal.timeout(15_000) }, (reply) => {
        response.writeHead(reply.statusCode!, reply.headers);
        response.flushHeaders();
        reply.pipe(response);
      });
      upstream.on("error", () => response.destroy());
      request.on("data", (chunk: Buffer) => { captured.body += chunk.toString(); upstream.write(chunk); });
      request.on("end", () => upstream.end());
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (address == null || typeof address === "string") throw new Error("recording proxy did not bind TCP");
    proxy = new RecordingProxy(server, `http://127.0.0.1:${address.port}`);
    return proxy;
  }

  baseURL(gateway: GatewayProcess): string {
    return this.url + gateway.baseURL.slice(gateway.url.length);
  }

  async stop(): Promise<void> {
    this.server.closeAllConnections();
    await new Promise<void>((resolve) => this.server.close(() => resolve()));
  }
}

async function startGateway(extraArgs: string[] = [], extraEnv: Record<string, string> = {}, backendModel = "backend-private"): Promise<[FakeAnthropic, GatewayProcess]> {
  const fake = await FakeAnthropic.start();
  fake.backendModel = backendModel;
  try {
    return [fake, await GatewayProcess.start(binaryPath, fake.url, extraArgs, extraEnv, "access-token", undefined, undefined, backendModel)];
  } catch (error) {
    await settleCleanup(() => fake.stop());
    throw error;
  }
}

async function startCompatibleGateway(extraArgs: string[] = [], extraEnv: Record<string, string> = {}): Promise<[FakeCompatible, GatewayProcess]> {
  const fake = await FakeCompatible.start();
  try {
    return [fake, await GatewayProcess.start(binaryPath, "", extraArgs, extraEnv, "access-token", compatibleConfig(fake.url))];
  } catch (error) {
    await settleCleanup(() => fake.stop());
    throw error;
  }
}

async function startOpenAIGateway(): Promise<[FakeOpenAI, GatewayProcess]> {
  const fake = await FakeOpenAI.start();
  try {
    return [fake, await GatewayProcess.start(binaryPath, "", [], {}, "access-token", openAIConfig(fake.url))];
  } catch (error) {
    await settleCleanup(() => fake.stop());
    throw error;
  }
}

function compatibleConfig(url: string): string {
  return `providers:\n  compatible-primary:\n    type: openai-compatible\n    apiKeyEnv: GATEWAY_TEST_COMPATIBLE_KEY\n    baseURL: ${url}/v1\n    providerName: compatible-backend\nmodels:\n  grafana/compatible:\n    name: Grafana Compatible\n    description: Integration model\n    primary:\n      provider: compatible-primary\n      model: backend-private\n    aliases:\n      - compatible\n`;
}

function openAIConfig(url: string, fallbackURL?: string): string {
  const secondary = fallbackURL ? `  openai-secondary:\n    type: openai\n    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY\n    baseURL: ${fallbackURL}/v1\n` : "";
  const fallback = fallbackURL ? `    fallback:\n      - provider: openai-secondary\n        model: backend-private\n` : "";
  return `providers:\n  openai-primary:\n    type: openai\n    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY\n    baseURL: ${url}/v1\n${secondary}models:\n  grafana/openai:\n    name: Grafana OpenAI\n    description: Integration model\n    primary:\n      provider: openai-primary\n      model: backend-private\n${fallback}    aliases:\n      - openai\n`;
}

function anthropicConfig(url: string, backendModel = "backend-private"): string {
  return `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${url}\nmodels:\n  grafana/assistant:\n    name: Grafana Assistant\n    description: Integration model\n    primary:\n      provider: anthropic-primary\n      model: ${backendModel}\n    aliases:\n      - assistant\n`;
}

function anthropicFallbackConfig(primaryURL: string, fallbackURL: string, backendModel = "backend-private"): string {
  return `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${primaryURL}\n  anthropic-secondary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${fallbackURL}\nmodels:\n  grafana/assistant:\n    name: Grafana Assistant\n    description: Integration model\n    primary:\n      provider: anthropic-primary\n      model: ${backendModel}\n    fallback:\n      - provider: anthropic-secondary\n        model: ${backendModel}\n    aliases:\n      - assistant\n`;
}

function withoutConfiguredExecution(value: unknown): unknown {
  if (typeof value === "string" && /^[\[{]/.test(value)) {
    let parsed: unknown;
    try { parsed = JSON.parse(value); } catch { return value; }
    return withoutConfiguredExecution(parsed);
  }
  if (Array.isArray(value)) return value.map(withoutConfiguredExecution);
  if (value == null || typeof value !== "object") return value;
  const result = Object.fromEntries(Object.entries(value).map(([key, item]) => [key, withoutConfiguredExecution(item)]));
  const metadata = (result.gateway ? result : result.providerMetadata) as Record<string, any> | undefined;
  if (metadata?.gateway?.execution) {
    const execution = metadata.gateway.execution;
    assert.deepEqual(Object.keys(execution).sort(), ["attempts", "canonicalModelId", "requestedModelId"]);
    for (const attempt of execution.attempts) {
      assert.ok(Object.keys(attempt).every(key => ["providerInstance", "provider", "modelId", "outcome", "error"].includes(key)));
      if (attempt.error) assert.ok(Object.keys(attempt.error).every(key => ["message", "type", "code", "statusCode"].includes(key)));
    }
    delete metadata.gateway.execution;
    if (Object.keys(metadata.gateway).length === 0) delete metadata.gateway;
    if (Object.keys(metadata).length === 0) delete result.providerMetadata;
  }
  if ((value as any).providerMetadata?.gateway?.execution && result.providerMetadata && Object.keys(result.providerMetadata as object).length === 0) {
    delete result.providerMetadata;
  }
  return result;
}

function assertPrivateValuesAbsent(value: unknown, fake: FakeAnthropic, extra: string[] = [], nativeResponse = false): void {
  const serialized = JSON.stringify(value);
  for (const secret of [TEST_TOKEN, "integration-anthropic-key", "GATEWAY_TEST_ANTHROPIC_KEY", "anthropic-primary", ...(nativeResponse ? [] : ["backend-private"]), "provider-secret-response", fake.url, ...extra]) {
    assert.ok(!serialized.includes(secret), `private test marker escaped into a client/service surface: ${secret}`);
  }
}

function assertConsumerSecretsAbsent(value: unknown, extra: string[] = []): void {
  const serialized = JSON.stringify(value);
  for (const credential of [TEST_TOKEN, "integration-anthropic-key", "integration-compatible-key", "integration-openai-key", "GATEWAY_TEST_ANTHROPIC_KEY", ...extra]) assert.ok(!serialized.includes(credential), `actual credential escaped into consumer output: ${credential}`);
}

function assertDiscoverySecretsAbsent(value: unknown, fake: FakeAnthropic, extra: string[] = []): void {
  const serialized = JSON.stringify(value);
  for (const secret of [TEST_TOKEN, "integration-anthropic-key", "GATEWAY_TEST_ANTHROPIC_KEY", "provider-secret-response", fake.url, ...extra]) {
    assert.ok(!serialized.includes(secret), "credential or unrelated configuration escaped into discovery");
  }
}

async function settleCleanup(...actions: Array<() => Promise<void>>): Promise<void> {
  const results = await Promise.allSettled(actions.map((action) => action()));
  const failures = results.flatMap((result) => result.status === "rejected" ? [result.reason] : []);
  if (failures.length > 0) throw new AggregateError(failures, "Gateway integration cleanup failed");
}

async function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  try {
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(() => reject(new Error(message)), timeoutMs);
      timer.unref();
    });
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer != null) clearTimeout(timer);
  }
}

type Topology = "cloud" | "self-hosted";
type CredentialRequest = { kind: "service" | "id-token" | "relay" | "delegated" | "session" | "access-policy"; service?: string; identityType?: string; uid?: string; subject?: string };

function flagEnvironment(args: string[]): Record<string, string> {
  const environment: Record<string, string> = {};
  for (const arg of args) {
    const match = /^--(no-)?([a-z0-9.-]+)(?:=(.*))?$/.exec(arg);
    if (match == null) throw new Error(`unsupported Gateway flag: ${arg}`);
    const name = `GF_PLUGIN_${match[2]!.toUpperCase().replace(/[.-]/g, "_")}`;
    environment[name] = match[3] ?? (match[1] ? "false" : "true");
  }
  return environment;
}

async function firstLine(stream: NodeJS.ReadableStream, exited: Promise<unknown>, timeoutMs: number): Promise<string | undefined> {
  let buffered = "";
  const line = new Promise<string>((resolve) => {
    stream.on("data", (chunk: Buffer) => {
      buffered += chunk.toString();
      const end = buffered.indexOf("\n");
      if (end >= 0) resolve(buffered.slice(0, end));
    });
  });
  return withTimeout(Promise.race([line, exited.then(() => undefined)]), timeoutMs, "timed out waiting for the plugin host");
}

type ProcessExit = { code: number | null; signal: NodeJS.Signals | null };

class GatewayProcess {
  static lastFailedDirectory: string | undefined;

  readonly process: ChildProcess;
  readonly url: string;
  readonly baseURL: string;
  readonly directory: string;
  nativeProxy?: Awaited<ReturnType<typeof startNativeProviderProxy>>;
  private stopped = false;
  private readonly exited: Promise<ProcessExit>;
  private readonly output: { stderr: string };

  private constructor(proc: ChildProcess, directory: string, exited: Promise<ProcessExit>, output: { stderr: string }, url: string, routePrefix: string) {
    this.process = proc;
    this.directory = directory;
    this.exited = exited;
    this.output = output;
    this.url = url;
    this.baseURL = `${url}${routePrefix}`;
  }

  get stderr(): string {
    return this.output.stderr;
  }

  static async start(binary: string, anthropicURL: string, extraArgs: string[] = [], extraEnv: Record<string, string> = {}, mode: "access-token" | "cloud-gateway" = "access-token", configYAML?: string, fallbackURL?: string, backendModel = "backend-private"): Promise<GatewayProcess> {
    const directory = mkdtempSync(join(tmpdir(), "grafana-ai-gateway-process-"));
    this.lastFailedDirectory = undefined;
    const topology: Topology = mode === "cloud-gateway" ? "cloud" : "self-hosted";
    let proc: ChildProcess | undefined;
    let exited: Promise<ProcessExit> | undefined;
    const output = { stderr: "" };
    try {
      const configPath = join(directory, "models.yaml");
      writeFileSync(configPath, fallbackURL
        ? anthropicFallbackConfig(anthropicURL, fallbackURL, backendModel)
        : configYAML ?? anthropicConfig(anthropicURL, backendModel));
      proc = spawn(hostBinaryPath, ["--plugin", binary, "--topology", topology, "--namespace", TEST_NAMESPACE, "--service-account-token", TEST_TOKEN, "--log-level", "debug"], {
        cwd: directory,
        stdio: ["ignore", "pipe", "pipe"],
        env: {
          ...Object.fromEntries(Object.entries(nodeProcess.env).filter(([name]) => !name.startsWith("AGENTO11Y_") && !name.startsWith("SIGIL_") && !name.startsWith("GRAFANA_JWKS") && !name.startsWith("GF_"))),
          GF_PLUGIN_CONFIG_FILE: configPath,
          GF_PLUGIN_ACCESS_MODE: topology,
          ...(topology === "cloud" ? { GF_PLUGIN_ACCESS_INTERNAL_SERVICES: INTERNAL_SERVICE } : {}),
          GATEWAY_TEST_ANTHROPIC_KEY: "integration-anthropic-key",
          GATEWAY_TEST_COMPATIBLE_KEY: "integration-compatible-key",
          GATEWAY_TEST_OPENAI_KEY: "integration-openai-key",
          ...flagEnvironment(extraArgs),
          ...extraEnv,
        },
      });
      proc.stderr?.on("data", (chunk: Buffer) => { output.stderr += chunk.toString(); });
      exited = new Promise<ProcessExit>((resolve, reject) => {
        proc!.once("error", reject);
        proc!.once("close", (code, signal) => resolve({ code, signal }));
      });
      const line = await firstLine(proc.stdout!, exited, READY_TIMEOUT_MS);
      if (line == null) {
        const exit = await exited;
        throw new Error(`gateway exited unsuccessfully: code=${exit.code} signal=${exit.signal}\n${output.stderr}`);
      }
      const ready = JSON.parse(line) as { url: string; routePrefix: string };
      const gateway = new GatewayProcess(proc, directory, exited, output, ready.url, ready.routePrefix);
      if (!await gateway.ready()) throw new Error(`gateway plugin is not healthy\n${gateway.stderr}`);
      return gateway;
    } catch (error) {
      this.lastFailedDirectory = directory;
      try {
        if (proc != null && exited != null && proc.exitCode == null && proc.signalCode == null) {
          proc.kill("SIGTERM");
          await withTimeout(exited, 5_000, "gateway did not exit").catch(() => proc!.kill("SIGKILL"));
        }
      } finally {
        rmSync(directory, { recursive: true, force: true });
      }
      throw error;
    }
  }

  client(token = TEST_TOKEN) {
    return createGateway({ apiKey: token, baseURL: this.baseURL });
  }

  accessTokenClient(token: string) {
    return createGateway({
      apiKey: "x-access-token-only",
      baseURL: this.baseURL,
      fetch: (input, init) => {
        const headers = new Headers(init?.headers);
        headers.delete("authorization");
        headers.set("x-access-token", token);
        return fetch(input, { ...init, headers });
      },
    });
  }

  async credential(request: CredentialRequest): Promise<string> {
    const response = await fetch(`${this.url}/platformhost/credentials`, { method: "POST", body: JSON.stringify(request), signal: AbortSignal.timeout(2_000) });
    assert.equal(response.status, 200, await response.clone().text());
    return (await response.json()).token;
  }

  async metrics(): Promise<string> {
    const response = await fetch(`${this.url}/platformhost/metrics`, { signal: AbortSignal.timeout(2_000) });
    assert.equal(response.status, 200);
    return response.text();
  }

  async ready(): Promise<boolean> {
    try {
      const response = await fetch(`${this.url}/platformhost/health`, { signal: AbortSignal.timeout(2_000) });
      return response.ok && (await response.json()).status === "OK";
    } catch {
      return false;
    }
  }

  async stop(): Promise<void> {
    try {
      if (!this.stopped) {
        this.stopped = true;
        try {
          await fetch(`${this.url}/platformhost/stop`, { method: "POST", signal: AbortSignal.timeout(5_000) });
        } catch {
          this.process.kill("SIGTERM");
        }
      }
      let exit: ProcessExit;
      try {
        exit = await withTimeout(this.exited, 5_000, `gateway did not exit: ${this.stderr}`);
      } catch (error) {
        if (this.process.exitCode == null && this.process.signalCode == null) this.process.kill("SIGKILL");
        await withTimeout(this.exited, 5_000, `gateway did not exit after SIGKILL: ${this.stderr}`);
        throw error;
      }
      if (exit.code !== 0 || exit.signal != null) {
        throw new Error(`gateway exited unsuccessfully: code=${exit.code} signal=${exit.signal}\n${this.stderr}`);
      }
    } finally {
      const proxy = this.nativeProxy;
      this.nativeProxy = undefined;
      await proxy?.stop();
      rmSync(this.directory, { recursive: true, force: true });
    }
  }
}

type FakeRequest = { path: string; apiKey?: string; headers: IncomingMessage["headers"]; body: Record<string, unknown> };

const largeText = "x".repeat(9 << 20);
const largeDelta = "d".repeat(1536 << 10);
const largeUsagePadding = "u".repeat(1536 << 10);

const longDelta = "l".repeat(1 << 20);
const longDeltaCount = 20;

function writeLongAnthropicStream(response: ServerResponse): void {
  response.writeHead(200, { "Content-Type": "text/event-stream" });
  const event = (value: { type: string; [key: string]: unknown }) => response.write(`event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`);
  event({ type: "message_start", message: { id: "msg_long", type: "message", role: "assistant", model: "backend-private", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 0 } } });
  event({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } });
  for (let i = 0; i < longDeltaCount; i++) event({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: longDelta } });
  event({ type: "content_block_stop", index: 0 });
  event({ type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null }, usage: { output_tokens: 3 } });
  event({ type: "message_stop" });
  response.end();
}

function writeLargeAnthropicOutput(streaming: boolean, response: ServerResponse): void {
  const usage = { input_tokens: 2, output_tokens: 3, native_padding: largeUsagePadding };
  if (!streaming) {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ id: "msg_large", type: "message", role: "assistant", model: "backend-private", content: [{ type: "text", text: largeText }], stop_reason: "end_turn", stop_sequence: null, usage }));
    return;
  }
  response.writeHead(200, { "Content-Type": "text/event-stream" });
  const event = (value: { type: string; [key: string]: unknown }) => response.write(`event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`);
  event({ type: "message_start", message: { id: "msg_large", type: "message", role: "assistant", model: "backend-private", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 0 } } });
  event({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } });
  event({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: largeDelta } });
  event({ type: "content_block_stop", index: 0 });
  event({ type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null }, usage });
  event({ type: "message_stop" });
  response.end();
}

class FakeAnthropic {
  metadata = false;
  readonly url: string;
  readonly requests: FakeRequest[] = [];
  readonly violations: string[] = [];
  readonly canceled = new Set<string>();
  redirectTo?: string;
  failureStatus?: number;
  streamErrorMessage?: string;
  failureCode?: string | number;
  functionTools = false;
  providerTools = false;
  providerToolCaller = false;
  backendModel = "backend-private";
  apiKey = "integration-anthropic-key";
  private readonly server: ReturnType<typeof createServer>;

  private constructor(server: ReturnType<typeof createServer>, url: string) {
    this.server = server;
    this.url = url;
  }

  static async start(): Promise<FakeAnthropic> {
    let fake: FakeAnthropic;
    const server = createServer((request, response) => void fake.handle(request, response));
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (address == null || typeof address === "string") throw new Error("fake server did not bind TCP");
    fake = new FakeAnthropic(server, `http://127.0.0.1:${address.port}`);
    return fake;
  }

  async stop(): Promise<void> {
    this.server.closeAllConnections();
    await new Promise<void>((resolve) => this.server.close(() => resolve()));
  }

  async waitForCancellation(marker: string): Promise<void> {
    await poll(async () => this.canceled.has(marker), 5_000, `Anthropic cancellation for ${marker}`);
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const body = JSON.parse(Buffer.concat(chunks).toString()) as Record<string, unknown>;
    const serialized = JSON.stringify(body);
    const marker = ["silent-abort", "silent-shutdown", "silent-unary-shutdown", "normal-stream", "provider-error", "redirect", "oversized"]
      .find((value) => serialized.includes(value));
    this.requests.push({ path: request.url ?? "", apiKey: singleHeader(request.headers["x-api-key"]), headers: { ...request.headers }, body });
    if (request.url !== "/v1/messages") this.violations.push(`path=${request.url}`);
    if (singleHeader(request.headers["x-api-key"]) !== this.apiKey) this.violations.push("api-key");
    if (body.model !== this.backendModel) this.violations.push(`model=${String(body.model)}`);
    if (request.headers["x-access-token"] != null || request.headers["x-grafana-id"] != null) this.violations.push("forwarded-caller-credential");

    if (serialized.includes("wp17-reasoning")) {
      response.writeHead(200,{"Content-Type":"text/event-stream"});
      const events=[
        {type:"message_start",message:{id:"r",type:"message",role:"assistant",model:"backend-private",content:[],stop_reason:null,usage:{input_tokens:2,output_tokens:0}}},
        {type:"content_block_start",index:0,content_block:{type:"thinking",thinking:"",signature:""}},
        {type:"content_block_delta",index:0,delta:{type:"thinking_delta",thinking:"private thought"}},
        {type:"content_block_delta",index:0,delta:{type:"signature_delta",signature:"end-signature"}},
        {type:"content_block_stop",index:0},
        {type:"message_delta",delta:{stop_reason:"end_turn",stop_sequence:null},usage:{output_tokens:3}},
        {type:"message_stop"},
      ];
      for(const event of events)response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
      response.end();return;
    }

    if (this.failureStatus != null) {
      response.writeHead(this.failureStatus, { "Content-Type": "application/json", "x-private-provider": "backend-private" });
      response.end(JSON.stringify({ type: "error", error: { type: "api_error", code: this.failureCode, message: serialized.includes("configured-key-summary") ? "integration-anthropic-key integration-openai-key integration-agento11y-key own-byok-key" : `provider-secret-response ${this.apiKey} backend-private` }, request_id: "backend-private-request" }));
      return;
    }
    if (this.metadata) { nativeMetadataReply("anthropic", body, response); return; }
    if (serialized.includes("wp112-fallback-marker")) {
      const fallbackBlock = { type: "fallback", from: { model: "claude-a" }, to: { model: "claude-b" } };
      if (body.stream !== true) {
        response.writeHead(200, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ id: "msg_fallback", type: "message", role: "assistant", model: "backend-private", content: [fallbackBlock, { type: "text", text: "fallback answer" }], stop_reason: "end_turn", stop_sequence: null, usage: { input_tokens: 2, output_tokens: 3 } }));
        return;
      }
      response.writeHead(200, { "Content-Type": "text/event-stream" });
      const events: Array<{ type: string; [key: string]: unknown }> = [
        { type: "message_start", message: { id: "msg_fallback", type: "message", role: "assistant", model: "backend-private", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 0 } } },
        { type: "content_block_start", index: 0, content_block: fallbackBlock },
        { type: "content_block_stop", index: 0 },
        { type: "content_block_start", index: 1, content_block: { type: "text", text: "" } },
        { type: "content_block_delta", index: 1, delta: { type: "text_delta", text: "fallback answer" } },
        { type: "content_block_stop", index: 1 },
        { type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null }, usage: { output_tokens: 3 } },
        { type: "message_stop" },
      ];
      for (const event of events) response.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
      response.end();
      return;
    }
    if (marker === "silent-unary-shutdown") {
      response.once("close", () => this.canceled.add(marker));
      return;
    }
    if (serialized.includes("long-output") && body.stream === true) {
      writeLongAnthropicStream(response);
      return;
    }
    if (serialized.includes("large-output")) {
      writeLargeAnthropicOutput(body.stream === true, response);
      return;
    }

    if (this.redirectTo != null) {
      response.writeHead(307, { Location: this.redirectTo });
      response.end();
      return;
    }
    if (marker === "provider-error") {
      response.writeHead(502, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ error: { type: "api_error", message: "provider-secret-response" } }));
      return;
    }
    if (this.providerTools || this.providerToolCaller) {
      const messages = body.messages as Array<{ content: Array<{ type: string }> }>;
      const continued = messages.some(message => message.content.some(part => part.type === "server_tool_use" || part.type === "tool_result"));
      const blocks = continued
        ? [{ type: "text", text: "Hosted tools finished." }]
        : this.providerToolCaller
          ? [{ type: "tool_use", id: "call-child", name: "execute", input: {}, caller: { type: "code_execution_20260120", tool_id: "call-parent" } }]
          : [
              { type: "server_tool_use", id: "call-code", name: "code_execution", input: { code: "1+1" }, caller: { type: "direct" } },
              { type: "code_execution_tool_result", tool_use_id: "call-code", content: { type: "code_execution_result", stdout: "2", stderr: "", return_code: 0 } },
              ...(body.mcp_servers ? [
                { type: "mcp_tool_use", id: "call-mcp", name: "echo", server_name: "echo", input: { message: "hello" } },
                { type: "mcp_tool_result", tool_use_id: "call-mcp", is_error: false, content: "done" },
              ] : []),
            ];
      const stopReason = !continued && this.providerToolCaller ? "tool_use" : "end_turn";
      if (body.stream !== true) {
        response.writeHead(200, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ id: "msg_provider_tools", type: "message", role: "assistant", model: "backend-private", content: blocks, stop_reason: stopReason, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 4 } }));
        return;
      }
      response.writeHead(200, { "Content-Type": "text/event-stream" });
      const event = (value: { type: string; [key: string]: unknown }) => response.write(`event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`);
      event({ type: "message_start", message: { id: "msg_provider_tools", type: "message", role: "assistant", model: "backend-private", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 0 } } });
      for (const [index, block] of blocks.entries()) {
        event({ type: "content_block_start", index, content_block: continued ? { type: "text", text: "" } : block });
        if (continued) event({ type: "content_block_delta", index, delta: { type: "text_delta", text: "Hosted tools finished." } });
        event({ type: "content_block_stop", index });
      }
      event({ type: "message_delta", delta: { stop_reason: stopReason, stop_sequence: null }, usage: { output_tokens: 4 } });
      event({ type: "message_stop" });
      response.end();
      return;
    }
    if (this.functionTools) {
      const messages = body.messages as Array<{ content: Array<{ type: string }> }>;
      const continued = messages.some(message => message.content.some(part => part.type === "tool_result"));
      if (body.stream !== true) {
        response.writeHead(200, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ id: "msg_tools", type: "message", role: "assistant", model: "backend-private", content: continued ? [{ type: "text", text: "It is sunny." }] : [{ type: "tool_use", id: "call-weather", name: "weather", input: { city: "Rio" } }], stop_reason: continued ? "end_turn" : "tool_use", stop_sequence: null, usage: { input_tokens: 2, output_tokens: 3 } }));
        return;
      }
      response.writeHead(200, { "Content-Type": "text/event-stream" });
      const event = (value: { type: string; [key: string]: unknown }) => response.write(`event: ${value.type}\ndata: ${JSON.stringify(value)}\n\n`);
      event({ type: "message_start", message: { id: "msg_tools", type: "message", role: "assistant", model: "backend-private", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 0 } } });
      if (continued) {
        event({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } });
        event({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "It is sunny." } });
      } else {
        event({ type: "content_block_start", index: 0, content_block: { type: "tool_use", id: "call-weather", name: "weather", input: {} } });
        for (const partial_json of ["", '{"city":', '"Rio"}']) event({ type: "content_block_delta", index: 0, delta: { type: "input_json_delta", partial_json } });
      }
      event({ type: "content_block_stop", index: 0 });
      event({ type: "message_delta", delta: { stop_reason: continued ? "end_turn" : "tool_use", stop_sequence: null }, usage: { output_tokens: 3 } });
      event({ type: "message_stop" });
      response.end();
      return;
    }
    if (body.stream !== true) {
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({
        id: "msg_test", type: "message", role: "assistant", model: "backend-private",
        content: [{ type: "text", text: "hello from fake Anthropic" }],
        stop_reason: "end_turn", stop_sequence: null,
        usage: { input_tokens: 2, output_tokens: 3 },
      }));
      return;
    }

    response.writeHead(200, { "Content-Type": "text/event-stream" });
    response.write(`event: message_start\ndata: ${JSON.stringify({ type: "message_start", message: { id: "msg_test", type: "message", role: "assistant", model: "backend-private", content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 2, output_tokens: 0 } } })}\n\n`);
    if (this.streamErrorMessage) {
      response.write(`event: content_block_start\ndata: ${JSON.stringify({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } })}\n\n`);
      response.write(`event: content_block_delta\ndata: ${JSON.stringify({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: this.streamErrorMessage } })}\n\n`);
      response.write(`event: content_block_stop\ndata: ${JSON.stringify({ type: "content_block_stop", index: 0 })}\n\n`);
      response.write(`event: error\ndata: ${JSON.stringify({ type: "error", error: { type: "api_error", message: this.streamErrorMessage } })}\n\n`);
      response.end();
      return;
    }
    if (marker === "normal-stream") {
      response.write(`event: content_block_start\ndata: ${JSON.stringify({ type: "content_block_start", index: 0, content_block: { type: "text", text: "" } })}\n\n`);
      response.write(`event: content_block_delta\ndata: ${JSON.stringify({ type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "hello from fake Anthropic stream" } })}\n\n`);
      response.write(`event: content_block_stop\ndata: ${JSON.stringify({ type: "content_block_stop", index: 0 })}\n\n`);
      response.write(`event: message_delta\ndata: ${JSON.stringify({ type: "message_delta", delta: { stop_reason: "end_turn", stop_sequence: null }, usage: { input_tokens: 2, output_tokens: 6 } })}\n\n`);
      response.end(`event: message_stop\ndata: {"type":"message_stop"}\n\n`);
      return;
    }
    request.once("close", () => { if (marker != null) this.canceled.add(marker); });
    response.once("close", () => { if (marker != null) this.canceled.add(marker); });
  }
}

class FakeCompatible {
  metadata = false;
  readonly url: string;
  readonly requests: FakeRequest[] = [];
  readonly violations: string[] = [];
  readonly canceled = new Set<string>();
  redirectTo?: string;
  failWithSecret = false;
  private readonly server: ReturnType<typeof createServer>;

  private constructor(server: ReturnType<typeof createServer>, url: string) {
    this.server = server;
    this.url = url;
  }

  static async start(): Promise<FakeCompatible> {
    let fake: FakeCompatible;
    const server = createServer((request, response) => void fake.handle(request, response));
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (address == null || typeof address === "string") throw new Error("fake server did not bind TCP");
    fake = new FakeCompatible(server, `http://127.0.0.1:${address.port}`);
    return fake;
  }

  async stop(): Promise<void> {
    this.server.closeAllConnections();
    await new Promise<void>((resolve) => this.server.close(() => resolve()));
  }

  async waitForCancellation(marker: string): Promise<void> {
    await poll(async () => this.canceled.has(marker), 5_000, `compatible cancellation for ${marker}`);
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const body = JSON.parse(Buffer.concat(chunks).toString()) as Record<string, unknown>;
    const marker = ["silent-abort", "normal-stream"].find((value) => JSON.stringify(body).includes(value));
    const authorization = singleHeader(request.headers.authorization);
    this.requests.push({ path: request.url ?? "", apiKey: authorization, headers: { ...request.headers }, body });
    if (request.url !== "/v1/chat/completions") this.violations.push(`path=${request.url}`);
    if (authorization !== "Bearer integration-compatible-key") this.violations.push("authorization");
    if (body.model !== "backend-private") this.violations.push(`model=${String(body.model)}`);

    if (JSON.stringify(body).includes("wp17-reasoning")) {
      if (body.stream !== true) {
        response.writeHead(200,{"Content-Type":"application/json"});
        response.end(JSON.stringify({id:"reasoning",object:"chat.completion",created:1,model:"backend-private",choices:[{index:0,message:{role:"assistant",content:null,reasoning_content:"private thought"},finish_reason:"stop"}],usage:{prompt_tokens:2,completion_tokens:3,total_tokens:5,completion_tokens_details:{reasoning_tokens:3}}}));
      } else {
        response.writeHead(200,{"Content-Type":"text/event-stream"});
        for(const choices of [[{index:0,delta:{role:"assistant",reasoning_content:"private thought"},finish_reason:null}],[{index:0,delta:{},finish_reason:"stop"}]]) response.write(`data: ${JSON.stringify({id:"reasoning",object:"chat.completion.chunk",created:1,model:"backend-private",choices})}\n\n`);
        response.end("data: [DONE]\n\n");
      }
      return;
    }

    if (this.redirectTo != null) {
      response.writeHead(307, { Location: this.redirectTo });
      response.end();
      return;
    }
    if (this.failWithSecret) {
      response.writeHead(502, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ error: { type: "server_error", message: "provider-secret-response" } }));
      return;
    }
    if (this.metadata) { nativeMetadataReply("compatible", body, response); return; }
    if (body.stream !== true) {
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({
        id: "chatcmpl-test", object: "chat.completion", created: 1, model: "backend-private",
        choices: [{ index: 0, message: { role: "assistant", content: "hello from fake compatible" }, finish_reason: "stop" }],
        usage: { prompt_tokens: 2, completion_tokens: 3, total_tokens: 5 },
      }));
      return;
    }

    response.writeHead(200, { "Content-Type": "text/event-stream" });
    response.flushHeaders();
    if (marker === "normal-stream") {
      const chunk = (fields: Record<string, unknown>) => `data: ${JSON.stringify({ id: "chatcmpl-test", object: "chat.completion.chunk", created: 1, model: "backend-private", ...fields })}\n\n`;
      response.write(chunk({ choices: [{ index: 0, delta: { role: "assistant", content: "hello from fake compatible stream" }, finish_reason: null }] }));
      response.write(chunk({ choices: [{ index: 0, delta: {}, finish_reason: "stop" }] }));
      response.write(chunk({ choices: [], usage: { prompt_tokens: 2, completion_tokens: 6, total_tokens: 8 } }));
      response.end("data: [DONE]\n\n");
      return;
    }
    request.once("close", () => { if (marker != null) this.canceled.add(marker); });
    response.once("close", () => { if (marker != null) this.canceled.add(marker); });
  }
}

class FakeOpenAI {
 approvals = false;
 approvalFailure = false;
  metadata = false;
  structuredOutput = false;
  readonly url: string;
  readonly requests: FakeRequest[] = [];
  readonly violations: string[] = [];
  redirectTo?: string;
  failWithSecret = false;
  failureCode?: string | number;
  sources = false;
  apiKey = "integration-openai-key";
  backendModel = "backend-private";
  private readonly server: ReturnType<typeof createServer>;

  private constructor(server: ReturnType<typeof createServer>, url: string) {
    this.server = server;
    this.url = url;
  }

  static async start(): Promise<FakeOpenAI> {
    let fake: FakeOpenAI;
    const server = createServer((request, response) => void fake.handle(request, response));
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (address == null || typeof address === "string") throw new Error("fake server did not bind TCP");
    fake = new FakeOpenAI(server, `http://127.0.0.1:${address.port}`);
    return fake;
  }

  async stop(): Promise<void> {
    this.server.closeAllConnections();
    await new Promise<void>((resolve) => this.server.close(() => resolve()));
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const body = JSON.parse(Buffer.concat(chunks).toString()) as Record<string, unknown>;
    const authorization = singleHeader(request.headers.authorization);
    this.requests.push({ path: request.url ?? "", apiKey: authorization, headers: { ...request.headers }, body });
    if (request.url !== "/v1/responses") this.violations.push(`path=${request.url}`);
    if (authorization !== `Bearer ${this.apiKey}`) this.violations.push("authorization");
    if (body.model !== this.backendModel) this.violations.push(`model=${String(body.model)}`);

    if (JSON.stringify(body).includes("wp17-reasoning")) {
      response.writeHead(200,{"Content-Type":"text/event-stream"});
      const item={type:"reasoning",id:"rs_wp17",summary:[{type:"summary_text",text:"private thought"}],encrypted_content:"final-encrypted"};
      const events=[
        {type:"response.created",response:{id:"resp_wp17",object:"response",created_at:1,status:"in_progress",model:"backend-private",output:[],usage:null}},
        {type:"response.output_item.added",output_index:0,item:{...item,summary:[],encrypted_content:null}},
        {type:"response.reasoning_summary_text.delta",output_index:0,item_id:"rs_wp17",summary_index:0,delta:"private thought"},
        {type:"response.output_item.done",output_index:0,item},
        {type:"response.completed",response:{id:"resp_wp17",object:"response",created_at:1,status:"completed",model:"backend-private",output:[item],usage:{input_tokens:2,output_tokens:3,output_tokens_details:{reasoning_tokens:3},total_tokens:5}}},
      ];
      events.forEach((event,index)=>response.write(`event: ${event.type}\ndata: ${JSON.stringify({...event,sequence_number:index})}\n\n`));
      response.end();return;
    }

    if (this.redirectTo != null) {
      response.writeHead(307, { Location: this.redirectTo });
      response.end();
      return;
    }
    if (this.failWithSecret) {
      response.writeHead(502, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ error: { type: "server_error", code: this.failureCode, message: "provider-secret-response" } }));
      return;
    }
    if (this.approvals) { nativeApprovalReply(body, response, this.approvalFailure); return; }
    if (this.metadata) { nativeMetadataReply("openai", body, response); return; }
    const stream = body.stream === true;
    const compaction = JSON.stringify(body).includes("wp112-compaction");
    const text = this.structuredOutput ? '{"value":"ok"}' : stream ? "hello from fake openai stream" : "hello from fake openai";
    const annotations=this.sources ? [
      {type:"url_citation",url:"https://public.example",title:"Public citation",start_index:0,end_index:5},
      {type:"file_path",file_id:"native-file-sk-application",index:0},
    ] : [];
    const message = { id: "msg_test", type: "message", status: "completed", role: "assistant", content: [{ type: "output_text", annotations, logprobs: [], text }] };
    const compactionItem = { type: "compaction", id: "cmp_1", encrypted_content: "ENC" };
    const outputItems = compaction ? [compactionItem, message] : [message];
    const completed = (outputTokens: number) => ({
      id: "resp_test", object: "response", created_at: 1, status: "completed", model: "backend-private", output: outputItems,
      usage: { input_tokens: 2, input_tokens_details: { cached_tokens: 0 }, output_tokens: outputTokens, output_tokens_details: { reasoning_tokens: 0 }, total_tokens: 2 + outputTokens },
    });
    if (!stream) {
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify(completed(3)));
      return;
    }
    response.writeHead(200, { "Content-Type": "text/event-stream" });
    const events: Array<Record<string, unknown>> = [
      { type: "response.created", response: { ...completed(0), status: "in_progress", output: [], usage: null } },
      ...(compaction ? [{ type: "response.output_item.done", output_index: 0, item: compactionItem }] : []),
      { type: "response.output_item.added", output_index: compaction ? 1 : 0, item: { ...message, status: "in_progress", content: [] } },
      { type: "response.output_text.delta", output_index: compaction ? 1 : 0, content_index: 0, item_id: "msg_test", delta: text, logprobs: [] },
      ...annotations.map((annotation,annotation_index)=>({type:"response.output_text.annotation.added",output_index:compaction ? 1 : 0,content_index:0,item_id:"msg_test",annotation_index,annotation})),
      { type: "response.output_item.done", output_index: compaction ? 1 : 0, item: message },
      { type: "response.completed", response: completed(6) },
    ];
    events.forEach((event, index) => response.write(`event: ${String(event.type)}\ndata: ${JSON.stringify({ ...event, sequence_number: index })}\n\n`));
    response.end();
  }
}

class FakeAgentObservability {
  readonly url: string;
  readonly generations: Array<Record<string, unknown>> = [];
  readonly violations: string[] = [];
  private readonly server: ReturnType<typeof createServer>;

  private constructor(server: ReturnType<typeof createServer>, url: string) {
    this.server = server;
    this.url = url;
  }

  static async start(): Promise<FakeAgentObservability> {
    let fake: FakeAgentObservability;
    const server = createServer((request, response) => void fake.handle(request, response));
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (address == null || typeof address === "string") throw new Error("fake Agent Observability server did not bind TCP");
    fake = new FakeAgentObservability(server, `http://127.0.0.1:${address.port}`);
    return fake;
  }

  async stop(): Promise<void> {
    this.server.closeAllConnections();
    await new Promise<void>((resolve) => this.server.close(() => resolve()));
  }

  async waitForGenerations(count: number): Promise<void> {
    await poll(async () => this.generations.length >= count, 10_000, `${count} Agent Observability generations`);
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    if (request.url !== "/api/v1/generations:export") this.violations.push(`path=${request.url}`);
    if (singleHeader(request.headers.authorization) !== "Bearer integration-agento11y-key") this.violations.push("authorization");
    if (singleHeader(request.headers["content-type"]) !== "application/json") this.violations.push("content-type");
    const payload = JSON.parse(Buffer.concat(chunks).toString()) as { generations?: Array<Record<string, unknown>> };
    const generations = payload.generations ?? [];
    if (generations.length === 0) this.violations.push("empty-generations");
    this.generations.push(...generations);
    response.writeHead(202, { "Content-Type": "application/json" });
    response.end(JSON.stringify({
      results: generations.map((generation) => ({ generation_id: generation.id, accepted: true })),
    }));
  }
}

async function collectGatewayStream(stream: ReadableStream<{ type: string; delta?: string }>): Promise<Array<{ type: string; delta?: string }>> {
  const reader = stream.getReader();
  const parts: Array<{ type: string; delta?: string }> = [];
  for (;;) {
    const next = await reader.read();
    if (next.done) return parts;
    parts.push(next.value);
  }
}

function sumPrometheusSamples(metrics: string, family: string, requiredLabels: string[]): number {
  let total = 0;
  for (const line of metrics.split("\n")) {
    if (!line.startsWith(`${family}{`) || !requiredLabels.every((label) => line.includes(label))) continue;
    const value = Number.parseFloat(line.slice(line.lastIndexOf(" ") + 1));
    assert.equal(Number.isFinite(value), true);
    total += value;
  }
  return total;
}

function rawProviderWireRequest(baseURL: string, text: string, modelID = "assistant", extraParts: readonly unknown[] = []): Promise<Response> {
  return fetch(`${baseURL}/language-model`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${TEST_TOKEN}`,
      "ai-language-model-specification-version": "4",
      "ai-language-model-id": modelID,
      "ai-language-model-streaming": "false",
    },
    body: JSON.stringify({
      prompt: [{ role: "user", content: [{ type: "text", text }, ...extraParts] }],
      maxOutputTokens: 32,
      temperature: 0.2,
    }),
  });
}

function singleHeader(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

function processLifecycleEvents(output: string): string[] {
  const events: string[] = [];
  for (const line of output.split("\n")) {
    let record: Record<string, unknown>;
    try {
      record = JSON.parse(line) as Record<string, unknown>;
    } catch {
      continue;
    }
    if (record["@message"] !== "gateway plugin lifecycle") continue;
    assert.equal(record["@level"], "info");
    assert.equal(typeof record.event, "string");
    events.push(record.event as string);
  }
  return events;
}

async function availablePort(): Promise<number> {
  const server = createNetServer();
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (address == null || typeof address === "string") throw new Error("port allocator did not bind TCP");
  const port = address.port;
  await new Promise<void>((resolve) => server.close(() => resolve()));
  return port;
}

async function poll(check: () => Promise<boolean>, timeoutMs: number, description: string): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await check()) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error(`timed out waiting for ${description}`);
}

describe("provider approval continuation", () => {
  for (const fallback of [false, true]) for (const store of [false, true]) {
    it(`actual outputs continue approved denied mixed, fallback=${fallback}, store=${store}`, async () => {
      const openai = await FakeOpenAI.start(); openai.approvals = true;
      const anthropic = await FakeAnthropic.start(); anthropic.failureStatus = 503;
      const config = `providers:
  openai:
    type: openai
    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY
    baseURL: ${openai.url}/v1
  anthropic:
    type: anthropic
    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY
    baseURL: ${anthropic.url}
models:
  approvals:
    name: Approvals
    primary:
      provider: ${fallback ? "anthropic" : "openai"}
      model: backend-private
${fallback ? "    fallback:\n      - provider: openai\n        model: backend-private\n" : ""}`;
      let gateway: GatewayProcess | undefined;
      try {
        gateway = await GatewayProcess.start(binaryPath, "", [], {}, "access-token", config);
        const model = gateway.client()("approvals");
        for (const decisions of [[true,true], [false,false], [true,false], [true,true,true], [false,false,false], [true,false,true]]) {
          const local = decisions.length === 3;
          for (const path of ["ts-unary", "ts-stream", "ts-high-generate", "ts-high-stream", "go-unary", "go-stream", "go-high-generate", "go-high-stream"]) {
            if(local && !path.includes("high")) continue;
            let executions = 0;
            const localTool = tool({inputSchema:jsonSchema<Record<string,unknown>>({type:"object"}),needsApproval:true,execute:async()=>{executions++;return "local-result";}});
            const offset = openai.requests.length;
            const options: LanguageModelV4CallOptions = { prompt: [{role:"user",content:[{type:"text",text:local?"approval-local":"approval"}]}], providerOptions:{openai:{store}}, maxOutputTokens:64 };
            if (path.startsWith("go")) {
              const go = await captureGoClient(goClientBinaryPath,{baseURL:gateway.baseURL,serviceAccountToken:TEST_TOKEN,modelID:"approvals",mode:path.replace("go-","approval-"),decisions,options});
              assert.equal(go.error,undefined,JSON.stringify(go.error)); assert.equal(go.approvals.length,decisions.length);executions=go.executions;
            } else if (path.includes("high")) {
              const initial = {model,tools:{local:localTool,"mcp.lookup":{...localTool,needsApproval:false}},prompt:local?"approval-local":"approval",providerOptions:{openai:{store}},maxRetries:0};
              const first = path.endsWith("generate") ? await generateText(initial) : streamText(initial);
              const messages = (await first.response).messages;
              const approvals = messages.flatMap<any>(message => Array.isArray(message.content) ? message.content : []).filter((part:any)=>part.type==="tool-approval-request") as any[];
              assert.equal(approvals.length,decisions.length);
              const calls = messages.flatMap<any>(message=>Array.isArray(message.content)?message.content:[]).filter((part:any)=>part.type==="tool-call");
              for (const approval of approvals) {const call=calls.find((part:any)=>part.toolCallId===approval.toolCallId);if(call?.providerExecuted)call.providerOptions={...call.providerOptions,openai:{...call.providerOptions?.openai,approvalRequestId:approval.approvalId}};}
              const resumed = {model,tools:{local:localTool,"mcp.lookup":{...localTool,needsApproval:false}},messages:[...messages,{role:"tool" as const,content:approvals.map((part,index)=>({type:"tool-approval-response" as const,approvalId:part.approvalId,approved:decisions[index],providerExecuted:calls.find((call:any)=>call.toolCallId===part.toolCallId)?.providerExecuted}))}],providerOptions:{openai:{store}},maxRetries:0};
              const second = path.endsWith("generate") ? await generateText(resumed) : streamText(resumed);
              const nextMessages=await second.responseMessages;
              const mcpResults=nextMessages.flatMap<any>(message=>Array.isArray(message.content)?message.content:[]).filter((part:any)=>part.type==="tool-result"&&part.toolName==="mcp.lookup");
              const results=mcpResults.filter((part:any)=>part.output.type!=="execution-denied");
              const expectedDenied=path.endsWith("generate")?decisions.slice(0,2).filter(value=>!value).length:0;
              assert.equal(mcpResults.filter((part:any)=>part.output.type==="execution-denied").length,expectedDenied,path);
              assert.equal(results.length,decisions.slice(0,2).filter(Boolean).length);
              for(const result of results)assert.ok(calls.some((call:any)=>call.toolCallId===result.toolCallId));
              const third={...resumed,messages:[...resumed.messages,...nextMessages,{role:"user" as const,content:"third-turn"}]};
              if(path.endsWith("generate"))await generateText(third);else await streamText(third).consumeStream();
            } else {
              const streaming = path.endsWith("stream");
              const first: any[] = streaming ? await collectGatewayStream((await model.doStream(options)).stream) : (await model.doGenerate(options)).content;
              const approvals = first.filter(part=>part.type==="tool-approval-request"); assert.equal(approvals.length,2);
              const calls = first.filter(part=>part.type==="tool-call").map(part=>({type:"tool-call" as const,toolCallId:part.toolCallId,toolName:part.toolName,input:JSON.parse(part.input),providerExecuted:part.providerExecuted,providerOptions:part.providerMetadata}));
              for(const approval of approvals){const call=calls.find(part=>part.toolCallId===approval.toolCallId);assert.ok(call);call.providerOptions={...call.providerOptions,openai:{...call.providerOptions?.openai,approvalRequestId:approval.approvalId}};}
              const responses = approvals.map((part,index)=>({type:"tool-approval-response" as const,approvalId:part.approvalId,approved:decisions[index],reason:"",providerOptions:{openai:{extension:false}}}));
              const next: LanguageModelV4CallOptions = {...options,prompt:[...options.prompt,{role:"assistant",content:calls},{role:"tool",content:responses}]};
              const resumed:any[]=streaming?await collectGatewayStream((await model.doStream(next)).stream):(await model.doGenerate(next)).content;
              const results=resumed.filter(part=>part.type==="tool-result");assert.equal(results.length,decisions.filter(Boolean).length);
              for(const result of results){const approval=approvals.find(part=>part.toolCallId===result.toolCallId);assert.ok(approval);assert.equal(result.providerMetadata?.openai?.itemId,`result-${approval.approvalId}`);}
            }
            const requests = openai.requests.slice(offset); assert.equal(requests.length,path.includes("high")?3:2,path);
            const input = requests[1].body.input as any[];
            assert.deepEqual(input.filter(item=>item.type==="mcp_approval_response"),decisions.slice(0,2).map((approve,index)=>({type:"mcp_approval_response",approval_request_id:`approval-${index}`,approve})),path);
            for (let index=0;index<2;index++) assert.equal(input.some(item=>item.type==="item_reference"&&item.id===`approval-${index}`),store,path);
            assert.equal(executions,local&&decisions[2]?1:0,path);
            const localResult=input.find(item=>item.type==="function_call_output");
            if(local){assert.ok(localResult,path);assert.equal(localResult.call_id,"local-call");assert.equal(localResult.output,decisions[2]?"local-result":"Tool call execution denied.",path);} else assert.equal(localResult,undefined,path);
          }
        }
      } finally { await settleCleanup(...(gateway?[()=>gateway!.stop()]:[]),()=>openai.stop(),()=>anthropic.stop()); }
    });
  }
});

describe("approval commitment",()=>{
 it("does not replay onto fallback after approval output and a native stream failure",async()=>{
  const openai=await FakeOpenAI.start();openai.approvals=true;openai.approvalFailure=true;
  const anthropic=await FakeAnthropic.start();let gateway:GatewayProcess|undefined;
  const config=`providers:
  openai:
    type: openai
    apiKeyEnv: GATEWAY_TEST_OPENAI_KEY
    baseURL: ${openai.url}/v1
  anthropic:
    type: anthropic
    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY
    baseURL: ${anthropic.url}
models:
  approvals:
    name: Approvals
    primary:
      provider: openai
      model: backend-private
    fallback:
      - provider: anthropic
        model: backend-private
`;
  try{
   gateway=await GatewayProcess.start(binaryPath,"",[],{},"access-token",config);
   const options:LanguageModelV4CallOptions={prompt:[{role:"user",content:[{type:"text",text:"approval"}]}]};
   const ts=await collectGatewayStream((await gateway.client()("approvals").doStream(options)).stream);
   const go=await captureGoClient(goClientBinaryPath,{baseURL:gateway.baseURL,serviceAccountToken:TEST_TOKEN,modelID:"approvals",mode:"stream",options});
   for(const parts of [ts,go.parts]){assert.ok(parts.some((part:any)=>part.type==="tool-approval-request"));assert.ok(parts.some((part:any)=>part.type==="error"));}
   assert.equal(openai.requests.length,2);assert.equal(anthropic.requests.length,0);
  }finally{await settleCleanup(...(gateway?[()=>gateway!.stop()]:[]),()=>openai.stop(),()=>anthropic.stop());}
 });
});

it("registered nested result options reach native OpenAI through both clients and modes", async () => {
  const [native, gateway] = await startOpenAIGateway();
  try {
    const breakpoint = { mode: "explicit" };
    const options: LanguageModelV4CallOptions = { prompt: [{ role: "tool", content: [{ type: "tool-result", toolCallId: "call", toolName: "lookup", output: { type: "content", value: [
      { type: "text", text: "result", providerOptions: { openai: { promptCacheBreakpoint: breakpoint } } },
      { type: "file", data: { type: "data", data: "AQI=" }, mediaType: "image/png", providerOptions: { openai: { imageDetail: "high", promptCacheBreakpoint: breakpoint } } },
    ] } }] }] };
    for (const mode of ["generate", "stream"] as const) for (const client of ["ts", "go"]) {
      const offset = native.requests.length;
      if (client === "go") {
        const result = await captureGoClient(goClientBinaryPath, { baseURL: gateway.baseURL, serviceAccountToken: TEST_TOKEN, modelID: "openai", mode, options });
        assert.equal(result.error, undefined);
      } else if (mode === "generate") await gateway.client()("openai").doGenerate(options);
      else await collectGatewayStream((await gateway.client()("openai").doStream(options)).stream);
      const output = (native.requests[offset].body.input as any[]).find(item => item.type === "function_call_output").output;
      assert.deepEqual(output, [
        { type: "input_text", text: "result", prompt_cache_breakpoint: breakpoint },
        { type: "input_image", image_url: "data:image/png;base64,AQI=", detail: "high", prompt_cache_breakpoint: breakpoint },
      ]);
    }
  } finally { await settleCleanup(() => gateway.stop(), () => native.stop()); }
});
