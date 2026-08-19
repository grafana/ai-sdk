import { execSync, spawn, type ChildProcess } from "node:child_process";
import { resolve } from "node:path";
import { writeFileSync, unlinkSync } from "node:fs";

const SERVER_DIR = resolve(import.meta.dirname, "testserver");
const BINARY_PATH = resolve(SERVER_DIR, "testserver");
const URL_FILE = resolve(import.meta.dirname, ".test-server-url");
const V4_URL_FILE = resolve(import.meta.dirname, ".test-server-v4-url");
const HEALTH_TIMEOUT_MS = 15_000;
const HEALTH_POLL_MS = 200;

let serverProcess: ChildProcess | undefined;

async function pollHealth(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${url}/health`);
      if (res.ok) return;
    } catch {
      // Server not ready yet
    }
    await new Promise((r) => setTimeout(r, HEALTH_POLL_MS));
  }
  throw new Error(`Test server did not become healthy within ${timeoutMs}ms`);
}

export async function setup(): Promise<void> {
  console.log("[global-setup] Building Go interop test server...");
  execSync("go build -o testserver .", { cwd: SERVER_DIR, stdio: "pipe" });

  console.log("[global-setup] Spawning interop test server...");
  const proc = spawn(BINARY_PATH, [], {
    cwd: SERVER_DIR,
    stdio: ["ignore", "pipe", "pipe"],
  });
  serverProcess = proc;

  const port = await new Promise<number>((resolvePort, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error("Timed out waiting for PORT from test server stdout"));
    }, 10_000);

    proc.stdout!.on("data", (chunk: Buffer) => {
      const match = chunk.toString().match(/PORT=(\d+)/);
      if (match) {
        clearTimeout(timeout);
        resolvePort(parseInt(match[1], 10));
      }
    });
    proc.on("error", (err) => {
      clearTimeout(timeout);
      reject(err);
    });
    proc.on("exit", (code) => {
      clearTimeout(timeout);
      if (code !== null) reject(new Error(`Test server exited with code ${code}`));
    });
  });

  const baseUrl = `http://127.0.0.1:${port}`;
  console.log(`[global-setup] Waiting for health at ${baseUrl}...`);
  await pollHealth(baseUrl, HEALTH_TIMEOUT_MS);

  // The provider-wire language-model route is mounted under /api/v1/aisdk, which
  // is the gateway baseURL the client points at.
  writeFileSync(URL_FILE, `${baseUrl}/api/v1/aisdk`, "utf-8");
  writeFileSync(V4_URL_FILE, `${baseUrl}/api/v1/aisdk-v4`, "utf-8");
  console.log(`[global-setup] Interop test server ready at ${baseUrl}`);
}

export async function teardown(): Promise<void> {
  if (serverProcess && !serverProcess.killed) {
    console.log("[global-teardown] Stopping interop test server...");
    serverProcess.kill("SIGTERM");
    await new Promise<void>((resolveDone) => {
      serverProcess!.on("exit", () => resolveDone());
      setTimeout(() => {
        serverProcess!.kill("SIGKILL");
        resolveDone();
      }, 5_000);
    });
  }
  for (const path of [URL_FILE, V4_URL_FILE]) {
    try {
      unlinkSync(path);
    } catch {
      // File may not exist
    }
  }
}
