import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useChat } from "@ai-sdk/react";
import { createAgentUIStream, DefaultChatTransport, ToolLoopAgent, type UIMessage, type UIMessageChunk } from "ai";
import { afterEach, expect, it } from "vitest";
import { fetchScenario, getServerUrl } from "./helpers.js";
import { canonicalModel, canonicalUI, json, readToolStateStream, referenceToolStateModel, toolStateReferenceTools } from "./ui-tool-state-helpers.js";

afterEach(cleanup);

const seedChunks: UIMessageChunk[] = [
  { type: "start", messageId: "persisted" },
  { type: "tool-input-available", toolCallId: "pre", toolName: "lookup", input: { q: "partial" }, title: "", toolMetadata: {}, providerExecuted: true, providerMetadata: { test: { phase: "call" } } },
  { type: "tool-output-available", toolCallId: "pre", output: "partial", preliminary: true, providerExecuted: true, providerMetadata: { test: { phase: "partial" } } },
  { type: "tool-input-error", toolCallId: "legacy", toolName: "lookup", input: "legacy", errorText: "", providerMetadata: { test: { phase: "error" } } },
  { type: "tool-input-available", toolCallId: "approve", toolName: "lookup", input: { q: "approve" }, title: "approval", toolMetadata: { scope: "all" } },
  { type: "tool-approval-request", toolCallId: "approve", approvalId: "approval", approvalDescriptor: { scope: "all" }, reason: "", signature: "sig", isAutomatic: true },
  { type: "finish", finishReason: "tool-calls" },
];

function ResumeProbe({ initial = [], seed = false, observe }: { initial?: UIMessage[]; seed?: boolean; observe: (result: Awaited<ReturnType<typeof readToolStateStream>>) => void }) {
  const { messages, sendMessage, addToolApprovalResponse, status, error } = useChat({
    id: seed ? "seed" : "resumed",
    messages: initial,
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/ui-tool-state-agent`,
      fetch: async (input, init) => {
        const submitted = JSON.parse(String(init?.body)) as { messages: UIMessage[] };
        const response = seed
          ? await fetchScenario("ui-tool-state-chunks", { headers: { "content-type": "application/json" }, body: JSON.stringify({ chunks: seedChunks }) })
          : await fetch(input, init);
        const last = submitted.messages.at(-1);
        observe(await readToolStateStream(response.clone(), !seed && last?.role === "assistant" ? json(last) : undefined));
        return response;
      },
    }),
  });
  return <div>
    <button data-testid="send" onClick={() => void sendMessage({ text: "start" })} />
    <button data-testid="approve" onClick={async () => { await addToolApprovalResponse({ id: "approval", approved: true, reason: "" }); await sendMessage(); }} />
    <div data-testid="messages">{JSON.stringify(messages)}</div>
    <div data-testid="status">{status}</div>
    <div data-testid="error">{error?.message}</div>
  </div>;
}

function currentMessages(): UIMessage[] {
  return JSON.parse(screen.getByTestId("messages").textContent ?? "[]") as UIMessage[];
}

it("useChat persists, remounts and resumes preliminary/approval tool state through the Go Agent", async () => {
  const observed: Awaited<ReturnType<typeof readToolStateStream>>[] = [];
  const observe = (result: Awaited<ReturnType<typeof readToolStateStream>>) => observed.push(result);
  const mounted = render(<ResumeProbe seed observe={observe} />);
  screen.getByTestId("send").click();
  await waitFor(() => {
    expect(screen.getByTestId("error").textContent).toBe("");
    expect(currentMessages().at(-1)?.parts.some(part => "state" in part && part.state === "approval-requested")).toBe(true);
    expect(screen.getByTestId("status").textContent).toBe("ready");
  });
  expect(observed[0].chunks).toEqual(seedChunks);
  const original = currentMessages();
  const response = await fetchScenario("ui-tool-state-persistence", { headers: { "content-type": "application/json" }, body: JSON.stringify({ messages: original }) });
  expect(response.status).toBe(200);
  const persisted = (await response.json()).uiMessages as UIMessage[];
  expect(canonicalUI(persisted)).toEqual(canonicalUI(original));
  mounted.unmount();
  render(<ResumeProbe initial={persisted} observe={observe} />);
  expect(canonicalUI(currentMessages())).toEqual(canonicalUI(persisted));
  screen.getByTestId("approve").click();
  await waitFor(() => {
    expect(screen.getByTestId("error").textContent).toBe("");
    expect(currentMessages().at(-1)?.parts.some(part => part.type === "text" && part.text === "resumed")).toBe(true);
    expect(screen.getByTestId("status").textContent).toBe("ready");
  });
  const final = currentMessages().at(-1)!;
  expect(final.id).toBe("persisted");
  const toolParts = final.parts.filter(part => part.type.startsWith("tool-") || part.type === "dynamic-tool") as Array<Record<string, unknown>>;
  expect(toolParts.map(part => part.toolCallId)).toEqual(["pre", "legacy", "approve"]);
  expect(toolParts[0]).toMatchObject({ state: "output-available", output: "final", title: "", toolMetadata: {}, callProviderMetadata: { test: { phase: "call" } }, resultProviderMetadata: { test: { phase: "partial" } } });
  expect(toolParts[0].preliminary).not.toBe(true);
  expect(toolParts[1]).toMatchObject({ input: "legacy", errorText: "", resultProviderMetadata: { test: { phase: "error" } } });
  expect(toolParts[2]).toMatchObject({ state: "output-available", output: "approved", title: "approval", toolMetadata: { scope: "all" }, approval: { id: "approval", approved: true, descriptor: { scope: "all" }, requestReason: "", reason: "", signature: "sig", isAutomatic: true } });
  const resumed = json(persisted);
  const part = resumed.at(-1)!.parts.find(part => "toolCallId" in part && part.toolCallId === "approve") as Record<string, unknown>;
  part.state = "approval-responded";
  part.approval = { ...(part.approval as object), approved: true, reason: "" };
  const model = referenceToolStateModel(true);
  const stream = await createAgentUIStream({ agent: new ToolLoopAgent({ model, tools: toolStateReferenceTools }), uiMessages: resumed });
  for await (const _ of stream) {}
  const metadata = final.metadata as { modelMessages: unknown; providerCalls: number };
  expect(metadata.providerCalls).toBe(1);
  expect(canonicalModel(metadata.modelMessages)).toEqual(canonicalModel(model.doStreamCalls[0].prompt));
  expect(observed[1].chunks.some(chunk => chunk.type === "tool-output-available" && chunk.toolCallId === "pre" && chunk.output === "final")).toBe(true);
  expect(canonicalUI([observed[1].snapshots.at(-1)])).toEqual(canonicalUI([final]));
});
