import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import {
  createAgentUIStream,
  createAgentUIStreamResponse,
  readUIMessageStream,
  simulateReadableStream,
  ToolLoopAgent,
  uiMessageChunkSchema,
  type DataUIPart,
  type FilePart,
  type TextPart,
  type UIDataTypes,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { MockLanguageModelV4 } from "ai/test";
import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";

function referenceModel() {
  return new MockLanguageModelV4({
    doStream: {
      stream: simulateReadableStream({ chunks: [
        { type: "text-start", id: "t1" },
        { type: "text-delta", id: "t1", delta: "Hello, " },
        { type: "text-delta", id: "t1", delta: "world!" },
        { type: "text-end", id: "t1" },
        {
          type: "finish", finishReason: { unified: "stop", raw: undefined },
          usage: { inputTokens: { total: 10, noCache: 10, cacheRead: 0, cacheWrite: 0 }, outputTokens: { total: 5, text: 5, reasoning: 0 } },
        },
      ] }),
    },
  });
}

function convertDataPart(part: DataUIPart<UIDataTypes>): TextPart | FilePart | undefined {
  if (part.type === "data-error") throw new Error("test: conversion failed");
  if (typeof part.data !== "string") return;
  if (part.type === "data-text") return { type: "text", text: part.data };
  if (part.type === "data-file") return { type: "file", mediaType: "text/plain", filename: `${part.id}.txt`, data: part.data };
}

function goResponse(messages: UIMessage[], write: boolean, convert: boolean) {
  return fetchScenario("agent-ui-data-conversion", {
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ messages, write, convert }),
  });
}

function parseUIResponse(response: Response, chunks?: UIMessageChunk[]) {
  expect(response.status).toBe(200);
  expect(response.headers.get("x-vercel-ai-ui-message-stream")).toBe("v1");
  return parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema }).pipeThrough(new TransformStream({
    transform(parsed, controller) {
      if (!parsed.success) throw parsed.error;
      chunks?.push(parsed.value);
      controller.enqueue(parsed.value);
    },
  }));
}

const messages: UIMessage[] = [
  { id: "u", role: "user", parts: [
    { type: "text", text: "Question" },
    { type: "data-text", id: "user-context", data: "User context" },
    { type: "data-file", id: "user-file", data: "dXNlcg==" },
  ] },
  { id: "a", role: "assistant", parts: [
    { type: "data-text", id: "assistant-context", data: "Assistant context" },
    { type: "text", text: "Previous answer" },
    { type: "data-local", id: "local", data: true },
    { type: "data-file", id: "assistant-file", data: "YXNzaXN0YW50" },
  ] },
];

const plainPrompt = [
  { role: "user", content: [{ type: "text", text: "Question" }] },
  { role: "assistant", content: [{ type: "text", text: "Previous answer" }] },
];
const convertedPrompt = [
  { role: "user", content: [
    { type: "text", text: "Question" },
    { type: "text", text: "User context" },
    { type: "file", mediaType: "text/plain", filename: "user-file.txt", data: { type: "data", data: "dXNlcg==" } },
  ] },
  { role: "assistant", content: [
    { type: "text", text: "Assistant context" },
    { type: "text", text: "Previous answer" },
    { type: "file", mediaType: "text/plain", filename: "assistant-file.txt", data: { type: "data", data: "YXNzaXN0YW50" } },
  ] },
];

describe.each(["create", "write"] as const)("Agent UI data conversion (%s)", helper => {
  const write = helper === "write";

  it.each([false, true])("matches the pinned helper prompt and assembled UI (convert: %s)", async convert => {
    const model = referenceModel();
    const options = { agent: new ToolLoopAgent({ model }), uiMessages: messages, convertDataPart: convert ? convertDataPart : undefined };
    const target = write
      ? parseUIResponse(await createAgentUIStreamResponse(options))
      : await createAgentUIStream(options);
    let expected: UIMessage | undefined;
    const initial = messages.at(-1)!;
    for await (const message of readUIMessageStream({ stream: target, message: structuredClone(initial), terminateOnError: true })) {
      expected = message;
    }
    expect(model.doStreamCalls).toHaveLength(1);
    const prompt = JSON.parse(JSON.stringify(model.doStreamCalls[0].prompt));
    expect(prompt).toEqual(convert ? convertedPrompt : plainPrompt);

    let assembled: UIMessage | undefined;
    const chunks: UIMessageChunk[] = [];
    const go = parseUIResponse(await goResponse(messages, write, convert), chunks);
    for await (const message of readUIMessageStream({ stream: go, message: structuredClone(initial), terminateOnError: true })) {
      assembled = message;
    }
    expect(chunks[0]).toMatchObject({ type: "start", messageId: "a" });
    expect(assembled?.metadata).toEqual({ prompt, providerCalls: 1 });
    expect(assembled?.id).toBe(expected?.id);
    expect(assembled?.id).toBe("a");
    expect(assembled?.parts).toEqual(expected?.parts);
    expect(assembled?.parts.slice(0, initial.parts.length)).toEqual(initial.parts);
    expect(assembled?.parts.at(-1)).toMatchObject({ type: "text", text: "Hello, world!" });
  });

  it("fails before model I/O or SSE on converter error", async () => {
    const uiMessages: UIMessage[] = [{ id: "u", role: "user", parts: [{ type: "data-error", data: true }] }];
    const model = referenceModel();
    const options = { agent: new ToolLoopAgent({ model }), uiMessages, convertDataPart };
    await expect(write ? createAgentUIStreamResponse(options) : createAgentUIStream(options)).rejects.toThrow("test: conversion failed");
    expect(model.doStreamCalls).toHaveLength(0);

    const response = await goResponse(uiMessages, write, true);
    expect(response.status).toBe(400);
    expect(response.headers.get("x-vercel-ai-ui-message-stream")).toBeNull();
    expect(await response.json()).toEqual({ error: 'aisdk: converting data part "error": test: conversion failed', providerCalls: 0 });
  });
});
