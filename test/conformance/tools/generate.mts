#!/usr/bin/env tsx

import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import {
  readFileSync,
  writeFileSync,
  readdirSync,
  existsSync,
} from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { Buffer } from "node:buffer";
import { createAnthropic } from "@ai-sdk/anthropic";
import { createOpenAI } from "@ai-sdk/openai";
import { createAmazonBedrock } from "@ai-sdk/amazon-bedrock";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import {
  convertToModelMessages,
  streamText,
  stepCountIs,
  type LanguageModelUsage,
} from "ai";
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

// --- Fixtures ---

function loadFixtures(dir: string): string[] {
  const singlePath = join(dir, "input.chunks.txt");
  if (existsSync(singlePath)) {
    return [readFileSync(singlePath, "utf8")];
  }

  const fixtures: string[] = [];
  for (let i = 1; ; i++) {
    const path = join(dir, `input-${i}.chunks.txt`);
    if (!existsSync(path)) break;
    fixtures.push(readFileSync(path, "utf8"));
  }
  return fixtures;
}

function extractEventType(json: string): string {
  try {
    const parsed = JSON.parse(json);
    return parsed.type ?? "unknown";
  } catch {
    return "unknown";
  }
}

function fixtureToSSE(fixture: string): string {
  return fixture
    .split("\n")
    .filter(Boolean)
    .map((line) => `event: ${extractEventType(line)}\ndata: ${line}\n\n`)
    .join("");
}

// --- Bedrock binary event-stream framing ---

const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let i = 0; i < 256; i++) {
    let c = i;
    for (let j = 0; j < 8; j++) c = (c & 1) ? (0xedb88320 ^ (c >>> 1)) : (c >>> 1);
    t[i] = c >>> 0;
  }
  return t;
})();

function crc32(buf: Uint8Array): number {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function encodeBedrockHeader(name: string, value: string): Buffer {
  const nameBytes = Buffer.from(name, "ascii");
  const valueBytes = Buffer.from(value, "utf8");
  const len = Buffer.alloc(2);
  len.writeUInt16BE(valueBytes.length, 0);
  return Buffer.concat([Buffer.from([nameBytes.length]), nameBytes, Buffer.from([7]), len, valueBytes]);
}

function encodeBedrockFrame(eventType: string, payload: Buffer): Buffer {
  const headers = Buffer.concat([
    encodeBedrockHeader(":message-type", "event"),
    encodeBedrockHeader(":event-type", eventType),
    encodeBedrockHeader(":content-type", "application/json"),
  ]);
  const headersLen = headers.length;
  const totalLen = 8 + 4 + headersLen + payload.length + 4;

  const prelude = Buffer.alloc(8);
  prelude.writeUInt32BE(totalLen, 0);
  prelude.writeUInt32BE(headersLen, 4);
  const preludeCRC = Buffer.alloc(4);
  preludeCRC.writeUInt32BE(crc32(new Uint8Array(prelude)), 0);

  const upToMsgCRC = Buffer.concat([prelude, preludeCRC, headers, payload]);
  const msgCRC = Buffer.alloc(4);
  msgCRC.writeUInt32BE(crc32(new Uint8Array(upToMsgCRC)), 0);

  return Buffer.concat([upToMsgCRC, msgCRC]);
}

function fixtureToBedrockEventStream(fixture: string): Buffer {
  const frames: Buffer[] = [];
  for (const line of fixture.split("\n").filter(Boolean)) {
    let wrapper: Record<string, unknown>;
    try {
      wrapper = JSON.parse(line);
    } catch {
      continue;
    }
    const keys = Object.keys(wrapper);
    if (keys.length !== 1) continue;
    const eventType = keys[0];
    const payload = Buffer.from(JSON.stringify(wrapper[eventType]), "utf8");
    frames.push(encodeBedrockFrame(eventType, payload));
  }
  return Buffer.concat(frames);
}

// --- Replay Server ---

type Framing = "sse" | "bedrock";

function framingFor(provider: string): Framing {
  return provider === "bedrock" ? "bedrock" : "sse";
}

async function startReplayServer(
  providerName: string,
  fixtures: string[],
  framing: Framing
): Promise<{ port: number; close: () => Promise<void>; requests: RequestSnapshot[] }> {
  let callIndex = 0;
  const responses: (string | Buffer)[] =
    framing === "bedrock"
      ? fixtures.map(fixtureToBedrockEventStream)
      : fixtures.map(fixtureToSSE);
  const contentType =
    framing === "bedrock" ? "application/vnd.amazon.eventstream" : "text/event-stream";
  const requests: RequestSnapshot[] = [];

  const server = createServer(
    (req: IncomingMessage, res: ServerResponse) => {
      const chunks: Buffer[] = [];
      req.on("data", (chunk: Buffer) => chunks.push(chunk));
      req.on("end", () => {
        const requestBody = Buffer.concat(chunks).toString("utf8");
        const idx = callIndex++;

        if (idx >= responses.length) {
          res.writeHead(500);
          res.end(`No more fixtures (got request ${idx + 1}, have ${responses.length})`);
          return;
        }

        try {
          requests.push(normalizeRequestSnapshot(providerName, req, requestBody));
        } catch (err) {
          res.writeHead(400);
          res.end(`Invalid request snapshot: ${err}`);
          return;
        }

        res.writeHead(200, {
          "Content-Type": contentType,
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
        });
        res.end(responses[idx]);
      });
    }
  );

  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address() as { port: number };
      resolve({
        port: addr.port,
        requests,
        close: () => new Promise<void>((r) => server.close(() => r())),
      });
    });
  });
}

