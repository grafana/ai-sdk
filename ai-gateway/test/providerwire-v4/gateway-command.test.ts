import assert from "node:assert/strict";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { generateKeyPairSync, sign } from "node:crypto";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, request as httpRequest, type IncomingMessage, type ServerResponse } from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import nodeProcess from "node:process";
import { after, before, describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";
import type { LanguageModelV4CallOptions } from "@ai-sdk/provider";
import { buildGoClientCapture, captureGoClient } from "./go-client-capture";

const AI_GATEWAY_ROOT = resolve(import.meta.dirname, "../..");
const COMMAND_DIR = resolve(AI_GATEWAY_ROOT, "cmd/grafana-ai-gateway");
const READY_TIMEOUT_MS = 15_000;
const TEST_TOKEN = unsafeAccessToken();
const TEST_USER_TOKEN = unsafeUserIDToken();

let buildDirectory: string;
let binaryPath: string;
let goClientBinaryPath: string;

before(() => {
  buildDirectory = mkdtempSync(join(tmpdir(), "grafana-ai-gateway-build-"));
  binaryPath = join(buildDirectory, "grafana-ai-gateway");
  execFileSync("go", ["build", "-o", binaryPath, "."], {
    cwd: COMMAND_DIR,
    stdio: "pipe",
    env: {
      ...nodeProcess.env,
      // CI and release verification always use immutable module pins. A local
      // unpublished-middleware checkout may opt into an explicit go.work path.
      GOWORK: nodeProcess.env.GATEWAY_TEST_GOWORK ?? "off",
      GOFLAGS: `${nodeProcess.env.GOFLAGS ? `${nodeProcess.env.GOFLAGS} ` : ""}-mod=readonly`,
    },
  });
  goClientBinaryPath = buildGoClientCapture(buildDirectory);
});

after(() => {
  rmSync(buildDirectory, { recursive: true, force: true });
});

describe("authenticated Anthropic Gateway command", () => {
  it("preserves authenticated Vercel and Go text behavior across ordered fallback", async () => {
    const keys = generateKeyPairSync("ec", { namedCurve: "P-256" });
    const jwk = { ...keys.publicKey.export({ format: "jwk" }), kid: "fallback-test", alg: "ES256", use: "sig" };
    let keyRequests = 0;
    const jwks = createServer((_request, response) => {
      keyRequests++;
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ keys: [jwk] }));
    });
    await new Promise<void>(resolve => jwks.listen(0, "127.0.0.1", resolve));
    const address = jwks.address();
    assert.ok(address && typeof address !== "string");
    const header = Buffer.from(JSON.stringify({ alg: "ES256", typ: "at+jwt", kid: "fallback-test" })).toString("base64url");
    const unsigned = `${header}.${TEST_TOKEN.split(".")[1]}`;
    const token = `${unsigned}.${sign("sha256", Buffer.from(unsigned), { key: keys.privateKey, dsaEncoding: "ieee-p1363" }).toString("base64url")}`;
    const primary = await FakeAnthropic.start();
    const secondary = await FakeAnthropic.start();
    let gateway: GatewayProcess | undefined;
    try {
      gateway = await GatewayProcess.start(binaryPath, primary.url, [`--auth.jwks-url=http://127.0.0.1:${address.port}/jwks`], {}, "access-token", undefined, secondary.url);
      const base = { baseURL: `${gateway.url}/api/v1/aisdk`, accessToken: token, modelID: "assistant" };
      const discovery = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery" });
      assert.equal(discovery.error, undefined);
      const client = gateway.client(token);
      const models = await client.getAvailableModels();
      assert.deepEqual(models.models.map(model => model.id).sort(), discovery.models.map((model: { id: string }) => model.id).sort());
      const denied = await captureGoClient(goClientBinaryPath, { ...base, accessToken: "invalid", mode: "generate", options: { prompt: [] } });
      assert.equal(denied.error?.statusCode, 401);
      assert.equal(primary.requests.length, 0);
      assert.equal(secondary.requests.length, 0);
      const call = { role: "assistant" as const, content: [{ type: "tool-call" as const, toolCallId: "private-call", toolName: "private-tool", input: { secret: "private-input" } }] };
      const result = { role: "tool" as const, content: [{ type: "tool-result" as const, toolCallId: "private-call", toolName: "private-tool", output: { type: "text" as const, value: "private-result" } }] };
      const effectRequests: LanguageModelV4CallOptions[] = [
        { prompt: [], tools: [{ type: "function", name: "private-tool", inputSchema: { type: "object" } }] },
        ...(["auto", "none", "required"] as const).map(type => ({ prompt: [], toolChoice: { type } })),
        { prompt: [], toolChoice: { type: "tool", toolName: "private-tool" } },
        { prompt: [call] },
        { prompt: [call, result] },
      ];
      for (const options of effectRequests) {
        const counts: [number, number] = [primary.requests.length, secondary.requests.length];
        const go = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", options });
        assert.deepEqual({ status: go.error?.statusCode, category: go.error?.category, code: go.error?.code, retryable: go.error?.isRetryable }, { status: 400, category: "invalid_request_error", code: "invalid_request", retryable: false });
        assert.equal(go.result, undefined);
        let failure: any;
        try { await client("assistant").doGenerate(options); } catch (error) { failure = error; }
        assert.deepEqual({ status: failure?.statusCode, category: failure?.type, retryable: failure?.isRetryable, message: failure?.message }, { status: 400, category: "invalid_request_error", retryable: false, message: "invalid request" });
        const raw: Response = await fetch(`${gateway.url}/api/v1/aisdk/language-model`, {
          method: "POST",
          headers: { "content-type": "application/json", "x-access-token": token, "ai-language-model-specification-version": "4", "ai-language-model-id": "assistant", "ai-language-model-streaming": "false" },
          body: JSON.stringify(options),
        });
        assert.equal(raw.status, 400);
        assert.equal(await raw.text(), '{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"}}');
        assert.deepEqual([primary.requests.length, secondary.requests.length], counts, "unary effects must not invoke either fallback candidate");
        assertPrivateValuesAbsent([go.error, { message: failure.message, type: failure.type, responseBody: failure.responseBody }], primary, [secondary.url, "private-call", "private-tool", "private-input", "private-result", token]);
      }
      const streamCall = { type: "tool-call" as const, toolCallId: "private-call", toolName: "private-tool", input: {} };
      const history = [{ role: "assistant" as const, content: [streamCall] }];
      for (const options of [
        { prompt: [], tools: [{ type: "function" as const, name: "private-tool", inputSchema: {} }] },
        { prompt: [], toolChoice: { type: "none" as const } },
        { prompt: history },
        { prompt: [...history, { role: "tool" as const, content: [{ type: "tool-result" as const, toolCallId: streamCall.toolCallId, toolName: streamCall.toolName, output: { type: "text" as const, value: "private-result" } }] }] },
      ]) {
        const go = await captureGoClient(goClientBinaryPath, { ...base, mode: "stream", options });
        assert.equal(go.error?.statusCode, 400);
        assert.equal(go.error?.code, "invalid_request");
        assert.equal(go.error?.message, "invalid request");
        assert.equal(go.error?.isRetryable, false);
        assert.equal(go.parts, undefined, "fallback rejection must precede SSE commitment");
        await assert.rejects(async () => await client("assistant").doStream(options), (error: any) => {
          assert.equal(error.statusCode, 400);
          assert.equal(error.message, "invalid request");
          assert.equal(error.isRetryable, false);
          return true;
        });
        assert.equal(primary.requests.length, 0, "streaming tools/history must not invoke primary");
        assert.equal(secondary.requests.length, 0, "streaming tools/history must not invoke fallback");
      }
      for (const row of [
        { primary: undefined, secondary: undefined, count: 0, status: undefined },
        { primary: 503, secondary: undefined, count: 1, status: undefined },
        { primary: 400, secondary: undefined, count: 0, status: 424 },
        { primary: 503, secondary: 503, count: 1, status: 503 },
      ]) {
        primary.failureStatus = row.primary;
        secondary.failureStatus = row.secondary;
        for (const mode of ["generate", "stream"] as const) {
          const options = { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "normal-stream" }] }], maxOutputTokens: 32 };
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
          assertPrivateValuesAbsent([result, failure, go, discovery, models], primary, [secondary.url, "anthropic-secondary", token]);
        }
      }
      assert.ok(keyRequests > 0);
      assert.deepEqual(primary.violations, []);
      assert.deepEqual(secondary.violations, []);
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      await gateway.stop();
      const logicalLogs = gateway.stderr.split("\n").filter(line => !line.includes('"event":"gateway_physical_attempt"')).join("\n");
      assertPrivateValuesAbsent([logicalLogs, metrics], primary, [secondary.url, "anthropic-secondary", token]);
    } finally {
      await settleCleanup(...(gateway ? [() => gateway!.stop()] : []), () => primary.stop(), () => secondary.stop(), () => new Promise<void>(resolve => jwks.close(() => resolve())));
    }
  });

  it("verifies JWKS auth and native function continuation for both clients and modes", async () => {
    const observer = await FakeAgentObservability.start();
    const keys = generateKeyPairSync("ec", { namedCurve: "P-256" });
    const jwk = { ...keys.publicKey.export({ format: "jwk" }), kid: "tool-test", alg: "ES256", use: "sig" };
    let keyRequests = 0;
    const jwks = createServer((_request, response) => {
      keyRequests++;
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ keys: [jwk] }));
    });
    await new Promise<void>(resolve => jwks.listen(0, "127.0.0.1", resolve));
    const address = jwks.address();
    assert.ok(address && typeof address !== "string");
    const header = Buffer.from(JSON.stringify({ alg: "ES256", typ: "at+jwt", kid: "tool-test" })).toString("base64url");
    const payload = TEST_TOKEN.split(".")[1];
    const unsigned = `${header}.${payload}`;
    const token = `${unsigned}.${sign("sha256", Buffer.from(unsigned), { key: keys.privateKey, dsaEncoding: "ieee-p1363" }).toString("base64url")}`;
    const corruptedSignature = Buffer.from(token.split(".")[2], "base64url");
    corruptedSignature[0] ^= 1;
    const invalidToken = `${unsigned}.${corruptedSignature.toString("base64url")}`;
    let resources: [FakeAnthropic, GatewayProcess] | undefined;
    try {
      resources = await startGateway([
        `--auth.jwks-url=http://127.0.0.1:${address.port}/jwks`,
        "--agento11y.enabled", "--agento11y.protocol=http", `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls", "--agento11y.auth-secret-env=GATEWAY_TEST_AGENTO11Y_KEY",
        "--agento11y.batch-size=1", "--agento11y.flush-interval=1ms",
        "--agento11y.flush-timeout=2s", "--agento11y.shutdown-timeout=2s",
      ], { GATEWAY_TEST_AGENTO11Y_KEY: "integration-agento11y-key" }, "claude-sonnet-4-6");
      const [fake, gateway] = resources;
      fake.functionTools = true;
      const client = gateway.client(token);
      const tools = [{ type: "function" as const, name: "weather", inputSchema: { type: "object" as const, properties: { city: { type: "string" } }, required: ["city"] }, strict: false, inputExamples: [{ input: { city: "Rio" } }] }];
      const prompt = [{ role: "user" as const, content: [{ type: "text" as const, text: "Weather in Rio?" }] }];
      const toolChoice = { type: "tool" as const, toolName: "weather" };
      const options = { prompt, tools, toolChoice, maxOutputTokens: 64 };
      await assert.rejects(async () => gateway.client(invalidToken)("assistant").doGenerate(options), (error: any) => {
        assert.equal(error.statusCode, 401);
        assert.equal(error.type, "authentication_error");
        return true;
      });
      assert.ok(keyRequests > 0, "signature rejection must resolve the retained key ID through JWKS");
      assert.equal(fake.requests.length, 0, "invalid signature must not reach provider");
      let executions = 0;
      for (const mode of ["generate", "stream"] as const) {
        for (const implementation of ["vercel", "go"] as const) {
          const invoke = async (requestOptions: any): Promise<any[]> => {
            if (implementation === "go") {
              const result = await captureGoClient(goClientBinaryPath, { baseURL: `${gateway.url}/api/v1/aisdk`, accessToken: token, modelID: "assistant", mode, options: requestOptions });
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
      assert.ok(keyRequests > 0);
      assert.deepEqual(fake.violations, []);
      fake.failureStatus = 500;
      await assert.rejects(async () => client("assistant").doGenerate(options), (error: any) => error.statusCode === 502);
      await observer.waitForGenerations(5);
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      await gateway.stop();
      assert.equal(observer.generations.length, 5, "each unary invocation finalizes one independent generation");
      assert.deepEqual(observer.violations, []);
      const successes = observer.generations.filter(generation => !generation.call_error);
      assert.equal(successes.length, 4);
      assert.deepEqual(successes.map(generation => generation.stop_reason).sort(), ["stop", "stop", "tool-calls", "tool-calls"]);
      assert.equal(new Set(observer.generations.map(generation => generation.id)).size, 5);
      for (const generation of observer.generations) {
        assert.deepEqual(generation.model, { provider: "grafana", name: "grafana/assistant" });
        assert.equal(typeof (generation.metadata as Record<string, unknown>)["gateway.correlation_id"], "string");
      }
      for (const generation of successes) {
        assert.equal((generation.usage as Record<string, unknown>).input_tokens, "2");
        assert.equal((generation.usage as Record<string, unknown>).output_tokens, "3");
      }
      assert.equal(observer.generations.filter(generation => generation.call_error === "server_error").length, 1);
      assertPrivateValuesAbsent([observer.generations, gateway.stderr, metrics], fake, [
        observer.url, token, "integration-agento11y-key", "call-weather", "weather", "Rio", "sunny", "claude-sonnet-4-6",
      ]);
    } finally {
      await settleCleanup(...(resources ? [() => resources![1].stop(), () => resources![0].stop()] : []), () => observer.stop(), async () => { jwks.closeAllConnections(); await new Promise<void>(resolve => jwks.close(() => resolve())); });
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
          const go = await captureGoClient(goClientBinaryPath, { baseURL: `${gateway.url}/api/v1/aisdk`, accessToken: TEST_TOKEN, userIDToken: TEST_USER_TOKEN, mode, modelID: "assistant", options });
          assert.deepEqual({ status: go.error?.statusCode, category: go.error?.category, code: go.error?.code, retryable: go.error?.isRetryable }, { status: row.status, category: row.category, code: row.code, retryable: row.retryable });
          let ts: any;
          try {
            const model = gateway.client()("assistant");
            await (mode === "generate" ? model.doGenerate(options) : model.doStream(options));
          } catch (error) { ts = error; }
          assert.ok(ts);
          assert.deepEqual({ status: go.error.statusCode, category: go.error.category, retryable: go.error.isRetryable }, { status: ts.statusCode, category: ts.type, retryable: ts.isRetryable });
          assertPrivateValuesAbsent([go, ts], fake);
        }
      }
      assert.equal(fake.requests.length, rows.length * 4, "neither client nor service should retry model calls");
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      await gateway.stop();
      assertPrivateValuesAbsent([gateway.stderr, metrics], fake);
      assert.deepEqual(fake.violations, []);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("returns Go internal error for bounded discovery and cancellation on process shutdown", async () => {
    const [fake, gateway] = await startGateway(["--discovery.response-bytes=256"]);
    const base = { baseURL: `${gateway.url}/api/v1/aisdk`, accessToken: TEST_TOKEN };
    try {
      const discovery = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery" });
      assert.equal(discovery.error?.statusCode, 500);
      assert.equal(discovery.error?.code, "internal_error");
      const pending = captureGoClient(goClientBinaryPath, { ...base, mode: "generate", modelID: "assistant", options: { prompt: [{ role: "user", content: [{ type: "text", text: "silent-unary-shutdown" }] }], maxOutputTokens: 32 } });
      await poll(async () => fake.requests.some((request) => JSON.stringify(request.body).includes("silent-unary-shutdown")), 5_000, "pending unary before shutdown");
      const stopped = gateway.stop();
      const canceled = await pending;
      assert.equal(canceled.error?.statusCode, 499);
      assert.equal(canceled.error?.code, "canceled");
      assert.equal(canceled.error?.isRetryable, false);
      await stopped;
      assertPrivateValuesAbsent([discovery, canceled, gateway.stderr], fake);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("serves Go discovery, canonical/alias unary, streaming, cancellation, and public errors", async () => {
    const [fake, gateway] = await startGateway();
    const base = { baseURL: `${gateway.url}/api/v1/aisdk`, accessToken: TEST_TOKEN };
    try {
      const discovery = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery" });
      assert.equal(discovery.error, undefined);
      assert.deepEqual(discovery.models.map((model: { id: string }) => model.id), ["assistant", "grafana/assistant"]);
      const actingUser = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery", userIDToken: TEST_USER_TOKEN });
      assert.equal(actingUser.error, undefined); assert.equal(actingUser.models.length, 2);
      const invalidUser = await captureGoClient(goClientBinaryPath, { ...base, mode: "discovery", userIDToken: "invalid-user-token" });
      assert.equal(invalidUser.error.category, "authentication_error");
      for (const modelID of ["assistant", "grafana/assistant"]) {
        const result = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", modelID, options: { prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }], maxOutputTokens: 32, temperature: 0.2 } });
        assert.equal(result.error, undefined);
        assert.deepEqual(result.result.content, [{ type: "text", text: "hello from fake Anthropic" }]);
        assert.equal(result.result.response.modelId, undefined);
        assertPrivateValuesAbsent(result, fake);
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
      const invalid = await captureGoClient(goClientBinaryPath, { ...base, mode: "generate", modelID: "assistant", options: { prompt: [], headers: { "x-call": "unsupported" } } });
      assert.equal(invalid.error.category, "invalid_request_error"); assert.equal(invalid.error.statusCode, 400);
      const unauthorized = await captureGoClient(goClientBinaryPath, { ...base, accessToken: "invalid-token", mode: "discovery" });
      assert.equal(unauthorized.error.category, "authentication_error"); assert.equal(unauthorized.error.statusCode, 401);
      assert.deepEqual(fake.violations, []); assert.equal(await gateway.ready(), true);
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      await gateway.stop();
      assertPrivateValuesAbsent([discovery, actingUser, invalidUser, stream, abort, missing, invalid, unauthorized, metrics, gateway.stderr], fake, ["invalid-user-token", "invalid-token"]);
    } finally { await settleCleanup(() => gateway.stop(), () => fake.stop()); }
  });

  it("exchanges a CAP token through the Go client before authenticated discovery", async () => {
    const [fake, gateway] = await startGateway();
    let exchanges = 0;
    const exchange = createServer(async (request, response) => {
      exchanges++; assert.equal(request.headers.authorization, "Bearer integration-cap");
      assert.equal(request.url, "/exchange/");
      const chunks: Buffer[] = []; for await (const chunk of request) chunks.push(Buffer.from(chunk));
      assert.deepEqual(JSON.parse(Buffer.concat(chunks).toString()), { namespace: "stack-integration", audiences: ["ai-sdk"] });
      response.writeHead(200, { "content-type": "application/json" }); response.end(JSON.stringify({ data: { token: TEST_TOKEN } }));
    });
    await new Promise<void>((resolve) => exchange.listen(0, "127.0.0.1", resolve));
    try {
      const address = exchange.address(); assert.ok(address && typeof address !== "string");
      const result = await captureGoClient(goClientBinaryPath, { mode: "discovery", cloud: { CAPToken: "integration-cap", Namespace: "stack-integration", BaseURL: `${gateway.url}/api/v1/aisdk`, TokenExchangeURL: `http://127.0.0.1:${address.port}/exchange/` } });
      assert.equal(result.error, undefined); assert.equal(result.models.length, 2); assert.equal(exchanges, 1);
      assert.equal(fake.requests.length, 0);
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      await gateway.stop();
      assertPrivateValuesAbsent([result, metrics, gateway.stderr], fake, [`http://127.0.0.1:${address.port}/exchange/`]);
    } finally {
      await new Promise<void>((resolve, reject) => exchange.close((error) => error ? reject(error) : resolve()));
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });

  it("discovers and invokes every canonical and alias model ID", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const rawDiscoveryResponse = await fetch(`${gateway.url}/api/v1/aisdk/config`, {
        headers: { "X-Access-Token": TEST_TOKEN },
      });
      assert.equal(rawDiscoveryResponse.status, 200);
      const rawDiscovery = await rawDiscoveryResponse.text();
      for (const privateValue of ["anthropic-primary", "backend-private", "GATEWAY_TEST_ANTHROPIC_KEY", "integration-anthropic-key", fake.url]) {
        assert.ok(!rawDiscovery.includes(privateValue));
      }
      const client = gateway.client();
      const metadata = await client.getAvailableModels();
      assert.deepEqual(metadata.models.map((model) => model.id), ["assistant", "grafana/assistant"]);
      for (const row of metadata.models) {
        assert.deepEqual(row.specification, {
          specificationVersion: "v4",
          provider: "grafana",
          modelId: row.id,
        });
        const result = await client(row.id).doGenerate({
          prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }],
          maxOutputTokens: 32,
          temperature: 0.2,
        });
        assert.deepEqual(result.content, [{ type: "text", text: "hello from fake Anthropic" }]);
      }
      const rawUnaryResponse = await rawProviderWireRequest(gateway.url, "unary");
      assert.equal(rawUnaryResponse.status, 200);
      const rawUnary = await rawUnaryResponse.json() as Record<string, unknown>;
      assert.deepEqual(Object.keys(rawUnary).sort(), ["content", "finishReason", "usage"]);

      assert.equal(fake.requests.length, 3);
      for (const request of fake.requests) {
        assert.equal(request.path, "/v1/messages?beta=true");
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

  it("cancels a fresh established stream and exits on SIGTERM", async () => {
    const [fake, gateway] = await startGateway();
    try {
      const result = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-shutdown" }] }],
        maxOutputTokens: 32,
      });
      const first = await result.stream.getReader().read();
      assert.equal(first.done, false);
      assert.equal(first.value?.type, "stream-start");
      const stopped = gateway.stop("SIGTERM");
      await fake.waitForCancellation("silent-shutdown");
      assert.equal(await gateway.ready(), false);
      await stopped;
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
        "--observation.region=test-region",
        "--observation.application=test-application",
        "--agento11y.enabled",
        "--agento11y.protocol=http",
        `--agento11y.endpoint=${observer.url}`,
        "--no-agento11y.tls",
        "--agento11y.auth-secret-env=GATEWAY_TEST_AGENTO11Y_KEY",
        "--agento11y.queue-size=16",
        "--agento11y.batch-size=1",
        "--agento11y.payload-max-bytes=1048576",
        "--agento11y.max-retries=1",
        "--agento11y.initial-backoff=1ms",
        "--agento11y.max-backoff=1ms",
        "--agento11y.flush-interval=1ms",
        "--agento11y.flush-timeout=2s",
        "--agento11y.shutdown-timeout=2s",
      ], { GATEWAY_TEST_AGENTO11Y_KEY: "integration-agento11y-key" });
      const [fake, gateway] = resources;

      const unary = await gateway.client()("assistant").doGenerate({
        prompt: [{ role: "user", content: [{ type: "text", text: "private-unary-input" }] }],
        maxOutputTokens: 32,
      });
      assert.deepEqual(unary.content, [{ type: "text", text: "hello from fake Anthropic" }]);

      const streamed = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "normal-stream" }] }],
        maxOutputTokens: 32,
      });
      const streamParts = await collectGatewayStream(streamed.stream);
      assert.equal(streamParts[0]?.type, "stream-start");
      assert.deepEqual(streamParts.map((part) => part.type), [
        "stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish",
      ]);
      assert.equal(streamParts.filter((part) => part.type === "text-delta").map((part) => part.delta).join(""), "hello from fake Anthropic stream");

      const providerFailure = await rawProviderWireRequest(gateway.url, "provider-error");
      assert.equal(providerFailure.status, 502);
      assert.deepEqual(await providerFailure.json(), {
        error: { message: "upstream failure", type: "internal_server_error", param: null, code: "upstream_error" },
      });

      const aborted = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-abort" }] }],
        maxOutputTokens: 32,
      });
      const abortReader = aborted.stream.getReader();
      const abortStart = await abortReader.read();
      assert.equal(abortStart.done, false);
      assert.equal(abortStart.value?.type, "stream-start");
      await abortReader.cancel("integration abort");
      await fake.waitForCancellation("silent-abort");

      const shuttingDown = await gateway.client()("assistant").doStream({
        prompt: [{ role: "user", content: [{ type: "text", text: "silent-shutdown" }] }],
        maxOutputTokens: 32,
      });
      const shutdownStart = await shuttingDown.stream.getReader().read();
      assert.equal(shutdownStart.done, false);
      assert.equal(shutdownStart.value?.type, "stream-start");

      await observer.waitForGenerations(4);
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      const canonicalLabels = ['provider="grafana"', 'model="grafana/assistant"'];
      assert.equal(sumPrometheusSamples(metrics, "aisdk_model_requests_total", canonicalLabels), 4);
      assert.equal(sumPrometheusSamples(metrics, "aisdk_model_inflight_requests", canonicalLabels), 1);
      assert.ok(metrics.includes('operation="generate"'));
      assert.ok(metrics.includes('operation="stream"'));
      assert.ok(metrics.includes('status="success"'));
      assert.ok(metrics.includes('status="error"'));
      assert.ok(metrics.includes('status="canceled"'));
      assert.ok(!metrics.includes("grafana_ai_gateway_agento11y_export_failures_total{"));

      await gateway.stop("SIGTERM");
      await fake.waitForCancellation("silent-shutdown");
      await observer.waitForGenerations(5);
      assert.ok(!gateway.stderr.includes('"msg":"agent observability export failed"'));
      assert.equal(fake.requests.length, 5);
      assert.deepEqual(fake.violations, []);
      assert.deepEqual(observer.violations, []);
      assert.equal(observer.generations.length, 5);
      const allowedMetadata = new Set([
        "gateway.application", "gateway.caller_service", "gateway.correlation_id", "gateway.namespace", "gateway.region",
        "agento11y.sdk.content_capture_mode", "agento11y.sdk.name", "call_error",
      ]);
      for (const generation of observer.generations) {
        assert.deepEqual(generation.model, { provider: "grafana", name: "grafana/assistant" });
        assert.equal(generation.agent_name ?? "", "");
        assert.equal(generation.user_id ?? "", "");
        const metadata = generation.metadata as Record<string, unknown>;
        assert.equal(metadata["gateway.application"], "test-application");
        assert.equal(metadata["gateway.caller_service"], "integration-service");
        assert.equal(metadata["gateway.namespace"], "stack-integration");
        assert.equal(metadata["gateway.region"], "test-region");
        assert.equal(typeof metadata["gateway.correlation_id"], "string");
        assert.ok(Object.keys(metadata).every((key) => allowedMetadata.has(key)));
        assert.ok([undefined, "server_error", "canceled"].includes(metadata.call_error as string | undefined));
      }
      const surfaces = JSON.stringify({ metrics, logs: gateway.stderr, generations: observer.generations });
      for (const privateValue of [
        "integration-anthropic-key", "integration-agento11y-key", "GATEWAY_TEST_AGENTO11Y_KEY",
        "backend-private", fake.url, observer.url, TEST_TOKEN, "private-unary-input",
        "hello from fake Anthropic", "provider-secret-response",
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
      "--agento11y.auth-secret-env=GATEWAY_TEST_AGENTO11Y_KEY",
      "--agento11y.queue-size=4",
      "--agento11y.batch-size=1",
      "--agento11y.payload-max-bytes=1048576",
      "--agento11y.max-retries=1",
      "--agento11y.initial-backoff=1ms",
      "--agento11y.max-backoff=1ms",
      "--agento11y.flush-interval=1ms",
      "--agento11y.flush-timeout=100ms",
      "--agento11y.shutdown-timeout=100ms",
    ], { GATEWAY_TEST_AGENTO11Y_KEY: "outage-agento11y-key" });
    try {
      const response = await rawProviderWireRequest(gateway.url, "outage-private-input");
      assert.equal(response.status, 200);
      const body = await response.json() as Record<string, unknown>;
      assert.deepEqual(Object.keys(body).sort(), ["content", "finishReason", "usage"]);
      let metrics = "";
      await poll(async () => {
        metrics = await (await fetch(`${gateway.url}/metrics`)).text();
        return metrics.includes('grafana_ai_gateway_agento11y_export_failures_total{class="transport"} 1');
      }, 5_000, "Agent Observability transport failure metric");
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

  it("rejects alternate auth, duplicate tokens, overflow, redirects, and private telemetry", async () => {
    const redirectTarget = await FakeAnthropic.start();
    let resources: [FakeAnthropic, GatewayProcess] | undefined;
    try {
      resources = await startGateway(["--discovery.response-bytes=256"]);
      const [fake, gateway] = resources;
      const authorizationOnly = await fetch(`${gateway.url}/api/v1/aisdk/config`, {
        headers: { Authorization: `Bearer ${TEST_TOKEN}` },
      });
      assert.equal(authorizationOnly.status, 401);

      const duplicateHeaders = new Headers();
      duplicateHeaders.append("X-Access-Token", TEST_TOKEN);
      duplicateHeaders.append("x-access-token", TEST_TOKEN);
      const duplicate = await fetch(`${gateway.url}/api/v1/aisdk/config`, { headers: duplicateHeaders });
      assert.equal(duplicate.status, 401);

      const discovery = await fetch(`${gateway.url}/api/v1/aisdk/config`, {
        headers: { "X-Access-Token": TEST_TOKEN },
      });
      assert.equal(discovery.status, 500);
      const discoveryBody = await discovery.text();

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

      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      assert.equal(await gateway.ready(), true);
      await gateway.stop();
      const logs = gateway.stderr;
      assert.deepEqual(processLifecycleEvents(logs), [
        "process_starting",
        "process_ready",
        "process_shutdown_started",
        "process_shutdown_completed",
      ]);
      for (const privateValue of [
        "integration-anthropic-key",
        "backend-private",
        fake.url,
        TEST_TOKEN,
        "authorization-is-ignored",
      ]) {
        assert.ok(!discoveryBody.includes(privateValue));
        assert.ok(!redirectError.includes(privateValue));
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

  it("fails malformed scalar startup before readiness without leaking configuration", async () => {
    const fake = await FakeAnthropic.start();
    try {
      let failure = "";
      try {
        await GatewayProcess.start(binaryPath, fake.url, ["--server.listen-address=not-a-tcp-address"]);
      } catch (error) {
        failure = String(error);
      }
      assert.ok(failure.includes("gateway exited unsuccessfully"));
      assert.deepEqual(processLifecycleEvents(failure), ["process_starting"]);
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
    assert.ok(source.includes("spawn(binary, args"));
    assert.doesNotMatch(imports, /cmd\/grafana-ai-gateway\/internal/);
    assert.doesNotMatch(imports, /providerwire|gateway\/catalog/);
  });

  it("bounds oversized Anthropic failures without exposing response data", async () => {
    const [fake, gateway] = await startGateway(["--anthropic.response-bytes=128"]);
    fake.oversizedErrors = true;
    try {
      const response = await rawProviderWireRequest(gateway.url, "oversized");
      assert.ok(response.status >= 500);
      const publicError = await response.text();
      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      assert.equal(await gateway.ready(), true);
      assert.deepEqual(fake.violations, []);
      await gateway.stop();
      const logs = gateway.stderr;
      for (const privateValue of ["provider-secret-response", "integration-anthropic-key", "backend-private", fake.url, TEST_TOKEN]) {
        assert.ok(!publicError.includes(privateValue));
        assert.ok(!logs.includes(privateValue));
        assert.ok(!metrics.includes(privateValue));
      }
    } finally {
      await settleCleanup(() => gateway.stop(), () => fake.stop());
    }
  });
});

describe("Trusted-proxy composition (dummy credentials, not production authentication)", () => {
  it("uses only the edge stack assertion for read discovery and write unary/stream, never forwarding customer credentials to Anthropic", async () => {
    const [fake, gateway, edge] = await startCloudGateway();
    const spoofed = {
      "X-Scope-OrgID": "client-invalid-stack",
      "X-Cloud-Org-ID": "client-invalid-organization",
      "X-Access-Policy-ID": "client invalid policy",
      "X-Access-Token": TEST_TOKEN,
      "X-Grafana-Id": "customer-internal-id-token",
      "x-api-key": "customer-provider-key",
    };
    try {
      const direct = await rawHTTPRequest(`${gateway.url}/api/v1/aisdk/config`, "GET",
        Object.entries(spoofed).filter(([name]) => !["X-Access-Token", "X-Grafana-Id"].includes(name)));
      assertAppAuthenticationFailure(direct);
      const metadata = await edge.client(EDGE_READ_KEY, spoofed).getAvailableModels();
      assert.deepEqual(metadata.models.map((model) => model.id), ["assistant", "grafana/assistant"]);
      const client = edge.client(EDGE_WRITE_KEY, spoofed);
      const unary = await client("assistant").doGenerate(cloudCall("unary"));
      assert.deepEqual(unary.content, [{ type: "text", text: "hello from fake Anthropic" }]);
      const stream = await client("grafana/assistant").doStream(cloudCall("normal-stream"));
      const parts = await withTimeout(collectStream(stream.stream), 5_000, "Cloud stream EOF");
      assert.equal(parts[0]?.type, "stream-start");
      assert.equal(parts.at(-1)?.type, "finish");
      assert.equal(parts.filter((part) => part.type === "text-delta").map((part) => part.delta).join(""),
        "hello from fake Anthropic stream");
      assert.equal(edge.forwarded.length, 3);
      assert.deepEqual(edge.forwarded.map((request) => request.path), [
        "/api/v1/aisdk/config", "/api/v1/aisdk/language-model", "/api/v1/aisdk/language-model",
      ]);
      for (const request of edge.forwarded) {
        for (const [name, value] of EDGE_ASSERTIONS) {
          assert.deepEqual(request.headers.filter(([key]) => key.toLowerCase() === name.toLowerCase()), [[name, value]]);
        }
        assert.ok(!request.headers.some(([name]) => PROHIBITED_HEADERS.includes(name.toLowerCase())));
        for (const name of ["x-cloud-org-id", "x-access-policy-id", "x-api-key"]) {
          assert.ok(!request.headers.some(([key]) => key.toLowerCase() === name));
        }
      }
      assert.equal(edge.received[0]?.headers.authorization, `Bearer ${EDGE_READ_KEY}`);
      assert.equal(edge.received[1]?.headers.authorization, `Bearer ${EDGE_WRITE_KEY}`);
      assert.equal(edge.received[1]?.headers["x-access-token"], TEST_TOKEN);
      assert.equal(edge.received[1]?.headers["x-api-key"], spoofed["x-api-key"]);
      assert.equal(fake.requests.length, 2);
      for (const request of fake.requests) {
        assert.equal(request.apiKey, "integration-anthropic-key");
        assert.equal(request.body.max_tokens, 32);
        for (const name of [...PROHIBITED_HEADERS, ...EDGE_ASSERTIONS.map(([name]) => name.toLowerCase()), "x-cloud-org-id", "x-access-policy-id"]) {
          assert.equal(request.headers[name], undefined);
        }
        assertCloudPrivateValuesAbsent(JSON.stringify({ headers: request.headers, body: request.body }), [
          ...CLOUD_PRIVATE_VALUES, ...Object.values(spoofed),
        ]);
      }
      assert.deepEqual(fake.violations, []);
      const ignoredHeadersDiscovery = await rawHTTPRequest(`${gateway.url}/api/v1/aisdk/config`, "GET", [
        ...EDGE_ASSERTIONS,
        ["X-Cloud-Org-ID", spoofed["X-Cloud-Org-ID"]],
        ["X-Access-Policy-ID", spoofed["X-Access-Policy-ID"]],
      ]);
      assert.equal(ignoredHeadersDiscovery.status, 200);
      assert.equal(fake.requests.length, 2);
      const metrics = await gateway.metrics();
      assert.equal(authenticationCount(metrics, "authenticated"), 4);
      assert.equal(authenticationCount(metrics, "authentication_failed"), 1);
      await gateway.stop();
      assertCloudPrivateValuesAbsent([direct.body, ignoredHeadersDiscovery.body, metrics, gateway.stderr].join("\n"), [
        ...CLOUD_PRIVATE_VALUES, ...Object.values(spoofed), fake.url, "backend-private", "integration-anthropic-key",
      ]);
    } finally {
      await settleCleanup(() => edge.stop(), () => gateway.stop(), () => fake.stop());
    }
  });

  it("denies read-only inference, write-only discovery, and invalid keys at the shim without app or provider calls", async () => {
    const [fake, gateway, edge] = await startCloudGateway();
    try {
      const before = protectedMetrics(await gateway.metrics());
      for (const [key, operation] of [
        [EDGE_READ_KEY, "unary"], [EDGE_READ_KEY, "stream"], [EDGE_WRITE_KEY, "discovery"],
        [EDGE_INVALID_KEY, "discovery"], [EDGE_INVALID_KEY, "unary"], [EDGE_INVALID_KEY, "stream"],
      ] as const) {
        const client = edge.client(key);
        await assert.rejects(withTimeout(
          Promise.resolve<unknown>(operation === "discovery" ? client.getAvailableModels()
            : operation === "unary" ? client("assistant").doGenerate(cloudCall("unary"))
              : client("assistant").doStream(cloudCall("normal-stream"))),
          5_000, "edge denial",
        ), (error: unknown) => {
          assert.equal((error as { statusCode?: number }).statusCode, key === EDGE_INVALID_KEY ? 401 : 403);
          return true;
        });
        assert.equal(edge.forwarded.length, 0);
        assert.equal(fake.requests.length, 0);
        assert.deepEqual(protectedMetrics(await gateway.metrics()), before);
      }
      assert.equal(edge.denied, 6);
      assert.equal(edge.received.length, 6);
    } finally {
      await settleCleanup(() => edge.stop(), () => gateway.stop(), () => fake.stop());
    }
  });

  it("rejects controlled post-replacement malformed assertions and surviving credentials with the exact app error", async () => {
    const [fake, gateway, edge] = await startCloudGateway();
    try {
      const cases: Array<{ name: string; mutate: HeaderMutation }> = [];
      for (const [name, value] of EDGE_ASSERTIONS) {
        const coalesced = new Headers();
        coalesced.append(name, value);
        coalesced.append(name.toLowerCase(), value);
        assert.equal(coalesced.get(name), `${value}, ${value}`);
        for (const [kind, replacement] of [
          ["missing", []], ["empty", [[name, ""]]],
          ["actual duplicate lines", [[name, value], [name, value]]],
          ["actual case-colliding lines", [[name, value], [name.toLowerCase(), value]]],
          ["fetch Headers comma-coalesced value (not duplicate lines)", [[name, coalesced.get(name)!]]],
        ] as Array<[string, HeaderPairs]>) {
          cases.push({ name: `${name}: ${kind}`, mutate: (headers) => replaceHeader(headers, name, replacement) });
        }
      }
      for (const value of ["0", "-1", "+1", "1.5", "1e3", "9223372036854775808", "invalid-private-number"]) {
        cases.push({ name: `X-Scope-OrgID: ${value}`, mutate: (headers) => replaceHeader(headers, "X-Scope-OrgID", [["X-Scope-OrgID", value]]) });
      }
      for (const [name, value] of [["Authorization", `Bearer ${EDGE_WRITE_KEY}`], ["X-Access-Token", TEST_TOKEN], ["X-Grafana-Id", "surviving-private-id"]]) {
        for (const credential of [value!, ""]) {
          cases.push({ name: `surviving ${name}`, mutate: (headers) => [...headers, [name!, credential]] });
        }
      }
      let rejected = 0;
      for (const testCase of cases) {
        edge.mutate = testCase.mutate;
        for (const operation of ["discovery", "unary", "stream"] as const) {
          const response = await edgeRawRequest(edge, operation);
          assertAppAuthenticationFailure(response, testCase.name);
          rejected++;
          assert.equal(edge.forwarded.length, rejected);
          assert.equal(fake.requests.length, 0, testCase.name);
        }
      }
      const metrics = await gateway.metrics();
      assert.equal(authenticationCount(metrics, "authentication_failed"), rejected);
      assert.equal(authenticationCount(metrics, "authenticated"), 0);
      const authLines = metrics.split("\n").filter((line) => line.startsWith("grafana_ai_gateway_authentication_total{"));
      assert.deepEqual(authLines, [`grafana_ai_gateway_authentication_total{outcome="authentication_failed",source="cloud-gateway"} ${rejected}`]);
      await gateway.stop();
      assert.equal(edge.appErrors.length, rejected);
      assert.ok(edge.appErrors.every((body) => body === APP_AUTHENTICATION_JSON));
      assertCloudPrivateValuesAbsent([metrics, gateway.stderr, ...edge.appErrors].join("\n"), [
        ...CLOUD_PRIVATE_VALUES, "surviving-private-id", "invalid-private-number",
      ]);
      const completions = gateway.stderr.split("\n").filter(Boolean).map((line) => JSON.parse(line) as Record<string, unknown>)
        .filter((record) => record.msg === "http request completed" && ["config", "language_model"].includes(String(record.route)));
      assert.equal(completions.length, rejected);
      assert.ok(completions.every((record) => record.authentication_source === "cloud-gateway" && record.authentication === "authentication_failed"));
    } finally {
      await settleCleanup(() => edge.stop(), () => gateway.stop(), () => fake.stop());
    }
  });

  it("starts without JWKS, separates operational routes, flushes, cancels, and exits both listeners on SIGTERM", async () => {
    const [fake, gateway, edge] = await startCloudGateway([
      "--auth.jwks-timeout=0s", "--auth.jwks-response-bytes=0", "--auth.jwks-max-keys=0", "--auth.audiences=",
    ]);
    try {
      assert.notEqual(gateway.url, gateway.operationalURL);
      assert.equal(fake.requests.length, 0);
      for (const path of ["/live", "/ready", "/metrics"]) {
        assert.equal((await rawHTTPRequest(`${gateway.operationalURL}${path}`, "GET", [])).status, 200);
        assert.equal((await rawHTTPRequest(`${gateway.url}${path}`, "GET", EDGE_ASSERTIONS)).status, 404);
      }
      for (const [path, method] of [["/api/v1/aisdk/config", "GET"], ["/api/v1/aisdk/language-model", "POST"]]) {
        assert.equal((await rawHTTPRequest(`${gateway.operationalURL}${path}`, method!, EDGE_ASSERTIONS)).status, 404);
      }
      for (const marker of ["silent-abort", "silent-shutdown"]) {
        const result = await withTimeout(Promise.resolve(edge.client(EDGE_WRITE_KEY)("assistant").doStream({
          ...cloudCall(marker), abortSignal: AbortSignal.timeout(15_000),
        })), 2_000, "Cloud stream setup");
        const reader = result.stream.getReader();
        const first = await withTimeout(reader.read(), 2_000, "Cloud stream-start must flush before provider EOF");
        assert.equal(first.done, false);
        assert.equal(first.value?.type, "stream-start");
        if (marker === "silent-abort") {
          await reader.cancel("Cloud client abort");
          await fake.waitForCancellation(marker);
          assert.equal(await gateway.ready(), true);
        } else {
          const stopped = gateway.stop("SIGTERM");
          await fake.waitForCancellation(marker);
          await stopped;
          await reader.cancel().catch(() => {});
        }
      }
      assert.equal(await gateway.ready(), false);
      for (const url of [gateway.url, gateway.operationalURL]) {
        await assert.rejects(fetch(`${url}/ready`, { signal: AbortSignal.timeout(500) }));
      }
      assert.deepEqual(processLifecycleEvents(gateway.stderr), [
        "process_starting", "process_ready", "process_shutdown_started", "process_shutdown_completed",
      ]);
      assert.deepEqual(fake.violations, []);
      assertCloudPrivateValuesAbsent(gateway.stderr, CLOUD_PRIVATE_VALUES);
    } finally {
      await settleCleanup(() => edge.stop(), () => gateway.stop(), () => fake.stop());
    }
  });

  it("fails invalid Cloud startup combinations before readiness and cleans up without configuration leaks", async () => {
    const fake = await FakeAnthropic.start();
    try {
      const samePort = await availablePort();
      for (const args of [
        ["--auth.mode=invalid-private-mode"],
        ["--auth.unsafe"],
        [`--auth.jwks-url=${fake.url}/private-jwks`],
        ["--server.operational-listen-address="],
        ["--server.operational-listen-address=private-invalid-address"],
        [`--server.listen-address=127.0.0.1:${samePort}`, `--server.operational-listen-address=127.0.0.1:${samePort}`],
      ]) {
        let unexpected: GatewayProcess | undefined;
        try {
          await assert.rejects(async () => {
            unexpected = await GatewayProcess.start(binaryPath, fake.url, args, {}, "cloud-gateway");
          }, (error: unknown) => {
            const failure = String(error);
            assert.match(failure, /gateway exited unsuccessfully/);
            assert.deepEqual(processLifecycleEvents(failure), ["process_starting"]);
            assertCloudPrivateValuesAbsent(failure, [...CLOUD_PRIVATE_VALUES, fake.url, "private-jwks", "private-invalid-address", "invalid-private-mode"]);
            return true;
          });
        } finally {
          await unexpected?.stop();
        }
        assert.ok(GatewayProcess.lastFailedDirectory);
        assert.equal(existsSync(GatewayProcess.lastFailedDirectory), false);
        assert.equal(fake.requests.length, 0);
      }
    } finally {
      await fake.stop();
    }
  });
});

describe("authenticated OpenAI-compatible Gateway command", () => {
  it("discovers, invokes, and streams usage without exposing private configuration", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    try {
      const rawDiscoveryResponse = await fetch(`${gateway.url}/api/v1/aisdk/config`, {
        headers: { "X-Access-Token": TEST_TOKEN },
      });
      assert.equal(rawDiscoveryResponse.status, 200);
      const rawDiscovery = await rawDiscoveryResponse.text();
      for (const privateValue of ["compatible-primary", "compatible-backend", "backend-private", "GATEWAY_TEST_COMPATIBLE_KEY", "integration-compatible-key", fake.url]) {
        assert.ok(!rawDiscovery.includes(privateValue));
      }
      const client = gateway.client();
      const metadata = await client.getAvailableModels();
      assert.deepEqual(metadata.models.map((model) => model.id), ["compatible", "grafana/compatible"]);
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

  it("rejects redirects and hides upstream failures without exposing private configuration", async () => {
    const [fake, gateway] = await startCompatibleGateway();
    const redirectTarget = await FakeCompatible.start();
    try {
      fake.failWithSecret = true;
      const failed = await rawProviderWireRequest(gateway.url, "unary", "compatible");
      assert.ok(failed.status >= 400);
      const publicError = await failed.text();
      assert.equal(fake.requests.length, 1);

      fake.failWithSecret = false;
      fake.redirectTo = redirectTarget.url;
      const redirected = await rawProviderWireRequest(gateway.url, "unary", "compatible");
      assert.ok(redirected.status >= 400);
      const redirectError = await redirected.text();
      assert.equal(redirectTarget.requests.length, 0);
      assert.deepEqual(fake.violations, []);

      const metrics = await (await fetch(`${gateway.url}/metrics`)).text();
      assert.equal(await gateway.ready(), true);
      await gateway.stop();
      const logs = gateway.stderr;
      for (const privateValue of ["provider-secret-response", "compatible-primary", "compatible-backend", "backend-private", "integration-compatible-key", fake.url, redirectTarget.url, TEST_TOKEN]) {
        assert.ok(!publicError.includes(privateValue));
        assert.ok(!redirectError.includes(privateValue));
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

const EDGE_READ_KEY = "dummy-read-key";
const EDGE_WRITE_KEY = "dummy-write-key";
const EDGE_INVALID_KEY = "dummy-invalid-key";
const EDGE_ASSERTIONS: HeaderPairs = [
  ["X-Scope-OrgID", "17319428876500123"],
];
const PROHIBITED_HEADERS = ["authorization", "x-access-token", "x-grafana-id"];
const CLOUD_PRIVATE_VALUES = [EDGE_READ_KEY, EDGE_WRITE_KEY, EDGE_INVALID_KEY, TEST_TOKEN, ...EDGE_ASSERTIONS.map(([, value]) => value)];
const APP_AUTHENTICATION_JSON = '{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}';
type HeaderPairs = Array<[string, string]>;
type HeaderMutation = (headers: HeaderPairs) => HeaderPairs;
type RawResponse = { status: number; body: string; headers: IncomingMessage["headers"] };

function cloudCall(text: string) {
  return {
    prompt: [{ role: "user" as const, content: [{ type: "text" as const, text }] }],
    maxOutputTokens: 32,
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

function assertAppAuthenticationFailure(response: RawResponse, message?: string): void {
  assert.equal(response.status, 401, message);
  assert.equal(response.headers["content-type"], "application/json", message);
  assert.equal(response.body, APP_AUTHENTICATION_JSON, message);
}

function authenticationCount(metrics: string, outcome: string): number {
  const prefix = `grafana_ai_gateway_authentication_total{outcome="${outcome}",source="cloud-gateway"} `;
  const line = metrics.split("\n").find((line) => line.startsWith(prefix));
  return line == null ? 0 : Number(line.slice(prefix.length));
}

function protectedMetrics(metrics: string): string[] {
  return metrics.split("\n").filter((line) => line.startsWith("grafana_ai_gateway_authentication_total{") ||
    (line.startsWith("grafana_ai_gateway_http_requests_total{") && /route="(?:config|language_model)"/.test(line)));
}

function replaceHeader(headers: HeaderPairs, name: string, replacement: HeaderPairs): HeaderPairs {
  return [...headers.filter(([key]) => key.toLowerCase() !== name.toLowerCase()), ...replacement];
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

function edgeRawRequest(edge: DummyCloudEdge, operation: "discovery" | "unary" | "stream"): Promise<RawResponse> {
  const discovery = operation === "discovery";
  const body = discovery ? "" : JSON.stringify({
    prompt: [{ role: "user", content: [{ type: "text", text: "unary" }] }], maxOutputTokens: 32,
  });
  return rawHTTPRequest(`${edge.url}/api/v1/aisdk/${discovery ? "config" : "language-model"}`, discovery ? "GET" : "POST", [
    ["Authorization", `Bearer ${discovery ? EDGE_READ_KEY : EDGE_WRITE_KEY}`],
    ["Content-Type", "application/json"], ["Content-Length", String(Buffer.byteLength(body))],
    ["ai-language-model-specification-version", "4"], ["ai-language-model-id", "assistant"],
    ["ai-language-model-streaming", String(operation === "stream")],
  ], body);
}

async function startCloudGateway(extraArgs: string[] = []): Promise<[FakeAnthropic, GatewayProcess, DummyCloudEdge]> {
  const fake = await FakeAnthropic.start();
  let gateway: GatewayProcess | undefined;
  try {
    gateway = await GatewayProcess.start(binaryPath, fake.url, extraArgs, {}, "cloud-gateway");
    return [fake, gateway, await DummyCloudEdge.start(gateway.url)];
  } catch (error) {
    await settleCleanup(() => fake.stop(), ...(gateway == null ? [] : [() => gateway!.stop()]));
    throw error;
  }
}

class DummyCloudEdge {
  readonly received: Array<{ path: string; headers: IncomingMessage["headers"] }> = [];
  readonly forwarded: Array<{ path: string; headers: HeaderPairs }> = [];
  readonly appErrors: string[] = [];
  denied = 0;
  mutate: HeaderMutation = (headers) => headers;
  private readonly pending = new Set<ReturnType<typeof httpRequest>>();

  private constructor(
    private readonly server: ReturnType<typeof createServer>,
    readonly url: string,
    private readonly appURL: string,
  ) {}

  static async start(appURL: string): Promise<DummyCloudEdge> {
    let edge: DummyCloudEdge;
    const server = createServer((request, response) => edge.handle(request, response));
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (address == null || typeof address === "string") throw new Error("edge did not bind TCP");
    edge = new DummyCloudEdge(server, `http://127.0.0.1:${address.port}`, appURL);
    return edge;
  }

  client(apiKey: string, headers: Record<string, string> = {}) {
    return createGateway({ apiKey, headers, baseURL: `${this.url}/api/v1/aisdk` });
  }

  async stop(): Promise<void> {
    for (const request of this.pending) request.destroy();
    this.server.closeAllConnections();
    await new Promise<void>((resolve) => this.server.close(() => resolve()));
  }

  private handle(request: IncomingMessage, response: ServerResponse): void {
    const path = request.url ?? "/";
    this.received.push({ path, headers: { ...request.headers } });
    const authorization = request.headers.authorization;
    const known = authorization === `Bearer ${EDGE_READ_KEY}` || authorization === `Bearer ${EDGE_WRITE_KEY}`;
    const allowed = (authorization === `Bearer ${EDGE_READ_KEY}` && request.method === "GET" && path === "/api/v1/aisdk/config") ||
      (authorization === `Bearer ${EDGE_WRITE_KEY}` && request.method === "POST" && path === "/api/v1/aisdk/language-model");
    if (!allowed) {
      this.denied++;
      request.resume();
      response.writeHead(known ? 403 : 401, { "Content-Type": "application/json" });
      response.end('{"error":{"message":"dummy edge denied","type":"authentication_error"}}');
      return;
    }
    const removed = [...PROHIBITED_HEADERS, ...EDGE_ASSERTIONS.map(([name]) => name.toLowerCase()), "x-cloud-org-id", "x-access-policy-id", "x-api-key", "host", "connection"];
    const headers: HeaderPairs = [];
    for (let i = 0; i < request.rawHeaders.length; i += 2) {
      const name = request.rawHeaders[i]!;
      if (!removed.includes(name.toLowerCase())) headers.push([name, request.rawHeaders[i + 1]!]);
    }
    const replaced = this.mutate([...headers, ...EDGE_ASSERTIONS]);
    this.forwarded.push({ path, headers: replaced });
    const upstream = httpRequest(this.appURL + path, {
      method: request.method,
      headers: [["Host", new URL(this.appURL).host], ["Connection", "close"], ...replaced].flat(),
      signal: AbortSignal.timeout(15_000),
    }, (appResponse) => {
      response.writeHead(appResponse.statusCode!, appResponse.headers);
      response.flushHeaders();
      if (appResponse.statusCode === 401) {
        const chunks: Buffer[] = [];
        appResponse.on("data", (chunk: Buffer) => chunks.push(chunk));
        appResponse.on("end", () => this.appErrors.push(Buffer.concat(chunks).toString()));
      }
      appResponse.on("error", () => response.destroy());
      appResponse.pipe(response);
    });
    this.pending.add(upstream);
    upstream.once("close", () => this.pending.delete(upstream));
    upstream.on("error", () => response.destroy());
    response.once("close", () => upstream.destroy());
    request.once("aborted", () => upstream.destroy());
    request.on("error", () => upstream.destroy());
    request.pipe(upstream);
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

async function startCompatibleGateway(): Promise<[FakeCompatible, GatewayProcess]> {
  const fake = await FakeCompatible.start();
  try {
    return [fake, await GatewayProcess.start(binaryPath, "", [], {}, "access-token", compatibleConfig(fake.url))];
  } catch (error) {
    await settleCleanup(() => fake.stop());
    throw error;
  }
}

function compatibleConfig(url: string): string {
  return `providers:\n  compatible-primary:\n    type: openai-compatible\n    apiKeyEnv: GATEWAY_TEST_COMPATIBLE_KEY\n    baseURL: ${url}/v1\n    providerName: compatible-backend\nmodels:\n  grafana/compatible:\n    name: Grafana Compatible\n    description: Integration model\n    primary:\n      provider: compatible-primary\n      model: backend-private\n    aliases:\n      - compatible\n`;
}

function anthropicConfig(url: string, backendModel = "backend-private"): string {
  return `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${url}\nmodels:\n  grafana/assistant:\n    name: Grafana Assistant\n    description: Integration model\n    primary:\n      provider: anthropic-primary\n      model: ${backendModel}\n    aliases:\n      - assistant\n`;
}

function anthropicFallbackConfig(primaryURL: string, fallbackURL: string, backendModel = "backend-private"): string {
  return `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${primaryURL}\n  anthropic-secondary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${fallbackURL}\nmodels:\n  grafana/assistant:\n    name: Grafana Assistant\n    description: Integration model\n    primary:\n      provider: anthropic-primary\n      model: ${backendModel}\n    fallback:\n      - provider: anthropic-secondary\n        model: ${backendModel}\n    aliases:\n      - assistant\n`;
}

function assertPrivateValuesAbsent(value: unknown, fake: FakeAnthropic, extra: string[] = []): void {
  const serialized = JSON.stringify(value);
  for (const secret of [TEST_TOKEN, TEST_USER_TOKEN, "integration-cap", "integration-anthropic-key", "GATEWAY_TEST_ANTHROPIC_KEY", "anthropic-primary", "backend-private", "provider-secret-response", fake.url, ...extra]) {
    assert.ok(!serialized.includes(secret), "private value escaped into a client/service surface");
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

class GatewayProcess {
  static lastFailedDirectory: string | undefined;

  readonly process: ChildProcess;
  readonly url: string;
  readonly operationalURL: string;
  readonly directory: string;
  stderr = "";
  private stopped = false;
  private readonly exited: Promise<{ code: number | null; signal: NodeJS.Signals | null }>;

  private constructor(proc: ChildProcess, url: string, operationalURL: string, directory: string) {
    this.process = proc;
    this.url = url;
    this.operationalURL = operationalURL;
    this.directory = directory;
    proc.stderr?.on("data", (chunk: Buffer) => { this.stderr += chunk.toString(); });
    this.exited = new Promise((resolve, reject) => {
      proc.once("error", reject);
      proc.once("close", (code, signal) => resolve({ code, signal }));
    });
  }

  static async start(binary: string, anthropicURL: string, extraArgs: string[] = [], extraEnv: Record<string, string> = {}, mode: "access-token" | "cloud-gateway" = "access-token", configYAML?: string, fallbackURL?: string, backendModel = "backend-private"): Promise<GatewayProcess> {
    const directory = mkdtempSync(join(tmpdir(), "grafana-ai-gateway-process-"));
    this.lastFailedDirectory = undefined;
    let gateway: GatewayProcess | undefined;
    try {
      const configPath = join(directory, "models.yaml");
      writeFileSync(configPath, fallbackURL
        ? anthropicFallbackConfig(anthropicURL, fallbackURL, backendModel)
        : configYAML ?? anthropicConfig(anthropicURL, backendModel));
      const port = await availablePort();
      const url = `http://127.0.0.1:${port}`;
      let operationalPort = port;
      if (mode === "cloud-gateway") {
        do { operationalPort = await availablePort(); } while (operationalPort === port);
      }
      const operationalURL = `http://127.0.0.1:${operationalPort}`;
      const args = [
        `--config.file=${configPath}`,
        "--deployment.mode=development",
        ...(mode === "cloud-gateway" ? [
          "--auth.mode=cloud-gateway",
          `--server.operational-listen-address=127.0.0.1:${operationalPort}`,
        ] : extraArgs.some(arg => arg.startsWith("--auth.jwks-url=")) ? [] : ["--auth.unsafe"]),
        `--server.listen-address=127.0.0.1:${port}`,
        "--server.shutdown-timeout=2s",
        ...extraArgs,
      ];
      const proc = spawn(binary, args, {
        cwd: directory,
        stdio: ["ignore", "ignore", "pipe"],
        env: {
          ...Object.fromEntries(Object.entries(nodeProcess.env).filter(([name]) => !name.startsWith("GRAFANA_AI_GATEWAY_") && !name.startsWith("AGENTO11Y_") && !name.startsWith("SIGIL_"))),
          GATEWAY_TEST_ANTHROPIC_KEY: "integration-anthropic-key",
          GATEWAY_TEST_COMPATIBLE_KEY: "integration-compatible-key",
          ...extraEnv,
        },
      });
      gateway = new GatewayProcess(proc, url, operationalURL, directory);
      const deadline = Date.now() + READY_TIMEOUT_MS;
      while (Date.now() < deadline) {
        const outcome = await Promise.race([
          gateway.ready().then((ready) => ({ ready })),
          gateway.exited.then((exit) => ({ exit })),
        ]);
        if ("exit" in outcome) {
          throw new Error(`gateway exited unsuccessfully: code=${outcome.exit.code} signal=${outcome.exit.signal}\n${gateway.stderr}`);
        }
        if (outcome.ready) return gateway;
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      throw new Error("timed out waiting for gateway readiness");
    } catch (error) {
      this.lastFailedDirectory = directory;
      try {
        if (gateway != null) await gateway.terminateAfterStartupFailure();
      } finally {
        rmSync(directory, { recursive: true, force: true });
      }
      throw error;
    }
  }

  client(token = TEST_TOKEN) {
    return createGateway({
      apiKey: "authorization-is-ignored",
      baseURL: `${this.url}/api/v1/aisdk`,
      headers: { "X-Access-Token": token },
    });
  }

  async metrics(): Promise<string> {
    const response = await fetch(`${this.operationalURL}/metrics`, { signal: AbortSignal.timeout(2_000) });
    assert.equal(response.status, 200);
    return response.text();
  }

  async ready(): Promise<boolean> {
    try {
      return (await fetch(`${this.operationalURL}/ready`, { signal: AbortSignal.timeout(500) })).ok;
    } catch {
      return false;
    }
  }

  private async terminateAfterStartupFailure(): Promise<void> {
    if (this.process.exitCode == null && this.process.signalCode == null) {
      this.process.kill("SIGKILL");
    }
    await withTimeout(this.exited, 5_000, `gateway startup cleanup did not reap process: ${this.stderr}`);
  }

  async stop(signal: NodeJS.Signals = "SIGTERM"): Promise<void> {
    if (!this.stopped) {
      this.stopped = true;
      this.process.kill(signal);
    }
    try {
      let exit: { code: number | null; signal: NodeJS.Signals | null };
      try {
        exit = await withTimeout(this.exited, 5_000, `gateway did not exit: ${this.stderr}`);
      } catch (error) {
        if (this.process.exitCode == null && this.process.signalCode == null) {
          this.process.kill("SIGKILL");
        }
        try {
          await withTimeout(this.exited, 5_000, `gateway did not exit after SIGKILL: ${this.stderr}`);
        } catch (reapError) {
          throw new AggregateError([error, reapError], "gateway could not be reaped");
        }
        throw error;
      }
      if (exit.code !== 0 || exit.signal != null) {
        throw new Error(`gateway exited unsuccessfully: code=${exit.code} signal=${exit.signal}\n${this.stderr}`);
      }
    } finally {
      rmSync(this.directory, { recursive: true, force: true });
    }
  }
}

type FakeRequest = { path: string; apiKey?: string; headers: IncomingMessage["headers"]; body: Record<string, unknown> };

class FakeAnthropic {
  readonly url: string;
  readonly requests: FakeRequest[] = [];
  readonly violations: string[] = [];
  readonly canceled = new Set<string>();
  redirectTo?: string;
  oversizedErrors = false;
  failureStatus?: number;
  functionTools = false;
  backendModel = "backend-private";
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
    if (request.url !== "/v1/messages?beta=true") this.violations.push(`path=${request.url}`);
    if (singleHeader(request.headers["x-api-key"]) !== "integration-anthropic-key") this.violations.push("api-key");
    if (body.model !== this.backendModel) this.violations.push(`model=${String(body.model)}`);
    if (request.headers["x-access-token"] != null || request.headers["x-grafana-id"] != null) this.violations.push("forwarded-caller-credential");

    if (this.failureStatus != null) {
      response.writeHead(this.failureStatus, { "Content-Type": "application/json", "x-private-provider": "backend-private" });
      response.end(JSON.stringify({ type: "error", error: { type: "api_error", message: "provider-secret-response integration-anthropic-key backend-private" }, request_id: "backend-private-request" }));
      return;
    }
    if (marker === "silent-unary-shutdown") {
      response.once("close", () => this.canceled.add(marker));
      return;
    }

    if (this.redirectTo != null) {
      response.writeHead(307, { Location: this.redirectTo });
      response.end();
      return;
    }
    if (this.oversizedErrors) {
      response.writeHead(502, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ error: { type: "api_error", message: `provider-secret-response-${"x".repeat(512)}` } }));
      return;
    }
    if (marker === "provider-error") {
      response.writeHead(502, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ error: { type: "api_error", message: "provider-secret-response" } }));
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
    await poll(async () => this.generations.length >= count, 5_000, `${count} Agent Observability generations`);
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

function rawProviderWireRequest(baseURL: string, text: string, modelID = "assistant"): Promise<Response> {
  return fetch(`${baseURL}/api/v1/aisdk/language-model`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Access-Token": TEST_TOKEN,
      "ai-language-model-specification-version": "4",
      "ai-language-model-id": modelID,
      "ai-language-model-streaming": "false",
    },
    body: JSON.stringify({
      prompt: [{ role: "user", content: [{ type: "text", text }] }],
      maxOutputTokens: 32,
      temperature: 0.2,
    }),
  });
}

function unsafeAccessToken(): string {
  const header = Buffer.from(JSON.stringify({ alg: "ES256", typ: "at+jwt" })).toString("base64url");
  const payload = Buffer.from(JSON.stringify({
    sub: "access-policy:integration",
    aud: ["ai-sdk"],
    exp: Math.floor(Date.now() / 1000) + 24 * 60 * 60,
    namespace: "stack-integration",
    serviceIdentity: "integration-service",
  })).toString("base64url");
  return `${header}.${payload}.${Buffer.alloc(64).toString("base64url")}`;
}

function unsafeUserIDToken(): string {
  const header = Buffer.from(JSON.stringify({ alg: "ES256", typ: "jwt" })).toString("base64url");
  const payload = Buffer.from(JSON.stringify({ sub: "user:42", identifier: "42", type: "user", namespace: "stack-integration", aud: ["ai-sdk"], exp: Math.floor(Date.now() / 1000) + 3600 })).toString("base64url");
  return `${header}.${payload}.${Buffer.alloc(64).toString("base64url")}`;
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
    if (record.msg !== "gateway process lifecycle") continue;
    assert.deepEqual(Object.keys(record).sort(), ["event", "level", "msg", "time"]);
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
