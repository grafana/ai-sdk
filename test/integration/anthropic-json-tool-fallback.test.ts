import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { readToolStateStream } from "./ui-tool-state-helpers.js";

describe("Anthropic JSON-tool fallback", () => {
  it("assembles the final plain text and its citation through schema-parsed SSE", async () => {
    const { chunks, snapshots } = await readToolStateStream(await fetchScenario("anthropic-json-tool-fallback"));
    const citation = {
      source: { type: "source", sourceType: "url", id: "https://example.com/report", url: "https://example.com/report", title: "Report" },
      citedText: "source",
    };
    const text = chunks.filter(chunk => chunk.type.startsWith("text-"));
    expect(text.map(chunk => chunk.type)).toEqual(["text-start", "text-delta", "text-end"]);
    expect(text[1]).toMatchObject({ id: "0", delta: '{"answer":"42"}' });
    expect(text[2]).toMatchObject({ id: "0", citations: [citation] });
    expect(snapshots.at(-1)?.parts.filter(part => part.type === "text")).toMatchObject([
      { text: '{"answer":"42"}', citations: [citation] },
    ]);
  });
});
