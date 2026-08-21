import {
  createGateway,
  GatewayAuthenticationError,
  GatewayError,
  GatewayModelNotFoundError,
} from "@ai-sdk/gateway";
import { describe, expect, it } from "vitest";
import { getServerUrl } from "./helpers";

type GatewayModel = ReturnType<ReturnType<typeof createGateway>>;
type CallOptions = Parameters<GatewayModel["doGenerate"]>[0];
type ObservedPart = Record<string, unknown> & { type: string };

const gatewayBasePath = "/api/v1/aisdk";
const unaryCanonicalId = "providerwire-v4/unary";
const unaryAlias = "providerwire-v4/unary-alias";
const streamWithStartId = "providerwire-v4/stream-with-start";
const streamWithoutStartId = "providerwire-v4/stream-without-start";
const streamErrorId = "providerwire-v4/stream-error";
const authenticationId = "providerwire-v4/authentication";
const authenticationMessage = "safe integration authentication failure";
const expectedUnaryResponse = `{"content":[{"type":"text","text":"mapped-unary"},{"type":"text","text":""}],"finishReason":{"unified":"stop","raw":"integration-stop"},"usage":{"inputTokens":{"total":7,"noCache":4,"cacheRead":2,"cacheWrite":1},"outputTokens":{"total":3,"text":3,"reasoning":0}},"warnings":[],"response":{"modelId":"${unaryCanonicalId}"}}`;
const expectedAuthenticationResponse = `{"error":{"message":"${authenticationMessage}","type":"authentication_error","param":null,"code":"authentication_error"}}`;

function providerWireGateway() {
  return createGateway({
    baseURL: `${getServerUrl()}${gatewayBasePath}`,
    apiKey: "providerwire-v4-integration-key",
  });
}

function unaryCallOptions(): CallOptions {
  return {
    prompt: [
      { role: "system", content: "" },
      {
        role: "user",
        content: [
          { type: "text", text: "hello" },
          { type: "text", text: "" },
        ],
      },
      { role: "assistant", content: [{ type: "text", text: "reply" }] },
    ],
    maxOutputTokens: 0,
    temperature: 0,
    topP: 0.5,
    topK: 7,
    presencePenalty: 0,
    frequencyPenalty: -0.5,
    stopSequences: [],
    seed: 42,
    reasoning: "high",
  };
}

function protocolHeaders(modelId: string, streaming: boolean): Record<string, string> {
  return {
    "Content-Type": "application/json",
    "ai-language-model-id": modelId,
    "ai-language-model-specification-version": "4",
    "ai-language-model-streaming": String(streaming),
  };
}

async function rawRequest(modelId: string, streaming: boolean, body: unknown) {
  return fetch(`${getServerUrl()}${gatewayBasePath}/language-model`, {
    method: "POST",
    headers: protocolHeaders(modelId, streaming),
    body: JSON.stringify(body),
  });
}

async function collect(modelId: string): Promise<ObservedPart[]> {
  const result = await providerWireGateway()(modelId).doStream({ prompt: [] });
  const parts: ObservedPart[] = [];
  for await (const part of result.stream) {
    parts.push(part as ObservedPart);
  }
  return parts;
}

async function gatewayError(modelId: string): Promise<GatewayError> {
  return captureGatewayError(modelId, () =>
    providerWireGateway()(modelId).doGenerate({ prompt: [] }),
  );
}

async function gatewayStreamError(modelId: string): Promise<GatewayError> {
  return captureGatewayError(modelId, () =>
    providerWireGateway()(modelId).doStream({ prompt: [] }),
  );
}

async function captureGatewayError(
  modelId: string,
  call: () => Promise<unknown>,
): Promise<GatewayError> {
  try {
    await call();
  } catch (error) {
    expect(GatewayError.isInstance(error)).toBe(true);
    if (GatewayError.isInstance(error)) {
      return error;
    }
    throw error;
  }
  throw new Error(`expected ${modelId} to fail`);
}