// --- Test Case Discovery ---

function discoverTestCases(): TestCase[] {
  const cases: TestCase[] = [];

  for (const entry of readdirSync(CONFORMANCE_ROOT, { withFileTypes: true })) {
    if (!entry.isDirectory() || entry.name === "tools") continue;
    const providerName = entry.name;
    const providerDir = join(CONFORMANCE_ROOT, providerName);

    for (const category of ["upstream", "recorded"]) {
      const catDir = join(providerDir, category);
      if (!existsSync(catDir)) continue;

      for (const tcEntry of readdirSync(catDir, { withFileTypes: true })) {
        if (!tcEntry.isDirectory()) continue;
        const dir = join(catDir, tcEntry.name);
        if (!existsSync(join(dir, "config.yaml"))) continue;

        cases.push({
          name: `${providerName}/${category}/${tcEntry.name}`,
          dir,
          provider: providerName,
        });
      }
    }
  }

  return cases;
}

// --- Provider Factory ---

function createModel(
  providerName: string,
  modelId: string,
  port: number
) {
  switch (providerName) {
    case "anthropic": {
      const provider = createAnthropic({
        baseURL: `http://127.0.0.1:${port}/v1`,
        apiKey: "test-api-key",
      });
      return provider(modelId);
    }
    case "openai": {
      const provider = createOpenAI({
        baseURL: `http://127.0.0.1:${port}/v1`,
        apiKey: "test-api-key",
      });
      return provider.responses(modelId);
    }
    case "bedrock": {
      // The upstream Bedrock client posts to <baseURL>/model/<modelId>/converse-stream
      // and authenticates via SigV4 or a Bearer token. For replay we point
      // baseURL at our local server and pass an API key so it skips SigV4.
      const provider = createAmazonBedrock({
        baseURL: `http://127.0.0.1:${port}`,
        apiKey: "test-api-key",
        region: "us-east-1",
      });
      return provider(modelId);
    }
    case "openai-compatible": {
      const provider = createOpenAICompatible({
        name: "openai-compatible",
        baseURL: `http://127.0.0.1:${port}/v1`,
        apiKey: "test-api-key",
        includeUsage: true,
        supportsStructuredOutputs: true,
      });
      return provider(modelId);
    }
    default:
      throw new Error(`Unknown provider: ${providerName}`);
  }
}

// --- Generate Expected Output ---

function toProviderUsage(usage: LanguageModelUsage) {
  return {
    inputTokens: {
      total: usage.inputTokens,
      noCache: usage.inputTokenDetails.noCacheTokens,
      cacheRead: usage.inputTokenDetails.cacheReadTokens,
      cacheWrite: usage.inputTokenDetails.cacheWriteTokens,
    },
    outputTokens: {
      total: usage.outputTokens,
      text: usage.outputTokenDetails.textTokens,
      reasoning: usage.outputTokenDetails.reasoningTokens,
    },
    raw: usage.raw,
  };
}

function normalizeOpenAIApprovalToolCallIds(
  providerName: string,
  chunks: unknown[],
): unknown[] {
  if (providerName !== "openai") return chunks;

  const mcpToolCallIds = new Set<string>();
  for (const chunk of chunks) {
    if (
      typeof chunk === "object" &&
      chunk !== null &&
      "type" in chunk &&
      chunk.type === "tool-input-available" &&
      "toolName" in chunk &&
      typeof chunk.toolName === "string" &&
      chunk.toolName.startsWith("mcp.") &&
      "toolCallId" in chunk &&
      typeof chunk.toolCallId === "string"
    ) {
      mcpToolCallIds.add(chunk.toolCallId);
    }
  }

  const replacements = new Map<string, string>();
  for (const chunk of chunks) {
    if (
      typeof chunk === "object" &&
      chunk !== null &&
      "type" in chunk &&
      chunk.type === "tool-approval-request" &&
      "toolCallId" in chunk &&
      typeof chunk.toolCallId === "string" &&
      mcpToolCallIds.has(chunk.toolCallId)
    ) {
      replacements.set(chunk.toolCallId, `src-${replacements.size}`);
    }
  }

  return chunks.map(chunk => {
    if (
      typeof chunk !== "object" ||
      chunk === null ||
      !("toolCallId" in chunk) ||
      typeof chunk.toolCallId !== "string"
    ) {
      return chunk;
    }
    const toolCallId = replacements.get(chunk.toolCallId);
    return toolCallId ? { ...chunk, toolCallId } : chunk;
  });
}

