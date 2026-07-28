#!/usr/bin/env tsx

import {
  createServer,
  request as httpRequest,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import { request as httpsRequest } from "node:https";
import {
  writeFileSync,
  readdirSync,
  existsSync,
} from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createAnthropic } from "@ai-sdk/anthropic";
import { createOpenAI } from "@ai-sdk/openai";
import { createAmazonBedrock } from "@ai-sdk/amazon-bedrock";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { convertToModelMessages, streamText, stepCountIs } from "ai";
import {
  type RequestSnapshot,
  type TestCase,
  buildMessages,
  buildOutput,
  buildStreamTextOptions,
  buildTools,
  createSourceIdNormalizer,
  loadConfig,
  mockId,
  normalizeRequestSnapshot,
  writeRequestSnapshots,
} from "./common.mts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const CONFORMANCE_ROOT = resolve(__dirname, "..");

// --- Provider Base URLs ---

const PROVIDER_BASE_URLS: Record<string, string> = {
  anthropic: "https://api.anthropic.com",
  openai: "https://api.openai.com",
  "openai-compatible": process.env.OPENAI_COMPATIBLE_BASE_URL ?? "https://api.openai.com",
  // Bedrock uses a different recording path (custom fetch wrapper) because
  // SigV4 signing depends on the exact URL hostname. The proxy approach
  // breaks signing; see startBedrockFetchCapture below.
};

// --- Recording Proxy ---

async function startRecordingProxy(
  providerName: string,
  onResponse: (index: number, body: string) => void,
  onRequest: (index: number, request: RequestSnapshot) => void,
): Promise<{ port: number; close: () => Promise<void> }> {
  let callIndex = 0;
  const targetBase = PROVIDER_BASE_URLS[providerName];
  if (!targetBase) throw new Error(`No base URL for provider: ${providerName}`);

  const targetURL = new URL(targetBase);

  const server = createServer(
    (clientReq: IncomingMessage, clientRes: ServerResponse) => {
      const currentIndex = callIndex++;
      const requestChunks: Buffer[] = [];

      clientReq.on("data", (chunk: Buffer) => requestChunks.push(chunk));
      clientReq.on("end", () => {
        const requestBody = Buffer.concat(requestChunks);

        try {
          onRequest(
            currentIndex,
            normalizeRequestSnapshot(providerName, clientReq, requestBody.toString("utf8")),
          );
        } catch (err) {
          clientRes.writeHead(400);
          clientRes.end(`Invalid request snapshot: ${err}`);
          return;
        }

        const isHttps = targetURL.protocol === "https:";
        const reqFn = isHttps ? httpsRequest : httpRequest;

        const headers = { ...clientReq.headers };
        delete headers.host;
        delete headers["accept-encoding"];
        headers.host = targetURL.host;

        const proxyReq = reqFn(
          {
            hostname: targetURL.hostname,
            port: targetURL.port || (isHttps ? 443 : 80),
            path: clientReq.url ?? "",
            method: clientReq.method,
            headers,
          },
          (proxyRes) => {
            const responseChunks: Buffer[] = [];

            clientRes.writeHead(
              proxyRes.statusCode ?? 200,
              proxyRes.headers
            );

            proxyRes.on("data", (chunk: Buffer) => {
              responseChunks.push(chunk);
              clientRes.write(chunk);
            });

            proxyRes.on("end", () => {
              clientRes.end();
              const fullBuf = Buffer.concat(responseChunks);
              let lines: string[];
              if (providerName === "bedrock") {
                lines = decodeBedrockEventStreamToJSONLines(fullBuf);
              } else {
                const fullBody = fullBuf.toString("utf8");
                lines = extractDataLines(fullBody);
              }
              onResponse(currentIndex, lines.join("\n") + "\n");
            });
          }
        );

        proxyReq.on("error", (err) => {
          console.error(`  Proxy error: ${err.message}`);
          clientRes.writeHead(502);
          clientRes.end("Proxy error");
        });

        proxyReq.write(requestBody);
        proxyReq.end();
      });
    }
  );

  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address() as { port: number };
      resolve({
        port: addr.port,
        close: () => new Promise<void>((r) => server.close(() => r())),
      });
    });
  });
}

function extractDataLines(sseBody: string): string[] {
  return sseBody
    .split("\n")
    .filter((line) => line.startsWith("data: "))
    .map((line) => line.slice(6).trim())
    .filter((line) => line !== "[DONE]" && line !== "");
}

