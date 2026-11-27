import { safeValidateTypes } from "@ai-sdk/provider-utils";
import { jsonSchema, readUIMessageStream, tool, ToolLoopAgent, uiMessageChunkSchema, type UIMessageChunk } from "ai";
import { MockLanguageModelV4 } from "ai/test";
import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers";

const instructions = "Use actual inventory results.";
const prompt = "Check inventory and recover any missing record.";
const failure = "inventory unavailable";
const lookupError = new Error(failure);
const answer = 'A: {"count":7}; B recovered: {"count":3}';
const inputA = { sku: "A" };
const inputB = { sku: "B" };
const stock = { count: 7 };
const recovered = { count: 3 };
const messageId = "agent-loop-message";
const inputSchema = jsonSchema<{ sku: string }>({
  type: "object", properties: { sku: { type: "string" } }, required: ["sku"], additionalProperties: false,
});

type ModelCallOptions = Parameters<MockLanguageModelV4["doStream"]>[0];
type ModelPart = Awaited<ReturnType<MockLanguageModelV4["doStream"]>>["stream"] extends ReadableStream<infer P> ? P : never;

const lookupA = { type: "tool-call", toolCallId: "stock-1", toolName: "lookup", input: inputA } as const;
const lookupB = { type: "tool-call", toolCallId: "stock-2", toolName: "lookup", input: inputB } as const;
const recoverB = { type: "tool-call", toolCallId: "recover-1", toolName: "recover", input: inputB } as const;
const result = (call: typeof lookupA | typeof lookupB | typeof recoverB, output: { type: "json"; value: object } | { type: "error-text"; value: string }) => ({
  type: "tool-result", toolCallId: call.toolCallId, toolName: call.toolName, output,
} as const);
const request1 = [
  { role: "system", content: instructions },
  { role: "user", content: [{ type: "text", text: prompt }] },
] as const;
const expectedPrompts = (errorText: string) => {
  const request2 = [...request1,
    { role: "assistant", content: [lookupA, lookupB] },
    { role: "tool", content: [result(lookupA, { type: "json", value: stock }), result(lookupB, { type: "error-text", value: errorText })] },
  ];
  return [[...request1], request2, [...request2,
    { role: "assistant", content: [recoverB] },
    { role: "tool", content: [result(recoverB, { type: "json", value: recovered })] },
  ]];
};

function stream<T>(parts: T[]) {
  return new ReadableStream<T>({ start(controller) {
    for (const part of parts) controller.enqueue(part);
    controller.close();
  } });
}

function receivedReply(turn: number, actual: ModelCallOptions["prompt"]): ModelPart[] {
  const expected = expectedPrompts(lookupError.toString())[turn - 1];
  if (expected === undefined) throw new Error(`unexpected model turn ${turn}`);
  expect(actual, `model request ${turn}`).toEqual(expected);
  const call = (part: typeof lookupA | typeof lookupB | typeof recoverB, input: unknown): ModelPart => ({ ...part, input: JSON.stringify(input) });
  const finish = (unified: "tool-calls" | "stop"): ModelPart => ({
    type: "finish", finishReason: { unified, raw: undefined },
    usage: {
      inputTokens: { total: 0, noCache: 0, cacheRead: undefined, cacheWrite: undefined },
      outputTokens: { total: 0, text: 0, reasoning: undefined },
    },
  });
  if (turn === 1) return [call(lookupA, inputA), call(lookupB, inputB), finish("tool-calls")];
  if (turn === 2) {
    const assistant = actual[2];
    if (assistant.role !== "assistant" || assistant.content[1].type !== "tool-call") throw new Error("missing failed call input");
    return [call(recoverB, assistant.content[1].input), finish("tool-calls")];
  }
  const count = (index: number) => {
    const message = actual[index];
    if (message.role !== "tool" || message.content[0].type !== "tool-result" || message.content[0].output.type !== "json") {
      throw new Error(`missing JSON output at message ${index}`);
    }
    return JSON.stringify(message.content[0].output.value);
  };
  return [
    { type: "text-start", id: "answer" },
    { type: "text-delta", id: "answer", delta: `A: ${count(3)}; B recovered: ${count(5)}` },
    { type: "text-end", id: "answer" }, finish("stop"),
  ];
}

