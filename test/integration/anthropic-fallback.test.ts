import { convertToModelMessages } from "ai";
import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { canonicalModel, canonicalUI, readToolStateStream } from "./ui-tool-state-helpers.js";

describe("Anthropic fallback continuation", () => {
  it("preserves Anthropic fallback boundaries through schema-parsed SSE and model continuation", async () => {
    const response = await fetchScenario("anthropic-fallback-content");
    expect(response.status).toBe(200);
    const parsed = await readToolStateStream(response);
    const custom = parsed.chunks.find(chunk => chunk.type === "custom");
    expect(custom).toEqual({ type: "custom", kind: "anthropic.fallback", providerMetadata: { anthropic: { type: "fallback", from: { model: "primary" }, to: { model: "secondary" } } } });
    const message = parsed.snapshots.at(-1)!;
    expect(message.parts.map(part => part.type)).toEqual(["step-start", "reasoning", "custom", "reasoning"]);
    const persisted = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ initialMessage: message, chunks: [] }) });
    expect(persisted.status).toBe(200);
    const go = await persisted.json();
    expect(canonicalUI(go.uiMessages)).toEqual(canonicalUI([message]));
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([message])));
  });
});
