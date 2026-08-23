import { createGateway } from "@ai-sdk/gateway";
import { describe, expect, it } from "vitest";
import { getServerUrl } from "./helpers.js";

describe("ProviderWire V4 runtime", () => {
  it("consumes the production Go handler through the pinned Gateway client", async () => {
    const model = createGateway({
      apiKey: "integration-test-key",
      baseURL: `${getServerUrl()}/providerwire-v4`,
    })("success");

    const result = await model.doGenerate({
      prompt: [{ role: "system", content: "hello" }],
    });

    expect(result.content).toEqual([{ type: "text", text: "hello from Go" }]);
    expect(result.finishReason).toEqual({ unified: "stop", raw: "test-stop" });
    expect(result.usage).toEqual({
      inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    });
  });

  it("consumes ordered production SSE through the pinned Gateway client", async () => {
    const model = createGateway({
      apiKey: "integration-test-key",
      baseURL: `${getServerUrl()}/providerwire-v4`,
    })("success");

    const result = await model.doStream({ prompt: [] });
    const reader = result.stream.getReader();
    const parts: Array<{ type: string; delta?: string }> = [];
    for (;;) {
      const next = await reader.read();
      if (next.done) break;
      parts.push(next.value);
    }

    expect(parts.map((part) => part.type)).toEqual([
      "stream-start",
      "response-metadata",
      "text-start",
      "text-delta",
      "text-delta",
      "text-end",
      "finish",
    ]);
    expect(parts.filter((part) => part.type === "text-delta").map((part) => part.delta)).toEqual([
      "",
      "hello from Go stream",
    ]);
  });
});