function latch() {
  let release!: () => void;
  const promise = new Promise<void>(resolve => { release = resolve; });
  return { promise, release };
}

async function runUpstream() {
  const requests: { prompt: ModelCallOptions["prompt"]; agentMarker: boolean }[] = [];
  const executed: unknown[][] = [];
  const secondStarted = latch();
  const firstEmitted = latch();
  const signal = AbortSignal.timeout(5_000);
  const waitFor = (ready: Promise<void>) => Promise.race([ready, new Promise<never>((_, reject) => {
    if (signal.aborted) reject(signal.reason);
    else signal.addEventListener("abort", () => reject(signal.reason), { once: true });
  })]);
  const model = new MockLanguageModelV4({
    doGenerate: async () => { throw new Error("unexpected unary call"); },
    doStream: async options => {
      const userAgent = Object.entries(options.headers ?? {}).find(([name]) => name.toLowerCase() === "user-agent")?.[1];
      requests.push({ prompt: options.prompt, agentMarker: userAgent?.split(/\s+/).includes("ai-sdk-agent/tool-loop") ?? false });
      return { stream: stream(receivedReply(requests.length, options.prompt)) };
    },
  });
  const agent = new ToolLoopAgent({
    model, instructions, maxRetries: 0,
    tools: {
      lookup: tool({ inputSchema, execute: async (input, { toolCallId }) => {
        executed.push([toolCallId, "lookup", input]);
        if (toolCallId === "stock-2") {
          secondStarted.release();
          await waitFor(firstEmitted.promise);
          throw lookupError;
        }
        await waitFor(secondStarted.promise);
        return stock;
      } }),
      recover: tool({ inputSchema, execute: async (input, { toolCallId }) => {
        executed.push([toolCallId, "recover", input]);
        return recovered;
      } }),
      unused: tool({ inputSchema, execute: async (input, { toolCallId }): Promise<{ count: number }> => {
        executed.push([toolCallId, "unused", input]);
        throw new Error("unexpected tool execution");
      } }),
    },
  });
  const run = await agent.stream({ prompt, abortSignal: signal });
  const chunks: UIMessageChunk[] = [];
  const events: unknown[][] = [];
  const reader = run.toUIMessageStream({
    generateMessageId: () => messageId,
    redactToolErrors: false,
    messageMetadata: ({ part }) => {
      switch (part.type) {
        case "tool-call": events.push([part.type, part.toolCallId, part.toolName, part.input]); break;
        case "tool-result": events.push([part.type, part.toolCallId, part.toolName, part.output, part.preliminary ?? false]); break;
        case "tool-error": events.push([part.type, part.toolCallId, part.toolName, String(part.error)]); break;
        case "finish-step": case "finish": events.push([part.type, part.finishReason]); break;
        case "text-delta": events.push([part.type, part.id, part.text]); break;
        case "text-start": case "text-end": events.push([part.type, part.id]); break;
        default: events.push([part.type]);
      }
    },
  }).getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    if (value.type === "tool-output-available" && value.toolCallId === "stock-1") firstEmitted.release();
  }
  const steps = (await run.steps).map(step => ({
    number: step.stepNumber, finish: step.finishReason,
    content: step.content.map(part => {
      switch (part.type) {
        case "tool-call": return [part.type, part.toolCallId, part.toolName, part.input];
        case "tool-result": return [part.type, part.toolCallId, part.toolName, part.output];
        case "tool-error": return [part.type, part.toolCallId, part.toolName, String(part.error)];
        case "text": return [part.type, part.text];
        default: return [part.type];
      }
    }),
    toolResultIds: step.toolResults.map(part => part.toolCallId),
  }));
  return { requests, executed, chunks, events, steps, text: await run.text };
}

