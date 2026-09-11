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

async function stats(): Promise<{ successCalls: number; blockingCalls: number; cancellations: number }> {
  const response = await fetch(`${baseURL}/providerwire-v4/stats`);
  assert.equal(response.ok, true);
  return await response.json() as { successCalls: number; blockingCalls: number; cancellations: number };
}

before(async () => { baseURL = await startServer(); });
after(async () => { await stopServer(); });

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
