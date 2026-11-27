import { createServer } from "node:http";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import {
  jsonSchema, readUIMessageStream, stepCountIs, streamText, tool, uiMessageChunkSchema,
  wrapLanguageModel, type UIMessage, type UIMessageChunk,
} from "ai";
import { expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";

const prompt = "Run both lookups once.";
const modelId = "synthetic-chat";
const badgeId = "call-badge";
const wordId = "call-word";
const badgeArgs = '{"locker":"north-7"}';
const wordArgs = '{"shelf":"south-4"}';
const badge = { badge: "badge:north-7" };
const error = "word unavailable: south-4";
const finalText = "Badge found; word unavailable.";
const usage = [
  { prompt_tokens: 13, completion_tokens: 8, total_tokens: 21 },
  { prompt_tokens: 27, completion_tokens: 6, total_tokens: 33 },
];

function choice(turn: number, delta: object, finish_reason: string | null = null) {
  return {
    id: `synthetic-${turn}`, model: modelId, created: 1700000000,
    choices: [{ index: 0, delta, finish_reason }],
  };
}

function toolDelta(index: number, args: string, call?: { id: string; name: string }) {
  return choice(1, {
    tool_calls: [{ index, ...(call && { id: call.id, type: "function" }),
      function: { ...(call && { name: call.name }), arguments: args } }],
  });
}

const frames = [
  [choice(1, { role: "assistant" }),
    toolDelta(0, '{"locker":"nor', { id: badgeId, name: "read_badge" }),
    toolDelta(1, '{"shelf":"sou', { id: wordId, name: "read_word" }),
    toolDelta(0, 'th-'), toolDelta(1, 'th-4"}'), toolDelta(0, '7"}'),
    choice(1, {}, "tool_calls"), { choices: [], usage: usage[0] }],
  [choice(2, { role: "assistant" }), choice(2, { content: "Badge found; " }),
    choice(2, { content: "word unavailable." }), choice(2, {}, "stop"),
    { choices: [], usage: usage[1] }],
];

type ChatMessage = { role: string; content?: string; tool_call_id?: string; tool_calls?: unknown[] };
type ChatRequest = { model: string; stream: boolean; stream_options: unknown; messages: ChatMessage[] };
type ProviderPart = { type: string; id?: string; toolCallId?: string; toolName?: string; input?: string; delta?: string; finishReason?: unknown };
type Usage = { inputTokens: { total: number }; outputTokens: { total: number }; raw: unknown };
type GoEvidence = { uiSSE: string; parts: ProviderPart[][]; usage: Usage[]; text: string; finishReason: string };

async function withSyntheticChat<T>(errorText: string, run: (port: number) => Promise<T>) {
  const requests: ChatRequest[] = [];
  let failure: unknown;
  const server = createServer(async (req, res) => {
    try {
      expect([req.method, req.url]).toEqual(["POST", "/v1/chat/completions"]);
      req.setEncoding("utf8");
      let json = "";
      for await (const chunk of req) json += chunk;
      const body = JSON.parse(json) as ChatRequest;
      requests.push(body);
      expect(body.model).toBe(modelId);
      expect(body.stream).toBe(true);
      expect(body.stream_options).toEqual({ include_usage: true });
      expect(body.messages[0]).toEqual({ role: "user", content: prompt });
      if (requests.length === 1) {
        expect(body.messages).toHaveLength(1);
      } else {
        expect(requests).toHaveLength(2);
        expect(body.messages.map(message => message.role)).toEqual(["user", "assistant", "tool", "tool"]);
        expect(body.messages[1].tool_calls).toEqual([
          { id: badgeId, type: "function", function: { name: "read_badge", arguments: badgeArgs } },
          { id: wordId, type: "function", function: { name: "read_word", arguments: wordArgs } },
        ]);
        expect(body.messages.slice(2), "exact second-request tool bindings").toEqual([
          { role: "tool", tool_call_id: badgeId, content: '{"badge":"badge:north-7"}' },
          { role: "tool", tool_call_id: wordId, content: errorText },
        ]);
      }
      res.setHeader("Content-Type", "text/event-stream");
      res.end(frames[requests.length - 1].map(frame => `data: ${JSON.stringify(frame)}\n\n`).join("") + "data: [DONE]\n\n");
    } catch (err) {
      failure ??= err;
      res.writeHead(422, { "Content-Type": "application/json" });
      res.end('{"error":{"message":"invalid synthetic Chat request"}}');
    }
  });
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  try {
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("synthetic Chat server has no port");
    try {
      const result = await run(address.port);
      expect(requests).toHaveLength(2);
      return result;
    } finally {
      if (failure) throw failure;
    }
  } finally {
    await new Promise<void>((resolve, reject) => {
      server.close(err => err ? reject(err) : resolve());
      server.closeAllConnections();
    });
  }
}

function providerTrace(calls: ProviderPart[][]) {
  return calls.map(parts => parts.map(part => {
    switch (part.type) {
      case "tool-input-start": return [part.type, part.id, part.toolName];
      case "tool-input-delta":
      case "text-delta": return [part.type, part.id, part.delta];
      case "tool-input-end":
      case "text-start":
      case "text-end": return [part.type, part.id];
      case "tool-call": return [part.type, part.toolCallId, part.toolName, part.input];
      case "finish": return [part.type, part.finishReason];
      default: return [part.type];
    }
  }));
}

async function assertUI(chunks: UIMessageChunk[], errorText: string) {
  expect(chunks.filter(chunk => chunk.type === "start-step" || chunk.type === "finish-step").map(chunk => chunk.type))
    .toEqual(["start-step", "finish-step", "start-step", "finish-step"]);
  expect(chunks.filter(chunk => chunk.type === "error" || chunk.type === "tool-input-error")).toEqual([]);
  const firstFinish = chunks.findIndex(chunk => chunk.type === "finish-step");
  const isToolOutput = (chunk: UIMessageChunk) => chunk.type === "tool-output-available" || chunk.type === "tool-output-error";
  expect(chunks.slice(0, firstFinish).filter(isToolOutput).sort((a, b) => a.toolCallId.localeCompare(b.toolCallId))).toEqual([
    { type: "tool-output-available", toolCallId: badgeId, output: badge },
    { type: "tool-output-error", toolCallId: wordId, errorText },
  ]);
  expect(chunks.slice(firstFinish).filter(isToolOutput)).toEqual([]);
  expect(chunks.at(-1)).toEqual({ type: "finish", finishReason: "stop" });
  let message: UIMessage | undefined;
  const stream = new ReadableStream<UIMessageChunk>({ start(controller) {
    for (const chunk of chunks) controller.enqueue(chunk);
    controller.close();
  } });
  for await (const next of readUIMessageStream({ stream, terminateOnError: true })) message = next;
  expect(message?.parts.filter(part => part.type !== "step-start")).toEqual([
    { type: "tool-read_badge", toolCallId: badgeId, state: "output-available", input: { locker: "north-7" }, output: badge },
    { type: "tool-read_word", toolCallId: wordId, state: "output-error", input: { shelf: "south-4" }, errorText },
    { type: "text", text: finalText, state: "done" },
  ]);
}

it("replays a synthetic interleaved Chat tool stream in Go and pinned Vercel", async () => {
  const go = await withSyntheticChat(error, async port => {
    const response = await fetchScenario("synthetic-chat-parallel", {
      headers: { "Content-Type": "application/json" }, body: JSON.stringify({ port }),
    });
    if (!response.ok) throw new Error(`Go synthetic Chat failed: ${await response.text()}`);
    return await response.json() as GoEvidence;
  });

  const pinnedError = `Error: ${error}`;
  const pinned = await withSyntheticChat(pinnedError, async port => {
    const provider = createOpenAICompatible({
      name: "openai-compatible", baseURL: `http://127.0.0.1:${port}/v1`,
      apiKey: "synthetic-key", includeUsage: true,
    });
    const parts: ProviderPart[][] = [];
    const result = streamText({
      model: wrapLanguageModel({ model: provider(modelId), middleware: {
        specificationVersion: "v4",
        wrapStream: async ({ doStream }) => {
          const native = await doStream();
          const call: ProviderPart[] = [];
          parts.push(call);
          return { ...native, stream: native.stream.pipeThrough(new TransformStream({ transform(part, controller) {
            call.push(part);
            controller.enqueue(part);
          } })) };
        },
      } }),
      prompt, stopWhen: stepCountIs(2), maxRetries: 0,
      tools: {
        read_badge: tool({
          inputSchema: jsonSchema<{ locker: string }>({ type: "object", properties: { locker: { type: "string" } }, required: ["locker"] }),
          execute: ({ locker }, { toolCallId }) => {
            expect(toolCallId).toBe(badgeId);
            return { badge: `badge:${locker}` };
          },
        }),
        read_word: tool({
          inputSchema: jsonSchema<{ shelf: string }>({ type: "object", properties: { shelf: { type: "string" } }, required: ["shelf"] }),
          execute: ({ shelf }, { toolCallId }): string => {
            expect(toolCallId).toBe(wordId);
            throw new Error(`word unavailable: ${shelf}`);
          },
        }),
      },
    });
    const ui: UIMessageChunk[] = [];
    for await (const chunk of result.toUIMessageStream({ onError: err => String(err) })) ui.push(chunk);
    return { ui, parts, steps: await result.steps, text: await result.text, finishReason: await result.finishReason };
  });

  const expectedTrace = [
    [["stream-start"], ["response-metadata"],
      ["tool-input-start", badgeId, "read_badge"], ["tool-input-delta", badgeId, '{"locker":"nor'],
      ["tool-input-start", wordId, "read_word"], ["tool-input-delta", wordId, '{"shelf":"sou'],
      ["tool-input-delta", badgeId, "th-"], ["tool-input-delta", wordId, 'th-4"}'], ["tool-input-delta", badgeId, '7"}'],
      ["tool-input-end", badgeId], ["tool-call", badgeId, "read_badge", badgeArgs],
      ["tool-input-end", wordId], ["tool-call", wordId, "read_word", wordArgs],
      ["finish", { unified: "tool-calls", raw: "tool_calls" }]],
    [["stream-start"], ["response-metadata"],
      ["text-start", "txt-0"], ["text-delta", "txt-0", "Badge found; "],
      ["text-delta", "txt-0", "word unavailable."], ["text-end", "txt-0"], ["finish", { unified: "stop", raw: "stop" }]],
  ];
  expect(providerTrace(pinned.parts)).toEqual(expectedTrace);
  expect(providerTrace(go.parts)).toEqual(expectedTrace);
  expect(pinned.steps.map(step => [step.usage.inputTokens, step.usage.outputTokens, step.usage.raw]))
    .toEqual(usage.map(u => [u.prompt_tokens, u.completion_tokens, u]));
  expect(go.usage.map(u => [u.inputTokens.total, u.outputTokens.total, u.raw]))
    .toEqual(usage.map(u => [u.prompt_tokens, u.completion_tokens, u]));
  expect([go.text, go.finishReason]).toEqual([finalText, "stop"]);
  expect([pinned.text, pinned.finishReason]).toEqual([finalText, "stop"]);

  const goUI: UIMessageChunk[] = [];
  for await (const parsed of parseJsonEventStream({ stream: new Response(go.uiSSE).body!, schema: uiMessageChunkSchema })) {
    if (!parsed.success) throw parsed.error;
    goUI.push(parsed.value);
  }
  await assertUI(goUI, error);
  await assertUI(pinned.ui, pinnedError);
});
