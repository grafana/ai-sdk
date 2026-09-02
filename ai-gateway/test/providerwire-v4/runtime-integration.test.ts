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

const TEST_DIR = dirname(fileURLToPath(import.meta.url));
const SERVER_DIR = resolve(TEST_DIR, "testserver");
const POLL_INTERVAL_MS = 10;
const POLL_TIMEOUT_MS = 15_000;

let serverProcess: ChildProcess | undefined;
let temporaryDirectory: string;
let baseURL: string;

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

before(async () => { baseURL = await startServer(); });
after(async () => { await stopServer(); });

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