function expectedTrace(errorText: string) {
  const first = ["tool-call", "stock-1", "lookup", inputA];
  const second = ["tool-call", "stock-2", "lookup", inputB];
  const third = ["tool-call", "recover-1", "recover", inputB];
  const success = ["tool-result", "stock-1", "lookup", stock];
  const failure = ["tool-error", "stock-2", "lookup", errorText];
  const recovery = ["tool-result", "recover-1", "recover", recovered];
  return {
    requests: expectedPrompts(errorText).map(prompt => ({ prompt, agentMarker: true })),
    events: [["start"], ["start-step"], first, second, [...success, false], failure, ["finish-step", "tool-calls"],
      ["start-step"], third, [...recovery, false], ["finish-step", "tool-calls"],
      ["start-step"], ["text-start", "answer"], ["text-delta", "answer", answer], ["text-end", "answer"], ["finish-step", "stop"], ["finish", "stop"]],
    steps: [
      { finish: "tool-calls", content: [first, second, success, failure] },
      { finish: "tool-calls", content: [third, recovery] },
      { finish: "stop", content: [["text", answer]] },
    ],
    chunks: [
      { type: "start", messageId }, { type: "start-step" },
      { type: "tool-input-available", toolCallId: "stock-1", toolName: "lookup", input: inputA },
      { type: "tool-input-available", toolCallId: "stock-2", toolName: "lookup", input: inputB },
      { type: "tool-output-available", toolCallId: "stock-1", output: stock },
      { type: "tool-output-error", toolCallId: "stock-2", errorText },
      { type: "finish-step" }, { type: "start-step" },
      { type: "tool-input-available", toolCallId: "recover-1", toolName: "recover", input: inputB },
      { type: "tool-output-available", toolCallId: "recover-1", output: recovered },
      { type: "finish-step" }, { type: "start-step" },
      { type: "text-start", id: "answer" }, { type: "text-delta", id: "answer", delta: answer },
      { type: "text-end", id: "answer" }, { type: "finish-step" }, { type: "finish", finishReason: "stop" },
    ],
  };
}

type Trace = Awaited<ReturnType<typeof runUpstream>>;

async function assertTrace(trace: Trace, errorText: string) {
  const expected = expectedTrace(errorText);
  expect(trace.requests).toEqual(expected.requests);
  expect(trace.events).toEqual(expected.events);
  expect(trace.steps.map(({ finish, content }) => ({ finish, content }))).toEqual(expected.steps);
  expect(trace.steps.map(step => step.number)).toEqual([0, 1, 2]);
  expect(trace.text).toBe(answer);
  expect(trace.executed).toHaveLength(3);
  expect(trace.executed).toEqual(expect.arrayContaining([
    ["stock-1", "lookup", inputA], ["stock-2", "lookup", inputB], ["recover-1", "recover", inputB],
  ]));
  for (const chunk of trace.chunks) expect((await safeValidateTypes({ schema: uiMessageChunkSchema, value: chunk })).success).toBe(true);
  expect(trace.chunks).toEqual(expected.chunks);
  let message;
  for await (const assembled of readUIMessageStream({ stream: stream(trace.chunks), terminateOnError: true })) message = assembled;
  expect(message).toMatchObject({ id: messageId, role: "assistant", parts: [
    { type: "step-start" },
    { type: "tool-lookup", toolCallId: "stock-1", state: "output-available", input: inputA, output: stock },
    { type: "tool-lookup", toolCallId: "stock-2", state: "output-error", input: inputB, errorText },
    { type: "step-start" },
    { type: "tool-recover", toolCallId: "recover-1", state: "output-available", input: inputB, output: recovered },
    { type: "step-start" }, { type: "text", text: answer, state: "done" },
  ] });
}

describe("ToolLoopAgent stream parity with the registered SDK", () => {
  it("recovers from one of two sibling tool calls over three dependent model requests", async () => {
    const response = await fetchScenario("agent-tool-loop");
    expect(response.status, response.ok ? undefined : await response.text()).toBe(200);
    const go: Trace = await response.json();
    const upstream = await runUpstream();
    await assertTrace(upstream, lookupError.toString());
    await assertTrace(go, failure);
    const successfulResultIds = [["stock-1"], ["recover-1"], []];
    expect(upstream.steps.map(step => step.toolResultIds)).toEqual(successfulResultIds);
    expect(go.steps.map(step => step.toolResultIds)).toEqual(successfulResultIds);
  });
});
