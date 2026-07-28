import { render, screen, waitFor } from "@testing-library/react";
import { useChat, useCompletion, useObject } from "@ai-sdk/react";
import { DefaultChatTransport } from "ai";
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
