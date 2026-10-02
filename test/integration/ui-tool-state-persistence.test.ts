import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { convertToModelMessages, createAgentUIStream, jsonSchema, readUIMessageStream, simulateReadableStream, tool, ToolLoopAgent, validateUIMessages, type UIMessage, type UIMessageChunk } from "ai";
import { describe, expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import { canonicalModel, canonicalUI, json, readToolStateStream, referenceToolStateModel, toolStateReferenceTools } from "./ui-tool-state-helpers.js";

const chunks = JSON.parse(readFileSync(resolve(import.meta.dirname, "fixtures/ui-tool-state-chunks.json"), "utf8")) as UIMessageChunk[];
const states = ["input-streaming", "input-available", "approval-requested", "approval-responded", "output-available", "output-error", "output-denied"] as const;

async function persist(body: unknown) {
  const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
  expect(response.status).toBe(200);
  return response.json();
}

function stateMessage(state: string, dynamic: boolean, fields: Record<string, unknown> = {}): UIMessage {
  const part: Record<string, unknown> = { type: dynamic ? "dynamic-tool" : "tool-lookup", ...(dynamic ? { toolName: "lookup" } : {}), toolCallId: "c", state, title: "", toolMetadata: {}, callProviderMetadata: {}, ...fields };
  if (state !== "input-streaming" && state !== "output-error") part.input ??= { q: "test" };
  if (state === "output-available") { if (!("output" in part)) part.output = "ok"; if (!("preliminary" in part)) part.preliminary = false; if (!("resultProviderMetadata" in part)) part.resultProviderMetadata = {}; }
  if (state === "output-error") { part.errorText = ""; if (!("rawInput" in part)) part.rawInput = "legacy"; if (!("resultProviderMetadata" in part)) part.resultProviderMetadata = {}; }
  if (state === "approval-requested") part.approval = { id: "a", descriptor: null, requestReason: "", signature: "sig", isAutomatic: true };
  if (state === "approval-responded" || state === "output-denied") part.approval = { id: "a", approved: state === "approval-responded", descriptor: { scope: "all" }, requestReason: "", reason: "", signature: "sig", isAutomatic: true };
  return { id: "m", role: "assistant", parts: [part] } as unknown as UIMessage;
}

describe("pinned UI tool state persistence", () => {
  it("matches schema-parsed chunks, every reader write point and persisted model conversion", async () => {
    const reference: UIMessage[] = [];
    for await (const message of readUIMessageStream({ stream: simulateReadableStream({ chunks: json(chunks) }), terminateOnError: true })) reference.push(json(message));
    const go = await persist({ chunks, convertData: true, convertOutput: true });
    expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(reference));
    expect(canonicalUI(go.uiMessages)).toEqual(canonicalUI([reference.at(-1)]));
    const response = await fetchScenario("ui-tool-state-chunks", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks }) });
    const parsed = await readToolStateStream(response);
    expect(parsed.chunks).toEqual(chunks);
    expect(canonicalUI(parsed.snapshots)).toEqual(canonicalUI(reference));
    const converted = await convertToModelMessages([reference.at(-1)!], {
      convertDataPart: part => ({ type: "text", text: JSON.stringify(part.data) }),
      tools: { lookup: tool({ inputSchema: jsonSchema({}), toModelOutput: ({ output }) => ({ type: "content", value: [{ type: "text", text: "converted:" + JSON.stringify(output) }] }) }) },
    });
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(converted));
  });

  for (const dynamic of [false, true]) {
    for (const state of states) {
      it(`${dynamic ? "dynamic" : "static"} ${state} survives persistence and matches both conversion modes`, async () => {
        const message = stateMessage(state, dynamic);
        await validateUIMessages({ messages: [message] });
        for (const ignoreIncomplete of [false, true]) {
          const go = await persist({ messages: [message], ignoreIncomplete });
          expect(canonicalUI(go.uiMessages)).toEqual(canonicalUI([message]));
          const expected = await convertToModelMessages([message], { ignoreIncompleteToolCalls: ignoreIncomplete });
          expect(canonicalModel(go.modelMessages ?? [])).toEqual(canonicalModel(expected));
        }
      });
    }
    it(`${dynamic ? "dynamic" : "static"} preliminary, input and metadata presence`, async () => {
      for (const preliminary of [undefined, false, true]) {
        for (const providerExecuted of [false, true]) {
          const message = stateMessage("output-available", dynamic, { providerExecuted, preliminary, callProviderMetadata: { test: { source: "call" } }, resultProviderMetadata: {} });
          for (const ignoreIncomplete of [false, true]) {
            const go = await persist({ messages: [message], ignoreIncomplete });
            const expected = await convertToModelMessages([message], { ignoreIncompleteToolCalls: ignoreIncomplete });
            expect(canonicalModel(go.modelMessages ?? [])).toEqual(canonicalModel(expected));
          }
        }
      }
      for (const input of [undefined, null, {}, "", 1]) {
        for (const rawInput of [undefined, null, "legacy", {}]) {
          const message = stateMessage("output-error", dynamic, { input, rawInput, callProviderMetadata: {}, resultProviderMetadata: { test: { source: "result" } } });
          const go = await persist({ messages: [message] });
          expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([message])));
        }
      }
    });
  }

  it("converts data/custom/files in their step blocks without broad empty-value normalization", async () => {
    const message: UIMessage = { id: "data", role: "assistant", parts: [
      { type: "text", text: "before" }, { type: "data-empty", id: "d", data: "" }, { type: "data-skip", data: null },
      { type: "step-start" }, { type: "custom", kind: "test.kind", providerMetadata: { test: { empty: "" } } },
      { type: "file", mediaType: "text/plain", url: "https://example.test/data", filename: "", providerReference: {} },
    ] };
    const go = await persist({ messages: [message], convertData: true });
    const expected = await convertToModelMessages([message], { convertDataPart: part => part.type === "data-skip" ? undefined : { type: "text", text: "" } });
    expect(go.uiMessages).toEqual([message]);
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(expected));
    expect(go.modelMessages[0].content[1]).toEqual({ type: "text", text: "" });
  });

  it.each([false, true])("matches block-local callback order and failure precedence (dynamic: %s)", async dynamic => {
    for (const tc of [
      {}, { providerExecuted: true }, { separate: true }, { failData: true },
      { failOutput: true }, { failData: true, failOutput: true },
      { providerExecuted: true, failData: true }, { providerExecuted: true, failData: true, failOutput: true },
    ] as Array<{ providerExecuted?: boolean; separate?: boolean; failData?: boolean; failOutput?: boolean }>) {
      const message = stateMessage("output-available", dynamic, { providerExecuted: tc.providerExecuted });
      if (tc.separate) message.parts.push({ type: "step-start" });
      message.parts.push({ type: "data-count", data: null });
      const callbacks: string[] = [];
      let outputCalls = 0;
      const expected = convertToModelMessages([message], {
        tools: { lookup: tool({ inputSchema: jsonSchema({}), toModelOutput: ({ output }) => {
          callbacks.push("output");
          outputCalls++;
          if (tc.failOutput) throw new Error("output converter failed");
          return { type: "content", value: [{ type: "text", text: "converted:" + JSON.stringify(output) }] };
        } }) },
        convertDataPart: () => {
          callbacks.push("data");
          if (tc.failData) throw new Error("data converter failed");
          return { type: "text", text: String(outputCalls) };
        },
      });
      const outcome = await expected.then(modelMessages => ({ modelMessages, error: undefined }), error => ({ modelMessages: undefined, error: (error as Error).message }));
      const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages: [message], convertData: true, convertOutput: true, traceCallbacks: true, ...tc }) });
      const go = await response.json();
      expect(go.callbacks).toEqual(callbacks);
      if (outcome.error) {
        expect(response.status).toBe(400);
        expect(go.error).toContain(outcome.error);
      } else {
        expect(response.status).toBe(200);
        expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(outcome.modelMessages));
      }
    }
  });

  it("resumes persisted approval/preliminary history with pinned update and placement rules", async () => {
    for (const dynamic of [false, true]) {
      for (const supplied of [false, true]) {
        const initialMessage = stateMessage("approval-requested", dynamic, { providerExecuted: true });
        const updates: UIMessageChunk[] = [
          { type: "tool-approval-response", approvalId: "a", approved: true, reason: "", ...(supplied ? { providerExecuted: false } : {}) },
          { type: "tool-output-available", toolCallId: "c", output: "partial", preliminary: true },
          { type: "tool-output-available", toolCallId: "c", output: "final", preliminary: false },
        ];
        const expected: UIMessage[] = [];
        for await (const message of readUIMessageStream({ message: json(initialMessage), stream: simulateReadableStream({ chunks: updates }), terminateOnError: true })) expected.push(json(message));
        const go = await persist({ initialMessage, chunks: updates });
        expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected));
        expect(go.uiMessages[0].parts).toHaveLength(1);
        expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([expected.at(-1)!])));
      }
    }
  });

  it.each([false, true])("clears approval metadata through schema-parsed seeded SSE (dynamic: %s)", async dynamic => {
    const initialMessage = stateMessage("approval-requested", dynamic, { callProviderMetadata: { test: { phase: "old" } } });
    const updates: UIMessageChunk[] = [
      { type: "tool-approval-response", approvalId: "a", approved: true, providerMetadata: {} },
      { type: "tool-output-available", toolCallId: "c", output: "final" },
    ];
    const expected: UIMessage[] = [];
    for await (const message of readUIMessageStream({ message: json(initialMessage), stream: simulateReadableStream({ chunks: json(updates) }), terminateOnError: true })) expected.push(json(message));
    const go = await persist({ initialMessage, chunks: updates });
    expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected));
    const response = await fetchScenario("ui-tool-state-chunks", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks: updates }) });
    const parsed = await readToolStateStream(response, json(initialMessage));
    expect(parsed.chunks).toEqual(updates);
    expect(canonicalUI(parsed.snapshots)).toEqual(canonicalUI(expected));
    expect(parsed.snapshots.at(-1)!.parts[0]).toMatchObject({ callProviderMetadata: {} });
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([parsed.snapshots.at(-1)!])));
  });

  it("matches static/dynamic raw-input transitions and supplied output execution values", async () => {
    for (const dynamic of [false, true]) {
      const initialMessage = stateMessage("output-error", dynamic, { input: { q: "old" }, rawInput: "legacy", title: "old", toolMetadata: { old: true } });
      const updates: UIMessageChunk[] = [
        { type: "tool-input-error", toolCallId: "c", toolName: "lookup", input: "rejected", errorText: "", dynamic, title: "ignored", toolMetadata: {}, providerMetadata: {} },
        { type: "tool-output-available", toolCallId: "c", output: "partial", preliminary: true },
        { type: "tool-output-available", toolCallId: "c", output: "final", preliminary: false },
        { type: "tool-output-error", toolCallId: "c", errorText: "", providerMetadata: { test: { source: "result" } } },
      ];
      const expected: UIMessage[] = [];
      for await (const message of readUIMessageStream({ message: json(initialMessage), stream: simulateReadableStream({ chunks: json(updates) }), terminateOnError: true })) expected.push(json(message));
      const go = await persist({ initialMessage, chunks: updates });
      expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected));
      expect(go.uiMessages[0].parts).toHaveLength(1);
      expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([expected.at(-1)!])));
      for (const kind of ["tool-output-available", "tool-output-error"] as const) {
        for (const supplied of [false, true]) {
          const seed = stateMessage("input-available", dynamic, { providerExecuted: true });
          const update: UIMessageChunk = kind === "tool-output-available"
            ? { type: kind, toolCallId: "c", output: "ok", ...(supplied ? { providerExecuted: false } : {}) }
            : { type: kind, toolCallId: "c", errorText: "", ...(supplied ? { providerExecuted: false } : {}) };
          let final: UIMessage | undefined;
          for await (const message of readUIMessageStream({ message: json(seed), stream: simulateReadableStream({ chunks: [update] }), terminateOnError: true })) final = json(message);
          const go = await persist({ initialMessage: seed, chunks: [update] });
          expect(canonicalUI(go.uiMessages)).toEqual(canonicalUI([final]));
          expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([final!])));
        }
      }
    }
  });

  it.each([false, true])("retains static raw input on error continuations (seeded: %s)", async seeded => {
    const initialMessage = seeded ? stateMessage("output-error", false) : undefined;
    const updates: UIMessageChunk[] = [
      ...(!seeded ? [{ type: "start" as const, messageId: "m" }, { type: "tool-input-error" as const, toolCallId: "c", toolName: "lookup", input: "legacy", errorText: "old" }] : []),
      { type: "tool-output-error", toolCallId: "c", errorText: "" },
    ];
    const expected: UIMessage[] = [];
    for await (const message of readUIMessageStream({ message: initialMessage ? json(initialMessage) : undefined, stream: simulateReadableStream({ chunks: updates }), terminateOnError: true })) expected.push(json(message));
    const go = await persist({ initialMessage, chunks: updates });
    expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected));
    expect(go.uiMessages[0].parts[0].rawInput).toBe("legacy");
    expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([expected.at(-1)!])));
  });

  it("normalizes reader-retained dynamic raw input on an isolated Agent history", async () => {
    const initialMessage = stateMessage("output-error", true, { input: { q: "old" } });
    const updates: UIMessageChunk[] = [{ type: "tool-output-available", toolCallId: "c", output: "final" }];
    const assembled = await persist({ initialMessage, chunks: updates });
    const messages = assembled.uiMessages as UIMessage[];
    const original = json(messages);
    expect(assembled.uiMessages[0].parts[0].rawInput).toBe("legacy");
    const normalized = await validateUIMessages({ messages });
    const model = referenceToolStateModel();
    const stream = await createAgentUIStream({ agent: new ToolLoopAgent({ model, tools: toolStateReferenceTools }), uiMessages: messages });
    for await (const _ of stream) {}
    const response = await fetchScenario("ui-tool-state-validation", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages }) });
    expect(response.status).toBe(200);
    const go = await response.json();
    expect(go.metadata.providerCalls).toBe(1);
    expect(go.message.parts[0]).toEqual(normalized[0].parts[0]);
    expect(canonicalModel(go.metadata.modelMessages)).toEqual(canonicalModel(model.doStreamCalls[0].prompt));
    expect(messages).toEqual(original);
  });

  it.each([false, true])("projects state-specific fields before Agent conversion (dynamic: %s)", async dynamic => {
    for (const state of states) {
      const message = stateMessage(state, dynamic, { rawInput: "legacy", preliminary: false, resultProviderMetadata: {} });
      const messages: UIMessage[] = [{ id: "u", role: "user", parts: [{ type: "text", text: "continue" }] }, message];
      const original = json(messages);
      const normalized = await validateUIMessages({ messages, tools: toolStateReferenceTools });
      const model = referenceToolStateModel();
      const stream = await createAgentUIStream({ agent: new ToolLoopAgent({ model, tools: toolStateReferenceTools }), uiMessages: messages });
      for await (const _ of stream) {}
      const response = await fetchScenario("ui-tool-state-validation", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages }) });
      expect(response.status).toBe(200);
      const go = await response.json();
      expect(go.metadata?.providerCalls ?? 0, state).toBe(model.doStreamCalls.length);
      if (model.doStreamCalls.length > 0) expect(canonicalModel(go.metadata.modelMessages)).toEqual(canonicalModel(model.doStreamCalls[0].prompt));
      for (const key of ["rawInput", "preliminary", "resultProviderMetadata"]) {
        if (!(key in normalized[1].parts[0])) expect(go.message.parts[0]).not.toHaveProperty(key);
      }
      expect(messages).toEqual(original);
    }
  });

  it.each([false, true])("accepts persisted retry/approval history with retained result metadata (dynamic: %s)", async dynamic => {
    const initialMessage = stateMessage("output-error", dynamic, { input: { q: "old" } });
    const updates: UIMessageChunk[] = [
      { type: "tool-input-available", toolCallId: "c", toolName: "lookup", input: { q: "new" }, dynamic },
      { type: "tool-approval-request", toolCallId: "c", approvalId: "a" },
      { type: "tool-approval-response", approvalId: "a", approved: true },
    ];
    const expected: UIMessage[] = [];
    for await (const message of readUIMessageStream({ message: json(initialMessage), stream: simulateReadableStream({ chunks: updates }), terminateOnError: true })) expected.push(json(message));
    const assembled = await persist({ initialMessage, chunks: updates });
    expect(canonicalUI(assembled.snapshots)).toEqual(canonicalUI(expected));
    expect(assembled.uiMessages[0].parts[0].resultProviderMetadata).toEqual({});
    const messages = assembled.uiMessages as UIMessage[];
    const original = json(messages);
    const model = referenceToolStateModel();
    const stream = await createAgentUIStream({ agent: new ToolLoopAgent({ model, tools: toolStateReferenceTools }), uiMessages: messages });
    for await (const _ of stream) {}
    const response = await fetchScenario("ui-tool-state-validation", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages }) });
    expect(response.status).toBe(200);
    const go = await response.json();
    expect(go.metadata.providerCalls).toBe(1);
    expect(go.message.parts[0]).not.toHaveProperty("resultProviderMetadata");
    expect(go.message.parts[0]).toMatchObject({ state: "output-available", output: "approved" });
    expect(canonicalModel(go.metadata.modelMessages)).toEqual(canonicalModel(model.doStreamCalls[0].prompt));
    expect(messages).toEqual(original);
  });

  it.each([{ messages: null }, { messages: [] }])("rejects empty Agent history before provider invocation: $messages", async ({ messages }) => {
    const model = referenceToolStateModel();
    await expect(createAgentUIStream({ agent: new ToolLoopAgent({ model, tools: toolStateReferenceTools }), uiMessages: messages as unknown as UIMessage[] })).rejects.toThrow();
    expect(model.doStreamCalls).toHaveLength(0);
    const response = await fetchScenario("ui-tool-state-agent", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages }) });
    expect(response.status).toBe(400);
    expect((await response.json()).providerCalls).toBe(0);
  });

  it("retains request fields through denied responses on static and dynamic tools", async () => {
    for (const dynamic of [false, true]) {
      const initialMessage = stateMessage("approval-requested", dynamic, { providerExecuted: true });
      const updates: UIMessageChunk[] = [
        { type: "tool-approval-response", approvalId: "a", approved: false, reason: "", providerExecuted: false },
        { type: "tool-approval-response", approvalId: "a", approved: false },
        { type: "tool-output-denied", toolCallId: "c" },
      ];
      const expected: UIMessage[] = [];
      for await (const message of readUIMessageStream({ message: json(initialMessage), stream: simulateReadableStream({ chunks: updates }), terminateOnError: true })) expected.push(json(message));
      const go = await persist({ initialMessage, chunks: updates });
      expect(canonicalUI(go.snapshots)).toEqual(canonicalUI(expected));
      expect(canonicalModel(go.modelMessages)).toEqual(canonicalModel(await convertToModelMessages([expected.at(-1)!])));
    }
  });

  it.each([
    { title: null }, { preliminary: null }, { toolMetadata: null }, { callProviderMetadata: [] },
    { resultProviderMetadata: { test: null } }, { approval: { id: "a", approved: true, reason: 1 } },
  ])("rejects malformed persisted optional fields before provider invocation: %j", async fields => {
    const message = stateMessage("output-available", false, fields);
    await expect(validateUIMessages({ messages: [message] })).rejects.toThrow();
    const response = await fetchScenario("ui-tool-state-agent", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages: [message] }) });
    expect(response.status).toBe(400);
    expect((await response.json()).providerCalls).toBe(0);
  });

  it.each([
    { name: "missing terminal", fields: { type: "tool-obsolete", input: {}, output: "ok" }, state: "output-available", valid: true },
    { name: "invalid error input", fields: { input: { q: 1 }, errorText: "" }, state: "output-error", valid: true },
    { name: "legacy error", fields: { rawInput: "legacy", errorText: "" }, state: "output-error", valid: true },
    { name: "empty available input", fields: { input: {}, output: "ok" }, state: "output-available", valid: true },
    { name: "nonempty available input", fields: { input: { q: 1 }, output: "ok" }, state: "output-available", valid: false },
    { name: "output checked before normalization", fields: { input: {}, output: 1 }, state: "output-available", valid: false },
    { name: "missing nonterminal", fields: { type: "tool-obsolete", input: {} }, state: "input-available", valid: false },
    { name: "invalid configured input", fields: { input: { q: 1 } }, state: "input-available", valid: false },
    { name: "missing error", fields: { input: {} }, state: "output-error", valid: false },
    { name: "denied without approval", fields: { input: { q: "test" } }, state: "output-denied", valid: false },
  ])("Agent-specific normalization: $name", async ({ state, fields, valid }) => {
    const message = { id: "m", role: "assistant", parts: [{ type: "tool-lookup", toolCallId: "c", state, ...fields }] } as unknown as UIMessage;
    const original = json(message);
    const model = referenceToolStateModel();
    const agent = new ToolLoopAgent({ model, tools: toolStateReferenceTools });
    if (valid) {
      const stream = await createAgentUIStream({ agent, uiMessages: [message] });
      for await (const _ of stream) {}
      const response = await fetchScenario("ui-tool-state-validation", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages: [message] }) });
      expect(response.status).toBe(200);
      const go = await response.json();
      expect(go.metadata.providerCalls).toBe(1);
      expect(canonicalModel(go.metadata.modelMessages)).toEqual(canonicalModel(model.doStreamCalls[0].prompt));
      const expected = await validateUIMessages({ messages: [go.message] });
      expect(expected).toHaveLength(1);
      if (state !== "output-error" || !("rawInput" in fields)) expect(go.message.parts[0].type).toBe("dynamic-tool");
    } else {
      await expect(createAgentUIStream({ agent, uiMessages: [message] })).rejects.toThrow();
      expect(model.doStreamCalls).toHaveLength(0);
      const response = await fetchScenario("ui-tool-state-agent", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages: [message] }) });
      expect(response.status).toBe(400);
      expect((await response.json()).providerCalls).toBe(0);
    }
    expect(message).toEqual(original);
  });
});
