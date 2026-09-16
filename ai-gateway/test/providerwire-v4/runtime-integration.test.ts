import assert from "node:assert/strict";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import nodeProcess from "node:process";
import { fileURLToPath } from "node:url";
import { after, before, describe, it } from "node:test";
import {
  createGateway,
  GatewayInvalidRequestError,
  GatewayModelNotFoundError,
} from "@ai-sdk/gateway";
import type { LanguageModelV4StreamPart } from "@ai-sdk/provider";
import { buildGoClientCapture, captureGoClient } from "./go-client-capture";
import { jsonSchema, stepCountIs, streamText, tool } from "ai";

const TEST_DIR = dirname(fileURLToPath(import.meta.url));
const SERVER_DIR = resolve(TEST_DIR, "testserver");
const POLL_INTERVAL_MS = 10;
const POLL_TIMEOUT_MS = 15_000;

let serverProcess: ChildProcess | undefined;
let temporaryDirectory: string;
let baseURL: string;
let goClientBinary: string;

async function waitFor<T>(load: () => Promise<T | undefined>, timeoutMs = POLL_TIMEOUT_MS): Promise<T> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const value = await load();
    if (value !== undefined) return value;
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
  throw new Error(`condition not met within ${timeoutMs}ms`);
}

async function startServer(): Promise<string> {
  temporaryDirectory = mkdtempSync(join(tmpdir(), "providerwire-v4-"));
  const binary = join(temporaryDirectory, "testserver");
  execFileSync("go", ["build", "-o", binary, "."], {
    cwd: SERVER_DIR,
    stdio: "pipe",
    env: {
      ...nodeProcess.env,
      GOWORK: "off",
      GOFLAGS: `${nodeProcess.env.GOFLAGS ? `${nodeProcess.env.GOFLAGS} ` : ""}-mod=readonly`,
    },
  });

  const process = spawn(binary, [], { cwd: SERVER_DIR, stdio: ["ignore", "pipe", "pipe"] });
  serverProcess = process;
  let stdout = "";
  let stderr = "";
  process.stdout?.on("data", (chunk: Buffer) => { stdout += chunk.toString(); });
  process.stderr?.on("data", (chunk: Buffer) => { stderr += chunk.toString(); });

  const port = await waitFor(async () => {
    if (process.exitCode !== null) {
      throw new Error(`test server exited with code ${process.exitCode}: ${stderr}`);
    }
    const match = stdout.match(/PORT=(\d+)/);
    return match ? Number.parseInt(match[1], 10) : undefined;
  });
  const url = `http://127.0.0.1:${port}`;
  await waitFor(async () => {
    try {
      return (await fetch(`${url}/health`)).ok ? true : undefined;
    } catch {
      return undefined;
    }
  });
  return url;
}

async function stopServer(): Promise<void> {
  const process = serverProcess;
  if (process && process.exitCode === null) {
    process.kill("SIGTERM");
    await new Promise<void>((resolveStop) => {
      const timeout = setTimeout(() => {
        process.kill("SIGKILL");
        resolveStop();
      }, 5_000);
      process.once("exit", () => {
        clearTimeout(timeout);
        resolveStop();
      });
    });
  }
  if (temporaryDirectory) rmSync(temporaryDirectory, { recursive: true, force: true });
}

function model(modelID: string) {
  return createGateway({
    apiKey: "runtime-test-key",
    baseURL: `${baseURL}/providerwire-v4`,
  })(modelID);
}

type RuntimeStats = {
  successCalls: number;
  streamCalls: number;
  blockingCalls: number;
  streamBlockingCalls: number;
  cancellations: number;
};

async function stats(): Promise<RuntimeStats> {
  const response = await fetch(`${baseURL}/providerwire-v4/stats`);
  assert.equal(response.ok, true);
  return await response.json() as RuntimeStats;
}

async function collect(stream: ReadableStream<LanguageModelV4StreamPart>): Promise<LanguageModelV4StreamPart[]> {
  const reader = stream.getReader();
  const parts: LanguageModelV4StreamPart[] = [];
  for (;;) {
    const result = await reader.read();
    if (result.done) return parts;
    parts.push(result.value);
  }
}

before(async () => { baseURL = await startServer(); goClientBinary = buildGoClientCapture(temporaryDirectory); });
after(async () => { await stopServer(); });