// makeBedrockFetchTap returns a fetch implementation that delegates to the
// real fetch (so SigV4 signing stays valid against bedrock-runtime) and tees
// the response body into a buffer that's reported to onResponse on stream
// completion. Used in place of the HTTP proxy for Bedrock recording because
// SigV4 signs the request URL — proxying through 127.0.0.1 breaks the
// canonical request hash.
function makeBedrockFetchTap(
  onResponse: (index: number, body: string) => void,
  onRequest: (index: number, request: RequestSnapshot) => void
): typeof fetch {
  let callIndex = 0;
  return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const idx = callIndex++;

    // Capture the request snapshot from the outbound fetch before delegating
    // to the real Bedrock endpoint. SigV4 is left untouched because we forward
    // the original input/init unchanged.
    try {
      const url = new URL(
        typeof input === "string" || input instanceof URL ? input.toString() : input.url
      );
      const method =
        init?.method ??
        (typeof input === "object" && "method" in input ? input.method : "GET");
      const headers: Record<string, string> = {};
      new Headers(init?.headers ?? (typeof input === "object" && "headers" in input ? input.headers : undefined)).forEach(
        (value, key) => {
          headers[key] = value;
        }
      );
      const bodyText =
        typeof init?.body === "string" ? init.body : init?.body ? String(init.body) : "";
      onRequest(
        idx,
        normalizeRequestSnapshot(
          "bedrock",
          { method, url: url.pathname + url.search, headers },
          bodyText
        )
      );
    } catch (err) {
      console.error(`  Bedrock request snapshot error: ${err}`);
    }

    const realResponse = await fetch(input, init);
    if (!realResponse.body) {
      return realResponse;
    }
    const [forwardStream, captureStream] = realResponse.body.tee();
    // Drain the capture stream in the background; emit decoded JSON lines
    // when complete.
    (async () => {
      const reader = captureStream.getReader();
      const chunks: Uint8Array[] = [];
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        if (value) chunks.push(value);
      }
      const buf = Buffer.concat(chunks.map((u) => Buffer.from(u)));
      const lines = decodeBedrockEventStreamToJSONLines(buf);
      onResponse(idx, lines.join("\n") + "\n");
    })().catch((err) => {
      console.error(`  Bedrock fetch tap error: ${err}`);
    });
    return new Response(forwardStream, {
      status: realResponse.status,
      statusText: realResponse.statusText,
      headers: realResponse.headers,
    });
  };
}

// decodeBedrockEventStreamToJSONLines walks an AWS Smithy event-stream
// payload and emits one JSON line per event frame, mirroring the upstream
// __fixtures__ format `{"<eventType>": {...inner JSON...}}`.
function decodeBedrockEventStreamToJSONLines(buf: Buffer): string[] {
  const out: string[] = [];
  let off = 0;
  while (off + 12 <= buf.length) {
    const totalLen = buf.readUInt32BE(off);
    const headersLen = buf.readUInt32BE(off + 4);
    if (off + totalLen > buf.length) break;

    const headersStart = off + 12;
    const payloadStart = headersStart + headersLen;
    const payloadEnd = off + totalLen - 4;

    let eventType = "unknown";
    let i = headersStart;
    while (i < payloadStart) {
      const nameLen = buf[i];
      i += 1;
      const name = buf.subarray(i, i + nameLen).toString("ascii");
      i += nameLen;
      const valueType = buf[i];
      i += 1;
      if (valueType === 7 || valueType === 6) {
        const valueLen = buf.readUInt16BE(i);
        i += 2;
        const value = buf.subarray(i, i + valueLen).toString("utf8");
        i += valueLen;
        if (name === ":event-type") {
          eventType = value;
        }
      } else {
        // Skip non-string typed headers by consuming the value bytes.
        const skip = headerValueSize(valueType);
        i += skip;
      }
    }

    const payload = buf.subarray(payloadStart, payloadEnd).toString("utf8");
    try {
      const inner = JSON.parse(payload);
      out.push(JSON.stringify({ [eventType]: inner }));
    } catch {
      // Skip undecodable payloads.
    }
    off += totalLen;
  }
  return out;
}

function headerValueSize(valueType: number): number {
  switch (valueType) {
    case 0:
    case 1:
      return 0;
    case 2:
      return 1;
    case 3:
      return 2;
    case 4:
      return 4;
    case 5:
    case 8:
      return 8;
    case 9:
      return 16;
    default:
      return 0;
  }
}

