import { render, screen, waitFor } from "@testing-library/react";
import { useChat, useCompletion, useObject } from "@ai-sdk/react";
import { DefaultChatTransport } from "ai";
import { useEffect, useState } from "react";
import { z } from "zod";
import { describe, expect, it } from "vitest";
import { getServerUrl } from "./helpers.js";

function ChatProbe() {
  const { messages, sendMessage, status } = useChat({
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/simple-text`,
    }),
  });

  return (
    <div>
      <button
        data-testid="chat-send"
        onClick={() =>
          sendMessage({ role: "user", parts: [{ type: "text", text: "hi" }] })
        }
      />
      <div data-testid="chat-status">{status}</div>
      <div data-testid="chat-text">
        {messages
          .flatMap((message) => message.parts)
          .map((part) => (part.type === "text" ? part.text : ""))
          .join("")}
      </div>
    </div>
  );
}

type AgentToolMessage = {
  role: string;
  parts: Array<{
    type: string;
    state?: string;
    input?: unknown;
    output?: unknown;
    text?: string;
  }>;
};

function AgentToolProbe() {
  const { messages, sendMessage } = useChat({
    transport: new DefaultChatTransport({
      api: `${getServerUrl()}/scenario/agent-tool`,
    }),
  });
  const [history, setHistory] = useState<AgentToolMessage[][]>([]);

  useEffect(() => {
    const snapshot = JSON.parse(JSON.stringify(messages)) as AgentToolMessage[];
    setHistory(current => {
      if (JSON.stringify(current.at(-1)) === JSON.stringify(snapshot)) {
        return current;
      }
      return [...current, snapshot];
    });
  }, [messages]);

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

function CompletionProbe() {
  const { completion, complete } = useCompletion({
    api: `${getServerUrl()}/scenario/text-stream`,
    streamProtocol: "text",
  });

  return (
    <div>
      <button data-testid="completion-send" onClick={() => void complete("json")} />
      <div data-testid="completion-text">{completion}</div>
    </div>
  );
}

function ObjectProbe() {
  const { object, submit } = useObject({
    api: `${getServerUrl()}/scenario/text-stream`,
    schema: z.object({
      name: z.string(),
      age: z.number(),
      active: z.boolean(),
    }),
  });

  return (
    <div>
      <button data-testid="object-send" onClick={() => submit("json")} />
      <div data-testid="object-text">{JSON.stringify(object)}</div>
    </div>
  );
}

describe("React hook interop", () => {
  it("useChat consumes Go UI message SSE", async () => {
    render(<ChatProbe />);

    screen.getByTestId("chat-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("chat-text").textContent).toContain("Hello, world!");
    });
  });

  it("useChat receives agent tool state and final text", async () => {
    render(<AgentToolProbe />);

    screen.getByTestId("agent-tool-send").click();

    await waitFor(() => {
      const historyState =
        screen.getByTestId("agent-tool-history").textContent ?? "[]";
      const history = JSON.parse(historyState) as AgentToolMessage[][];
      const inputAvailable = history
        .flatMap(snapshot => snapshot)
        .flatMap(message => message.parts)
        .find(
          part =>
            part.type === "tool-get_weather" && part.state === "input-available",
        );
      expect({
        state: inputAvailable?.state,
        input: inputAvailable?.input,
      }).toEqual({
        state: "input-available",
        input: { city: "Paris" },
      });

      const state = screen.getByTestId("agent-tool-state").textContent ?? "[]";
      const messages = JSON.parse(state) as AgentToolMessage[];
      const assistant = messages.find(message => message.role === "assistant");
      const tool = assistant?.parts.find(part => part.type === "tool-get_weather");

      expect({
        state: tool?.state,
        input: tool?.input,
        output: tool?.output,
      }).toEqual({
        state: "output-available",
        input: { city: "Paris" },
        output: { city: "Paris", celsius: 18, conditions: "partly cloudy" },
      });
      expect(
        assistant?.parts
          .filter(part => part.type === "text")
          .map(part => part.text)
          .join(""),
      ).toBe("Paris is 18°C and partly cloudy.");
    });
  });

  it("useCompletion consumes Go text stream", async () => {
    render(<CompletionProbe />);

    screen.getByTestId("completion-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("completion-text").textContent).toContain("\"Alice\"");
    });
  });

  it("useObject consumes Go streamed JSON", async () => {
    render(<ObjectProbe />);

    screen.getByTestId("object-send").click();

    await waitFor(() => {
      expect(screen.getByTestId("object-text").textContent).toContain("\"active\":true");
    });
  });
});