describe("unary function tools through the authenticated real handler", () => {
  const tools = [{ type: "function" as const, name: "weather", inputSchema: { type: "object" as const }, strict: false }];
  const prompt = [{ role: "user" as const, content: [{ type: "text" as const, text: "Weather in Rio?" }] }];

  it("completes two requests with one local execution in both clients", async () => {
    const gateway = createGateway({ apiKey: "test", baseURL: `${baseURL}/function-tools`, headers: { "x-access-token": "function-test-token" } });
    let executions = 0;
    const execute = (input: unknown) => { assert.deepEqual(input, { city: "Rio" }); executions++; return "sunny"; };
    const first = await gateway("unary-tools").doGenerate({ prompt, tools });
    const call = first.content.find(part => part.type === "tool-call");
    assert.ok(call && call.type === "tool-call");
    const value = execute(JSON.parse(call.input));
    const continuation = [...prompt,
      { role: "assistant" as const, content: [{ type: "tool-call" as const, toolCallId: call.toolCallId, toolName: call.toolName, input: JSON.parse(call.input) }] },
      { role: "tool" as const, content: [{ type: "tool-result" as const, toolCallId: call.toolCallId, toolName: call.toolName, output: { type: "text" as const, value } }] }];
    const final = await gateway("unary-tools").doGenerate({ prompt: continuation, tools });
    assert.deepEqual(final.content, [{ type: "text", text: "It is sunny." }]);
    assert.equal(executions, 1);

    const config = { baseURL: `${baseURL}/function-tools`, accessToken: "function-test-token", modelID: "unary-tools", mode: "generate" };
    const goFirst = await captureGoClient(goClientBinary, { ...config, options: { prompt, tools } });
    assert.equal(goFirst.error, undefined);
    const goCall = goFirst.result.content.find((part: any) => part.type === "tool-call");
    assert.deepEqual(goCall, { type: "tool-call", toolCallId: call.toolCallId, toolName: call.toolName, input: call.input });
    assert.deepEqual(goFirst.result.finishReason, first.finishReason);
    assert.deepEqual(goFirst.result.usage, first.usage);
    assert.equal(first.finishReason.unified, "tool-calls");
    const goValue = execute(JSON.parse(goCall.input));
    const goFinal = await captureGoClient(goClientBinary, { ...config, options: { prompt: [...prompt,
      { role: "assistant", content: [{ type: "tool-call", toolCallId: goCall.toolCallId, toolName: goCall.toolName, input: JSON.parse(goCall.input) }] },
      { role: "tool", content: [{ type: "tool-result", toolCallId: goCall.toolCallId, toolName: goCall.toolName, output: { type: "text", value: goValue } }] }], tools } });
    assert.equal(goFinal.error, undefined);
    assert.deepEqual(goFinal.result.content, final.content);
    assert.deepEqual(goFinal.result.finishReason, final.finishReason);
    assert.deepEqual(goFinal.result.usage, final.usage);
    assert.equal(final.finishReason.unified, "stop");
    assert.equal(executions, 2);
  });

  it("rejects enabled execution markers before either client can execute", async () => {
    for (const modelID of ["unary-tools-provider-executed", "unary-tools-dynamic"]) {
      let executions = 0;
      const gateway = createGateway({ apiKey: "test", baseURL: `${baseURL}/function-tools`, headers: { "x-access-token": "function-test-token" } });
      await assert.rejects(async () => {
        const result = await gateway(modelID).doGenerate({ prompt, tools });
        for (const part of result.content) if (part.type === "tool-call") executions++;
      }, (error: any) => {
        assert.equal(error.statusCode, 500);
        assert.equal(error.type, "internal_server_error");
        assert.equal(error.message, "internal error");
        assert.equal(error.isRetryable, true);
        return true;
      });
      const go = await captureGoClient(goClientBinary, { baseURL: `${baseURL}/function-tools`, accessToken: "function-test-token", modelID, mode: "generate", options: { prompt, tools } });
      assert.ok(go.error);
      assert.deepEqual({ status: go.error.statusCode, category: go.error.category, code: go.error.code, retryable: go.error.isRetryable }, { status: 500, category: "internal_server_error", code: "internal_error", retryable: true });
      assert.equal(go.result, undefined);
      assert.equal(executions, 0);
    }
  });
});

