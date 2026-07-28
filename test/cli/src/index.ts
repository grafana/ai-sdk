import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { uiMessageChunkSchema } from "ai";
import { execSync, spawn, type ChildProcess } from "node:child_process";
import { resolve } from "node:path";
import { parseArgs } from "node:util";

const SERVER_DIR = resolve(import.meta.dirname, "../../integration/testserver");
const BINARY_PATH = resolve(SERVER_DIR, "testserver");

const args = process.argv.slice(2).filter((a) => a !== "--");
const { values } = parseArgs({
  args,
  options: {
    url: { type: "string" },
    scenario: { type: "string" },
    method: { type: "string", default: "POST" },
  },
  strict: false,
  allowPositionals: true,
});

if (!values.url && !values.scenario) {
  console.error(
    "Usage:\n" +
      "  test-cli --scenario <name> [--method POST]   (auto-starts Go server)\n" +
      "  test-cli --url <URL> [--method POST]          (connects to existing server)",
  );
  process.exit(1);
}

const method = values.method ?? "POST";
let serverProcess: ChildProcess | undefined;

async function startServer(): Promise<string> {
  console.log("Building Go test server...");
  execSync("go build -o testserver .", { cwd: SERVER_DIR, stdio: "pipe" });

  console.log("Starting test server...");
  const proc = spawn(BINARY_PATH, [], {
    cwd: SERVER_DIR,
    stdio: ["ignore", "pipe", "pipe"],
  });
  serverProcess = proc;

  const port = await new Promise<number>((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error("Timed out waiting for PORT from test server"));
    }, 10_000);

    proc.stdout!.on("data", (chunk: Buffer) => {
      const match = chunk.toString().match(/PORT=(\d+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(parseInt(match[1], 10));
      }
    });

    proc.on("error", (err) => {
      clearTimeout(timeout);
      reject(err);
    });
    proc.on("exit", (code) => {
      clearTimeout(timeout);
      if (code !== null) reject(new Error(`Server exited with code ${code}`));
    });
  });

  const baseUrl = `http://127.0.0.1:${port}`;

  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseUrl}/health`);
      if (res.ok) {
        console.log(`Test server ready at ${baseUrl}`);
        return baseUrl;
      }
    } catch {}
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error("Test server did not become healthy");
}

function stopServer(): void {
  if (serverProcess && !serverProcess.killed) {
    serverProcess.kill("SIGTERM");
  }
}

async function run(): Promise<void> {
  let url: string;

  if (values.scenario) {
    const baseUrl = await startServer();
    url = `${baseUrl}/scenario/${values.scenario}`;
  } else {
    url = values.url!;
  }

  console.log(`\nConnecting to ${url} (${method})...`);

  let res: Response;
  try {
    res = await fetch(url, { method });
  } catch (err) {
    console.error(
      `Connection failed: ${err instanceof Error ? err.message : err}`,
    );
    process.exit(1);
  }

  console.log(`Status: ${res.status} ${res.statusText}`);
  console.log(`Content-Type: ${res.headers.get("content-type")}`);
  console.log("---");

  if (!res.ok) {
    const body = await res.text();
    console.error(`Error response: ${body}`);
    process.exit(1);
  }

  const contentType = res.headers.get("content-type") ?? "";

  if (contentType.includes("text/event-stream")) {
    await handleSSE(res);
  } else {
    await handleText(res);
  }
}

async function handleSSE(res: Response): Promise<void> {
  const stream = parseJsonEventStream({
    stream: res.body!,
    schema: uiMessageChunkSchema,
  });

  const reader = stream.getReader();
  let index = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    if (value.success) {
      console.log(`[${index}] ${JSON.stringify(value.value, null, 2)}`);
    } else {
      console.error(`[${index}] PARSE ERROR: ${value.error}`);
    }
    index++;
  }
  console.log("--- Stream complete ---");
}

async function handleText(res: Response): Promise<void> {
  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let index = 0;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    const text = decoder.decode(value, { stream: true });
    console.log(`[${index}] ${text}`);
    index++;
  }
  console.log("--- Stream complete ---");
}

run()
  .catch((err) => {
    console.error(err);
    process.exitCode = 1;
  })
  .finally(() => {
    stopServer();
  });
