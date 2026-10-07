import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { parseJsonEventStream } from "@ai-sdk/provider-utils";
import { useChat, useCompletion, useObject } from "@ai-sdk/react";
import {
  DefaultChatTransport,
  lastAssistantMessageIsCompleteWithApprovalResponses,
  readUIMessageStream,
  uiMessageChunkSchema,
  type ChatStatus,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { useCallback, useEffect, useState } from "react";
import { z } from "zod";
import { afterEach, describe, expect, it } from "vitest";
import { getServerUrl } from "./helpers.js";

afterEach(cleanup);

function useSnapshotHistory<T>(value: T): T[] {
  const [history, setHistory] = useState<T[]>([]);

  useEffect(() => {
    const snapshot = JSON.parse(JSON.stringify(value)) as T;
    setHistory(current => {
      if (JSON.stringify(current.at(-1)) === JSON.stringify(snapshot)) {
        return current;
      }
      return [...current, snapshot];
    });
  }, [value]);

  return history;
}

function assistantText(messages: AgentToolMessage[]): string {
  return messages
    .filter(message => message.role === "assistant")
    .flatMap(message => message.parts)
    .filter(part => part.type === "text")
    .map(part => part.text ?? "")
    .join("");
}

function readProbe<T>(testId: string): T {
  return JSON.parse(screen.getByTestId(testId).textContent ?? "null") as T;
}

async function readHookStream(response: Response): Promise<{ chunks: UIMessageChunk[]; messages: UIMessage[] }> {
  expect(response.ok).toBe(true);
  const parsed = parseJsonEventStream({ stream: response.body!, schema: uiMessageChunkSchema });
  const chunks: UIMessageChunk[] = [];
  const stream = parsed.pipeThrough(new TransformStream({
    transform(result, controller) {
      expect(result.success).toBe(true);
      if (result.success) {
        chunks.push(result.value);
        controller.enqueue(result.value);
      }
    },
  }));
  const messages: UIMessage[] = [];
  for await (const message of readUIMessageStream({ stream, terminateOnError: true })) {
    messages.push(message);
  }
  return { chunks, messages };
}

function expectOrderedSubsequence<T>(values: T[], expected: T[]): void {
  let previousIndex = -1;
  for (const value of expected) {
    const index = values.indexOf(value, previousIndex + 1);
    expect(index).toBeGreaterThan(previousIndex);
    previousIndex = index;
  }
}

function useAbortTrackingFetch(): {
  trackedFetch: typeof fetch;
  abortCount: number;
} {
  const [abortCount, setAbortCount] = useState(0);
  const trackedFetch = useCallback<typeof fetch>((input, init) => {
    init?.signal?.addEventListener(
      "abort",
      () => setAbortCount(current => current + 1),
      { once: true },
    );
    return fetch(input, init);
  }, []);

  return { trackedFetch, abortCount };
}

type AgentToolPart = {
  type: string;
  state?: string;
  toolCallId?: string;
  input?: unknown;
  output?: unknown;
  text?: string;
  url?: string;
  mediaType?: string;
  approval?: {
    id: string;
    approved?: boolean;
    reason?: string;
  };
};

type AgentToolMessage = {
  role: string;
  parts: AgentToolPart[];
};

function ChatProbe({ scenario }: { scenario: string }) {
  const { trackedFetch, abortCount } = useAbortTrackingFetch();
  const [finishCalls, setFinishCalls] = useState<unknown[]>([]);
  const [errorCalls, setErrorCalls] = useState<string[]>([]);
  const [dataCalls, setDataCalls] = useState<unknown[]>([]);
  const [resumeCount, setResumeCount] = useState(0);
  const { messages, sendMessage, regenerate, resumeStream, status, error, stop } = useChat({
    id: "chat-interop",
    messages: scenario.startsWith("reconnect-")
      ? [{ id: "user-seeded", role: "user", parts: [{ type: "text", text: "hi" }] }]
      : undefined,
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/${scenario}`,
      fetch: trackedFetch,
    }),
    onFinish: result => setFinishCalls(current => [...current, {
      messageId: result.message.id,
      isAbort: result.isAbort,
      isError: result.isError,
      finishReason: result.finishReason ?? null,
    }]),
    onError: callbackError => setErrorCalls(current => [...current, callbackError.message]),
    onData: data => setDataCalls(current => [...current, data]),
  });
  const statusHistory = useSnapshotHistory<ChatStatus>(status);
  const messageHistory = useSnapshotHistory(messages);
  const files = (messages as AgentToolMessage[])
    .flatMap(message => message.parts)
    .filter(part => part.type === "file");

  return (
    <div>
      <button
        data-testid="chat-send"
        onClick={() =>
          sendMessage({ role: "user", parts: [{ type: "text", text: "hi" }] })
        }
      />
      <button data-testid="chat-stop" onClick={stop} />
      <button data-testid="chat-regenerate" onClick={() => void regenerate()} />
      <button
        data-testid="chat-resume"
        onClick={() => void resumeStream().then(() => setResumeCount(count => count + 1))}
      />
      <div data-testid="chat-resume-count">{resumeCount}</div>
      <div data-testid="chat-messages">{JSON.stringify(messages)}</div>
      <div data-testid="chat-message-history">{JSON.stringify(messageHistory)}</div>
      <div data-testid="chat-finish-calls">{JSON.stringify(finishCalls)}</div>
      <div data-testid="chat-error-calls">{JSON.stringify(errorCalls)}</div>
      <div data-testid="chat-data-calls">{JSON.stringify(dataCalls)}</div>
      <div data-testid="chat-status">{status}</div>
      <div data-testid="chat-status-history">{JSON.stringify(statusHistory)}</div>
      <div data-testid="chat-error">{error?.message}</div>
      <div data-testid="chat-abort-count">{abortCount}</div>
      <div data-testid="chat-text">
        {assistantText(messages as AgentToolMessage[])}
      </div>
      <div data-testid="chat-files">{JSON.stringify(files)}</div>
    </div>
  );
}

function AgentToolProbe() {
  const { messages, sendMessage } = useChat({
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/agent-tool`,
    }),
  });
  const history = useSnapshotHistory(messages as AgentToolMessage[]);

  return (
    <div>
      <button
        data-testid="agent-tool-send"
        onClick={() =>
          sendMessage({
            role: "user",
            parts: [{ type: "text", text: "Weather in Paris?" }],
          })
        }
      />
      <div data-testid="agent-tool-state">{JSON.stringify(messages)}</div>
      <div data-testid="agent-tool-history">{JSON.stringify(history)}</div>
    </div>
  );
}

