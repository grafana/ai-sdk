import { describe, it, expect } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import {
  uiMessageChunkSchema,
  readUIMessageStream,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { fetchScenario } from "./helpers.js";

async function readScenario(name: string): Promise<{
  chunks: UIMessageChunk[];
  messages: UIMessage[];
}> {
  const res = await fetchScenario(name);
  expect(res.status).toBe(200);

  const parseStream = parseJsonEventStream({
    stream: res.body!,
    schema: uiMessageChunkSchema,
  });
  const chunks: UIMessageChunk[] = [];
  const chunkStream = new ReadableStream<UIMessageChunk>({
    async start(controller) {
      const reader = parseStream.getReader();
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (!value.success) {
          controller.error(value.error);
          return;
        }
        chunks.push(value.value);
        controller.enqueue(value.value);
      }
      controller.close();
    },
  });

  const messages: UIMessage[] = [];
  for await (const message of readUIMessageStream({
    stream: chunkStream,
    terminateOnError: true,
  })) {
    messages.push(message);
  }
  return { chunks, messages };
}

async function readScenarioMessages(name: string): Promise<UIMessage[]> {
  return (await readScenario(name)).messages;
}

describe("SSE message assembly", () => {
  it("simple-text chunks assemble into a valid UIMessage", async () => {
    const messages = await readScenarioMessages("simple-text");
    expect(messages.length).toBeGreaterThanOrEqual(1);

    const lastMessage = messages[messages.length - 1];
    expect(lastMessage.role).toBe("assistant");
    expect(lastMessage.parts).toBeDefined();

    const textParts = lastMessage.parts.filter(
      (p: { type: string }) => p.type === "text",
    );
    expect(textParts.length).toBeGreaterThanOrEqual(1);

    const fullText = textParts.map((p: { text: string }) => p.text).join("");
    expect(fullText).toBe("Hello, world!");
  });

  it("preserves reasoning block IDs in assembled messages", async () => {
    const messages = await readScenarioMessages("reasoning");
    const lastMessage = messages[messages.length - 1];
    const reasoning = lastMessage.parts.filter(part => part.type === "reasoning");

    expect(reasoning).toEqual([
      {
        type: "reasoning",
        id: "reasoning-1",
        text: "First thought.",
        state: "done",
        providerMetadata: undefined,
      },
      {
        type: "reasoning",
        id: "reasoning-2",
        text: "Second thought.",
        state: "done",
        providerMetadata: undefined,
      },
    ]);
  });

  it("assembles unique text and reasoning parts when providers reuse IDs", async () => {
    const { chunks, messages } = await readScenario("duplicate-part-ids");
    const starts = chunks.filter(
      chunk => chunk.type === "text-start" || chunk.type === "reasoning-start",
    );
    expect(starts.map(chunk => ({ type: chunk.type, id: chunk.id }))).toEqual([
      { type: "text-start", id: "0" },
      { type: "reasoning-start", id: "0" },
      { type: "text-start", id: "generated-2" },
      { type: "reasoning-start", id: "generated-3" },
    ]);

    const lastMessage = messages[messages.length - 1];
    expect(lastMessage.parts.filter(part => part.type === "text")).toEqual([
      {
        type: "text",
        text: "First answer.",
        state: "done",
        providerMetadata: undefined,
      },
      {
        type: "text",
        text: "Second answer.",
        state: "done",
        providerMetadata: undefined,
      },
    ]);
    expect(lastMessage.parts.filter(part => part.type === "reasoning")).toEqual([
      {
        type: "reasoning",
        id: "0",
        text: "First thought.",
        state: "done",
        providerMetadata: undefined,
      },
      {
        type: "reasoning",
        id: "generated-3",
        text: "Second thought.",
        state: "done",
        providerMetadata: undefined,
      },
    ]);
  });

  it("assembles alternating reasoning and text blocks from one provider chunk", async () => {
    const { chunks, messages } = await readScenario("alternating-content");
    const starts = chunks.filter(
      chunk => chunk.type === "text-start" || chunk.type === "reasoning-start",
    );
    expect(starts.map(chunk => ({ type: chunk.type, id: chunk.id }))).toEqual([
      { type: "reasoning-start", id: "reasoning-0" },
      { type: "text-start", id: "txt-0" },
      { type: "reasoning-start", id: "generated-1" },
    ]);

    const lastMessage = messages[messages.length - 1];
    expect(lastMessage.parts.filter(part => part.type === "reasoning")).toEqual([
      {
        type: "reasoning",
        id: "reasoning-0",
        text: "think",
        state: "done",
        providerMetadata: undefined,
      },
      {
        type: "reasoning",
        id: "generated-1",
        text: "again",
        state: "done",
        providerMetadata: undefined,
      },
    ]);
    expect(lastMessage.parts.filter(part => part.type === "text")).toEqual([
      {
        type: "text",
        text: "answer",
        state: "done",
        providerMetadata: undefined,
      },
    ]);
  });

  it("assembles a message terminated by a length finish reason", async () => {
    const messages = await readScenarioMessages("finish-reason-length");
    const lastMessage = messages[messages.length - 1];
    const text = lastMessage.parts.find((part) => part.type === "text");

    expect(text).toMatchObject({
      type: "text",
      text: "Partial response",
    });
  });

  it("merges metadata-only text deltas into the assembled text part", async () => {
    const messages = await readScenarioMessages("text-metadata-only-delta");
    const lastMessage = messages[messages.length - 1];
    const text = lastMessage.parts.find((part) => part.type === "text");

    expect(text).toMatchObject({
      type: "text",
      text: '{"value":"ok"}',
      providerMetadata: { test: { signature: "test-signature" } },
    });
  });

  it("assembles invalid provider tool errors into output-error state", async () => {
    const messages = await readScenarioMessages("invalid-provider-tool");
    const lastMessage = messages[messages.length - 1];
    const tool = lastMessage.parts.find(
      (part) => part.type === "tool-web_search" && part.toolCallId === "search-1",
    );

    expect(tool).toMatchObject({
      type: "tool-web_search",
      toolCallId: "search-1",
      state: "output-error",
      input: {},
      providerExecuted: true,
      errorText:
        '{"type":"web_search_tool_result_error","errorCode":"invalid_tool_input"}',
    });
  });

  it("preserves custom provider content", async () => {
    const messages = await readScenarioMessages("provider-tool-metadata");
    const lastMessage = messages[messages.length - 1];
    const custom = lastMessage.parts.find((part) => part.type === "custom");

    expect(custom).toEqual({
      type: "custom",
      kind: "openai.compaction",
      providerMetadata: {
        openai: { itemId: "cmp-1", encryptedContent: "encrypted" },
      },
    });
  });

  it("preserves document source filenames", async () => {
    const messages = await readScenarioMessages("source-document");
    const lastMessage = messages[messages.length - 1];
    const source = lastMessage.parts.find(
      (part) => part.type === "source-document",
    );

    expect(source).toMatchObject({
      type: "source-document",
      sourceId: "source-1",
      mediaType: "application/pdf",
      title: "Financial Report",
      filename: "financial-report.pdf",
    });
  });
});
