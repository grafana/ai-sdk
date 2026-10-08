import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("ordered provider error evidence at the frontend boundary", () => {
  it("keeps the existing core error policy and never reflects diagnostic data in UI errors", async () => {
    const response = await fetchScenario("ordered-error-evidence");
    expect(response.status).toBe(200);
    const chunks: UIMessageChunk[] = [];
    for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
      expect(parsed.success).toBe(true);
      if (parsed.success) chunks.push(parsed.value);
    }
    expect(chunks.find(chunk => chunk.type === "error")).toEqual({ type: "error", errorText: "generation failed" });
    expect(chunks.filter(chunk => chunk.type === "text-delta").map(chunk => chunk.delta).join("")).toBe("beforeafter");
    expect(chunks.at(-1)?.type).toBe("finish");
    const types = chunks.map(chunk => chunk.type);
    expect(types.indexOf("text-delta")).toBeLessThan(types.indexOf("error"));
    expect(types.indexOf("error")).toBeLessThan(types.indexOf("finish"));
    expect(JSON.stringify(chunks)).not.toContain("private-native-message");
    expect(JSON.stringify(chunks)).not.toContain("gateway");
    let message: UIMessage | undefined;
    for await (const next of readUIMessageStream({
      stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }),
      terminateOnError: false,
      onError() {},
    })) message = next;
    expect(message?.parts.filter(part => part.type === "text").map(part => part.text).join("")).toBe("beforeafter");
  });
});