// --- Test Case Discovery (recorded/ only) ---

function discoverRecordedCases(): TestCase[] {
  const cases: TestCase[] = [];

  for (const entry of readdirSync(CONFORMANCE_ROOT, { withFileTypes: true })) {
    if (!entry.isDirectory() || entry.name === "tools") continue;
    const providerName = entry.name;
    const recordedDir = join(CONFORMANCE_ROOT, providerName, "recorded");
    if (!existsSync(recordedDir)) continue;

    for (const tcEntry of readdirSync(recordedDir, { withFileTypes: true })) {
      if (!tcEntry.isDirectory()) continue;
      const dir = join(recordedDir, tcEntry.name);
      if (!existsSync(join(dir, "config.yaml"))) continue;

      cases.push({
        name: `${providerName}/recorded/${tcEntry.name}`,
        dir,
        provider: providerName,
      });
    }
  }

  return cases;
}

// --- Record ---

async function recordTestCase(tc: TestCase): Promise<void> {
  const cfg = loadConfig(tc.dir);

  if (!cfg.prompt) {
    console.error(`  SKIP: no prompt in config (required for recording)`);
    return;
  }

  const fixtures: Map<number, string> = new Map();
  const requestSnapshots: Map<number, RequestSnapshot> = new Map();

  let port = 0;
  let close: () => Promise<void> = async () => {};
  let bedrockFetch: typeof fetch | undefined;
  if (tc.provider === "bedrock") {
    // Bedrock cannot use a host-changing HTTP proxy because SigV4 binds
    // the signature to the URL host. Use a fetch tap instead.
    bedrockFetch = makeBedrockFetchTap(
      (index, body) => {
        fixtures.set(index, body);
      },
      (index, request) => {
        requestSnapshots.set(index, request);
      }
    );
  } else {
    const proxy = await startRecordingProxy(
      tc.provider,
      (index, body) => {
        fixtures.set(index, body);
      },
      (index, request) => {
        requestSnapshots.set(index, request);
      }
    );
    port = proxy.port;
    close = proxy.close;
  }

  try {
    const providerFactory: Record<string, (model: string, port: number) => any> = {
      anthropic: (modelId, p) => {
        const provider = createAnthropic({
          baseURL: `http://127.0.0.1:${p}/v1`,
          apiKey: process.env.ANTHROPIC_API_KEY ?? "",
        });
        return provider(modelId);
      },
      openai: (modelId, p) => {
        const provider = createOpenAI({
          baseURL: `http://127.0.0.1:${p}/v1`,
          apiKey: process.env.OPENAI_API_KEY ?? "",
        });
        return provider.responses(modelId);
      },
      bedrock: (modelId, _p) => {
        // Bedrock SigV4 binds signatures to the URL host. Instead of
        // routing through a local proxy, we use a custom fetch that
        // delegates to the real Bedrock endpoint and tees the response
        // body into our fixture buffer.
        const provider = createAmazonBedrock({
          region: process.env.AWS_REGION ?? process.env.AWS_DEFAULT_REGION ?? "us-east-1",
          fetch: bedrockFetch,
        });
        return provider(modelId);
      },
      "openai-compatible": (modelId, p) => {
        const provider = createOpenAICompatible({
          name: "openai-compatible",
          baseURL: `http://127.0.0.1:${p}/v1`,
          apiKey: process.env.OPENAI_COMPATIBLE_API_KEY ?? process.env.OPENAI_API_KEY ?? "",
          includeUsage: true,
          supportsStructuredOutputs: true,
        });
        return provider(modelId);
      },
    };

    const createModel = providerFactory[tc.provider];
    if (!createModel) throw new Error(`Unknown provider: ${tc.provider}`);

    const model = createModel(cfg.model, port);
    const tools = buildTools(cfg.tools, cfg.providerTools);
    const stopWhenStepCount = cfg.stopWhenStepCount ?? 1;
    const messages = cfg.uiMessages
      ? await convertToModelMessages(cfg.uiMessages, { tools })
      : buildMessages(cfg, cfg.prompt);

    const output = buildOutput(cfg);

    const result = streamText(buildStreamTextOptions(cfg, {
      model,
      messages,
      prompt: cfg.prompt,
      tools,
      output,
      stopWhen: stepCountIs(stopWhenStepCount),
      generateId: mockId("id"),
    }) as Parameters<typeof streamText>[0]);

    const uiStream = result.toUIMessageStream(cfg.streamOptions ?? {});

    const chunks: unknown[] = [];
    const normalizeSourceId =
      tc.provider === "openai" ? createSourceIdNormalizer() : (chunk: unknown) => chunk;
    for await (const chunk of uiStream) {
      chunks.push(normalizeSourceId(chunk));
    }

    const sortedKeys = [...fixtures.keys()].sort((a, b) => a - b);

    for (const key of sortedKeys) {
      const content = fixtures.get(key)!;
      if (content.trim() === "") {
        throw new Error(
          `Empty fixture captured for step ${key + 1} — the API likely returned an error`
        );
      }
    }

    if (sortedKeys.length === 1) {
      writeFileSync(join(tc.dir, "input.chunks.txt"), fixtures.get(sortedKeys[0])!);
    } else {
      for (let i = 0; i < sortedKeys.length; i++) {
        writeFileSync(
          join(tc.dir, `input-${i + 1}.chunks.txt`),
          fixtures.get(sortedKeys[i])!
        );
      }
    }
    writeRequestSnapshots(
      join(tc.dir, "expected-requests.jsonl"),
      sortedKeys.map((key) => {
        const request = requestSnapshots.get(key);
        if (!request) throw new Error(`Missing request snapshot for step ${key + 1}`);
        return request;
      }),
    );

    const jsonl = chunks.map((c) => JSON.stringify(c)).join("\n") + "\n";
    writeFileSync(join(tc.dir, "expected.jsonl"), jsonl);

    console.log(
      `  OK: ${sortedKeys.length} fixture(s), ${requestSnapshots.size} request(s), ${chunks.length} chunks`
    );
  } finally {
    await close();
  }
}

