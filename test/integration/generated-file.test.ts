import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("generated file output", () => {
  it("delivers resolved file bytes through SSE and message assembly", async () => {
    const response = await fetchScenario("generated-file");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.find(chunk => chunk.type === "file")).toMatchObject({
      type: "file", mediaType: "text/plain", url: "data:text/plain;base64,SGVsbG8=",
    });
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: true,
    })) message = next;
    expect(message?.parts.find(part => part.type === "file")).toMatchObject({
      type: "file", mediaType: "text/plain", url: "data:text/plain;base64,SGVsbG8=",
    });
  });
});
