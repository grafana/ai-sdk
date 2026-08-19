import { createGateway } from "@ai-sdk/gateway";
import { describe, expect, it } from "vitest";
import {
  cleanStreamProjection,
  doneStreamProjection,
  positiveProjection,
  unaryProjection,
} from "./projections";
import { startRecorder } from "./recorder";

describe("ProviderWire V4 pinned stock-client response consumption", () => {
  it("consumes the curated unary projection", async () => {
    const recorder = await startRecorder([{ contentType: "application/json", body: unaryProjection() }]);
    try {
      const result = await model(recorder.baseURL).doGenerate(minimalOptions());
      expect(result.content).toEqual([{ type: "text", text: "projected unary" }]);
      expect(result.finishReason).toEqual({ unified: "stop", raw: "stop" });
    } finally {
      await recorder.close();
    }
  });

  it("consumes exact data framing through final event and clean EOF", async () => {
    const body = cleanStreamProjection();
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
    const hidden = await consumeStream(cleanStreamProjection(), false);
    const visible = await consumeStream(cleanStreamProjection(), true);
    expect(hidden.map((part) => part.type)).not.toContain("raw");
    expect(visible.map((part) => part.type)).toContain("raw");
  });

  it("tolerates a DONE frame derived from the clean stream seed", async () => {
    const clean = await consumeStream(cleanStreamProjection(), true);
    const done = await consumeStream(doneStreamProjection(), true);
    expect(done).toEqual(clean);
    expect(done.some((part) => JSON.stringify(part).includes("DONE"))).toBe(false);
  });

  it.each([
    { source: "error invalid request retry override", stockRetryable: false },
    { source: "error internal nonretry override", stockRetryable: true },
  ])("recognizes $source while deriving stock retryability from status", async ({ source, stockRetryable }) => {
    const projection = positiveProjection(source);
    expect(projection.status).toBeDefined();
    const wire = JSON.parse(projection.body) as {
      generationId?: string;
      error: { type: string; message: string; statusCode: number; isRetryable: boolean };
    };
    expect(wire.error.statusCode).toBe(projection.status);

    const recorder = await startRecorder([
      { status: projection.status, contentType: "application/json", body: projection.body },
    ]);
    try {
      let caught: unknown;
      try {
        await model(recorder.baseURL).doGenerate(minimalOptions());
      } catch (error) {
        caught = error;
      }
      expect(caught).toMatchObject({
        type: wire.error.type,
        statusCode: projection.status,
        isRetryable: stockRetryable,
        generationId: wire.generationId,
      });
      expect((caught as Error).message).toContain(wire.error.message);
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
