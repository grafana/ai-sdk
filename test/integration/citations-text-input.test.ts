import { describe, expect, it } from "vitest";
import { readUIMessageStream, simulateReadableStream, validateUIMessages, type UIMessage } from "ai";
import { fetchScenario } from "./helpers.js";
import { canonicalUI, json, readToolStateStream } from "./ui-tool-state-helpers.js";

const url = { type: "source", sourceType: "url", id: "u", url: "url" };
const cited = (citation: unknown) => ({ type: "text", text: "answer", citations: [citation] });

describe("citations and text-encoded tool input", () => {
  it.each([
    { name: "negative offset", part: cited({ source: url, startIndex: -1 }), go: "reject" },
    { name: "fractional offset", part: cited({ source: url, endIndex: 1.5 }), go: "reject" },
    { name: "unsafe offset", part: cited({ source: url, startIndex: 2 ** 53 }), go: "reject" },
    { name: "unknown source type", part: cited({ source: { type: "source", sourceType: "other", id: "u" } }), go: "reject" },
    { name: "null metadata entry", part: cited({ source: { ...url, providerMetadata: { test: null } } }), go: "reject" },
    { name: "missing citation URL", part: cited({ source: { type: "source", sourceType: "url", id: "u" } }), go: "tolerate" },
    { name: "missing citation source ID", part: cited({ source: { type: "source", sourceType: "url", url: "url" } }), go: "tolerate" },
    { name: "missing citation document title", part: cited({ source: { type: "source", sourceType: "document", id: "d", mediaType: "text/plain" } }), go: "tolerate" },
    { name: "missing citation document media type", part: cited({ source: { type: "source", sourceType: "document", id: "d", title: "" } }), go: "tolerate" },
    { name: "wrong citation source discriminator", part: cited({ source: { ...url, type: "other" } }), go: "tolerate" },
    { name: "missing source-url URL", part: { type: "source-url", sourceId: "u" }, go: "tolerate" },
    { name: "missing source-document title", part: { type: "source-document", sourceId: "d", mediaType: "text/plain" }, go: "tolerate" },
  ])("classifies upstream-invalid UI sources on Agent input: $name", async ({ part, go }) => {
    const messages = [{ id: "u", role: "user", parts: [{ type: "text", text: "question" }] }, { id: "m", role: "assistant", parts: [part] }];
    await expect(validateUIMessages({ messages })).rejects.toThrow();
    const response = await fetchScenario("ui-tool-state-agent", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages }) });
    const body = await response.text();
    if (go === "reject") {
      expect(response.status, body).toBe(400);
      expect(JSON.parse(body).providerCalls).toBe(0);
    } else {
      expect(response.ok, body).toBe(true);
    }
  });

  it("persists citations and empty-field sources that the pinned validator accepts", async () => {
    const chunks = [
      { type: "start", messageId: "m" },
      { type: "text-start", id: "t" },
      { type: "text-delta", id: "t", delta: "answer" },
      { type: "text-end", id: "t", citations: [{ source: url }, { source: { type: "source", sourceType: "document", id: "", mediaType: "", title: "" }, citedText: "" }] },
      { type: "source-url", sourceId: "u", url: "https://example.com" },
      { type: "source-document", sourceId: "d", mediaType: "text/plain", title: "" },
      { type: "finish" },
    ];
    const frontend: UIMessage[] = [];
    for await (const message of readUIMessageStream({ stream: simulateReadableStream({ chunks: chunks as never[] }), terminateOnError: true })) frontend.push(json(message));
    const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks }) });
    expect(response.ok, await response.clone().text()).toBe(true);
    const persisted = (await response.json()).snapshots.at(-1);
    expect(canonicalUI([persisted])).toEqual(canonicalUI([frontend.at(-1)]));
    await expect(validateUIMessages({ messages: [persisted] })).resolves.toHaveLength(1);
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
    const persistedSnapshots = (await persisted.json()).snapshots;
    expect(canonicalUI(persistedSnapshots)).toEqual(canonicalUI(snapshots));
    await expect(validateUIMessages({ messages: [persistedSnapshots.at(-1)] })).resolves.toHaveLength(1);
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
