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

  it("unknown scenario returns 404", async () => {
    const res = await fetchScenario("nonexistent");
    expect(res.status).toBe(404);
  });
});
