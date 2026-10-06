import { describe, it, expect } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { uiMessageChunkSchema, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("SSE wire format", () => {
  it("simple-text scenario produces parseable SSE chunks", async () => {
    const res = await fetchScenario("simple-text");

    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("text/event-stream");
    expect(res.headers.get("x-vercel-ai-ui-message-stream")).toBe("v1");

    const stream = parseJsonEventStream({
      stream: res.body!,
      schema: uiMessageChunkSchema,
    });

    const chunks: UIMessageChunk[] = [];
    const reader = stream.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      expect(value.success).toBe(true);
      if (value.success) {
        chunks.push(value.value);
      }
    }

    expect(chunks.length).toBeGreaterThanOrEqual(5);

    const types = chunks.map((c) => c.type);
    expect(types).toContain("start");
    expect(types).toContain("text-delta");
    expect(types).toContain("finish");
    expect(types).toContain("text-start");
    expect(types).toContain("text-end");
    expect(types).toContain("start-step");
    expect(types).toContain("finish-step");

    const textDeltas = chunks.filter(
      (c): c is UIMessageChunk & { type: "text-delta" } =>
        c.type === "text-delta",
    );
    const fullText = textDeltas.map((c) => c.delta).join("");
    expect(fullText).toBe("Hello, world!");
  });

  it("preserves a length finish reason", async () => {
    const res = await fetchScenario("finish-reason-length");
    const stream = parseJsonEventStream({
      stream: res.body!,
      schema: uiMessageChunkSchema,
    });

    const chunks: UIMessageChunk[] = [];
    for await (const value of stream) {
      expect(value.success).toBe(true);
      if (value.success) chunks.push(value.value);
    }

    expect(chunks).toContainEqual({
      type: "finish",
      finishReason: "length",
    });
  });

  it("preserves metadata-only structured text deltas", async () => {
    const res = await fetchScenario("text-metadata-only-delta");
    const stream = parseJsonEventStream({
      stream: res.body!,
      schema: uiMessageChunkSchema,
    });

    const chunks: UIMessageChunk[] = [];
    for await (const value of stream) {
      expect(value.success).toBe(true);
      if (value.success) chunks.push(value.value);
    }

    expect(chunks).toContainEqual({
      type: "text-delta",
      id: "text-1",
      delta: "",
      providerMetadata: { test: { signature: "test-signature" } },
    });
  });

  it("invalid provider tool input preserves separate input and provider errors", async () => {
    const res = await fetchScenario("invalid-provider-tool");
    const stream = parseJsonEventStream({
      stream: res.body!,
      schema: uiMessageChunkSchema,
    });

    const chunks: UIMessageChunk[] = [];
    for await (const value of stream) {
      expect(value.success).toBe(true);
      if (value.success) chunks.push(value.value);
    }

    expect(chunks).toContainEqual({
      type: "tool-input-error",
      toolCallId: "search-1",
      toolName: "web_search",
      input: {},
      providerExecuted: true,
      errorText: "An error occurred.",
    });
    expect(chunks).toContainEqual({
      type: "tool-output-error",
      toolCallId: "search-1",
      providerExecuted: true,
      errorText:
        '{"type":"web_search_tool_result_error","errorCode":"invalid_tool_input"}',
    });
  });

  it("provider tool output omits absent dynamic and preliminary metadata", async () => {
    const res = await fetchScenario("provider-tool-metadata");
    const stream = parseJsonEventStream({
      stream: res.body!,
      schema: uiMessageChunkSchema,
    });

    const chunks: UIMessageChunk[] = [];
    for await (const value of stream) {
      expect(value.success).toBe(true);
      if (value.success) chunks.push(value.value);
    }

    const toolCall = chunks.find(
      (chunk) => chunk.type === "tool-input-available",
    );
    expect(toolCall).toMatchObject({ dynamic: false });

    const toolOutputs = chunks.filter(
      (chunk) => chunk.type === "tool-output-available",
    );
    expect(toolOutputs).toHaveLength(2);
    for (const chunk of toolOutputs) {
      expect(chunk).not.toHaveProperty("dynamic");
      expect(chunk).not.toHaveProperty("preliminary");
    }

    expect(chunks).toContainEqual({
      type: "custom",
      kind: "openai.compaction",
      providerMetadata: {
        openai: { itemId: "cmp-1", encryptedContent: "encrypted" },
      },
    });
  });

  it("unknown scenario returns 404", async () => {
    const res = await fetchScenario("nonexistent");
    expect(res.status).toBe(404);
  });
});
