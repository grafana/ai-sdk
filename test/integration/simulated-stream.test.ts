import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import {
  readUIMessageStream,
  uiMessageChunkSchema,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { fetchScenario } from "./helpers.js";

describe("simulated streaming", () => {
  it("preserves generated text metadata and document sources in chunks and assembled messages", async () => {
    const response = await fetchScenario("simulated-stream");
    expect(response.status).toBe(200);

    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({
      stream: response.body!,
      schema: uiMessageChunkSchema,
    })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }

    expect(chunks.find((chunk) => chunk.type === "text-start")).toMatchObject({
      providerMetadata: { test: { signature: "text-signature" } },
    });
    expect(chunks.find((chunk) => chunk.type === "text-delta")).toMatchObject({
      delta: "Generated answer",
    });
    expect(chunks.find((chunk) => chunk.type === "source-document")).toMatchObject({
      sourceId: "document-1",
      title: "Generated Report",
      mediaType: "application/pdf",
      filename: "generated.pdf",
      providerMetadata: { test: { citation: "page-1" } },
    });
    expect(chunks.find((chunk) => chunk.type === "custom")).toMatchObject({
      kind: "test.custom",
      providerMetadata: { test: { value: "preserved" } },
    });

    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({
        start(controller) {
          chunks.forEach((chunk) => controller.enqueue(chunk));
          controller.close();
        },
      }),
      terminateOnError: true,
    })) {
      message = next;
    }

    expect(message?.parts.find((part) => part.type === "text")).toMatchObject({
      text: "Generated answer",
      providerMetadata: { test: { signature: "text-signature" } },
    });
    expect(message?.parts.find((part) => part.type === "source-document")).toMatchObject({
      sourceId: "document-1",
      title: "Generated Report",
      mediaType: "application/pdf",
      filename: "generated.pdf",
      providerMetadata: { test: { citation: "page-1" } },
    });
    expect(message?.parts.find((part) => part.type === "custom")).toMatchObject({
      kind: "test.custom",
      providerMetadata: { test: { value: "preserved" } },
    });
  });
});
