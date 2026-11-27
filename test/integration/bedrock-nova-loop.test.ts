import { crc32 } from "node:zlib";
import { createAmazonBedrock } from "@ai-sdk/amazon-bedrock";
import { jsonSchema, stepCountIs, tool, ToolLoopAgent } from "ai";
import { expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";

const modelId = "us.amazon.nova-2-lite-v1:0";
const baseURL = "https://bedrock-runtime.us-east-1.invalid";
const prompt = "Read temperature and humidity.";
const maxOutputTokens = 1024;
const calls = [
  { toolUseId: "temp-1", name: "temperature", input: { station: "north" } },
  { toolUseId: "humidity-1", name: "humidity", input: { station: "south" } },
];

function event(type: string, body: unknown): Buffer {
  const headers = Buffer.concat([
    [":message-type", "event"], [":event-type", type], [":content-type", "application/json"],
  ].map(([name, value]) => Buffer.concat([
    Buffer.from([name.length]), Buffer.from(name), Buffer.from([7, 0, value.length]), Buffer.from(value),
  ])));
  const payload = Buffer.from(JSON.stringify(body));
  const frame = Buffer.alloc(16 + headers.length + payload.length);
  frame.writeUInt32BE(frame.length, 0);
  frame.writeUInt32BE(headers.length, 4);
  frame.writeUInt32BE(crc32(frame.subarray(0, 8)), 8);
  headers.copy(frame, 12);
  payload.copy(frame, 12 + headers.length);
  frame.writeUInt32BE(crc32(frame.subarray(0, -4)), frame.length - 4);
  return frame;
}

const start = event("messageStart", { role: "assistant" });
const usage = event("metadata", { usage: { inputTokens: 12, outputTokens: 8, totalTokens: 20 }, metrics: { latencyMs: 1 } });
// Synthetic ConverseStream event shapes, not a recording or evidence Nova returns these events.
const syntheticResponses = [
  Buffer.concat([
    start,
    ...calls.flatMap(({ name, toolUseId, input }, contentBlockIndex) => [
      event("contentBlockStart", { contentBlockIndex, start: { toolUse: { name, toolUseId } } }),
      event("contentBlockDelta", { contentBlockIndex, delta: { toolUse: { input: JSON.stringify(input) } } }),
      event("contentBlockStop", { contentBlockIndex }),
    ]),
    event("messageStop", { stopReason: "tool_use" }),
    usage,
  ]),
  Buffer.concat([
    start,
    event("contentBlockDelta", { contentBlockIndex: 0, delta: { text: "Both readings ready." } }),
    event("contentBlockStop", { contentBlockIndex: 0 }),
    event("messageStop", { stopReason: "end_turn" }),
    usage,
  ]),
];

type NativeRequest = {
  method: string;
  url: string;
  body: {
    additionalModelRequestFields: unknown;
    inferenceConfig?: { maxTokens?: number };
    messages: { role: string; content: { toolResult?: { toolUseId: string }; [key: string]: unknown }[] }[];
    toolConfig: { tools: { toolSpec: { name: string } }[] };
  };
};

async function runVercelNovaLoop() {
  const requests: NativeRequest[] = [];
  const bedrock = createAmazonBedrock({
    baseURL,
    region: "us-east-1",
    apiKey: "",
    accessKeyId: "nova-test",
    secretAccessKey: "nova-secret",
    fetch: async (url, init) => {
      const request = new Request(url, init);
      const turn = requests.length;
      if (request.method !== "POST" || new URL(request.url).origin !== baseURL ||
          !request.headers.get("authorization")?.startsWith("AWS4-HMAC-SHA256 Credential=nova-test/") ||
          turn >= syntheticResponses.length) {
        throw new Error(`Unexpected Nova transport request on turn ${turn + 1}: ${request.method} ${request.url}`);
      }
      requests.push({ method: request.method, url: request.url, body: await request.json() });
      return new Response(new Uint8Array(syntheticResponses[turn]), {
        headers: { "content-type": "application/vnd.amazon.eventstream" },
      });
    },
  });
  let release!: () => void;
  const bothStarted = new Promise<void>(resolve => { release = resolve; });
  let started = 0;
  const reading = (value: string) => tool({
    inputSchema: jsonSchema<{ station: string }>({
      type: "object",
      properties: { station: { type: "string" } },
      required: ["station"],
      additionalProperties: false,
    }),
    execute: async ({ station }) => {
      if (++started === 2) release();
      let timer: ReturnType<typeof setTimeout> | undefined;
      try {
        await Promise.race([bothStarted, new Promise<never>((_, reject) => {
          timer = setTimeout(() => reject(new Error("Both Nova tools did not start")), 5_000);
        })]);
      } finally {
        clearTimeout(timer);
      }
      return `${station}: ${value}`;
    },
  });
  const agent = new ToolLoopAgent({
    model: bedrock(modelId),
    reasoning: "high",
    maxOutputTokens,
    maxRetries: 0,
    tools: { temperature: reading("21 C"), humidity: reading("58%") },
    stopWhen: stepCountIs(2),
    prepareStep: ({ stepNumber }) => stepNumber === 1 ? {
      providerOptions: { amazonBedrock: { reasoningConfig: { maxReasoningEffort: "medium" } } },
    } : {},
  });
  const result = await agent.stream({ prompt });
  for await (const part of result.fullStream) {
    if (part.type === "error" || part.type === "tool-error") throw part.error;
  }
  return {
    requests,
    steps: (await result.steps).map(step => ({
      warnings: step.warnings,
      finishReason: { unified: step.finishReason, raw: step.rawFinishReason },
    })),
    text: await result.text,
  };
}

const initialMessage = { role: "user", content: [{ text: prompt }] };
const expectedResults = [
  { toolResult: { toolUseId: "humidity-1", content: [{ text: "south: 58%" }] } },
  { toolResult: { toolUseId: "temp-1", content: [{ text: "north: 21 C" }] } },
];

it("matches pinned Vercel on Nova 2 Lite high-to-medium ConverseStream with parallel tool results", async () => {
  const reference = await runVercelNovaLoop();
  const response = await fetchScenario("bedrock-nova-loop", {
    headers: { "content-type": "application/json" },
    body: JSON.stringify(syntheticResponses.map(body => body.toString("base64"))),
    signal: AbortSignal.timeout(15_000),
  });
  const raw = await response.text();
  expect(response.status, raw).toBe(200);
  const candidate: typeof reference = JSON.parse(raw);

  for (const [sdk, actual] of [["Vercel", reference], ["Go", candidate]] as const) {
    expect(actual.requests, sdk).toHaveLength(2);
    const [high, medium] = actual.requests;
    for (const request of actual.requests) {
      const url = new URL(request.url);
      expect(request.method, sdk).toBe("POST");
      expect(url.origin, sdk).toBe(baseURL);
      expect(decodeURIComponent(url.pathname), sdk).toBe(`/model/${modelId}/converse-stream`);
      expect(url.search, sdk).toBe("");
      expect(request.body.toolConfig.tools.map(({ toolSpec }) => toolSpec.name).sort(), sdk)
        .toEqual(["humidity", "temperature"]);
    }

    expect(high.body.additionalModelRequestFields, `${sdk} high reasoning`).toStrictEqual({
      reasoningConfig: { type: "enabled", maxReasoningEffort: "high" },
    });
    expect(high.body.inferenceConfig, `${sdk} high reasoning maxTokens`).toBeUndefined();
    expect(medium.body.additionalModelRequestFields, `${sdk} medium override`).toStrictEqual({
      reasoningConfig: { type: "enabled", maxReasoningEffort: "medium" },
    });
    expect(medium.body.inferenceConfig, `${sdk} medium reasoning maxTokens`).toStrictEqual({ maxTokens: maxOutputTokens });
    expect(actual.steps, sdk).toStrictEqual([
      {
        finishReason: { unified: "tool-calls", raw: "tool_use" },
        warnings: [{
          type: "unsupported", feature: "maxOutputTokens",
          details: `maxOutputTokens is not supported by ${modelId} when high reasoning is enabled and will be ignored`,
        }],
      },
      { finishReason: { unified: "stop", raw: "end_turn" }, warnings: [] },
    ]);

    expect(high.body.messages, `${sdk} initial history`).toStrictEqual([initialMessage]);
    const messages = medium.body.messages;
    expect(messages, `${sdk} continuation history`).toHaveLength(3);
    expect(messages.slice(0, 2), `${sdk} ordered assistant tool calls`).toStrictEqual([
      initialMessage,
      { role: "assistant", content: calls.map(toolUse => ({ toolUse })) },
    ]);
    expect(messages[2].role, sdk).toBe("user");
    expect([...messages[2].content].sort((a, b) =>
      (a.toolResult?.toolUseId ?? "").localeCompare(b.toolResult?.toolUseId ?? ""),
    ), `${sdk} result payloads by original toolUseId`).toStrictEqual(expectedResults);
    expect(actual.text, sdk).toBe("Both readings ready.");
  }
});
