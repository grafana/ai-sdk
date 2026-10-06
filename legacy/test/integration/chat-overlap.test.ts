import {
  AbstractChat,
  type ChatInit,
  type ChatState,
  type ChatStatus,
  type UIMessage,
  type UIMessageChunk,
} from "ai";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

class TestChatState implements ChatState<UIMessage> {
  status: ChatStatus = "ready";
  messages: UIMessage[] = [];
  error: Error | undefined;

  pushMessage = (message: UIMessage) => {
    this.messages = this.messages.concat(message);
  };

  popMessage = () => {
    this.messages = this.messages.slice(0, -1);
  };

  replaceMessage = (index: number, message: UIMessage) => {
    this.messages = [
      ...this.messages.slice(0, index),
      message,
      ...this.messages.slice(index + 1),
    ];
  };

  snapshot = <T>(value: T): T => value;
}

class TestChat extends AbstractChat<UIMessage> {
  constructor(init: ChatInit<UIMessage>) {
    super({ ...init, state: new TestChatState() });
  }
}

function idGenerator() {
  let id = 0;
  return () => `id-${id++}`;
}

describe("Chat overlapping requests", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("finishes a resumed stream after a newer request clears the active response", async () => {
    let resumeController!: ReadableStreamDefaultController<UIMessageChunk>;
    const resumeStream = new ReadableStream<UIMessageChunk>({
      start(controller) {
        resumeController = controller;
        controller.enqueue({ type: "start" });
        controller.enqueue({ type: "start-step" });
        controller.enqueue({ type: "text-start", id: "text-1" });
        controller.enqueue({ type: "text-delta", id: "text-1", delta: "resumed" });
      },
    });
    const submitStream = new ReadableStream<UIMessageChunk>({
      start(controller) {
        controller.enqueue({ type: "start" });
        controller.enqueue({ type: "start-step" });
        controller.enqueue({ type: "text-start", id: "text-2" });
        controller.enqueue({ type: "text-delta", id: "text-2", delta: "submitted" });
        controller.enqueue({ type: "text-end", id: "text-2" });
        controller.enqueue({ type: "finish-step" });
        controller.enqueue({ type: "finish", finishReason: "stop" });
        controller.close();
      },
    });
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    const chat = new TestChat({
      id: "chat-1",
      generateId: idGenerator(),
      transport: {
        sendMessages: async () => submitStream,
        reconnectToStream: async () => resumeStream,
      },
      onFinish: () => {},
    });

    const resumePromise = chat.resumeStream();
    while (chat.messages.length === 0) {
      await vi.advanceTimersByTimeAsync(0);
    }

    await chat.sendMessage({ text: "new request" });
    resumeController.enqueue({ type: "text-end", id: "text-1" });
    resumeController.enqueue({ type: "finish-step" });
    resumeController.enqueue({ type: "finish", finishReason: "stop" });
    resumeController.close();
    await resumePromise;

    expect(consoleError).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });
});