describe("ProviderWire V4 exact-pinned runtime", () => {
  it("maps a unary alias request and is consumed through the public client", async () => {
    const result = await providerWireGateway()(unaryAlias).doGenerate(unaryCallOptions());

    expect(result.content).toEqual([
      { type: "text", text: "mapped-unary" },
      { type: "text", text: "" },
    ]);
    expect(result.finishReason).toEqual({ unified: "stop", raw: "integration-stop" });
    expect(result.usage).toEqual({
      inputTokens: { total: 7, noCache: 4, cacheRead: 2, cacheWrite: 1 },
      outputTokens: { total: 3, text: 3, reasoning: 0 },
    });
  });

  it("proves unary canonical identity and empty warnings from raw HTTP", async () => {
    const response = await rawRequest(unaryAlias, false, unaryCallOptions());
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toBe("application/json");
    const body = await response.text();
    expect(body).toBe(expectedUnaryResponse);
    expect(JSON.parse(body)).toEqual({
      content: [
        { type: "text", text: "mapped-unary" },
        { type: "text", text: "" },
      ],
      finishReason: { unified: "stop", raw: "integration-stop" },
      usage: {
        inputTokens: { total: 7, noCache: 4, cacheRead: 2, cacheWrite: 1 },
        outputTokens: { total: 3, text: 3, reasoning: 0 },
      },
      warnings: [],
      response: { modelId: unaryCanonicalId },
    });
  });

  it.each([
    [streamWithStartId, streamWithStartId],
    [streamWithoutStartId, streamWithoutStartId],
  ])("normalizes one start and canonical identity for %s", async (modelId, canonicalId) => {
    const parts = await collect(modelId);

    expect(parts.map((part) => part.type)).toEqual([
      "stream-start",
      "response-metadata",
      "text-start",
      "text-delta",
      "text-delta",
      "text-end",
      "finish",
    ]);
    expect(parts.filter((part) => part.type === "stream-start")).toHaveLength(1);
    expect(parts[0]).toEqual({ type: "stream-start", warnings: [] });
    expect(parts[1]).toEqual({
      type: "response-metadata",
      id: "integration-response",
      modelId: canonicalId,
    });
    expect(parts[3]).toEqual({ type: "text-delta", id: "integration-text", delta: "" });
    expect(parts[4]).toEqual({
      type: "text-delta",
      id: "integration-text",
      delta: "streamed-text",
    });
    expect(parts.at(-1)).toEqual({
      type: "finish",
      finishReason: { unified: "stop", raw: "integration-stop" },
      usage: {
        inputTokens: { total: 2 },
        outputTokens: { total: 1, text: 1 },
      },
    });
  });

  it("proves SSE headers and clean EOF without DONE from raw HTTP", async () => {
    const response = await rawRequest(streamWithoutStartId, true, { prompt: [] });
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toBe("text/event-stream");
    expect(response.headers.get("cache-control")).toBe("no-cache, no-transform");
    const body = await response.text();
    expect(body).toBe(
      `data: {"type":"stream-start","warnings":[]}\n\n` +
        `data: {"type":"response-metadata","id":"integration-response","modelId":"${streamWithoutStartId}"}\n\n` +
        `data: {"type":"text-start","id":"integration-text"}\n\n` +
        `data: {"type":"text-delta","id":"integration-text","delta":""}\n\n` +
        `data: {"type":"text-delta","id":"integration-text","delta":"streamed-text"}\n\n` +
        `data: {"type":"text-end","id":"integration-text"}\n\n` +
        `data: {"type":"finish","finishReason":{"unified":"stop","raw":"integration-stop"},"usage":{"inputTokens":{"total":2},"outputTokens":{"total":1,"text":1}}}\n\n`,
    );
    expect(body).not.toContain("[DONE]");
  });

  it("classifies authentication without treating the contextual client message as server authority", async () => {
    const error = await gatewayError(authenticationId);
    expect(GatewayAuthenticationError.isInstance(error)).toBe(true);
    expect(error.statusCode).toBe(401);
    expect(error.isRetryable).toBe(false);

    const response = await rawRequest(authenticationId, false, { prompt: [] });
    expect(response.status).toBe(401);
    const body = await response.text();
    expect(body).toBe(expectedAuthenticationResponse);
    expect(JSON.parse(body)).toEqual({
      error: {
        message: authenticationMessage,
        type: "authentication_error",
        param: null,
        code: "authentication_error",
      },
    });
  });

  it("classifies an unknown model through both unary and streaming setup", async () => {
    for (const error of await Promise.all([
      gatewayError("providerwire-v4/unknown"),
      gatewayStreamError("providerwire-v4/unknown"),
    ])) {
      expect(GatewayModelNotFoundError.isInstance(error)).toBe(true);
      expect(error.statusCode).toBe(404);
      expect(error.isRetryable).toBe(false);
    }
  });

  it.each([
    ["invalid-request", 400, false],
    ["authentication", 401, false],
    ["permission", 403, false],
    ["not-found", 404, false],
    ["rate-limit", 429, true],
    ["overload", 503, true],
    ["failed-dependency", 424, false],
    ["upstream-failure", 502, true],
    ["timeout", 504, true],
    ["cancellation", 499, false],
    ["internal-failure", 500, true],
  ])("derives retryability for safe failure %s", async (name, status, retryable) => {
    const error = await gatewayError(`providerwire-v4/failure/${name}`);
    expect(error.statusCode).toBe(status);
    expect(error.isRetryable).toBe(retryable);
  });

  it("consumes the exact closed post-commit error part", async () => {
    const parts = await collect(streamErrorId);
    expect(parts).toEqual([
      { type: "stream-start", warnings: [] },
      {
        type: "error",
        error: {
          message: "provider request failed",
          type: "failed_dependency",
          param: null,
          code: "failed_dependency",
          statusCode: 424,
          retryable: false,
        },
      },
    ]);
  });
});
