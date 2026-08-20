import { generateText } from "ai";
import { describe, expect, it } from "vitest";
import {
  getV4GatewayBaseURL,
  newV4Gateway,
} from "../helpers";

type ScenarioStats = {
  resolverCalls: number;
  generateCalls: number;
  streamCalls: number;
  last?: {
    promptRoles: string[];
    toolsPresent: boolean;
    toolCount: number;
    stopSequencesPresent: boolean;
    stopSequenceCount: number;
    headersPresent: boolean;
    providerOptions: unknown;
    responseFormat: {
      type: string;
      name: string;
      description: string;
      hasSchema: boolean;
    } | null;
  };
};

function minimalOptions() {
  return {
    prompt: [
      {
        role: "user" as const,
        content: [{ type: "text" as const, text: "interop" }],
      },
    ],
  };
}

async function stats(modelId: string): Promise<ScenarioStats> {
  const serverURL = new URL(getV4GatewayBaseURL());
  const response = await fetch(
    `${serverURL.origin}/v4-stats?model=${encodeURIComponent(modelId)}`,
  );
  expect(response.ok).toBe(true);
  return (await response.json()) as ScenarioStats;
}

async function capturedError(run: () => PromiseLike<unknown>): Promise<unknown> {
  try {
    await run();
  } catch (error) {
    return error;
  }
  throw new Error("expected ProviderWire V4 request to fail");
}

describe("pinned unary client <-> strict Go ProviderWire V4 handler", () => {
  it("runs direct doGenerate with lossless adaptation and safe disclosure", async () => {
    const modelId = "v4-direct-adaptation";
    const model = newV4Gateway()(modelId);
    const result = await model.doGenerate({
      ...minimalOptions(),
      tools: [],
      stopSequences: [],
      providerOptions: {
        interop: {
          opaque: { nested: [0, false, null] },
          empty: {},
        },
      },
      responseFormat: {
        type: "json",
        schema: {
          type: "object",
          properties: { answer: { type: "string" } },
        },
        name: "answer",
        description: "structured answer",
      },
    });

    expect(result.content).toEqual([
      { type: "text", text: "direct unary from go" },
    ]);
    expect(result.finishReason).toEqual({ unified: "stop", raw: "stop" });
    expect(result.usage).toEqual({
      inputTokens: { total: 7, noCache: 7 },
      outputTokens: { total: 4, text: 4 },
    });
    const publicResult = result as unknown as {
      request?: unknown;
      response?: {
        provider?: unknown;
        modelId?: unknown;
        headers?: unknown;
        body?: unknown;
      };
    };
    expect(result.providerMetadata).toBeUndefined();
    expect(result.usage.raw).toBeUndefined();
    expect(result.warnings).toEqual([]);
    expect(publicResult.request).toBeDefined();
    expect(publicResult.response).toBeDefined();
    expect(publicResult.response?.provider).toBeUndefined();
    expect(publicResult.response?.modelId).toBeUndefined();
    expect(JSON.stringify(result)).not.toMatch(
      /backend-secret|secretRequest|secretResponse|unsafeTokens|authorization/,
    );

    expect(await stats(modelId)).toEqual({
      resolverCalls: 1,
      generateCalls: 1,
      streamCalls: 0,
      last: {
        promptRoles: ["user"],
        toolsPresent: true,
        toolCount: 0,
        stopSequencesPresent: true,
        stopSequenceCount: 0,
        headersPresent: false,
        providerOptions: {
          interop: {
            opaque: { nested: [0, false, null] },
            empty: {},
          },
        },
        responseFormat: {
          type: "json",
          name: "answer",
          description: "structured answer",
          hasSchema: true,
        },
      },
    });
  });

  it("runs ai.generateText through the pinned orchestration user-agent policy", async () => {
    const modelId = "v4-generate-text";
    const result = await generateText({
      model: newV4Gateway()(modelId),
      maxRetries: 0,
      prompt: "generate through V4",
    });

    expect(result.text).toBe("generated text from v4 go");
    expect(result.finishReason).toBe("stop");
    expect(result.usage.inputTokens).toBe(7);
    expect(await stats(modelId)).toMatchObject({
      resolverCalls: 1,
      generateCalls: 1,
      streamCalls: 0,
      last: {
        promptRoles: ["user"],
        headersPresent: false,
      },
    });
  });

  it.each([
    {
      modelId: "v4-error-429",
      statusCode: 429,
      type: "rate_limit_exceeded",
    },
    {
      modelId: "v4-error-503",
      statusCode: 503,
      type: "failed_dependency",
    },
  ])(
    "normalizes $statusCode safely while stock Gateway derives retryability",
    async ({ modelId, statusCode, type }) => {
      const error = await capturedError(() =>
        newV4Gateway()(modelId).doGenerate(minimalOptions()),
      );
      expect(error).toMatchObject({
        type,
        statusCode,
        isRetryable: true,
      });
      expect(String(error)).not.toMatch(/backend-secret|credential/);
      expect(await stats(modelId)).toMatchObject({
        resolverCalls: 1,
        generateCalls: 1,
        streamCalls: 0,
      });
    },
  );

  it.each([
    {
      name: "unsupported body headers",
      modelId: "v4-reject-headers",
      run: () =>
        newV4Gateway()("v4-reject-headers").doGenerate({
          ...minimalOptions(),
          headers: { authorization: "caller-secret" },
        }),
    },
    {
      name: "Gateway controls",
      modelId: "v4-reject-gateway-controls",
      run: () =>
        newV4Gateway()("v4-reject-gateway-controls").doGenerate({
          ...minimalOptions(),
          providerOptions: {
            gateway: { order: [] },
          } as never,
        }),
    },
    {
      name: "raw chunk intent",
      modelId: "v4-reject-raw",
      run: () =>
        newV4Gateway()("v4-reject-raw").doGenerate({
          ...minimalOptions(),
          includeRawChunks: true,
        }),
    },
    {
      name: "streaming selection",
      modelId: "v4-reject-streaming",
      run: () =>
        newV4Gateway()("v4-reject-streaming").doStream(minimalOptions()),
    },
  ])(
    "rejects $name before resolver and model invocation",
    async ({ modelId, run }) => {
      const error = await capturedError(run);
      expect(error).toMatchObject({
        type: "invalid_request_error",
        statusCode: 400,
        isRetryable: false,
      });
      expect(await stats(modelId)).toEqual({
        resolverCalls: 0,
        generateCalls: 0,
        streamCalls: 0,
      });
    },
  );
});
