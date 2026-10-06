import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createGateway, GatewayError, GatewayResponseError, type GatewayProviderSettings } from "@ai-sdk/gateway";
import { APICallError, type LanguageModelV4StreamPart } from "@ai-sdk/provider";
import { generateText, streamText, StreamProviderError, wrapLanguageModel } from "ai";

const usage = { inputTokens: { total: 1, noCache: 1, cacheRead: 0, cacheWrite: 0 }, outputTokens: { total: 1, text: 1, reasoning: 0 } };

function metadata() {
  return {
    future: { value: [null, false, 0, "", {}, []] },
    gateway: {
      routing: { originalModelId: "assistant", canonicalSlug: "grafana/assistant", resolvedProvider: "openai", resolvedProviderApiModelId: "native-model" },
      evidence: {
        requestedModelId: "assistant", canonicalModelId: "grafana/assistant", selectedAttempt: 2,
        attempts: [
          { index: 1, provider: "anthropic", modelId: "primary", selection: "failed", nativeError: { statusCode: 429, isRetryable: true } },
          { index: 2, provider: "openai", modelId: "native-model", selection: "selected", completion: "completed" },
        ],
        native: { requestBody: { state: "available", value: { input: "sk-ordinary-application-text" } }, responseHeaders: { state: "available", value: { "x-native": "native" } } },
      },
      nativeMetadata: { routing: { resolvedProvider: "untrusted-native-claim" }, evidence: { value: "opaque" } },
    },
  };
}

function unary() {
  return {
    content: [{ type: "text", text: "hello" }], finishReason: { unified: "stop" }, usage, warnings: [],
    providerMetadata: metadata(),
    request: { body: { input: "native" } },
    response: { id: "native-response", modelId: "native-model", headers: { "x-native": "native" }, body: { output: "native" } },
  };
}

function events(error?: unknown): LanguageModelV4StreamPart[] {
  return [
    { type: "stream-start", warnings: [] },
    { type: "response-metadata", id: "native-response", modelId: "native-model", timestamp: new Date("2026-10-01T00:00:00Z") },
    { type: "text-start", id: "text" },
    { type: "text-delta", id: "text", delta: "before" },
    ...(error ? [{ type: "error" as const, error }] : []),
    { type: "text-delta", id: "text", delta: "after" },
    { type: "text-end", id: "text" },
    { type: "finish", finishReason: { unified: "stop", raw: "end" }, usage, providerMetadata: metadata() },
  ];
}

function sse(parts: LanguageModelV4StreamPart[]) {
  return new Response(parts.map(part => `data: ${JSON.stringify(part)}\n\n`).join(""), {
    headers: { "content-type": "text/event-stream", "x-gateway": "hop" },
  });
}

function gateway(fetch: NonNullable<GatewayProviderSettings["fetch"]>) {
  return createGateway({ apiKey: "dummy-gateway-key", baseURL: "https://contract.invalid/api/v1/aisdk", fetch });
}

function failure(allFailed: boolean) {
  const attempts = [
    { index: 1, provider: "anthropic", modelId: "primary", selection: "failed", nativeError: { statusCode: allFailed ? 429 : 401, isRetryable: allFailed } },
    ...(allFailed ? [{ index: 2, provider: "openai", modelId: "native-model", selection: "failed", nativeError: { statusCode: 401, isRetryable: false } }] : []),
  ];
  return {
    error: { message: "candidate unavailable", type: "failed_dependency", code: "failed_dependency", param: null },
    providerMetadata: { gateway: { evidence: { requestedModelId: "assistant", canonicalModelId: "grafana/assistant", attempts } } },
  };
}

async function collect<T>(stream: ReadableStream<T>): Promise<T[]> {
  const reader = stream.getReader();
  const parts: T[] = [];
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) return parts;
      parts.push(value);
    }
  } finally {
    reader.releaseLock();
  }
}

async function boundedJSON(response: Response, maxBytes: number): Promise<unknown> {
  assert.equal(response.ok, true);
  const reader = response.body!.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > maxBytes) throw new Error("discovery byte limit");
      chunks.push(value);
    }
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } finally {
    await reader.cancel();
    reader.releaseLock();
  }
}

async function configuredRoutes(fetch: NonNullable<GatewayProviderSettings["fetch"]>, maxBytes: number) {
  return boundedJSON(await fetch("https://contract.invalid/api/v1/aisdk/config", {
    method: "GET", headers: { "x-access-token": "dummy-access-token" }, redirect: "error",
  }), maxBytes);
}

