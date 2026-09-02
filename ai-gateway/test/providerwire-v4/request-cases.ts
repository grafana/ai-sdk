import { createGateway } from "@ai-sdk/gateway";
import type { LanguageModelV4, LanguageModelV4CallOptions } from "@ai-sdk/provider";
import { createCaptureFetch, drainStream, type SemanticRequest } from "./capture.ts";

export type RequestGoldenCase = {
  name: string;
  fileName: string;
  capture: () => Promise<SemanticRequest[]>;
};

const opaqueOptions = {
  provider: {
    nested: { nullValue: null, falseValue: false, zero: 0, empty: "" },
    array: [null, false, 0, "", [], {}],
  },
};

async function captureCalls({
  modelId,
  configuredHeaders,
  capturedHeaderNames,
  calls,
}: {
  modelId: string;
  configuredHeaders?: Record<string, string>;
  capturedHeaderNames?: string[];
  calls: (model: LanguageModelV4) => Promise<void>;
}): Promise<SemanticRequest[]> {
  const capture = createCaptureFetch({ additionalHeaderNames: capturedHeaderNames });
  const gateway = createGateway({
    apiKey: "contract-test-key",
    baseURL: "https://contract.invalid",
    headers: configuredHeaders,
    fetch: capture.fetch,
  });
  await calls(gateway(modelId));
  return capture.requests;
}

async function generate(model: LanguageModelV4, options: LanguageModelV4CallOptions): Promise<void> {
  await model.doGenerate(options);
}

async function stream(model: LanguageModelV4, options: LanguageModelV4CallOptions): Promise<void> {
  const result = await model.doStream(options);
  await drainStream(result.stream);
}

function scalarOptions(): LanguageModelV4CallOptions {
  return {
    prompt: [
      { role: "system", content: "" },
      { role: "user", content: [{ type: "text", text: "" }] },
    ],
    maxOutputTokens: 0,
    temperature: 0,
    stopSequences: [],
    topP: 0,
    topK: 0,
    presencePenalty: 0,
    frequencyPenalty: 0,
    responseFormat: { type: "text" },
    seed: 0,
    tools: [],
    includeRawChunks: false,
    headers: {
      "x-contract-body": "",
      "x-contract-undefined": undefined,
    },
    reasoning: "provider-default",
    providerOptions: {
      empty: {},
      opaque: opaqueOptions.provider,
    },
  };
}

function comprehensiveOptions(): LanguageModelV4CallOptions {
  return {
    prompt: [
      { role: "system", content: "system", providerOptions: opaqueOptions },
      {
        role: "user",
        providerOptions: opaqueOptions,
        content: [
          { type: "text", text: "", providerOptions: opaqueOptions },
          {
            type: "file",
            filename: "bytes.bin",
            data: { type: "data", data: new Uint8Array([0, 1, 2]) },
            mediaType: "application/octet-stream",
            providerOptions: opaqueOptions,
          },
          {
            type: "file",
            data: { type: "url", url: new URL("https://example.com/%") },
            mediaType: "text/plain",
          },
          {
            type: "file",
            data: { type: "reference", reference: { provider: "file-1" } },
            mediaType: "application/pdf",
          },
          {
            type: "file",
            data: { type: "text", text: "inline" },
            mediaType: "text/plain",
          },
        ],
      },
      {
        role: "assistant",
        providerOptions: opaqueOptions,
        content: [
          { type: "text", text: "assistant", providerOptions: opaqueOptions },
          {
            type: "file",
            data: { type: "data", data: "YWxyZWFkeS1iYXNlNjQ=" },
            mediaType: "application/octet-stream",
          },
          { type: "custom", kind: "provider.kind", providerOptions: opaqueOptions },
          { type: "reasoning", text: "reasoning", providerOptions: opaqueOptions },
          {
            type: "reasoning-file",
            data: { type: "data", data: new Uint8Array([3, 4]) },
            mediaType: "application/octet-stream",
            providerOptions: opaqueOptions,
          },
          {
            type: "reasoning-file",
            data: { type: "url", url: new URL("https://example.test/reasoning") },
            mediaType: "application/pdf",
          },
          {
            type: "tool-call",
            toolCallId: "call-1",
            toolName: "lookup",
            input: { query: "value", nested: { nullValue: null } },
            providerExecuted: false,
            providerOptions: opaqueOptions,
          },
          {
            type: "tool-result",
            toolCallId: "call-text",
            toolName: "lookup",
            output: { type: "text", value: "", providerOptions: opaqueOptions },
          },
          {
            type: "tool-result",
            toolCallId: "call-json",
            toolName: "lookup",
            output: { type: "json", value: null, providerOptions: opaqueOptions },
          },
          {
            type: "tool-result",
            toolCallId: "call-denied",
            toolName: "lookup",
            output: { type: "execution-denied", reason: "", providerOptions: opaqueOptions },
          },
          {
            type: "tool-result",
            toolCallId: "call-error-text",
            toolName: "lookup",
            output: { type: "error-text", value: "", providerOptions: opaqueOptions },
          },
          {
            type: "tool-result",
            toolCallId: "call-error-json",
            toolName: "lookup",
            output: { type: "error-json", value: { code: 0 }, providerOptions: opaqueOptions },
          },
          {
            type: "tool-result",
            toolCallId: "call-content",
            toolName: "lookup",
            providerOptions: opaqueOptions,
            output: {
              type: "content",
              value: [
                { type: "text", text: "", providerOptions: opaqueOptions },
                {
                  type: "file",
                  filename: "result.bin",
                  data: { type: "data", data: new Uint8Array([5, 6]) },
                  mediaType: "application/octet-stream",
                  providerOptions: opaqueOptions,
                },
                {
                  type: "file",
                  data: { type: "url", url: new URL("https://example.test/result") },
                  mediaType: "text/plain",
                },
                {
                  type: "file",
                  data: { type: "reference", reference: { provider: "file-2" } },
                  mediaType: "application/pdf",
                },
                {
                  type: "file",
                  data: { type: "text", text: "tool text" },
                  mediaType: "text/plain",
                },
                { type: "custom", providerOptions: opaqueOptions },
              ],
            },
          },
        ],
      },
      {
        role: "tool",
        providerOptions: opaqueOptions,
        content: [
          {
            type: "tool-result",
            toolCallId: "call-tool",
            toolName: "lookup",
            output: { type: "json", value: { ok: false } },
          },
          {
            type: "tool-approval-response",
            approvalId: "approval-1",
            approved: false,
            reason: "",
            providerOptions: opaqueOptions,
          },
        ],
      },
    ],
    maxOutputTokens: 42,
    temperature: 0.25,
    stopSequences: ["stop", ""],
    topP: 0.75,
    topK: 4,
    presencePenalty: -0.5,
    frequencyPenalty: 0.5,
    responseFormat: {
      type: "json",
      schema: {
        type: "object",
        properties: { value: { type: "string" } },
        required: ["value"],
      },
      name: "",
      description: "",
    },
    seed: 7,
    tools: [
      {
        type: "function",
        name: "lookup",
        description: "",
        inputSchema: {
          type: "object",
          properties: { query: { type: "string" } },
          required: ["query"],
        },
        inputExamples: [{ input: { query: "example" } }],
        strict: false,
        providerOptions: opaqueOptions,
      },
      {
        type: "provider",
        id: "provider.search",
        name: "search",
        args: { limit: 0, enabled: false, nested: { nullValue: null } },
      },
    ],
    toolChoice: { type: "tool", toolName: "lookup" },
    includeRawChunks: true,
    headers: { "x-contract-comprehensive": "body-and-outer" },
    reasoning: "high",
    providerOptions: opaqueOptions,
  };
}

