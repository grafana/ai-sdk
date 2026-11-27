import { createAnthropic } from "@ai-sdk/anthropic";
import { jsonSchema, stepCountIs, streamText, tool, type ModelMessage } from "ai";
import { expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";
import syntheticResponses from "./testserver/fixtures/anthropic-synthetic-signed-continuation.json";

const modelId = "claude-sonnet-4-6";
const baseURL = "https://anthropic.invalid/v1";
const thinking = { type: "adaptive", display: "summarized" } as const;
const initialUser = "Look up the shelf label for b7.";
const nextUser = "Where is the atlas?";
const summary = "Résumé : b7 → atlas  \n";
const summarySignature = "synthetic-summary-signature+/=";
const compactBeta = "compact-2026-09-04";
const inputSchema = jsonSchema<{ slot: string }>({
  type: "object",
  properties: { slot: { type: "string" } },
  required: ["slot"],
});

type NativeMessage = { role: string; content: Record<string, unknown>[] };
type NativeRequest = {
  method: string;
  target: string;
  version: string;
  beta: string;
  body: { messages: NativeMessage[]; model: string; thinking?: object; compaction?: object; stream: boolean };
};
type ToolExecution = { id: string; slot: string };
type GoHistory = { role: string; content: { providerOptions?: { anthropic?: { type?: string; signature?: string } } }[] }[];
type Turn = {
  requests: NativeRequest[];
  executions: ToolExecution[];
  finishes: { unified: string; raw: string | undefined }[];
  text: string;
};

async function goTurn(history?: GoHistory): Promise<Turn & { history: GoHistory }> {
  const response = await fetchScenario("anthropic-signed-continuation", {
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ history }),
  });
  if (!response.ok) throw new Error(`Go continuation: ${response.status} ${await response.text()}`);
  return response.json();
}