describe("streaming function tools through the authenticated real handler", () => {
  it("cancels between stateless steps without a second request in either client", async () => {
    const gateway = createGateway({ apiKey: "test", baseURL: `${baseURL}/function-tools`, headers: { "x-access-token": "function-test-token" } });
    const abort = new AbortController();
    let executions = 0;
    const before = await stats();
    const result = streamText({ model: gateway("stream-tools"), prompt: "Weather in Rio?", abortSignal: abort.signal, maxRetries: 0, stopWhen: stepCountIs(2), tools: {
      weather: tool({ inputSchema: jsonSchema<{city:string}>({ type: "object", properties: { city: { type: "string" } }, required: ["city"] }), execute: async input => { assert.equal(input.city, "Rio"); executions++; abort.abort(); return "sunny"; } }),
    } });
    let aborted = false;
    for await (const part of result.fullStream) if (part.type === "abort") aborted = true;
    assert.equal(aborted, true);
    assert.equal(executions, 1);
    assert.equal((await stats()).streamCalls - before.streamCalls, 1);
    const goBefore = await stats();
    const go = await captureGoClient(goClientBinary, { baseURL: `${baseURL}/function-tools`, accessToken: "function-test-token", modelID: "stream-tools", mode: "stream-loop", abortBetweenSteps: true });
    assert.deepEqual(go, { canceled: true, executions: 1 });
    assert.equal((await stats()).streamCalls - goBefore.streamCalls, 1);
  });

  it("transports basic tool results including selected empty scalars in both clients", async () => {
    const gateway = createGateway({ apiKey: "test", baseURL: `${baseURL}/function-tools`, headers: { "x-access-token": "function-test-token" } });
    const parts = await collect((await gateway("stream-tool-results").doStream({ prompt: [] })).stream);
    const go = await captureGoClient(goClientBinary, { baseURL: `${baseURL}/function-tools`, accessToken: "function-test-token", modelID: "stream-tool-results", mode: "stream", options: { prompt: [] } });
    assert.equal(go.error, undefined);
    const tools = parts.filter(part => part.type === "tool-call" || part.type === "tool-result");
    assert.deepEqual(go.parts.filter((part: any) => part.type === "tool-call" || part.type === "tool-result"), tools);
    assert.deepEqual(tools.filter(part => part.type === "tool-result").map(part => part.result), [false, 0, "", [], {}]);
    assert.deepEqual(tools.map(part => part.toolCallId), ["a", "a", "b", "b", "c", "c", "d", "d", "e", "e"]);
    assert.equal(tools.at(-1)?.type, "tool-result");
    assert.equal((tools.at(-1) as any).isError, true);
  });

  it("preserves exact provider input IDs, order, and empty deltas for both clients", async () => {
    const gateway = createGateway({ apiKey: "test", baseURL: `${baseURL}/function-tools`, headers: { "x-access-token": "function-test-token" } });
    const result = await gateway("stream-tools").doStream({ prompt: [] });
    const parts = await collect(result.stream);
    const go = await captureGoClient(goClientBinary, { baseURL: `${baseURL}/function-tools`, accessToken: "function-test-token", modelID: "stream-tools", mode: "stream", options: { prompt: [] } });
    assert.equal(go.error, undefined);
    const normalize = (part: any) => part.type === "stream-start" ? { ...part, warnings: part.warnings ?? [] } : part;
    assert.deepEqual(go.parts.map(normalize), parts.map(normalize));
    assert.deepEqual(parts.map(part => part.type), ["stream-start", "tool-input-start", "tool-input-delta", "tool-input-delta", "tool-input-end", "tool-call", "finish"]);
    assert.deepEqual(parts[2], { type: "tool-input-delta", id: "call-weather", delta: "" });
  });

  it("runs stateless two-step Vercel and Go orchestration with one execution each", async () => {
    const gateway = createGateway({ apiKey: "test", baseURL: `${baseURL}/function-tools`, headers: { "x-access-token": "function-test-token" } });
    let executions = 0;
    const result = streamText({
      model: gateway("stream-tools"), prompt: "Weather in Rio?", maxRetries: 0, stopWhen: stepCountIs(2),
      tools: { weather: tool({ inputSchema: jsonSchema<{city:string}>({ type: "object", properties: { city: { type: "string" } }, required: ["city"] }), execute: async input => { assert.equal(input.city, "Rio"); executions++; return "sunny"; } }) },
    });
    for await (const part of result.fullStream) if (part.type === "error") throw part.error;
    assert.equal(await result.text, "It is sunny.");
    assert.equal((await result.steps).length, 2);
    assert.equal(executions, 1);
    const go = await captureGoClient(goClientBinary, { baseURL: `${baseURL}/function-tools`, accessToken: "function-test-token", modelID: "stream-tools", mode: "stream-loop" });
    assert.equal(go.error, undefined);
    assert.deepEqual(go, { text: "It is sunny.", steps: 2, executions: 1 });
  });
});

