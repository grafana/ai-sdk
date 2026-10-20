import { describe, expect, it } from "vitest";
import { readUIMessageStream, simulateReadableStream, validateUIMessages, type UIMessage } from "ai";
import { fetchScenario } from "./helpers.js";
import { canonicalUI, json, readToolStateStream } from "./ui-tool-state-helpers.js";

describe("citations and text-encoded tool input", () => {
  it.each([
    { source: { type: "source", sourceType: "url", id: "u", url: "url" }, startIndex: -1 },
    { source: { type: "source", sourceType: "other", id: "u" } },
    { source: { type: "source", sourceType: "url", id: "u", url: "url", providerMetadata: { test: null } } },
  ])("rejects malformed citations before Agent invocation: %j", async citation => {
    const message = { id: "m", role: "assistant", parts: [{ type: "text", text: "answer", citations: [citation] }] };
    await expect(validateUIMessages({ messages: [message] })).rejects.toThrow();
    const response = await fetchScenario("ui-tool-state-agent", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages: [message] }) });
    expect(response.status).toBe(400);
    expect((await response.json()).providerCalls).toBe(0);
  });
  it("preserves citations, raw partial text and persisted resume across Go and the pinned frontend", async () => {
    const { chunks, snapshots } = await readToolStateStream(await fetchScenario("citations-text-input"));
    const end = chunks.find(chunk => chunk.type === "text-end");
    expect(end?.type === "text-end" && end.citations).toEqual([
      { source: { type: "source", sourceType: "url", id: "url", url: "https://example.com" }, startIndex: 0, endIndex: 6 },
      { source: { type: "source", sourceType: "document", id: "", mediaType: "text/plain", title: "" }, citedText: "" },
    ]);
    expect(chunks.find(chunk => chunk.type === "tool-input-start")).toMatchObject({ inputEncoding: "text" });
    expect(snapshots.some(message => message.parts.some(part => part.type === "dynamic-tool" && part.state === "input-streaming" && part.input === "SELECT \""))).toBe(true);
    const persisted = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks, ignoreIncomplete: true }) });
    expect(persisted.ok).toBe(true);
    expect(canonicalUI((await persisted.json()).snapshots)).toEqual(canonicalUI(snapshots));
    const split = chunks.findIndex(chunk => chunk.type === "tool-input-delta") + 1;
    const initial: UIMessage[] = [];
    for await (const message of readUIMessageStream({ stream: simulateReadableStream({ chunks: chunks.slice(0, split) }), terminateOnError: true })) initial.push(json(message));
    const resumed: UIMessage[] = [];
    for await (const message of readUIMessageStream({ message: json(initial.at(-1)!), stream: simulateReadableStream({ chunks: chunks.slice(split) }), terminateOnError: true })) resumed.push(json(message));
    const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ initialMessage: initial.at(-1), chunks: chunks.slice(split), ignoreIncomplete: true }) });
    expect(response.ok, await response.clone().text()).toBe(true);
    expect(canonicalUI((await response.json()).snapshots)).toEqual(canonicalUI(resumed));
  });
});
