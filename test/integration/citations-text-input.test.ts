import { describe, expect, it } from "vitest";
import { readUIMessageStream, simulateReadableStream, validateUIMessages, type UIMessage } from "ai";
import { fetchScenario } from "./helpers.js";
import { canonicalUI, json, readToolStateStream } from "./ui-tool-state-helpers.js";

const url = { type: "source", sourceType: "url", id: "u", url: "url" };
const cited = (citation: unknown) => ({ type: "text", text: "answer", citations: [citation] });
const document = { type: "source", sourceType: "document", id: "", mediaType: "", title: "" };
const assistant = (...parts: unknown[]) => ({ id: "m", role: "assistant", parts });
const user = { id: "u", role: "user", parts: [{ type: "text", text: "question" }] };
const postMessages = (scenario: string, messages: unknown[]) => fetchScenario(scenario, {
  headers: { "content-type": "application/json" },
  body: JSON.stringify({ messages }),
});

const toolStates = {
  "input-streaming": { input: null },
  "input-available": { input: null },
  "approval-requested": { input: null, approval: { id: "a", descriptor: null } },
  "approval-responded": { input: null, approval: { id: "a", approved: true } },
  "output-available": { input: null, output: null, approval: { id: "a", approved: true } },
  "output-error": { input: null, rawInput: null, errorText: "", approval: { id: "a", approved: true } },
  "output-denied": { input: null, approval: { id: "a", approved: false } },
};
const toolParts = ["tool-lookup", "dynamic-tool"].flatMap(type => Object.entries(toolStates).map(([state, fields]) => ({
  type, ...(type === "dynamic-tool" ? { toolName: "lookup" } : {}), toolCallId: "c", state, ...fields,
})));