describe("real ProviderWire V4 streaming runtime", () => {
  it("consumes normalized text, metadata, warnings, finish, and clean EOF", async () => {
    const result = await model("success").doStream({ prompt: [] });
    const parts = await collect(result.stream);

    assert.deepEqual(parts.map((part) => part.type), [
      "stream-start",
      "response-metadata",
      "text-start",
      "text-delta",
      "text-delta",
      "text-end",
      "finish",
    ]);
    assert.deepEqual(parts[0], {
      type: "stream-start",
      warnings: [{ type: "other", message: "the model reported a warning" }],
    });
    assert.deepEqual(parts.filter((part) => part.type === "text-delta").map((part) => part.delta), [
      "",
      "hello from Go stream",
    ]);
    const metadata = parts.find((part) => part.type === "response-metadata");
    assert.equal(metadata?.type, "response-metadata");
    if (metadata?.type === "response-metadata") {
      assert.equal(metadata.id, "stream-response-1");
      assert.equal(metadata.modelId, "success");
      assert.equal(metadata.timestamp instanceof Date, true);
    }
  });

  it("preserves ordered provider errors and emits terminal timeout", async () => {
    const withErrors = await model("stream-errors").doStream({ prompt: [] });
    const errorParts = await collect(withErrors.stream);
    assert.deepEqual(errorParts.map((part) => part.type), [
      "stream-start",
      "error",
      "text-start",
      "error",
      "text-delta",
      "text-end",
      "finish",
    ]);
    assert.deepEqual(
      errorParts
        .filter((part) => part.type === "error")
        .map((part) => (part.error as { code: string }).code),
      ["overloaded", "failed_dependency"],
    );

    const timedOut = await model("stream-timeout").doStream({ prompt: [] });
    const timeoutParts = await collect(timedOut.stream);
    assert.deepEqual(timeoutParts.map((part) => part.type), ["stream-start", "error"]);
    const timeout = timeoutParts[1];
    assert.equal(timeout.type, "error");
    if (timeout.type === "error") {
      assert.equal((timeout.error as { code: string }).code, "timeout");
    }
  });

  it("propagates abort after stream establishment", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const result = await model("stream-blocking").doStream({ prompt: [], abortSignal: controller.signal });
    await waitFor(
      async () => (await stats()).streamBlockingCalls > initial.streamBlockingCalls ? true : undefined,
      2_000,
    );

    controller.abort();
    await assert.rejects(async () => await collect(result.stream));
    await waitFor(async () => (await stats()).cancellations > initial.cancellations ? true : undefined, 2_000);
  });
});

describe("real ProviderWire V4 unary runtime", () => {
  it("consumes the minimal production response", async () => {
    const result = await model("success").doGenerate({
      prompt: [{ role: "system", content: "hello" }],
    });

    assert.deepEqual(result.content, [{ type: "text", text: "hello from Go" }]);
    assert.deepEqual(result.finishReason, { unified: "stop", raw: "test-stop" });
    assert.deepEqual(result.usage, {
      inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    });
    assert.deepEqual(result.warnings, []);
    assert.deepEqual(result.response?.body, {
      content: [{ type: "text", text: "hello from Go" }],
      finishReason: { unified: "stop", raw: "test-stop" },
      usage: {
        inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
        outputTokens: { total: 1, text: 1, reasoning: 0 },
      },
    });
    assert.equal((await stats()).successCalls > 0, true);
  });

  it("maps representative failures", async () => {
    await assert.rejects(
      async () => await model("success").doGenerate({
        prompt: [],
        headers: { "x-body-header": "unsupported" },
      }),
      (error: unknown) => GatewayInvalidRequestError.isInstance(error),
    );
    await assert.rejects(
      async () => await model("missing").doGenerate({ prompt: [] }),
      (error: unknown) => GatewayModelNotFoundError.isInstance(error),
    );
  });

  it("propagates cancellation", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const pending = model("blocking").doGenerate({ prompt: [], abortSignal: controller.signal });
    await waitFor(async () => (await stats()).blockingCalls > initial.blockingCalls ? true : undefined, 2_000);

    controller.abort();
    await assert.rejects(async () => await pending);
    await waitFor(async () => (await stats()).cancellations > initial.cancellations ? true : undefined, 2_000);
  });
});
