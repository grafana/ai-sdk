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
  GatewayAuthenticationError,
  GatewayFailedDependencyError,
  GatewayForbiddenError,
  GatewayInternalServerError,
  GatewayInvalidRequestError,
  GatewayModelNotFoundError,
  GatewayRateLimitError,
} from "@ai-sdk/gateway";
import type { LanguageModelV4StreamPart } from "@ai-sdk/provider";

const TEST_DIR = dirname(fileURLToPath(import.meta.url));
const SERVER_DIR = resolve(TEST_DIR, "../integration/testserver");
const CONTROL_HEADER = "x-providerwire-test-error";
const POLL_INTERVAL_MS = 10;
const START_TIMEOUT_MS = 15_000;
const CANCELLATION_TIMEOUT_MS = 2_000;

let serverProcess: ChildProcess | undefined;
let temporaryDirectory: string;
let baseURL: string;

async function waitFor<T>(load: () => Promise<T | undefined>, timeoutMs = START_TIMEOUT_MS): Promise<T> {
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
    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        process.kill("SIGKILL");
        resolve();
      }, 5_000);
      process.once("exit", () => {
        clearTimeout(timeout);
        resolve();
      });
    });
  }
  if (temporaryDirectory) rmSync(temporaryDirectory, { recursive: true, force: true });
}

function model(modelID: string, control?: string) {
  return createGateway({
    apiKey: "runtime-test-key",
    baseURL: `${baseURL}/providerwire-v4`,
    headers: control ? { [CONTROL_HEADER]: control } : undefined,
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

async function collect(stream: ReadableStream<LanguageModelV4StreamPart>) {
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
  it("consumes normalized text, ordered errors, timeout, finish, and clean EOF", async () => {
    const normal = await model("success").doStream({ prompt: [] });
    const normalParts = await collect(normal.stream);
    assert.deepEqual(normalParts.map((part) => part.type), [
      "stream-start", "response-metadata", "text-start", "text-delta", "text-delta", "text-end", "finish",
    ]);
    assert.deepEqual(normalParts[0], {
      type: "stream-start",
      warnings: [{ type: "other", message: "the model reported a warning" }],
    });
    assert.deepEqual(normalParts.filter((part) => part.type === "text-delta").map((part) => part.delta), ["", "hello from Go stream"]);
    const metadata = normalParts.find((part) => part.type === "response-metadata");
    assert.equal(metadata?.type, "response-metadata");
    if (metadata?.type === "response-metadata") {
      assert.equal(metadata.modelId, "success");
      assert.equal(metadata.timestamp instanceof Date, true);
    }

    const withErrors = await model("stream-errors").doStream({ prompt: [] });
    const errorParts = await collect(withErrors.stream);
    assert.deepEqual(errorParts.map((part) => part.type), [
      "stream-start", "error", "text-start", "error", "text-delta", "text-end", "finish",
    ]);
    const codes = errorParts
      .filter((part) => part.type === "error")
      .map((part) => (part.error as { code: string }).code);
    assert.deepEqual(codes, ["overloaded", "failed_dependency"]);

    const timedOut = await model("stream-timeout").doStream({ prompt: [] });
    const timeoutParts = await collect(timedOut.stream);
    assert.deepEqual(timeoutParts.map((part) => part.type), ["stream-start", "error"]);
    const timeout = timeoutParts[1];
    assert.equal(timeout.type, "error");
    if (timeout.type === "error") assert.equal((timeout.error as { code: string }).code, "timeout");
  });

  it("aborts only after the silent stream is established and cancels provider context", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const result = await model("stream-blocking").doStream({ prompt: [], abortSignal: controller.signal });
    await waitFor(async () => (await stats()).streamBlockingCalls > initial.streamBlockingCalls ? true : undefined);

    controller.abort();
    await assert.rejects(async () => await collect(result.stream));
    await waitFor(
      async () => (await stats()).cancellations > initial.cancellations ? true : undefined,
      CANCELLATION_TIMEOUT_MS,
    );
  });
});

describe("real ProviderWire V4 unary runtime", () => {
  it("consumes production-handler text, usage, and finish output", async () => {
    const initial = await stats();
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
    assert.equal((await stats()).successCalls, initial.successCalls + 1);
  });

  type ErrorCase = {
    name: string;
    modelID: string;
    control?: string;
    requestHeaders?: boolean;
    status: number;
    type: string;
    retryable: boolean;
    isInstance: (error: unknown) => boolean;
  };
  const errorCases: readonly ErrorCase[] = [
    { name: "invalid request", modelID: "success", requestHeaders: true, status: 400, type: "invalid_request_error", retryable: false, isInstance: GatewayInvalidRequestError.isInstance },
    { name: "authentication", modelID: "success", control: "authentication", status: 401, type: "authentication_error", retryable: false, isInstance: GatewayAuthenticationError.isInstance },
    { name: "permission", modelID: "success", control: "permission", status: 403, type: "forbidden", retryable: false, isInstance: GatewayForbiddenError.isInstance },
    { name: "public model not found", modelID: "missing", status: 404, type: "model_not_found", retryable: false, isInstance: GatewayModelNotFoundError.isInstance },
    { name: "rate limit", modelID: "success", control: "rate-limit", status: 429, type: "rate_limit_exceeded", retryable: true, isInstance: GatewayRateLimitError.isInstance },
    { name: "overload fallback", modelID: "success", control: "overload", status: 503, type: "internal_server_error", retryable: true, isInstance: GatewayInternalServerError.isInstance },
    { name: "failed dependency", modelID: "failed-dependency", status: 424, type: "failed_dependency", retryable: false, isInstance: GatewayFailedDependencyError.isInstance },
    { name: "upstream fallback", modelID: "upstream", status: 502, type: "internal_server_error", retryable: true, isInstance: GatewayInternalServerError.isInstance },
    { name: "timeout fallback", modelID: "timeout", status: 504, type: "internal_server_error", retryable: true, isInstance: GatewayInternalServerError.isInstance },
    { name: "cancellation fallback", modelID: "cancellation", status: 499, type: "internal_server_error", retryable: false, isInstance: GatewayInternalServerError.isInstance },
    { name: "internal fallback", modelID: "internal", status: 500, type: "internal_server_error", retryable: true, isInstance: GatewayInternalServerError.isInstance },
  ];

  for (const testCase of errorCases) {
    it(`maps ${testCase.name} to the pinned client class`, async () => {
      await assert.rejects(
        async () => await model(testCase.modelID, testCase.control).doGenerate({
          prompt: [],
          ...(testCase.requestHeaders ? { headers: { "x-body-header": "unsupported" } } : {}),
        }),
        (error: unknown) => {
          assert.equal(testCase.isInstance(error), true);
          const gatewayError = error as { statusCode: number; type: string; isRetryable: boolean };
          assert.equal(gatewayError.statusCode, testCase.status);
          assert.equal(gatewayError.type, testCase.type);
          assert.equal(gatewayError.isRetryable, testCase.retryable);
          return true;
        },
      );
    });
  }

  it("propagates an abort signal and the server model observes cancellation", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const pending = model("blocking").doGenerate({ prompt: [], abortSignal: controller.signal });
    await waitFor(async () => (await stats()).blockingCalls > initial.blockingCalls ? true : undefined);

    controller.abort();
    await assert.rejects(async () => await pending);
    assert.equal(controller.signal.aborted, true);
    await waitFor(
      async () => (await stats()).cancellations > initial.cancellations ? true : undefined,
      CANCELLATION_TIMEOUT_MS,
    );
  });
});