function ApprovalProbe() {
  const { messages, sendMessage, addToolApprovalResponse, status } = useChat({
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/tool-approval`,
    }),
    sendAutomaticallyWhen: lastAssistantMessageIsCompleteWithApprovalResponses,
  });
  const history = useSnapshotHistory(messages as AgentToolMessage[]);
  const pendingApproval = (messages as AgentToolMessage[])
    .flatMap(message => message.parts)
    .find(part => part.state === "approval-requested")?.approval;

  const respond = (approved: boolean, reason: string) => {
    if (pendingApproval != null) {
      void addToolApprovalResponse({
        id: pendingApproval.id,
        approved,
        reason,
      });
    }
  };

  return (
    <div>
      <button
        data-testid="approval-send"
        onClick={() =>
          sendMessage({
            role: "user",
            parts: [{ type: "text", text: "Deploy the change" }],
          })
        }
      />
      <button
        data-testid="approval-approve"
        onClick={() => respond(true, "approved by integration test")}
      />
      <button
        data-testid="approval-deny"
        onClick={() => respond(false, "denied by integration test")}
      />
      <div data-testid="approval-status">{status}</div>
      <div data-testid="approval-state">{JSON.stringify(messages)}</div>
      <div data-testid="approval-history">{JSON.stringify(history)}</div>
    </div>
  );
}

type CompletionFinishCall = {
  prompt: string;
  completion: string;
};

type CompletionErrorCall = {
  message: string;
  errorIsError: boolean;
};

function CompletionProbe({ scenario }: { scenario: string }) {
  const { trackedFetch, abortCount } = useAbortTrackingFetch();
  const [errorCalls, setErrorCalls] = useState<CompletionErrorCall[]>([]);
  const [finishCalls, setFinishCalls] = useState<CompletionFinishCall[]>([]);
  const { completion, complete, error, isLoading, stop } = useCompletion({
    api: `${getServerUrl()}/scenario/${scenario}`,
    streamProtocol: "text",
    fetch: trackedFetch,
    onError: callbackError => {
      setErrorCalls(current => [
        ...current,
        {
          message: callbackError.message,
          errorIsError: callbackError instanceof Error,
        },
      ]);
    },
    onFinish: (prompt, finalCompletion) => {
      setFinishCalls(current => [
        ...current,
        { prompt, completion: finalCompletion },
      ]);
    },
  });
  const loadingHistory = useSnapshotHistory(isLoading);

  return (
    <div>
      <button
        data-testid="completion-send"
        onClick={() => void complete("json")}
      />
      <button data-testid="completion-first" onClick={() => void complete("first")} />
      <button data-testid="completion-first-error" onClick={() => void complete("first-error")} />
      <button data-testid="completion-second" onClick={() => void complete("second")} />
      <button data-testid="completion-stop" onClick={stop} />
      <div data-testid="completion-text">{completion}</div>
      <div data-testid="completion-error">{error?.message}</div>
      <div data-testid="completion-abort-count">{abortCount}</div>
      <div data-testid="completion-loading">{JSON.stringify(isLoading)}</div>
      <div data-testid="completion-loading-history">
        {JSON.stringify(loadingHistory)}
      </div>
      <div data-testid="completion-error-calls">{JSON.stringify(errorCalls)}</div>
      <div data-testid="completion-finish-calls">
        {JSON.stringify(finishCalls)}
      </div>
    </div>
  );
}

type ObjectFinishSnapshot = {
  objectDefined: boolean;
  object: unknown;
  errorIsError: boolean;
};

function ObjectProbe({ scenario }: { scenario: string }) {
  const { trackedFetch, abortCount } = useAbortTrackingFetch();
  const [finishCalls, setFinishCalls] = useState<ObjectFinishSnapshot[]>([]);
  const [errorCalls, setErrorCalls] = useState<string[]>([]);
  const { object, submit, stop, error, isLoading } = useObject({
    api: `${getServerUrl()}/scenario/${scenario}`,
    fetch: trackedFetch,
    schema: z.object({
      name: z.string(),
      age: z.number(),
      active: z.boolean(),
    }),
    onError: callbackError => setErrorCalls(current => [...current, callbackError.message]),
    onFinish: result => {
      setFinishCalls(current => [
        ...current,
        {
          objectDefined: result.object !== undefined,
          object: result.object ?? null,
          errorIsError: result.error instanceof Error,
        },
      ]);
    },
  });

  const loadingHistory = useSnapshotHistory(isLoading);
  return (
    <div>
      <button data-testid="object-send" onClick={() => submit("json")} />
      <button data-testid="object-stop" onClick={stop} />
      <div data-testid="object-text">{JSON.stringify(object)}</div>
      <div data-testid="object-error">{error?.message}</div>
      <div data-testid="object-abort-count">{abortCount}</div>
      <div data-testid="object-loading">{JSON.stringify(isLoading)}</div>
      <div data-testid="object-loading-history">{JSON.stringify(loadingHistory)}</div>
      <div data-testid="object-error-calls">{JSON.stringify(errorCalls)}</div>
      <div data-testid="object-finish-calls">{JSON.stringify(finishCalls)}</div>
    </div>
  );
}

describe("React hook interop", () => {
  it("useChat consumes Go UI message SSE with ordered status changes", async () => {
    render(<ChatProbe scenario="controlled-ui-stream" />);

    screen.getByTestId("chat-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toBe("Hello, world!");
      const history = JSON.parse(
        screen.getByTestId("chat-status-history").textContent ?? "[]",
      ) as ChatStatus[];
      expectOrderedSubsequence(history, ["submitted", "streaming", "ready"]);
    });
  });

  it("useChat assembles a resolved generated file from Go SSE", async () => {
    render(<ChatProbe scenario="generated-file" />);
    screen.getByTestId("chat-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-status").textContent).toBe("ready");
      expect(JSON.parse(screen.getByTestId("chat-files").textContent ?? "[]")).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            type: "file",
            mediaType: "text/plain",
            url: "data:text/plain;base64,SGVsbG8=",
          }),
        ]),
      );
    });
  });

  it.each([
    {
      name: "HTTP error",
      scenario: "http-error",
      expectedError: "intentional server error",
    },
    {
      name: "UI stream error",
      scenario: "ui-stream-error",
      expectedError: "intentional stream error",
    },
  ])("useChat surfaces $name", async ({ scenario, expectedError }) => {
    render(<ChatProbe scenario={scenario} />);

    screen.getByTestId("chat-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-error").textContent).toBe(expectedError);
      expect(screen.getByTestId("chat-status").textContent).toBe("error");
    });
  });

  it("useChat stop retains partial output and returns to ready", async () => {
    render(<ChatProbe scenario="abortable-ui-stream" />);

    screen.getByTestId("chat-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toBe("Hello");
      expect(screen.getByTestId("chat-status").textContent).toBe("streaming");
    });
    screen.getByTestId("chat-stop").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-status").textContent).toBe("ready");
      expect(screen.getByTestId("chat-abort-count").textContent).toBe("1");
      expect(screen.getByTestId("chat-text").textContent).toBe("Hello");
    });
  });

  it("useChat receives step-boundary tool state and final text", async () => {
    render(<AgentToolProbe />);

    screen.getByTestId("agent-tool-send").click();

    await waitFor(() => {
      const historyState =
        screen.getByTestId("agent-tool-history").textContent ?? "[]";
      const history = JSON.parse(historyState) as AgentToolMessage[][];
      const firstStep = history
        .flatMap(snapshot => snapshot)
        .find(message => {
          const tool = message.parts.find(
            part =>
              part.type === "tool-get_weather" &&
              part.state === "output-available",
          );
          return tool != null && assistantText([message]) === "";
        });
      const firstStepTool = firstStep?.parts.find(
        part => part.type === "tool-get_weather",
      );
      expect({
        state: firstStepTool?.state,
        input: firstStepTool?.input,
        output: firstStepTool?.output,
      }).toEqual({
        state: "output-available",
        input: { city: "Paris" },
        output: { city: "Paris", celsius: 18, conditions: "partly cloudy" },
      });

      const state = screen.getByTestId("agent-tool-state").textContent ?? "[]";
      const messages = JSON.parse(state) as AgentToolMessage[];
      const assistant = messages.find(message => message.role === "assistant");
      const tool = assistant?.parts.find(
        part => part.type === "tool-get_weather",
      );
      expect(tool?.state).toBe("output-available");
      expect(assistantText(messages)).toBe(
        "Paris is 18°C and partly cloudy.",
      );
    });
  });

  it.each([
    {
      name: "approved",
      button: "approval-approve",
      approved: true,
      reason: "approved by integration test",
      finalState: "output-available",
      finalText:
        "The approved action was executed. Reason: approved by integration test",
      output: { action: "deploy", executed: true },
    },
    {
      name: "denied",
      button: "approval-deny",
      approved: false,
      reason: "denied by integration test",
      finalState: "output-denied",
      finalText: "The action was denied. Reason: denied by integration test",
      output: undefined,
    },
  ])(
    "useChat resumes an $name tool approval response",
    async ({ button, approved, reason, finalState, finalText, output }) => {
      render(<ApprovalProbe />);

      screen.getByTestId("approval-send").click();
      await waitFor(() => {
        const state = JSON.parse(
          screen.getByTestId("approval-state").textContent ?? "[]",
        ) as AgentToolMessage[];
        const tool = state
          .flatMap(message => message.parts)
          .find(part => part.type === "tool-confirm_action");
        expect(tool?.state).toBe("approval-requested");
      });

      screen.getByTestId(button).click();

      await waitFor(() => {
        const history = JSON.parse(
          screen.getByTestId("approval-history").textContent ?? "[]",
        ) as AgentToolMessage[][];
        const toolHistory = history
          .flatMap(snapshot => snapshot)
          .flatMap(message => message.parts)
          .filter(part => part.type === "tool-confirm_action");
        const toolCallId = toolHistory.at(0)?.toolCallId;
        const stateHistory = toolHistory
          .filter(part => part.toolCallId === toolCallId)
          .map(part => part.state)
          .filter((state): state is string => state != null);
        expectOrderedSubsequence(stateHistory, [
          "approval-requested",
          "approval-responded",
        ]);
        expect(
          toolHistory.some(
            part =>
              part.state === "approval-responded" &&
              part.approval?.approved === approved &&
              part.approval.reason === reason,
          ),
        ).toBe(true);

        const state = JSON.parse(
          screen.getByTestId("approval-state").textContent ?? "[]",
        ) as AgentToolMessage[];
        const finalTool = state
          .flatMap(message => message.parts)
          .find(part => part.type === "tool-confirm_action");
        expect({ state: finalTool?.state, output: finalTool?.output }).toEqual({
          state: finalState,
          output,
        });
        expect(assistantText(state)).toBe(finalText);
        expect(screen.getByTestId("approval-status").textContent).toBe("ready");
      });
    },
  );

  it("useCompletion consumes Go text stream", async () => {
    render(<CompletionProbe scenario="text-stream" />);

    screen.getByTestId("completion-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("completion-text").textContent).toContain(
        '"Alice"',
      );
    });
  });

  it("useCompletion reports an HTTP error and resets loading", async () => {
    render(<CompletionProbe scenario="http-error" />);

    screen.getByTestId("completion-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("completion-error").textContent).toBe(
        "intentional server error",
      );
      expect(
        JSON.parse(
          screen.getByTestId("completion-error-calls").textContent ?? "[]",
        ),
      ).toEqual([
        {
          message: "intentional server error",
          errorIsError: true,
        },
      ]);
      expect(
        JSON.parse(
          screen.getByTestId("completion-finish-calls").textContent ?? "[]",
        ),
      ).toEqual([]);
      expect(screen.getByTestId("completion-loading").textContent).toBe("false");
      const loadingHistory = JSON.parse(
        screen.getByTestId("completion-loading-history").textContent ?? "[]",
      ) as boolean[];
      expectOrderedSubsequence(loadingHistory, [true, false]);
    });
  });

  it("useCompletion stop retains partial output and clears loading", async () => {
    render(<CompletionProbe scenario="abortable-text-stream" />);

    screen.getByTestId("completion-send").click();
    await waitFor(() => {
      expect(screen.getByTestId("completion-text").textContent).toBe("Hello");
      expect(screen.getByTestId("completion-loading").textContent).toBe("true");
    });

    screen.getByTestId("completion-stop").click();

    await waitFor(() => {
      expect(screen.getByTestId("completion-loading").textContent).toBe("false");
      expect(screen.getByTestId("completion-abort-count").textContent).toBe("1");
      expect(screen.getByTestId("completion-text").textContent).toBe("Hello");
      expect(
        JSON.parse(
          screen.getByTestId("completion-error-calls").textContent ?? "[]",
        ),
      ).toEqual([]);
      expect(
        JSON.parse(
          screen.getByTestId("completion-finish-calls").textContent ?? "[]",
        ),
      ).toEqual([]);
    });
  });

  it("useObject consumes Go streamed JSON", async () => {
    render(<ObjectProbe scenario="text-stream" />);

    screen.getByTestId("object-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("object-text").textContent).toContain(
        '"active":true',
      );
    });
  });

  it("useObject reports final schema mismatch through onFinish", async () => {
    render(<ObjectProbe scenario="invalid-object" />);

    screen.getByTestId("object-send").click();

    await waitFor(() => {
      expect(
        JSON.parse(
          screen.getByTestId("object-finish-calls").textContent ?? "[]",
        ),
      ).toEqual([
        {
          objectDefined: false,
          object: null,
          errorIsError: true,
        },
      ]);
    });
  });

  it("useChat regenerates with the same user ID and a replacement assistant", async () => {
    render(<ChatProbe scenario="chat-regenerate" />);
    screen.getByTestId("chat-send").click();
    await waitFor(() => {
      expect(screen.getByTestId("chat-status").textContent).toBe("ready");
      expect(screen.getByTestId("chat-text").textContent).toBe("First response");
    });
    const initial = readProbe<UIMessage[]>("chat-messages");
    expect(initial.map(message => message.role)).toEqual(["user", "assistant"]);
    expect(initial[1].id).toBe("assistant-first");

    screen.getByTestId("chat-regenerate").click();
    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toBe("Second response");
      expect(screen.getByTestId("chat-status").textContent).toBe("ready");
    });
    const replacement = readProbe<UIMessage[]>("chat-messages");
    expect(replacement.map(message => message.id)).toEqual([initial[0].id, "assistant-second"]);
    expect(readProbe<UIMessage[][]>("chat-message-history")).toEqual(
      expect.arrayContaining([initial]),
    );
    expect(readProbe<unknown[]>("chat-finish-calls")).toEqual([
      { messageId: "assistant-first", isAbort: false, isError: false, finishReason: "stop" },
      { messageId: "assistant-second", isAbort: false, isError: false, finishReason: "stop" },
    ]);
  });

  it.each([
    { scenario: "reconnect-stream", status: "ready", text: "Reconnected response" },
    { scenario: "reconnect-empty", status: "ready", text: "" },
    { scenario: "reconnect-error", status: "error", text: "" },
  ])("useChat reconnects through test transport: $scenario", async ({ scenario, status, text }) => {
    render(<ChatProbe scenario={scenario} />);
    screen.getByTestId("chat-resume").click();
    await waitFor(() => {
      expect(screen.getByTestId("chat-resume-count").textContent).toBe("1");
      expect(screen.getByTestId("chat-status").textContent).toBe(status);
      expect(screen.getByTestId("chat-text").textContent).toBe(text);
      if (scenario === "reconnect-stream") {
        expect(readProbe<UIMessage[]>("chat-messages")).toHaveLength(2);
        expectOrderedSubsequence(readProbe<ChatStatus[]>("chat-status-history"), ["submitted", "streaming", "ready"]);
      } else if (scenario === "reconnect-error") {
        expect(readProbe<string[]>("chat-error-calls")).toEqual(["intentional server error"]);
      }
    });
    const messages = readProbe<UIMessage[]>("chat-messages");
    expect(messages[0].id).toBe("user-seeded");
    if (scenario === "reconnect-stream") {
      expect(messages[1].id).toBe("assistant-reconnected");
      expect(readProbe<unknown[]>("chat-finish-calls")).toEqual([
        { messageId: "assistant-reconnected", isAbort: false, isError: false, finishReason: "stop" },
      ]);
    } else {
      expect(messages).toHaveLength(1);
      expect(readProbe<ChatStatus[]>("chat-status-history")).not.toContain("submitted");
      expect(readProbe<unknown[]>("chat-finish-calls")).toEqual([]);
    }
  });

  it("useChat delivers updated metadata and transient data without retaining it", async () => {
    render(<ChatProbe scenario="chat-data" />);
    screen.getByTestId("chat-send").click();
    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toBe("Data");
      expect(readProbe<UIMessage[]>("chat-messages")[1].metadata).toEqual({ phase: "initial" });
    });
    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toBe("Data received");
      expect(screen.getByTestId("chat-status").textContent).toBe("ready");
    });
    const message = readProbe<UIMessage[]>("chat-messages")[1];
    expect(message.id).toBe("assistant-data");
    expect(message.metadata).toEqual({ phase: "updated", source: "go" });
    const snapshots = readProbe<UIMessage[][]>("chat-message-history")
      .map(messages => messages[1])
      .filter(Boolean);
    expect(snapshots.map(snapshot => snapshot.metadata)).toContainEqual({ phase: "initial" });
    expect(snapshots.map(snapshot => snapshot.metadata)).toContainEqual({ phase: "updated", source: "go" });
    expect(readProbe<unknown[]>("chat-finish-calls")).toEqual([
      { messageId: "assistant-data", isAbort: false, isError: false, finishReason: "stop" },
    ]);
    expect(message.parts.filter(part => part.type.startsWith("data-"))).toEqual([
      { type: "data-weather", data: { temp: 70 } },
    ]);
    expect(readProbe<unknown[]>("chat-data-calls")).toEqual([
      { type: "data-weather", data: { temp: 70 } },
      { type: "data-notice", data: { status: "sent" }, transient: true },
    ]);

    const response = await fetch(`${getServerUrl()}/scenario/chat-data`, { method: "POST" });
    const { chunks, messages: assembled } = await readHookStream(response);
    expect(chunks.map(chunk => chunk.type)).toContain("message-metadata");
    expect(chunks.map(chunk => chunk.type)).toContain("data-notice");
    expect(assembled.at(-1)).toMatchObject({ id: "assistant-data", metadata: { phase: "updated", source: "go" } });
    expect(assembled.at(-1)?.parts.filter(part => part.type.startsWith("data-"))).toEqual([
      { type: "data-weather", data: { temp: 70 } },
    ]);
  });

  it("parses and assembles regenerate and reconnect UI SSE through the pinned schema", async () => {
    for (const [trigger, id, text] of [
      ["submit-message", "assistant-first", "First response"],
      ["regenerate-message", "assistant-second", "Second response"],
    ]) {
      const response = await fetch(`${getServerUrl()}/scenario/chat-regenerate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ trigger, messages: [{ role: "user", parts: [{ type: "text", text: "hi" }] }] }),
      });
      const { chunks, messages } = await readHookStream(response);
      expect(chunks[0]).toMatchObject({ type: "start", messageId: id });
      expect(messages.at(-1)).toMatchObject({
        id,
        parts: expect.arrayContaining([expect.objectContaining({ type: "text", text })]),
      });
    }
    const response = await fetch(`${getServerUrl()}/scenario/reconnect-stream/chat-interop/stream`);
    const { chunks, messages } = await readHookStream(response);
    expect(chunks[0]).toMatchObject({ type: "start", messageId: "assistant-reconnected" });
    expect(messages.at(-1)).toMatchObject({
      id: "assistant-reconnected",
      parts: expect.arrayContaining([
        expect.objectContaining({ type: "text", text: "Reconnected response" }),
      ]),
    });
  });

  it("useChat distinguishes stopped and failed response callbacks", async () => {
    render(<ChatProbe scenario="abortable-ui-stream" />);
    screen.getByTestId("chat-send").click();
    await waitFor(() => expect(screen.getByTestId("chat-text").textContent).toBe("Hello"));
    screen.getByTestId("chat-stop").click();
    await waitFor(() => {
      expect(readProbe<unknown[]>("chat-finish-calls")).toEqual([
        expect.objectContaining({ isAbort: true, isError: false }),
      ]);
      expect(screen.getByTestId("chat-status").textContent).toBe("ready");
    });
    expect(readProbe<string[]>("chat-error-calls")).toEqual([]);
    expect(screen.getByTestId("chat-text").textContent).toBe("Hello");
  });

  it.each(["http-error", "ui-stream-error"])("useChat reports callback failure for %s", async scenario => {
    render(<ChatProbe scenario={scenario} />);
    screen.getByTestId("chat-send").click();
    await waitFor(() => expect(screen.getByTestId("chat-status").textContent).toBe("error"));
    expect(readProbe<string[]>("chat-error-calls")).toEqual([
      scenario === "http-error" ? "intentional server error" : "intentional stream error",
    ]);
    expect(readProbe<unknown[]>("chat-finish-calls")).toEqual([
      expect.objectContaining({ isAbort: false, isError: true, finishReason: null }),
    ]);
  });

  it.each(["first", "first-error"])("useCompletion keeps newer request state when %s settles", async prompt => {
    render(<CompletionProbe scenario="completion-overlap" />);
    screen.getByTestId(prompt === "first" ? "completion-first" : "completion-first-error").click();
    if (prompt === "first") {
      await waitFor(() => expect(screen.getByTestId("completion-text").textContent).toBe("first partial"));
    } else {
      await waitFor(() => expect(screen.getByTestId("completion-loading").textContent).toBe("true"));
    }
    expect(readProbe<CompletionFinishCall[]>("completion-finish-calls")).toEqual([]);
    expect(readProbe<CompletionErrorCall[]>("completion-error-calls")).toEqual([]);
    screen.getByTestId("completion-second").click();
    await waitFor(() => expect(screen.getByTestId("completion-text").textContent).toBe("second partial"));
    expect(readProbe<CompletionFinishCall[]>("completion-finish-calls")).toEqual([]);
    expect(readProbe<CompletionErrorCall[]>("completion-error-calls")).toEqual([]);
    await waitFor(() => {
      if (prompt === "first") {
        expect(readProbe<CompletionFinishCall[]>("completion-finish-calls")).toContainEqual({
          prompt: "first",
          completion: "first partial finished",
        });
      } else {
        expect(readProbe<CompletionErrorCall[]>("completion-error-calls")).toContainEqual({
          message: "intentional server error",
          errorIsError: true,
        });
      }
    }, { timeout: 2500 });
    expect(screen.getByTestId("completion-text").textContent).toBe("second partial");
    expect(screen.getByTestId("completion-error").textContent).toBe("");
    expect(screen.getByTestId("completion-loading").textContent).toBe("true");
    await waitFor(() => {
      expect(screen.getByTestId("completion-text").textContent).toBe("second partial finished");
      expect(screen.getByTestId("completion-loading").textContent).toBe("false");
    }, { timeout: 2500 });
    expect(readProbe<CompletionFinishCall[]>("completion-finish-calls")).toEqual(
      prompt === "first"
        ? [
            { prompt: "first", completion: "first partial finished" },
            { prompt: "second", completion: "second partial finished" },
          ]
        : [{ prompt: "second", completion: "second partial finished" }],
    );
    expect(readProbe<CompletionErrorCall[]>("completion-error-calls")).toEqual(
      prompt === "first-error"
        ? [{ message: "intentional server error", errorIsError: true }]
        : [],
    );
  });

  it.each([
    { scenario: "http-error", error: "intentional server error" },
    { scenario: "object-body-error", error: undefined },
  ])("useObject reports $scenario through onError without onFinish", async ({ scenario, error }) => {
    render(<ObjectProbe scenario={scenario} />);
    screen.getByTestId("object-send").click();
    if (scenario === "object-body-error") {
      await waitFor(() => expect(readProbe<unknown>("object-text")).toMatchObject({ name: "Alice" }));
    }
    await waitFor(() => {
      expect(screen.getByTestId("object-loading").textContent).toBe("false");
      expect(readProbe<string[]>("object-error-calls")).toHaveLength(1);
    });
    const message = readProbe<string[]>("object-error-calls")[0];
    if (error) expect(message).toBe(error);
    else expect(message).toMatch(/terminated|fetch|network/i);
    expect(screen.getByTestId("object-error").textContent).toBe(message);
    await waitFor(() => expectOrderedSubsequence(
      readProbe<boolean[]>("object-loading-history"), [true, false],
    ));
    expect(readProbe<unknown[]>("object-finish-calls")).toEqual([]);
    if (scenario === "object-body-error") {
      expect(readProbe<unknown>("object-text")).toMatchObject({ name: "Alice" });
    }
  });

  it("useObject stop retains partial JSON without callbacks", async () => {
    render(<ObjectProbe scenario="object-abortable" />);
    screen.getByTestId("object-send").click();
    await waitFor(() => expect(readProbe<unknown>("object-text")).toMatchObject({ name: "Alice" }));
    screen.getByTestId("object-stop").click();
    await waitFor(() => {
      expect(screen.getByTestId("object-loading").textContent).toBe("false");
      expect(screen.getByTestId("object-abort-count").textContent).toBe("1");
    });
    expect(readProbe<unknown>("object-text")).toMatchObject({ name: "Alice" });
    expect(readProbe<unknown[]>("object-error-calls")).toEqual([]);
    expect(readProbe<unknown[]>("object-finish-calls")).toEqual([]);
    await waitFor(() => expectOrderedSubsequence(
      readProbe<boolean[]>("object-loading-history"), [true, false],
    ));
  });

  it("useObject completes valid JSON with one successful onFinish", async () => {
    render(<ObjectProbe scenario="text-stream" />);
    screen.getByTestId("object-send").click();
    await waitFor(() => expect(readProbe<ObjectFinishSnapshot[]>("object-finish-calls")).toEqual([
      { objectDefined: true, object: { name: "Alice", age: 30, active: true }, errorIsError: false },
    ]));
    expect(readProbe<string[]>("object-error-calls")).toEqual([]);
    expect(screen.getByTestId("object-loading").textContent).toBe("false");
    await waitFor(() => expectOrderedSubsequence(
      readProbe<boolean[]>("object-loading-history"), [true, false],
    ));
  });
});
