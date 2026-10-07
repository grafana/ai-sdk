import { describe, expect, it } from "vitest";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { readUIMessageStream, uiMessageChunkSchema, type UIMessage, type UIMessageChunk } from "ai";
import { fetchScenario } from "./helpers.js";

describe("Gateway guard client to UI messages", () => {
  for (const mode of ["allow", "deny", "transform"] as const) {
    it(`schema-parses ${mode} without rejected content`, async () => {
      const response = await fetchScenario(`gateway-guards-${mode}`);
      expect(response.status).toBe(200);
      const chunks: UIMessageChunk[] = [];
      for await (const parsed of parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema })) {
        expect(parsed.success).toBe(true);
        if (parsed.success) chunks.push(parsed.value);
      }
      expect(JSON.stringify(chunks)).not.toContain("prompt-canary");
      expect(chunks.some(chunk => chunk.type.startsWith("tool-"))).toBe(false);
      if (mode === "allow") {
        expect(chunks.filter(chunk => chunk.type === "text-delta").map(chunk => chunk.delta)).toEqual(["sanitized answer"]);
        let message: UIMessage | undefined;
        for await (const next of readUIMessageStream({ stream: new ReadableStream<UIMessageChunk>({ start(controller) { chunks.forEach(chunk => controller.enqueue(chunk)); controller.close(); } }), terminateOnError: true })) message = next;
        expect(message?.parts.filter(part => part.type === "text").map(part => part.text)).toEqual(["sanitized answer"]);
      } else {
        expect(chunks.filter(chunk => chunk.type === "error").map(chunk => chunk.errorText)).toEqual([mode === "deny" ? "forbidden" : "failed dependency"]);
        expect(chunks.filter(chunk => chunk.type === "text-delta")).toEqual([]);
        expect(JSON.stringify(chunks)).not.toContain("sanitized answer");
      }
    });
  }
});
