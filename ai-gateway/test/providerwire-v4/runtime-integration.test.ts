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

async function stats(): Promise<{ successCalls: number; blockingCalls: number; cancellations: number }> {
  const response = await fetch(`${baseURL}/providerwire-v4/stats`);
  assert.equal(response.ok, true);
  return await response.json() as { successCalls: number; blockingCalls: number; cancellations: number };
}

before(async () => { baseURL = await startServer(); });
after(async () => { await stopServer(); });

describe("real ProviderWire V4 unary runtime", () => {
  it("consumes production-handler text, usage, and finish output", async () => {
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
    assert.equal((await stats()).successCalls, 1);
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
