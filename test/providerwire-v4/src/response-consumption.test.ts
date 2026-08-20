import assert from "node:assert/strict";
import { createServer, type RequestListener } from "node:http";
import type { AddressInfo } from "node:net";
import { describe, it } from "node:test";
import { createGateway, GatewayInvalidRequestError } from "@ai-sdk/gateway";
import type {
  LanguageModelV4CallOptions,
  LanguageModelV4StreamPart,
} from "@ai-sdk/provider";

const callOptions: LanguageModelV4CallOptions = { prompt: [] };

describe("smoke-only pinned-client response consumption", () => {
  it("consumes a minimal unary JSON result", async () => {
    await withServer((_request, response) => {
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify(generateResult()));
    }, async (baseURL) => {
      const result = await model(baseURL).doGenerate(callOptions);
      assert.deepEqual(result.content, [{ type: "text", text: "unary-ok" }]);
      assert.deepEqual(result.finishReason, { unified: "stop", raw: "stop" });
      assert.equal(result.usage.inputTokens.total, 1);
    });
  });

  it("consumes ordered JSON SSE events through clean EOF without DONE", async () => {
    await withServer((_request, response) => {
      const parts: LanguageModelV4StreamPart[] = [
        { type: "stream-start", warnings: [] },
        { type: "text-start", id: "text-1" },
        { type: "text-delta", id: "text-1", delta: "stream-ok" },
        { type: "text-end", id: "text-1" },
        finishPart(),
      ];
      response.writeHead(200, { "Content-Type": "text/event-stream" });
      response.end(parts.map((part) => `data: ${JSON.stringify(part)}\n\n`).join(""));
    }, async (baseURL) => {
      const result = await model(baseURL).doStream(callOptions);
      const observed: LanguageModelV4StreamPart[] = [];
      for await (const part of result.stream) {
        observed.push(part);
      }
      assert.deepEqual(
        observed.map((part) => part.type),
        ["stream-start", "text-start", "text-delta", "text-end", "finish"],
      );
      assert.equal(
        (observed.find((part) => part.type === "text-delta") as { delta: string }).delta,
        "stream-ok",
      );
    });
  });

  it("normalizes a safe non-2xx response through the public client", async () => {
    await withServer((_request, response) => {
      response.writeHead(400, { "Content-Type": "application/json" });
      response.end(
        JSON.stringify({
          error: {
            type: "invalid_request_error",
            message: "safe invalid request",
            param: null,
            code: "bad_request",
          },
          generationId: "generation-safe-1",
        }),
      );
    }, async (baseURL) => {
      await assert.rejects(
        Promise.resolve(model(baseURL).doGenerate(callOptions)),
        (error: unknown) => {
          assert.equal(GatewayInvalidRequestError.isInstance(error), true);
          if (!GatewayInvalidRequestError.isInstance(error)) {
            return false;
          }
          assert.equal(error.message, "safe invalid request [generation-safe-1]");
          assert.equal(error.statusCode, 400);
          assert.equal(error.generationId, "generation-safe-1");
          return true;
        },
      );
    });
  });
});

function model(baseURL: string) {
  return createGateway({ baseURL, apiKey: "response-smoke-key" })("provider/response-smoke");
}

async function withServer(listener: RequestListener, action: (baseURL: string) => Promise<void>) {
  const server = createServer(listener);
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  try {
    await action(`http://127.0.0.1:${port}/api/v1/aisdk`);
  } finally {
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
}

function generateResult() {
  return {
    content: [{ type: "text", text: "unary-ok" }],
    finishReason: { unified: "stop", raw: "stop" },
    usage: usage(),
  };
}

function finishPart(): Extract<LanguageModelV4StreamPart, { type: "finish" }> {
  return {
    type: "finish",
    finishReason: { unified: "stop", raw: "stop" },
    usage: usage(),
  };
}

function usage() {
  return {
    inputTokens: { total: 1, noCache: 1, cacheRead: 0, cacheWrite: 0 },
    outputTokens: { total: 1, text: 1, reasoning: 0 },
  };
}
