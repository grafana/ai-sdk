import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  createGateway,
  GatewayInvalidRequestError,
  type GatewayProviderSettings,
} from "@ai-sdk/gateway";
import type { LanguageModelV4StreamPart } from "@ai-sdk/provider";

function modelWithFetch(fetch: NonNullable<GatewayProviderSettings["fetch"]>) {
  return createGateway({
    apiKey: "contract-test-key",
    baseURL: "https://contract.invalid",
    fetch,
  })("grafana/client-probe");
}

function sseResponse(parts: Array<unknown | "[DONE]">): Response {
  const body = `${parts
    .map((part) => `data: ${part === "[DONE]" ? part : JSON.stringify(part)}\n\n`)
    .join("")}`;
  return new Response(body, {
    status: 200,
    headers: { "content-type": "text/event-stream", "x-server": "stream" },
  });
}

async function collect(stream: ReadableStream<LanguageModelV4StreamPart>) {
  const reader = stream.getReader();
  const parts: LanguageModelV4StreamPart[] = [];
  for (;;) {
    const result = await reader.read();
    if (result.done) {
      return parts;
    }
    parts.push(result.value);
  }
}

function invalidRequestResponse(): Response {
  return Response.json(
    {
      error: {
        message: "request rejected",
        type: "invalid_request_error",
        param: null,
        code: "invalid_request",
      },
      generationId: "generation-1",
    },
    { status: 400 },
  );
}

function assertInvalidRequest(error: unknown): boolean {
  assert.equal(GatewayInvalidRequestError.isInstance(error), true);
  if (!GatewayInvalidRequestError.isInstance(error)) {
    return false;
  }
  assert.equal(error.type, "invalid_request_error");
  assert.equal(error.statusCode, 400);
  assert.equal(error.message, "request rejected [generation-1]");
  return true;
}

describe("registered Gateway client consumption", () => {
  it("consumes unary output while replacing client-owned fields", async () => {
    const serverBody = {
      content: [{ type: "text", text: "hello" }],
      finishReason: { unified: "stop", raw: "end_turn" },
      usage: {
        inputTokens: { total: 2, noCache: 2, cacheRead: 0, cacheWrite: 0 },
        outputTokens: { total: 1, text: 1, reasoning: 0 },
      },
      warnings: [{ type: "other", message: "server warning" }],
      request: { body: "server request" },
      response: {
        id: "server-id",
        modelId: "server-model",
        timestamp: "2026-08-22T00:00:00.000Z",
        headers: { "x-private": "server" },
        body: "server response",
      },
    };
    const model = modelWithFetch(async () =>
      Response.json(serverBody, { headers: { "x-server": "unary" } }),
    );

    const result = await model.doGenerate({ prompt: [] });

    assert.deepEqual(result.content, serverBody.content);
    assert.deepEqual(result.finishReason, serverBody.finishReason);
    assert.deepEqual(result.usage, serverBody.usage);
    assert.deepEqual(result.warnings, []);
    assert.deepEqual(result.request, { body: { prompt: [] } });
    assert.equal(result.response?.headers?.["x-server"], "unary");
    assert.deepEqual(result.response?.body, serverBody);
    assert.equal(result.response?.id, undefined);
    assert.equal(result.response?.modelId, undefined);
    assert.equal(result.response?.timestamp, undefined);
  });

  it("consumes finish followed by clean EOF", async () => {
    const model = modelWithFetch(async () =>
      sseResponse([
        { type: "stream-start", warnings: [] },
        { type: "text-start", id: "text-1" },
        { type: "text-delta", id: "text-1", delta: "hello" },
        { type: "text-end", id: "text-1" },
        {
          type: "finish",
          usage: { inputTokens: {}, outputTokens: {} },
          finishReason: { unified: "stop" },
        },
      ]),
    );

    const result = await model.doStream({ prompt: [] });
    const parts = await collect(result.stream);

    assert.deepEqual(
      parts.map((part) => part.type),
      ["stream-start", "text-start", "text-delta", "text-end", "finish"],
    );
    assert.equal(result.response?.headers?.["x-server"], "stream");
  });

  it("ignores DONE and converts response metadata timestamps", async () => {
    const timestamp = "2026-08-22T01:02:03.456Z";
    const model = modelWithFetch(async () =>
      sseResponse([
        { type: "stream-start", warnings: [] },
        {
          type: "response-metadata",
          id: "response-1",
          modelId: "public-model",
          timestamp,
        },
        "[DONE]",
      ]),
    );

    const result = await model.doStream({ prompt: [] });
    const parts = await collect(result.stream);

    assert.equal(parts.length, 2);
    const metadata = parts[1];
    assert.equal(metadata.type, "response-metadata");
    if (metadata.type === "response-metadata") {
      assert.equal(metadata.timestamp instanceof Date, true);
      assert.equal(metadata.timestamp?.toISOString(), timestamp);
    }
  });

  it("filters raw parts unless requested", async () => {
    const events = [
      { type: "stream-start", warnings: [] },
      { type: "raw", rawValue: { secret: "opaque" } },
      {
        type: "finish",
        usage: { inputTokens: {}, outputTokens: {} },
        finishReason: { unified: "stop" },
      },
    ];
    const model = modelWithFetch(async () => sseResponse(events));

    const filtered = await collect((await model.doStream({ prompt: [] })).stream);
    const explicitlyFiltered = await collect(
      (await model.doStream({ prompt: [], includeRawChunks: false })).stream,
    );
    const included = await collect(
      (await model.doStream({ prompt: [], includeRawChunks: true })).stream,
    );

    assert.deepEqual(
      filtered.map((part) => part.type),
      ["stream-start", "finish"],
    );
    assert.deepEqual(
      explicitlyFiltered.map((part) => part.type),
      ["stream-start", "finish"],
    );
    assert.deepEqual(
      included.map((part) => part.type),
      ["stream-start", "raw", "finish"],
    );
    assert.deepEqual(included[1], {
      type: "raw",
      rawValue: { secret: "opaque" },
    });
  });

  it("maps structured unary and streaming setup errors", async () => {
    const model = modelWithFetch(async () => invalidRequestResponse());

    await assert.rejects(async () => await model.doGenerate({ prompt: [] }), assertInvalidRequest);
    await assert.rejects(async () => await model.doStream({ prompt: [] }), assertInvalidRequest);
  });
});
