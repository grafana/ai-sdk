import { describe, it, expect } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessageChunk, type UIMessage } from "ai";
import { fetchScenario } from "./helpers.js";

describe("native sources and explicitly mapped response metadata", () => {
  it("preserves repeated/cross-variant IDs and display through schema parsing and assembly", async () => {
    const response = await fetchScenario("native-source-identity");
    expect(response.status).toBe(200);
    const parsed = parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema });
    const chunks: UIMessageChunk[] = [];
    for await (const value of parsed) {
      expect(value.success).toBe(true);
      if (!value.success) throw value.error;
      chunks.push(value.value);
    }
    const sources = [
      { type: "source-url", sourceId: "native-shared", url: "https://example.com", title: "Native URL" },
      { type: "source-url", sourceId: "native-shared", url: "https://example.com/repeated" },
      { type: "source-document", sourceId: "native-shared", mediaType: "application/octet-stream", title: "file-native", filename: "file-native" },
      { type: "source-document", sourceId: "", mediaType: "text/plain", title: "" },
    ];
    expect(chunks.filter(chunk => chunk.type === "source-url" || chunk.type === "source-document")).toEqual(sources);
    const metadata = { nativeResponse: { id: "native-response", modelId: "native model ☃", timestamp: "2026-09-30T12:00:00.123Z" } };
    expect(chunks.filter(chunk => chunk.type === "message-metadata")).toEqual([{ type: "message-metadata", messageMetadata: metadata }]);
    expect(chunks.some(chunk => "warnings" in chunk || "modelId" in chunk || "responseId" in chunk)).toBe(false);
    const stream = new ReadableStream<UIMessageChunk>({ start(controller) { for (const chunk of chunks) controller.enqueue(chunk); controller.close(); } });
    const messages: UIMessage[] = [];
    for await (const message of readUIMessageStream({ stream, terminateOnError: true })) messages.push(message);
    const message = messages.at(-1)!;
    expect(message.role).toBe("assistant");
    expect(message.parts.filter(part => part.type === "source-url" || part.type === "source-document")).toEqual(sources);
    expect(message.metadata).toEqual(metadata);
  });
});