describe("#321 pinned developer access witnesses (current baseline, not permanent losses)", () => {
  it("unary preserves metadata/collision provenance but replaces native transport", async () => {
    const body = unary();
    const model = gateway(async () => Response.json(body, { headers: { "x-gateway": "hop" } }))("assistant");
    const options = { prompt: [], providerOptions: { gateway: { byok: { openai: [{ apiKey: "dummy-byok-secret" }] } } } };
    const low = await model.doGenerate(options);
    assert.deepEqual(low.providerMetadata, body.providerMetadata);
    assert.deepEqual(low.request?.body, options);
    assert.equal(low.response?.headers?.["x-gateway"], "hop");
    assert.equal(low.response?.modelId, undefined);
    assert.deepEqual(low.response?.body, body);
    const high = await generateText({ model, prompt: "hi", maxRetries: 0 });
    assert.deepEqual(high.providerMetadata, body.providerMetadata);
    assert.notEqual(high.response.modelId, "native-model");
    assert.equal(high.response.body, undefined);
    const included = await generateText({ model, prompt: "hi", maxRetries: 0, include: { responseBody: true } });
    assert.deepEqual(included.response.body, body);
    assert.equal(high.providerMetadata?.gateway?.routing && (high.providerMetadata.gateway.routing as { resolvedProvider: string }).resolvedProvider, "openai");
  });

  it("stream setup and completion expose hop headers, native identity and finish metadata", async () => {
    const model = gateway(async () => sse(events()))("assistant");
    const low = await model.doStream({ prompt: [] });
    assert.equal(low.response?.headers?.["x-gateway"], "hop");
    assert.deepEqual(low.request?.body, { prompt: [] });
    const parts = await collect(low.stream);
    assert.deepEqual(parts.at(-1), events().at(-1));
    const identity = parts[1];
    assert.equal(identity.type, "response-metadata");
    if (identity.type === "response-metadata") assert.ok(identity.timestamp instanceof Date);
    const high = streamText({ model, prompt: "hi", maxRetries: 0 });
    await high.consumeStream();
    assert.equal(await high.text, "beforeafter");
    assert.deepEqual(await high.providerMetadata, metadata());
    assert.equal((await high.response).modelId, "native-model");
  });

  it("direct/setup/all-failed envelopes are accessible through public API-call cause, not top-level fields", async () => {
    for (const allFailed of [false, true]) {
      const envelope = failure(allFailed);
      const model = gateway(async () => Response.json(envelope, { status: 424 }))("assistant");
      for (const call of [() => model.doGenerate({ prompt: [] }), () => model.doStream({ prompt: [] }), () => generateText({ model, prompt: "hi", maxRetries: 0 })]) {
        await assert.rejects(async () => await call(), (error: unknown) => {
          assert.ok(GatewayError.isInstance(error));
          assert.equal(error.statusCode, 424);
          assert.equal(error.isRetryable, false);
          assert.equal("providerMetadata" in error, false);
          assert.ok(APICallError.isInstance(error.cause));
          assert.deepEqual(error.cause.data, envelope);
          assert.deepEqual(JSON.parse(error.cause.responseBody!), envelope);
          return true;
        });
      }
      const errors: unknown[] = [];
      const high = streamText({ model, prompt: "hi", maxRetries: 0, onError: ({ error }) => { errors.push(error); } });
      await high.consumeStream();
      assert.equal(errors.length, 1);
      assert.ok(GatewayError.isInstance(errors[0]));
      assert.ok(APICallError.isInstance(errors[0].cause));
      assert.deepEqual(errors[0].cause.data, envelope);
    }
  });

  it("malformed error envelope is a response error rather than attributed failure", async () => {
    await assert.rejects(async () => await gateway(async () => Response.json({ error: { message: 12 } }, { status: 424 }))("assistant").doGenerate({ prompt: [] }),
      (error: unknown) => GatewayResponseError.isInstance(error));
  });

  it("committed error preserves native/Gateway distinction and continues through high-level normalization", async () => {
    const error = { message: "native account rejected", type: "failed_dependency", code: "failed_dependency", statusCode: 424, retryable: false,
      data: { providerMetadata: metadata(), nativeError: { statusCode: 401, code: "native_auth", isRetryable: false } } };
    const model = gateway(async () => sse(events(error)))("assistant");
    const low = await collect((await model.doStream({ prompt: [] })).stream);
    assert.deepEqual(low[4], { type: "error", error });
    assert.equal(low.at(-1)?.type, "finish");
    const high = streamText({ model, prompt: "hi", maxRetries: 0, onError: () => {} });
    const parts = await collect(high.fullStream);
    const normalized = parts.find(part => part.type === "error");
    assert.ok(normalized?.type === "error" && StreamProviderError.isInstance(normalized.error));
    assert.equal(normalized.error.statusCode, 424);
    assert.deepEqual(normalized.error.data, error);
    assert.equal(await high.text, "beforeafter");
    assert.deepEqual(await high.providerMetadata, metadata());
  });

  it("consumer middleware access is independently configured, including failures", async () => {
    for (const capture of [false, true]) {
      const observed: unknown[] = [];
      const observedErrors: unknown[] = [];
      const model = wrapLanguageModel({ model: gateway(async () => Response.json(unary()))("assistant"), middleware: {
        specificationVersion: "v4",
        wrapGenerate: async ({ doGenerate }) => {
          const result = await doGenerate();
          if (capture) observed.push(result.providerMetadata);
          return result;
        },
      } });
      const result = await generateText({ model, prompt: "hi", maxRetries: 0 });
      assert.deepEqual(result.providerMetadata, metadata());
      assert.equal(observed.length, capture ? 1 : 0);
      const streamModel = wrapLanguageModel({ model: gateway(async () => sse(events()))("assistant"), middleware: {
        specificationVersion: "v4",
        wrapStream: async ({ doStream }) => {
          const result = await doStream();
          return { ...result, stream: result.stream.pipeThrough(new TransformStream({ transform(part, controller) {
            if (capture && part.type === "finish") observed.push(part.providerMetadata);
            controller.enqueue(part);
          } })) };
        },
      } });
      await streamText({ model: streamModel, prompt: "hi", maxRetries: 0 }).consumeStream();
      assert.deepEqual(observed, capture ? [metadata(), metadata()] : []);
      const failing = wrapLanguageModel({ model: gateway(async () => Response.json(failure(true), { status: 424 }))("assistant"), middleware: {
        specificationVersion: "v4", wrapGenerate: async ({ doGenerate }) => {
          try { return await doGenerate(); } catch (error) { if (capture) observedErrors.push(error); throw error; }
        },
        wrapStream: async ({ doStream }) => {
          try { return await doStream(); } catch (error) { if (capture) observedErrors.push(error); throw error; }
        },
      } });
      await assert.rejects(() => generateText({ model: failing, prompt: "hi", maxRetries: 0 }));
      assert.equal(observedErrors.length, capture ? 1 : 0);
      await streamText({ model: failing, prompt: "hi", maxRetries: 0, onError: () => {} }).consumeStream();
      assert.equal(observedErrors.length, capture ? 2 : 0);
    }
  });

  it("stock discovery strips candidates; proposed bounded authenticated access preserves them without inference", async () => {
    const configured = { canonicalModelId: "grafana/assistant", aliases: ["assistant"], candidates: [{ providerInstance: "anthropic-primary", provider: "anthropic", modelId: "primary" }, { providerInstance: "openai-secondary", provider: "openai", modelId: "native-model" }] };
    const document = { models: ["assistant", "grafana/assistant"].map(id => ({ id, name: "Assistant", specification: { specificationVersion: "v4", provider: "grafana", modelId: id }, gateway: configured })) };
    const stock = await gateway(async () => Response.json(document)).getAvailableModels();
    assert.deepEqual(stock.models.map(row => row.id), ["assistant", "grafana/assistant"]);
    assert.equal("gateway" in stock.models[0], false);
    let calls = 0;
    const fetch: NonNullable<GatewayProviderSettings["fetch"]> = async (url, init) => {
      calls++;
      assert.equal(String(url), "https://contract.invalid/api/v1/aisdk/config");
      assert.equal(init?.method, "GET");
      assert.equal(new Headers(init?.headers).get("x-access-token"), "dummy-access-token");
      assert.equal(init?.redirect, "error");
      return Response.json(document);
    };
    const bytes = Buffer.byteLength(JSON.stringify(document));
    assert.deepEqual(await configuredRoutes(fetch, bytes), document);
    await assert.rejects(() => configuredRoutes(fetch, bytes - 1), /discovery byte limit/);
    assert.equal(calls, 2);
  });
});
