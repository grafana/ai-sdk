import { describe, it, expect } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import {
  uiMessageChunkSchema,
  readUIMessageStream,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { fetchScenario } from "./helpers.js";

async function readScenarioMessages(name: string): Promise<UIMessage[]> {
  const res = await fetchScenario(name);
  expect(res.status).toBe(200);

  const parseStream = parseJsonEventStream({
    stream: res.body!,
    schema: uiMessageChunkSchema,
  });

  const chunkStream = new ReadableStream<UIMessageChunk>({
    async start(controller) {
      const reader = parseStream.getReader();
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (value.success) {
          controller.enqueue(value.value);
        }
      }
      controller.close();
    },
  });

  const messages: UIMessage[] = [];
  for await (const message of readUIMessageStream({ stream: chunkStream })) {
    messages.push(message);
  }
  return messages;
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