async function tsTurn(history?: ModelMessage[]): Promise<Turn & { history: ModelMessage[] }> {
  const requests: NativeRequest[] = [];
  const executions: ToolExecution[] = [];
  const responses = history === undefined ? syntheticResponses.slice(0, 2) : syntheticResponses.slice(2);
  const model = createAnthropic({
    apiKey: "test-key",
    baseURL,
    fetch: async (input, init) => {
      const request = new Request(input, init);
      const events = responses[requests.length];
      requests.push({
        method: request.method,
        target: request.url,
        version: request.headers.get("anthropic-version") ?? "",
        beta: request.headers.get("anthropic-beta") ?? "",
        body: await request.json(),
      });
      if (!events) throw new Error(`Unexpected TypeScript native request ${requests.length}`);
      return new Response(events.map(event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join(""), {
        headers: { "Content-Type": "text/event-stream" },
      });
    },
  })(modelId);
  const messages: ModelMessage[] = [{ role: "user", content: initialUser }];
  if (history !== undefined) messages.push(...history, { role: "user", content: nextUser });
  const result = streamText({
    model,
    messages,
    stopWhen: stepCountIs(2),
    maxOutputTokens: 1024,
    maxRetries: 0,
    providerOptions: { anthropic: { thinking } },
    tools: {
      lookup: tool({
        description: "Look up a shelf label.",
        inputSchema,
        execute: ({ slot }, { toolCallId }) => {
          executions.push({ id: toolCallId, slot });
          return { label: "atlas", slot };
        },
        toModelOutput: ({ output }) => ({
          type: "content",
          value: [
            { type: "text", text: `slot=${output.slot}` },
            { type: "text", text: `label=${output.label}` },
          ],
        }),
      }),
    },
    prepareStep: ({ stepNumber }) => history === undefined && stepNumber === 1
      ? { providerOptions: { anthropic: { thinking, compaction: { type: "summarize" } } } }
      : undefined,
  });
  for await (const part of result.fullStream) {
    if (part.type === "error") throw part.error;
  }
  return {
    history: await result.responseMessages,
    requests,
    executions,
    finishes: (await result.steps).map(step => ({ unified: step.finishReason, raw: step.rawFinishReason })),
    text: await result.text,
  };
}

function betaTokens(request: NativeRequest) {
  return request.beta.split(",").map(beta => beta.trim()).filter(Boolean).sort();
}

function compactBetaCount(request: NativeRequest) {
  return betaTokens(request).filter(beta => beta === compactBeta).length;
}

const inputMessage = { role: "user", content: [{ type: "text", text: initialUser }] };
const toolHistory = [
  {
    role: "assistant",
    content: [
      { type: "thinking", thinking: "Look up the shelf label. \n", signature: "synthetic-thinking-signature+/=" },
      { type: "tool_use", id: "toolu_lookup", name: "lookup", input: { slot: "b7" } },
    ],
  },
  {
    role: "user",
    content: [{
      type: "tool_result",
      tool_use_id: "toolu_lookup",
      content: [{ type: "text", text: "slot=b7" }, { type: "text", text: "label=atlas" }],
    }],
  },
];

function expectSignedReplay(request: NativeRequest) {
  expect(request.body.messages, "signed continuation messages").toEqual([
    inputMessage,
    ...toolHistory,
    { role: "assistant", content: [{ type: "compaction", content: summary, signature: summarySignature }] },
    { role: "user", content: [{ type: "text", text: nextUser }] },
  ]);
  expect(request.body).not.toHaveProperty("compaction");
  expect(compactBetaCount(request)).toBe(1);
}

it("matches pinned Anthropic signed thinking, local tool and compaction across a fresh turn (synthetic)", async () => {
  const goFirst = await goTurn();
  const tsFirst = await tsTurn();
  expect(goFirst.history.map(message => message.role)).toEqual(["assistant", "tool", "assistant"]);
  expect(tsFirst.history.map(message => message.role)).toEqual(["assistant", "tool", "assistant"]);
  const goNext = await goTurn(goFirst.history);
  const tsNext = await tsTurn(tsFirst.history);
  const canonicalRequests = (turn: Turn) => turn.requests.map(request => ({ ...request, beta: betaTokens(request) }));
  expect(canonicalRequests(goFirst)).toEqual(canonicalRequests(tsFirst));
  expect(canonicalRequests(goNext)).toEqual(canonicalRequests(tsNext));

  for (const [first, next] of [[goFirst, goNext], [tsFirst, tsNext]] as const) {
    expect(first.requests).toHaveLength(2);
    expect(next.requests).toHaveLength(1);
    expect(first.executions).toEqual([{ id: "toolu_lookup", slot: "b7" }]);
    expect(next.executions).toEqual([]);
    expect(first.finishes).toEqual([
      { unified: "tool-calls", raw: "tool_use" },
      { unified: "other", raw: "compaction" },
    ]);
    expect(next.finishes).toEqual([{ unified: "stop", raw: "end_turn" }]);
    expect(first.text).toBe(summary);
    expect(next.text).toBe("The atlas is in b7.");

    const [lookup, compact] = first.requests;
    expect(lookup.body.messages).toEqual([inputMessage]);
    expect(lookup.body).not.toHaveProperty("compaction");
    expect(compactBetaCount(lookup)).toBe(0);
    expect(compact.body.messages).toEqual([inputMessage, ...toolHistory]);
    expect(compact.body.compaction).toEqual({ type: "summarize" });
    expect(compactBetaCount(compact)).toBe(1);
    expectSignedReplay(next.requests[0]);

    for (const request of [...first.requests, ...next.requests]) {
      expect(request).toMatchObject({ method: "POST", target: `${baseURL}/messages`, version: "2023-06-01" });
      expect(request.body).toMatchObject({ model: modelId, thinking, stream: true });
    }
  }

  const unsignedHistory = structuredClone(goFirst.history);
  const metadata = unsignedHistory.flatMap(message => message.content)
    .find(part => part.providerOptions?.anthropic?.type === "compaction")?.providerOptions?.anthropic;
  if (!metadata) throw new Error("Expected compaction metadata in returned Go history");
  expect(metadata.signature).toBe(summarySignature);
  delete metadata.signature;
  const unsigned = await goTurn(unsignedHistory);
  expect(unsigned.requests).toHaveLength(1);
  expect(unsigned.executions).toEqual([]);
  expect(unsigned.requests[0].body.messages[3]?.content).toEqual([{ type: "compaction", content: summary }]);
  expect(compactBetaCount(unsigned.requests[0])).toBe(0);
  expect(() => expectSignedReplay(unsigned.requests[0])).toThrow("signed continuation messages");
});
