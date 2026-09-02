import assert from "node:assert/strict";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import nodeProcess from "node:process";
import { after, before, describe, it } from "node:test";
import { createGateway } from "@ai-sdk/gateway";

const AI_GATEWAY_ROOT = resolve(import.meta.dirname, "../..");
const COMMAND_DIR = resolve(AI_GATEWAY_ROOT, "cmd/grafana-ai-gateway");
const READY_TIMEOUT_MS = 15_000;
const TEST_TOKEN = unsafeAccessToken();

let buildDirectory: string;
let binaryPath: string;

before(() => {
  buildDirectory = mkdtempSync(join(tmpdir(), "grafana-ai-gateway-build-"));
  binaryPath = join(buildDirectory, "grafana-ai-gateway");
  execFileSync("go", ["build", "-o", binaryPath, "."], {
    cwd: COMMAND_DIR,
    stdio: "pipe",
    env: {
      ...nodeProcess.env,
      GOWORK: "off",
      GOFLAGS: `${nodeProcess.env.GOFLAGS ? `${nodeProcess.env.GOFLAGS} ` : ""}-mod=readonly`,
    },
  });
});

after(() => {
  rmSync(buildDirectory, { recursive: true, force: true });
});

describe("authenticated Anthropic Gateway command", () => {
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

async function startGateway(extraArgs: string[] = []): Promise<[FakeAnthropic, GatewayProcess]> {
  const fake = await FakeAnthropic.start();
  try {
    return [fake, await GatewayProcess.start(binaryPath, fake.url, extraArgs)];
  } catch (error) {
    await settleCleanup(() => fake.stop());
    throw error;
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
  readonly directory: string;
  stderr = "";
  private stopped = false;
  private readonly exited: Promise<{ code: number | null; signal: NodeJS.Signals | null }>;

  private constructor(proc: ChildProcess, url: string, directory: string) {
    this.process = proc;
    this.url = url;
    this.directory = directory;
    proc.stderr?.on("data", (chunk: Buffer) => { this.stderr += chunk.toString(); });
    this.exited = new Promise((resolve, reject) => {
      proc.once("error", reject);
      proc.once("close", (code, signal) => resolve({ code, signal }));
    });
  }

  static async start(binary: string, anthropicURL: string, extraArgs: string[] = []): Promise<GatewayProcess> {
    const directory = mkdtempSync(join(tmpdir(), "grafana-ai-gateway-process-"));
    this.lastFailedDirectory = undefined;
    let gateway: GatewayProcess | undefined;
    try {
      const configPath = join(directory, "models.yaml");
      writeFileSync(configPath, `providers:\n  anthropic-primary:\n    type: anthropic\n    apiKeyEnv: GATEWAY_TEST_ANTHROPIC_KEY\n    baseURL: ${anthropicURL}\nmodels:\n  grafana/assistant:\n    name: Grafana Assistant\n    description: Integration model\n    primary:\n      provider: anthropic-primary\n      model: backend-private\n    aliases:\n      - assistant\n`);
      const port = await availablePort();
      const url = `http://127.0.0.1:${port}`;
      const args = [
        `--config.file=${configPath}`,
        "--deployment.mode=development",
        "--auth.unsafe",
        `--server.listen-address=127.0.0.1:${port}`,
        "--server.shutdown-timeout=2s",
        ...extraArgs,
      ];
      const proc = spawn(binary, args, {
        cwd: directory,
        stdio: ["ignore", "ignore", "pipe"],
        env: { ...nodeProcess.env, GATEWAY_TEST_ANTHROPIC_KEY: "integration-anthropic-key" },
      });
      gateway = new GatewayProcess(proc, url, directory);
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

  client() {
    return createGateway({
      apiKey: "authorization-is-ignored",
      baseURL: `${this.url}/api/v1/aisdk`,
      headers: { "X-Access-Token": TEST_TOKEN },
    });
  }

  async ready(): Promise<boolean> {
    try {
      return (await fetch(`${this.url}/ready`, { signal: AbortSignal.timeout(500) })).ok;
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

type FakeRequest = { path: string; apiKey?: string; body: Record<string, unknown> };

class FakeAnthropic {
  readonly url: string;
  readonly requests: FakeRequest[] = [];
  readonly violations: string[] = [];
  readonly canceled = new Set<string>();
  redirectTo?: string;
  oversizedErrors = false;
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
    const marker = ["silent-abort", "silent-shutdown", "normal-stream", "redirect", "oversized"]
      .find((value) => serialized.includes(value));
    this.requests.push({ path: request.url ?? "", apiKey: singleHeader(request.headers["x-api-key"]), body });
    if (request.url !== "/v1/messages?beta=true") this.violations.push(`path=${request.url}`);
    if (singleHeader(request.headers["x-api-key"]) !== "integration-anthropic-key") this.violations.push("api-key");
    if (body.model !== "backend-private") this.violations.push(`model=${String(body.model)}`);

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

function rawProviderWireRequest(baseURL: string, text: string): Promise<Response> {
  return fetch(`${baseURL}/api/v1/aisdk/language-model`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Access-Token": TEST_TOKEN,
      "ai-language-model-specification-version": "4",
      "ai-language-model-id": "assistant",
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