async function generateExpected(tc: TestCase): Promise<void> {
  const cfg = loadConfig(tc.dir);
  const fixtures = loadFixtures(tc.dir);

  if (fixtures.length === 0) {
    console.error(`  SKIP: no fixture files found`);
    return;
  }

  const { port, close, requests } = await startReplayServer(
    tc.provider,
    fixtures,
    framingFor(tc.provider)
  );

  try {
    // Cast through `any` to bridge minor provider-type version drift between
    // the @ai-sdk/{anthropic,amazon-bedrock} subdependencies and the root
    // @ai-sdk/provider used by `ai`. The runtime contract is the same; only
    // the TS path differs between hoisted package copies.
    const model = createModel(tc.provider, cfg.model, port) as any;
    const tools = buildTools(cfg.tools, cfg.providerTools);
    const stopWhenStepCount = cfg.stopWhenStepCount ?? 1;
    const prompt = cfg.prompt ?? "test";
    const messages = cfg.uiMessages
      ? await convertToModelMessages(cfg.uiMessages, { tools })
      : buildMessages(cfg, prompt);

    const output = buildOutput(cfg);

    const result = streamText(buildStreamTextOptions(cfg, {
      model,
      messages,
      prompt,
      tools,
      output,
      stopWhen: stepCountIs(stopWhenStepCount),
      generateId: mockId("id"),
    }) as Parameters<typeof streamText>[0]);

    const uiStream = result.toUIMessageStream(cfg.streamOptions ?? {});

    const chunks: unknown[] = [];
    const normalizeSourceId =
      tc.provider === "openai" ? createSourceIdNormalizer() : (chunk: unknown) => chunk;
    const objectPromise = output
      ? Promise.resolve(result.output).catch((err: unknown) => ({ __error: String(err) }))
      : undefined;
    const usagePath = join(tc.dir, "expected-usage.json");
    const usagePromise = existsSync(usagePath)
      ? result.steps.then(steps => steps.map(step => toProviderUsage(step.usage)))
      : undefined;

    for await (const chunk of uiStream) {
      chunks.push(normalizeSourceId(chunk));
    }

    const normalizedChunks = normalizeOpenAIApprovalToolCallIds(tc.provider, chunks);
    const jsonl = normalizedChunks.map((c: unknown) => JSON.stringify(c)).join("\n") + "\n";
    const outputPath = join(tc.dir, "expected.jsonl");
    writeFileSync(outputPath, jsonl);
    writeRequestSnapshots(join(tc.dir, "expected-requests.jsonl"), requests);
    if (usagePromise) {
      writeFileSync(usagePath, JSON.stringify(await usagePromise, null, 2) + "\n");
    }

    if (objectPromise) {
      const obj = await objectPromise;
      if (obj && typeof obj === "object" && "__error" in obj) {
        console.log(`  OK: ${chunks.length} chunks, ${requests.length} request(s) -> expected.jsonl + expected-requests.jsonl (output validation failed: ${obj.__error})`);
      } else {
        const objectPath = join(tc.dir, "expected-object.json");
        writeFileSync(objectPath, JSON.stringify(obj, null, 2) + "\n");
        console.log(`  OK: ${chunks.length} chunks, ${requests.length} request(s) -> expected.jsonl + expected-requests.jsonl + expected-object.json`);
      }
    } else {
      console.log(`  OK: ${chunks.length} chunks, ${requests.length} request(s) -> expected.jsonl + expected-requests.jsonl`);
    }
  } finally {
    await close();
  }
}

// --- Main ---

async function main() {
  const scenarioIdx = process.argv.indexOf("--scenario");
  const scenarioFilter = scenarioIdx !== -1 ? process.argv[scenarioIdx + 1] : null;

  let cases = discoverTestCases();

  if (scenarioFilter) {
    cases = cases.filter((tc) => tc.name.includes(scenarioFilter));
    if (cases.length === 0) {
      console.error(`No test cases matching: ${scenarioFilter}`);
      process.exit(1);
    }
  }

  if (cases.length === 0) {
    console.log("No test cases found.");
    return;
  }

  console.log(`Found ${cases.length} test case(s)\n`);

  let errors = 0;
  for (const tc of cases) {
    console.log(`Generating: ${tc.name}`);
    try {
      await generateExpected(tc);
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
