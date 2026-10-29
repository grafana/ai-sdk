import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("provider error after partial output", () => {
  it.each(["text", "tool"])("preserves partial %s and reports the error", async kind => {
    const response = await fetchScenario(`ui-partial-stream-error-${kind}`);
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.filter(chunk => chunk.type === "error")).toEqual([
      { type: "error", errorText: "intentional stream error" },
    ]);
    expect(chunks.at(-1)).toMatchObject({ type: "finish", finishReason: "error" });
    expect(chunks.filter(chunk => chunk.type === "start-step")).toHaveLength(1);
    expect(chunks.some(chunk => chunk.type === "tool-input-available" || chunk.type.startsWith("tool-output-"))).toBe(false);

    const errors: unknown[] = [];
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      onError: error => errors.push(error),
    })) message = next;
    expect(errors).toHaveLength(1);
    expect(errors[0]).toHaveProperty("message", "intentional stream error");
    if (kind === "text") {
      expect(message?.parts.find(part => part.type === "text")).toMatchObject({ text: "partial" });
    } else {
      expect(message?.parts.find(part => part.type === "tool-weather")).toMatchObject({
        toolCallId: "call-1", state: "input-streaming", input: { city: "Lon" },
      });
    }
  });

  it("reports an error after a finish as the finish reason", async () => {
    const response = await fetchScenario("ui-stream-error-after-finish");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.map(chunk => chunk.type)).toEqual(["start", "start-step", "text-start", "text-delta", "text-end", "error", "finish-step", "finish"]);
    expect(chunks.filter(chunk => chunk.type === "error")).toEqual([{ type: "error", errorText: "intentional stream error" }]);
    expect(chunks.at(-1)).toEqual({ type: "finish", finishReason: "error" });

    const errors: unknown[] = [];
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      onError: error => errors.push(error),
    })) message = next;
    expect(errors).toHaveLength(1);
    expect(message?.parts.find(part => part.type === "text")).toMatchObject({ text: "complete", state: "done" });
  });
});
