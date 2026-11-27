import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createOpenAI } from "@ai-sdk/openai";
import { dynamicTool, jsonSchema, stepCountIs, streamText, type ModelMessage } from "ai";
import { expect, it } from "vitest";
import { fetchScenario } from "./helpers.js";

const baseURL = "https://responses.invalid/v1";

function replayLines(stage: number): string[] {
  const path = resolve(import.meta.dirname, `../conformance/openai/upstream/mcp-tool-approval-${stage}/input.chunks.txt`);
  return readFileSync(path, "utf8").trimEnd().split("\n");
}

// Link two authentic, separately imported responses by approval ID. This does
// not reconstruct the original conversation or test live server acceptance.
// Pinned core strips the approval-request part from provider history, so the
// resumed call/result use the native MCP item ID, not the first SDK's dummy ID.
it("carries a hosted approval from the first SDK response into the second request and result", async () => {
  const streams = [replayLines(3), replayLines(4)];
  const completedCalls = streams[1].map(line => JSON.parse(line)).filter(event =>
    event.type === "response.output_item.done" && event.item.type === "mcp_call" && event.item.status === "completed"
  );
  expect(completedCalls).toHaveLength(1);
  const { approval_request_id: binding, id: itemId } = completedCalls[0].item;
  expect(binding).toBeTruthy();
  expect(itemId).toBeTruthy();

  const requests: unknown[] = [];
  const openai = createOpenAI({
    apiKey: "offline-test-key",
    baseURL,
    fetch: async (url, init) => {
      const step = requests.length;
      expect(String(url)).toBe(`${baseURL}/responses`);
      expect(init?.method).toBe("POST");
      expect(step).toBeLessThan(streams.length);
      const { input, tools, store, stream, previous_response_id, conversation } = JSON.parse(String(init?.body));
      requests.push({ input, tools });
      expect({ stream, store, previous_response_id, conversation }).toStrictEqual({
        stream: true, store: true, previous_response_id: undefined, conversation: undefined,
      });
      const items: Array<Record<string, unknown>> = input;
      expect(items.filter(item => item.type === "function_call_output")).toEqual([]);
      expect(items.filter(item => item.type === "mcp_approval_response")).toEqual(step === 0 ? [] : [
        { type: "mcp_approval_response", approval_request_id: binding, approve: true },
      ]);
      expect(items.filter(item => item.type === "item_reference" && item.id === binding)).toEqual(step === 0 ? [] : [
        { type: "item_reference", id: binding },
      ]);
      return new Response(streams[step].map(line => `data: ${line}\n\n`).join(""), {
        headers: { "Content-Type": "text/event-stream" },
      });
    },
  });
  let localExecutions = 0;
  const tools = {
    MCP: openai.tools.mcp({ serverLabel: "zip1", serverUrl: "https://mcp.invalid", requireApproval: "always" }),
    "mcp.create_short_url": dynamicTool({
      inputSchema: jsonSchema({ type: "object" }),
      execute() {
        localExecutions++;
        throw new Error("hosted tool must not execute locally");
      },
    }),
  };
  const model = openai.responses("gpt-5-mini");
  const run = async (messages: ModelMessage[]) => {
    const result = streamText({
      model,
      messages,
      tools,
      activeTools: ["MCP"],
      maxRetries: 0,
      stopWhen: stepCountIs(3),
      providerOptions: { openai: { store: true } },
    });
    for await (const part of result.fullStream) {
      if (part.type === "error") throw part.error;
      if (part.type === "abort") throw new Error("unexpected stream abort");
    }
    expect(await result.steps).toHaveLength(1);
    return result;
  };
  const history: ModelMessage[] = [{ role: "user", content: "shorten ai-sdk.dev" }];
  const first = await run(history);
  expect(requests).toHaveLength(1);
  expect(await first.toolResults).toEqual([]);
  const approvals = (await first.content).filter(part => part.type === "tool-approval-request");
  expect(approvals).toHaveLength(1);
  const [{ approvalId, toolCall }] = approvals;
  const firstCalls = await first.toolCalls;
  expect(firstCalls).toHaveLength(1);
  const [firstCall] = firstCalls;
  expect(firstCall.providerExecuted).toBe(true);
  expect(firstCall.toolCallId).toBeTruthy();
  expect(toolCall.toolCallId).toBe(firstCall.toolCallId);
  expect(approvalId).toBe(binding);

  const second = await run([...history, ...(await first.responseMessages), {
    role: "tool",
    content: [{ type: "tool-approval-response", approvalId, approved: true, providerExecuted: true }],
  }]);
  expect(requests).toHaveLength(2);
  expect((await second.content).filter(part => part.type === "tool-approval-request")).toEqual([]);
  const secondCalls = await second.toolCalls;
  const results = await second.toolResults;
  expect(secondCalls).toHaveLength(1);
  expect(results).toHaveLength(1);
  const [secondCall] = secondCalls;
  const [result] = results;
  for (const part of [secondCall, result]) {
    expect(part.toolCallId).toBe(itemId);
    expect(part.providerExecuted).toBe(true);
  }
  expect(result.preliminary).not.toBe(true);
  expect(result.providerMetadata?.openai?.itemId).toBe(itemId);
  expect(localExecutions).toBe(0);
  expect(await second.finishReason).toBe("stop");
  expect(await second.text).not.toBe("");

  const go = await fetchScenario("hosted-approval");
  expect(go.status, await go.clone().text()).toBe(200);
  expect(await go.json()).toStrictEqual({
    requests, localExecutions, approvalId,
    first: { input: firstCall.input, toolName: firstCall.toolName, text: await first.text, finish: await first.finishReason },
    second: {
      input: secondCall.input, toolName: secondCall.toolName, resultName: result.toolName, output: result.output,
      text: await second.text, finish: await second.finishReason, toolCallId: result.toolCallId, metadata: result.providerMetadata,
    },
  });
});
