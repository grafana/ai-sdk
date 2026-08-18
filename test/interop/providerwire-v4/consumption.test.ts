import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createGateway } from "@ai-sdk/gateway";
import { describe, expect, it } from "vitest";
import { startRecorder } from "./recorder";

const projection = (name: string) => readFileSync(resolve(import.meta.dirname, "projections", name), "utf8");

describe("ProviderWire V4 pinned stock-client response consumption", () => {
  it("consumes the curated unary projection", async () => {
    const recorder = await startRecorder([{ contentType: "application/json", body: projection("unary.json") }]);
    try {
      const result = await model(recorder.baseURL).doGenerate(minimalOptions());
      expect(result.content).toEqual([{ type: "text", text: "projected unary" }]);
      expect(result.finishReason).toEqual({ unified: "stop", raw: "stop" });
    } finally {
      await recorder.close();
    }
  });

  it("consumes exact data framing through final event and clean EOF", async () => {
    const body = projection("stream-clean.sse");
    expect(body).toContain("data: {\"type\":\"stream-start\"");
    expect(body).not.toContain("event:");
    expect(body).not.toContain("[DONE]");

    const parts = await consumeStream(body, true);
    expect(parts.map((part) => part.type)).toEqual([
      "stream-start",
      "response-metadata",
      "text-start",
      "raw",
      "text-delta",
      "text-end",
      "finish",
    ]);
    expect(parts.at(-1)?.type).toBe("finish");
    const metadata = parts.find((part) => part.type === "response-metadata");
    expect(metadata).toBeDefined();
    expect(metadata?.timestamp).toBeInstanceOf(Date);
    expect((metadata?.timestamp as Date).toISOString()).toBe("2026-01-02T03:04:05.000Z");
  });

  it("filters raw parts when includeRawChunks is false", async () => {
    const hidden = await consumeStream(projection("stream-clean.sse"), false);
    const visible = await consumeStream(projection("stream-clean.sse"), true);
    expect(hidden.map((part) => part.type)).not.toContain("raw");
    expect(visible.map((part) => part.type)).toContain("raw");
  });

  it("tolerates DONE without exposing it as a stream part", async () => {
    const parts = await consumeStream(projection("stream-done.sse"), true);
    expect(parts.map((part) => part.type)).toEqual([
      "stream-start",
      "text-start",
      "text-delta",
      "text-end",
      "finish",
    ]);
    expect(parts.some((part) => JSON.stringify(part).includes("DONE"))).toBe(false);
  });

  it.each([
    {
      name: "wire retryable 400",
      status: 400,
      fixture: "error-retryable-400.json",
      type: "invalid_request_error",
      message: "retryable invalid request",
      wireRetryable: true,
      stockRetryable: false,
      generationId: undefined,
    },
    {
      name: "wire non-retryable 500",
      status: 500,
      fixture: "error-nonretryable-500.json",
      type: "internal_server_error",
      message: "non-retryable internal failure",
      wireRetryable: false,
      stockRetryable: true,
      generationId: "generation-safe",
    },
  ])("recognizes $name while deriving stock retryability from status", async (testCase) => {
    const body = projection(testCase.fixture);
    const wire = JSON.parse(body) as { error: { isRetryable: boolean } };
    expect(wire.error.isRetryable).toBe(testCase.wireRetryable);
    const recorder = await startRecorder([
      { status: testCase.status, contentType: "application/json", body },
    ]);
    try {
      let caught: unknown;
      try {
        await model(recorder.baseURL).doGenerate(minimalOptions());
      } catch (error) {
        caught = error;
      }
      expect(caught).toMatchObject({
        type: testCase.type,
        statusCode: testCase.status,
        isRetryable: testCase.stockRetryable,
        generationId: testCase.generationId,
      });
      expect((caught as Error).message).toContain(testCase.message);
    } finally {
      await recorder.close();
    }
  });
});

async function consumeStream(body: string, includeRawChunks: boolean): Promise<Array<{ type: string; [key: string]: unknown }>> {
  const recorder = await startRecorder([{ contentType: "text/event-stream", body }]);
  try {
    const result = await model(recorder.baseURL).doStream({ ...minimalOptions(), includeRawChunks });
    const parts: Array<{ type: string; [key: string]: unknown }> = [];
    for await (const part of result.stream) {
      parts.push(part as { type: string; [key: string]: unknown });
    }
    return parts;
  } finally {
    await recorder.close();
  }
}

function model(baseURL: string) {
  return createGateway({ apiKey: "projection-not-a-real-key", baseURL })("capture/model");
}

function minimalOptions() {
  return { prompt: [{ role: "user" as const, content: [{ type: "text" as const, text: "consume" }] }] };
}
