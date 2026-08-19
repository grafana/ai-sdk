import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useChat, useCompletion, useObject } from "@ai-sdk/react";
import {
  DefaultChatTransport,
  lastAssistantMessageIsCompleteWithApprovalResponses,
  type ChatStatus,
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
  const { messages, sendMessage, status, error, stop } = useChat({
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/${scenario}`,
      fetch: trackedFetch,
    }),
  });
  const statusHistory = useSnapshotHistory<ChatStatus>(status);

  return (
    <div>
      <button
        data-testid="chat-send"
        onClick={() =>
          sendMessage({ role: "user", parts: [{ type: "text", text: "hi" }] })
        }
      />
      <button data-testid="chat-stop" onClick={stop} />
      <div data-testid="chat-status">{status}</div>
      <div data-testid="chat-status-history">{JSON.stringify(statusHistory)}</div>
      <div data-testid="chat-error">{error?.message}</div>
      <div data-testid="chat-abort-count">{abortCount}</div>
      <div data-testid="chat-message-count">{messages.length}</div>
      <div data-testid="chat-text">
        {assistantText(messages as AgentToolMessage[])}
      </div>
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
  const [finishCalls, setFinishCalls] = useState<ObjectFinishSnapshot[]>([]);
  const { object, submit } = useObject({
    api: `${getServerUrl()}/scenario/${scenario}`,
    schema: z.object({
      name: z.string(),
      age: z.number(),
      active: z.boolean(),
    }),
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

  return (
    <div>
      <button data-testid="object-send" onClick={() => submit("json")} />
      <div data-testid="object-text">{JSON.stringify(object)}</div>
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

  it("useChat stays submitted after a start ID until content arrives", async () => {
    render(<ChatProbe scenario="start-id-ui-stream" />);

    screen.getByTestId("chat-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-message-count").textContent).toBe("2");
      expect(screen.getByTestId("chat-status").textContent).toBe("submitted");
      expect(screen.getByTestId("chat-text").textContent).toBe("");
    });

    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toBe("Hello, world!");
      const history = JSON.parse(
        screen.getByTestId("chat-status-history").textContent ?? "[]",
      ) as ChatStatus[];
      expectOrderedSubsequence(history, ["submitted", "streaming", "ready"]);
    }, { timeout: 3000 });
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
});