async function scalarCapture(): Promise<SemanticRequest[]> {
  return captureCalls({
    modelId: "grafana/scalar",
    configuredHeaders: { "x-contract-config": "configured" },
    calls: (model) => generate(model, scalarOptions()),
  });
}

async function comprehensiveCapture(): Promise<SemanticRequest[]> {
  return captureCalls({
    modelId: "grafana/comprehensive",
    calls: (model) => generate(model, comprehensiveOptions()),
  });
}

async function streamingCapture(): Promise<SemanticRequest[]> {
  const controller = new AbortController();
  return captureCalls({
    modelId: "grafana/stream",
    calls: (model) =>
      stream(model, {
        prompt: [{ role: "user", content: [{ type: "text", text: "stream" }] }],
        abortSignal: controller.signal,
      }),
  });
}

async function headersCapture(): Promise<SemanticRequest[]> {
  const previousDeploymentId = process.env.VERCEL_DEPLOYMENT_ID;
  process.env.VERCEL_DEPLOYMENT_ID = "deployment-1";
  try {
    const ordinary = await captureCalls({
      modelId: "actual",
      configuredHeaders: {
        "x-contract-config": "configured",
        "x-contract-precedence": "configured",
      },
      capturedHeaderNames: ["ai-o11y-deployment-id"],
      calls: (model) =>
        generate(model, {
          prompt: [],
          headers: {
            "x-contract-call": "call",
            "x-contract-precedence": "call",
          },
        }),
    });
    const collision = await captureCalls({
      modelId: "actual",
      configuredHeaders: { "ai-language-model-id": "configured" },
      calls: (model) =>
        generate(model, {
          prompt: [],
          headers: { "AI-Language-Model-Id": "call" },
        }),
    });
    return [...ordinary, ...collision];
  } finally {
    if (previousDeploymentId === undefined) {
      delete process.env.VERCEL_DEPLOYMENT_ID;
    } else {
      process.env.VERCEL_DEPLOYMENT_ID = previousDeploymentId;
    }
  }
}

async function sequenceCapture(): Promise<SemanticRequest[]> {
  return captureCalls({
    modelId: "grafana/sequence",
    calls: async (model) => {
      await generate(model, {
        prompt: [{ role: "user", content: [{ type: "text", text: "first" }] }],
      });
      await stream(model, {
        prompt: [{ role: "user", content: [{ type: "text", text: "second" }] }],
      });
    },
  });
}

export const scalarGoldenCase: RequestGoldenCase = {
  name: "scalar presence",
  fileName: "scalar-presence.json",
  capture: scalarCapture,
};
export const comprehensiveGoldenCase: RequestGoldenCase = {
  name: "comprehensive unions",
  fileName: "comprehensive-unions.json",
  capture: comprehensiveCapture,
};
export const streamingGoldenCase: RequestGoldenCase = {
  name: "streaming",
  fileName: "streaming.json",
  capture: streamingCapture,
};
export const headersGoldenCase: RequestGoldenCase = {
  name: "header precedence",
  fileName: "headers.json",
  capture: headersCapture,
};
export const sequenceGoldenCase: RequestGoldenCase = {
  name: "ordered sequence",
  fileName: "sequence.json",
  capture: sequenceCapture,
};

export const requestGoldenCases: RequestGoldenCase[] = [
  scalarGoldenCase,
  comprehensiveGoldenCase,
  streamingGoldenCase,
  headersGoldenCase,
  sequenceGoldenCase,
];
