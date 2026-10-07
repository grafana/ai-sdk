import { convertToModelMessages, readUIMessageStream, simulateReadableStream, type UIMessage, type UIMessageChunk } from "ai";
import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { canonicalModel, canonicalUI, json, readToolStateStream } from "./ui-tool-state-helpers.js";

async function persist(initialMessage: UIMessage, chunks: UIMessageChunk[]) {
  const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ initialMessage, chunks }) });
  expect(response.status).toBe(200);
  return response.json();
}

describe("fixed upstream target compatibility", () => {
  it.each([false, true])("drops unresolved approvals superseded by a later user turn (dynamic: %s)", async dynamic => {
    const initialMessage = { id: "m", role: "assistant", parts: [{ type: "text", text: "pending" }, { type: dynamic ? "dynamic-tool" : "tool-lookup", ...(dynamic ? { toolName: "lookup" } : {}), toolCallId: "c", state: "approval-requested", input: {}, approval: { id: "a" } }] } as UIMessage;
    const messages: UIMessage[] = [initialMessage, { id: "u", role: "user", parts: [{ type: "text", text: "new turn" }] }];
    const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages }) });
    expect(response.status).toBe(200);
    const go = await response.json();
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages(messages)));
    expect(go.modelMessages[0].content).toEqual([{ type: "text", text: "pending" }]);
  });
  it("preserves Anthropic fallback boundaries through schema-parsed SSE and model continuation", async () => {
    const response = await fetchScenario("anthropic-fallback-content");
    expect(response.status).toBe(200);
    const parsed = await readToolStateStream(response);
    const custom = parsed.chunks.find(chunk => chunk.type === "custom");
    expect(custom).toEqual({ type: "custom", kind: "anthropic.fallback", providerMetadata: { anthropic: { type: "fallback", from: { model: "primary" }, to: { model: "secondary" } } } });
    const message = parsed.snapshots.at(-1)!;
    expect(message.parts.map(part => part.type)).toEqual(["step-start", "reasoning", "custom", "reasoning"]);
    const go = await persist(message, []);
    expect(canonicalUI(go.uiMessages)).toEqual(canonicalUI([message]));
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([message])));
  });

  it.each([false, true])("replaces tool metadata on output errors (dynamic: %s)", async dynamic => {
    const initialMessage = { id: "m", role: "assistant", parts: [{ type: dynamic ? "dynamic-tool" : "tool-lookup", ...(dynamic ? { toolName: "lookup" } : {}), toolCallId: "c", state: "input-available", input: {}, toolMetadata: { phase: "input" } }] } as UIMessage;
    const updates: UIMessageChunk[] = [{ type: "tool-output-error", toolCallId: "c", errorText: "failed", toolMetadata: {} }];
    const response = await fetchScenario("ui-tool-state-chunks", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks: updates }) });
    const expected = await readToolStateStream(response, json(initialMessage));
    expect(expected.chunks).toEqual(updates);
    const go = await persist(initialMessage, updates);
    expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected.snapshots));
    expect(go.uiMessages[0].parts[0].toolMetadata).toEqual({});
  });

  it.each([false, true])("resumes last-step persisted raw tool input (dynamic: %s)", async dynamic => {
    const initialMessage = { id: "m", role: "assistant", parts: [
      { type: "tool-lookup", toolCallId: "old", state: "input-streaming", input: {}, rawInput: "{" },
      { type: "step-start" },
      { type: dynamic ? "dynamic-tool" : "tool-lookup", ...(dynamic ? { toolName: "lookup" } : {}), toolCallId: "c", state: "input-streaming", input: { q: "par" }, rawInput: '{"q":"par', title: "", toolMetadata: {} },
    ] } as UIMessage;
    const updates: UIMessageChunk[] = [
      { type: "tool-input-delta", toolCallId: "c", inputTextDelta: 'tial"}' },
      { type: "tool-input-available", toolCallId: "c", toolName: "lookup", input: { q: "partial" }, dynamic },
      { type: "tool-output-available", toolCallId: "c", output: "ok", toolMetadata: { phase: "result" } },
    ];
    const expected: UIMessage[] = [];
    for await (const message of readUIMessageStream({ message: json(initialMessage), stream: simulateReadableStream({ chunks: json(updates) }), terminateOnError: true })) expected.push(json(message));
    const go = await persist(initialMessage, updates);
    expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected));
    expect(go.snapshots[0].parts[2]).toMatchObject({ input: { q: "partial" }, rawInput: '{"q":"partial"}' });
    expect(go.uiMessages[0].parts[2]).not.toHaveProperty("rawInput");
    expect(go.uiMessages[0].parts[2].toolMetadata).toEqual({ phase: "result" });
    const response = await fetchScenario("ui-tool-state-chunks", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks: updates }) });
    const parsed = await readToolStateStream(response, json(initialMessage));
    expect(parsed.chunks).toEqual(updates);
    expect(canonicalUI(parsed.snapshots)).toEqual(canonicalUI(expected));
  });
});
