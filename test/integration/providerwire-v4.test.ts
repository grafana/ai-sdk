import {
  createGateway,
  GatewayInvalidRequestError,
  GatewayModelNotFoundError,
} from "@ai-sdk/gateway";
import type { LanguageModelV4StreamPart } from "@ai-sdk/provider";
import { describe, expect, it } from "vitest";
import { getServerUrl } from "./helpers.js";

const POLL_INTERVAL_MS = 10;
const POLL_TIMEOUT_MS = 2_000;

function model(modelID: string) {
  return createGateway({
    apiKey: "integration-test-key",
    baseURL: `${getServerUrl()}/providerwire-v4`,
  })(modelID);
}

type RuntimeStats = {
  successCalls: number;
  streamCalls: number;
  blockingCalls: number;
  streamBlockingCalls: number;
  cancellations: number;
};

async function stats(): Promise<RuntimeStats> {
  const response = await fetch(`${getServerUrl()}/providerwire-v4/stats`);
  expect(response.ok).toBe(true);
  return await response.json() as RuntimeStats;
}

async function waitFor(load: () => Promise<boolean>): Promise<void> {
  const deadline = Date.now() + POLL_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (await load()) return;
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
  throw new Error(`condition not met within ${POLL_TIMEOUT_MS}ms`);
}

async function collect(stream: ReadableStream<LanguageModelV4StreamPart>): Promise<LanguageModelV4StreamPart[]> {
  const reader = stream.getReader();
  const parts: LanguageModelV4StreamPart[] = [];
  for (;;) {
    const result = await reader.read();
    if (result.done) return parts;
    parts.push(result.value);
  }
}

describe("ProviderWire V4 streaming runtime", () => {
  it("consumes normalized text, metadata, warnings, finish, and clean EOF", async () => {
    const result = await model("success").doStream({ prompt: [] });
    const parts = await collect(result.stream);

    expect(parts.map((part) => part.type)).toEqual([
      "stream-start",
      "response-metadata",
      "text-start",
      "text-delta",
      "text-delta",
      "text-end",
      "finish",
    ]);
    expect(parts[0]).toEqual({
      type: "stream-start",
      warnings: [{ type: "other", message: "the model reported a warning" }],
    });
    expect(parts.filter((part) => part.type === "text-delta").map((part) => part.delta)).toEqual([
      "",
      "hello from Go stream",
    ]);
    const metadata = parts.find((part) => part.type === "response-metadata");
    expect(metadata?.type).toBe("response-metadata");
    if (metadata?.type === "response-metadata") {
      expect(metadata.id).toBe("stream-response-1");
      expect(metadata.modelId).toBe("success");
      expect(metadata.timestamp).toBeInstanceOf(Date);
    }
  });

  it("preserves ordered provider errors and emits terminal timeout", async () => {
    const withErrors = await model("stream-errors").doStream({ prompt: [] });
    const errorParts = await collect(withErrors.stream);
    expect(errorParts.map((part) => part.type)).toEqual([
      "stream-start",
      "error",
      "text-start",
      "error",
      "text-delta",
      "text-end",
      "finish",
    ]);
    expect(errorParts
      .filter((part) => part.type === "error")
      .map((part) => (part.error as { code: string }).code))
      .toEqual(["overloaded", "failed_dependency"]);

    const timedOut = await model("stream-timeout").doStream({ prompt: [] });
    const timeoutParts = await collect(timedOut.stream);
    expect(timeoutParts.map((part) => part.type)).toEqual(["stream-start", "error"]);
    const timeout = timeoutParts[1];
    expect(timeout.type).toBe("error");
    if (timeout.type === "error") {
      expect((timeout.error as { code: string }).code).toBe("timeout");
    }
  });

  it("propagates abort after stream establishment", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const result = await model("stream-blocking").doStream({ prompt: [], abortSignal: controller.signal });
    await waitFor(async () => (await stats()).streamBlockingCalls > initial.streamBlockingCalls);

    controller.abort();
    await expect(collect(result.stream)).rejects.toBeDefined();
    await waitFor(async () => (await stats()).cancellations > initial.cancellations);
  });
});

describe("ProviderWire V4 unary runtime", () => {
  it("consumes the minimal production response through the pinned Gateway client", async () => {
    const result = await model("success").doGenerate({
      prompt: [{ role: "system", content: "hello" }],
    });

    expect(result.content).toEqual([{ type: "text", text: "hello from Go" }]);
    expect(result.finishReason).toEqual({ unified: "stop", raw: "test-stop" });
    expect(result.usage).toEqual({
      inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    });
    expect(result.warnings).toEqual([]);
    expect(result.response?.body).toEqual({
      content: [{ type: "text", text: "hello from Go" }],
      finishReason: { unified: "stop", raw: "test-stop" },
      usage: {
        inputTokens: { total: 2, noCache: 1, cacheRead: 1, cacheWrite: 0 },
        outputTokens: { total: 1, text: 1, reasoning: 0 },
      },
    });
    expect((await stats()).successCalls).toBeGreaterThan(0);
  });

  it("maps representative runtime failures to pinned client classes", async () => {
    await expect(model("success").doGenerate({
      prompt: [],
      headers: { "x-body-header": "unsupported" },
    })).rejects.toSatisfy((error: unknown) => GatewayInvalidRequestError.isInstance(error));

    await expect(model("missing").doGenerate({ prompt: [] }))
      .rejects.toSatisfy((error: unknown) => GatewayModelNotFoundError.isInstance(error));
  });

  it("propagates cancellation to the production handler", async () => {
    const initial = await stats();
    const controller = new AbortController();
    const pending = model("blocking").doGenerate({ prompt: [], abortSignal: controller.signal });
    await waitFor(async () => (await stats()).blockingCalls > initial.blockingCalls);

    controller.abort();
    await expect(pending).rejects.toBeDefined();
    await waitFor(async () => (await stats()).cancellations > initial.cancellations);
  });
});