// --- Main ---

function checkAPIKeys(providers: Set<string>): void {
  const keyEnvVars: Record<string, string> = {
    anthropic: "ANTHROPIC_API_KEY",
    openai: "OPENAI_API_KEY",
  };
  const missing: string[] = [];
  for (const p of providers) {
    if (p === "bedrock") {
      // Bedrock relies on AWS env vars or AWS_BEARER_TOKEN_BEDROCK. Surface
      // a hint, but don't require a fixed env var name since the SDK
      // accepts any of the standard credential sources.
      const hasAWS =
        process.env.AWS_BEARER_TOKEN_BEDROCK ||
        (process.env.AWS_ACCESS_KEY_ID && process.env.AWS_SECRET_ACCESS_KEY);
      if (!hasAWS) {
        missing.push(
          "AWS credentials (AWS_BEARER_TOKEN_BEDROCK or AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY)"
        );
      }
      continue;
    }
    if (p === "openai-compatible") {
      if (!process.env.OPENAI_COMPATIBLE_API_KEY && !process.env.OPENAI_API_KEY) {
        missing.push("OPENAI_COMPATIBLE_API_KEY or OPENAI_API_KEY (for openai-compatible)");
      }
      continue;
    }
    const envVar = keyEnvVars[p];
    if (envVar && !process.env[envVar]) {
      missing.push(`${envVar} (for ${p})`);
    }
  }
  if (missing.length > 0) {
    console.error(`Missing API keys:\n  ${missing.join("\n  ")}`);
    process.exit(1);
  }
}

async function main() {
  const scenarioIdx = process.argv.indexOf("--scenario");
  const scenarioFilter =
    scenarioIdx !== -1 ? process.argv[scenarioIdx + 1] : null;

  let cases = discoverRecordedCases();

  if (scenarioFilter) {
    cases = cases.filter((tc) => tc.name.includes(scenarioFilter));
    if (cases.length === 0) {
      console.error(`No recorded test cases matching: ${scenarioFilter}`);
      process.exit(1);
    }
  }

  if (cases.length === 0) {
    console.log("No recorded test cases found.");
    console.log("Create a config.yaml in a <provider>/recorded/<name>/ directory.");
    return;
  }

  const providers = new Set(cases.map((tc) => tc.provider));
  checkAPIKeys(providers);

  console.log(`Found ${cases.length} recorded test case(s)\n`);

  let errors = 0;
  for (const tc of cases) {
    console.log(`Recording: ${tc.name}`);
    try {
      await recordTestCase(tc);
    } catch (err) {
      console.error(`  ERROR: ${err}`);
      errors++;
    }
  }

  if (errors > 0) {
    console.error(`\n${errors} error(s) encountered.`);
    process.exit(1);
  }

  console.log("\nDone.");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
