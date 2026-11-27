import { convertToModelMessages } from "ai";
import { expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { readToolStateStream } from "./ui-tool-state-helpers.js";

it("preserves signed Anthropic compaction through the UI stream and model history", async () => {
  const response = await fetchScenario("anthropic-signed-compaction");
  const { chunks, snapshots } = await readToolStateStream(response);
  const summary = "Résumé  \n";
  const providerMetadata = {
    anthropic: { type: "compaction", signature: "test-signature" },
  };

  expect(chunks.filter(
    chunk => chunk.type === "text-start" || chunk.type === "text-delta" || chunk.type === "text-end",
  )).toEqual([
    { type: "text-start", id: "0", providerMetadata },
    { type: "text-delta", id: "0", delta: summary },
    { type: "text-end", id: "0" },
  ]);

  const message = snapshots.at(-1)!;
  expect(message.parts.filter(part => part.type === "text")).toEqual([
    { type: "text", text: summary, state: "done", providerMetadata },
  ]);
  const history = await convertToModelMessages([message]);
  expect(history).toHaveLength(1);
  const [assistant] = history;
  if (assistant.role !== "assistant" || typeof assistant.content === "string") {
    throw new Error("Expected structured assistant history");
  }
  expect(assistant.content.filter(part => part.type === "text")).toEqual([
    { type: "text", text: summary, providerOptions: providerMetadata },
  ]);
});