describe("citations and text-encoded tool input", () => {
  it.each([
    { name: "negative offset", part: cited({ source: url, startIndex: -1 }) },
    { name: "fractional offset", part: cited({ source: url, endIndex: 1.5 }) },
    { name: "unsafe offset", part: cited({ source: url, startIndex: 2 ** 53 }) },
    { name: "unknown source type", part: cited({ source: { type: "source", sourceType: "other", id: "u" } }) },
    { name: "null metadata entry", part: cited({ source: { ...url, providerMetadata: { test: null } } }) },
    { name: "missing citation URL", part: cited({ source: { type: "source", sourceType: "url", id: "u" } }) },
    { name: "missing citation source ID", part: cited({ source: { type: "source", sourceType: "url", url: "url" } }) },
    { name: "missing citation document title", part: cited({ source: { type: "source", sourceType: "document", id: "d", mediaType: "text/plain" } }) },
    { name: "missing citation document media type", part: cited({ source: { type: "source", sourceType: "document", id: "d", title: "" } }) },
    { name: "wrong citation source discriminator", part: cited({ source: { ...url, type: "other" } }) },
    { name: "missing source-url URL", part: { type: "source-url", sourceId: "u" } },
    { name: "missing source-document title", part: { type: "source-document", sourceId: "d", mediaType: "text/plain" } },
  ])("rejects upstream-invalid UI sources before model I/O: $name", async ({ part }) => {
    const messages = [user, assistant(part)];
    await expect(validateUIMessages({ messages })).rejects.toThrow();
    const response = await postMessages("ui-tool-state-agent", messages);
    const body = await response.text();
    expect(response.status, body).toBe(400);
    expect(JSON.parse(body).providerCalls).toBe(0);
  });

  it.each([
    { name: "message", make: () => assistant(), fields: ["id", "role", "parts"] },
    { name: "part envelope", make: () => assistant({ type: "step-start" }), fields: ["parts.0.type"] },
    { name: "text", make: () => assistant({ type: "text", text: "" }), fields: ["parts.0.text"] },
    { name: "reasoning", make: () => assistant({ type: "reasoning", text: "" }), fields: ["parts.0.text"] },
    { name: "custom", make: () => assistant({ type: "custom", kind: "" }), fields: ["parts.0.kind"] },
    ...["file", "reasoning-file"].map(type => ({ name: type, make: () => assistant({ type, mediaType: "", url: "" }), fields: ["parts.0.mediaType", "parts.0.url"] })),
    { name: "source-url", make: () => assistant({ type: "source-url", sourceId: "", url: "" }), fields: ["parts.0.sourceId", "parts.0.url"] },
    { name: "source-document", make: () => assistant({ type: "source-document", sourceId: "", mediaType: "", title: "" }), fields: ["parts.0.sourceId", "parts.0.mediaType", "parts.0.title"] },
    { name: "URL citation", make: () => assistant(cited({ source: { ...url } })), fields: ["parts.0.citations.0.source", ...["type", "sourceType", "id", "url"].map(key => `parts.0.citations.0.source.${key}`)] },
    { name: "document citation", make: () => assistant(cited({ source: { ...document } })), fields: ["parts.0.citations.0.source.mediaType", "parts.0.citations.0.source.title"] },
    ...toolParts.map(part => ({
      name: `${part.type}/${part.state}`,
      make: () => assistant(structuredClone(part)),
      fields: ["toolCallId", "state", ...(part.type === "dynamic-tool" ? ["toolName"] : []),
        ...(part.state === "output-error" ? ["errorText"] : []),
        ...(part.approval ? ["approval.id", ...("approved" in part.approval ? ["approval.approved"] : [])] : []),
        ...(["approval-requested", "approval-responded", "output-denied"].includes(part.state) ? ["approval"] : []),
      ].map(field => `parts.0.${field}`),
    })),
  ])("rejects missing and null required non-opaque fields: $name", async ({ make, fields }) => {
    await expect(validateUIMessages({ messages: [user, make()] })).resolves.toHaveLength(2);
    for (const field of fields) {
      for (const nullValue of [false, true]) {
        const message = make();
        const path = field.split(".");
        const key = path.pop()!;
        const parent = path.reduce<Record<string, unknown>>((node, segment) => node[segment] as Record<string, unknown>, message);
        if (nullValue) parent[key] = null;
        else delete parent[key];
        const messages = [user, message];
        await expect(validateUIMessages({ messages }), field).rejects.toThrow();
        const response = await postMessages("ui-tool-state-agent", messages);
        expect(response.status, `${field} (${nullValue ? "null" : "missing"})`).toBe(400);
        const body = await response.json();
        expect(body.providerCalls).toBe(0);
        expect(body.error).toContain(key);
        if (!nullValue) expect(body.error).toContain("required UI field");
      }
    }
  });

  it.each([
    { type: "data-test", data: null },
    ...toolParts,
  ])("round-trips opaque null and checks omitted payloads: $type $state", async part => {
    const messages = [user, assistant(part)];
    const [parsedUser, parsedAssistant] = await validateUIMessages({ messages });
    expect(parsedAssistant.parts[0]).toEqual(part);
    const roundTrip = await postMessages("ui-tool-state-persistence", messages);
    expect(roundTrip.ok, await roundTrip.clone().text()).toBe(true);
    const { uiMessages } = await roundTrip.json();
    await expect(validateUIMessages({ messages: uiMessages })).resolves.toEqual([parsedUser, parsedAssistant]);
    for (const field of ["data", "input", "output", "rawInput", "descriptor"]) {
      const without = structuredClone(part) as Record<string, unknown>;
      const parent = field === "descriptor" ? without.approval as Record<string, unknown> | undefined : without;
      if (!parent || !Object.hasOwn(parent, field)) continue;
      delete parent[field];
      const mutated = [user, assistant(without)];
      if (field === "descriptor" || field === "rawInput" || ((without.state === "input-streaming" || without.state === "output-error") && field === "input")) {
        await expect(validateUIMessages({ messages: mutated })).resolves.toHaveLength(2);
        const response = await postMessages("ui-tool-state-persistence", mutated);
        expect(response.ok, await response.clone().text()).toBe(true);
      } else {
        await expect(validateUIMessages({ messages: mutated })).rejects.toThrow();
        const response = await postMessages("ui-tool-state-agent", mutated);
        expect(response.status, await response.clone().text()).toBe(400);
        expect(await response.json()).toMatchObject({ providerCalls: 0, error: expect.stringContaining(`required UI field "${field}"`) });
      }
    }
  });

  it("round-trips a required empty approval ID through Go persistence", async () => {
    const messages = [user, assistant({
      type: "dynamic-tool", toolName: "lookup", toolCallId: "c", state: "approval-requested", input: {}, approval: { id: "" },
    })];
    const expected = await validateUIMessages({ messages });
    const response = await postMessages("ui-tool-state-persistence", messages);
    expect(response.ok, await response.clone().text()).toBe(true);
    const { uiMessages } = await response.json();
    expect(uiMessages).toEqual(messages);
    await expect(validateUIMessages({ messages: uiMessages })).resolves.toEqual(expected);
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
