import { createGateway } from "@ai-sdk/gateway";
import type { LanguageModelV4CallOptions } from "@ai-sdk/provider";
import { generateText, stepCountIs, streamText, tool } from "ai";
import { z } from "zod";
import { startRecorder, type RecordedRequest, type ScriptedResponse } from "./recorder";

export interface RequestCapture {
  scenario: string;
  sequence: number;
  request: RecordedRequest;
}

export interface CaptureArtifact {
  formatVersion: 1;
  captures: RequestCapture[];
}

const unaryResponse: ScriptedResponse = {
  contentType: "application/json",
  body: JSON.stringify({
    content: [{ type: "text", text: "captured response" }],
    finishReason: { unified: "stop", raw: "stop" },
    usage: { inputTokens: { total: 1 }, outputTokens: { total: 1 } },
    warnings: [],
  }),
};

const streamResponse: ScriptedResponse = {
  contentType: "text/event-stream",
  body: [
    `data: ${JSON.stringify({ type: "stream-start", warnings: [] })}\n\n`,
    `data: ${JSON.stringify({ type: "text-start", id: "text-1" })}\n\n`,
    `data: ${JSON.stringify({ type: "text-delta", id: "text-1", delta: "captured stream" })}\n\n`,
    `data: ${JSON.stringify({ type: "text-end", id: "text-1" })}\n\n`,
    `data: ${JSON.stringify({ type: "finish", finishReason: { unified: "stop", raw: "stop" }, usage: { inputTokens: { total: 1 }, outputTokens: { total: 1 } } })}\n\n`,
  ].join(""),
};

export async function captureAllRequests(): Promise<CaptureArtifact> {
  return withSyntheticObservability(async () => {
    const captures: RequestCapture[] = [];
    captures.push(...(await captureDirectUnary()));
    captures.push(...(await captureDirectStream()));
    captures.push(...(await captureGenerateText()));
    captures.push(...(await captureClientToolFlow()));
    captures.push(...(await captureStreamText()));
    return { formatVersion: 1, captures };
  });
}

async function captureDirectUnary(): Promise<RequestCapture[]> {
  const options: LanguageModelV4CallOptions = {
    prompt: [
      {
        role: "system",
        content: "capture system",
        providerOptions: { system: {} },
      },
      {
        role: "user",
        content: [
          { type: "text", text: "capture user" },
          {
            type: "file",
            data: { type: "data", data: new Uint8Array([1, 2, 3, 4]) },
            mediaType: "application/octet-stream",
            filename: "prompt.bin",
          },
          {
            type: "file",
            data: { type: "url", url: new URL("https://example.test/prompt.txt") },
            mediaType: "text/plain",
          },
          {
            type: "file",
            data: { type: "reference", reference: {} },
            mediaType: "application/pdf",
          },
          {
            type: "file",
            data: { type: "text", text: "inline prompt" },
            mediaType: "text/plain",
          },
        ],
      },
      {
        role: "assistant",
        content: [
          { type: "text", text: "assistant text" },
          { type: "reasoning", text: "assistant reasoning" },
          { type: "custom", kind: "capture.custom", providerOptions: { custom: {} } },
          {
            type: "reasoning-file",
            data: { type: "data", data: new Uint8Array([5, 6]) },
            mediaType: "application/octet-stream",
          },
          {
            type: "tool-call",
            toolCallId: "provider-call",
            toolName: "providerSearch",
            input: { query: "capture" },
            providerExecuted: true,
          },
          {
            type: "tool-result",
            toolCallId: "provider-call",
            toolName: "providerSearch",
            output: { type: "json", value: null },
          },
        ],
      },
      {
        role: "tool",
        content: [
          {
            type: "tool-result",
            toolCallId: "nested-file-call",
            toolName: "fileTool",
            output: {
              type: "content",
              value: [
                { type: "text", text: "nested text" },
                {
                  type: "file",
                  data: { type: "data", data: new Uint8Array([7, 8, 9]) },
                  mediaType: "application/octet-stream",
                  filename: "nested.bin",
                },
                { type: "custom", providerOptions: { nested: { safe: true } } },
              ],
            },
          },
          {
            type: "tool-approval-response",
            approvalId: "approval-1",
            approved: false,
            reason: "capture denial",
          },
        ],
      },
    ],
    maxOutputTokens: 0,
    temperature: 0,
    stopSequences: [],
    topP: 0,
    topK: 0,
    presencePenalty: 0,
    frequencyPenalty: 0,
    responseFormat: {
      type: "json",
      schema: { type: "object", properties: { value: { type: "string" } } },
      name: "capture",
      description: "capture schema",
    },
    seed: 0,
    tools: [
      {
        type: "function",
        name: "echo",
        description: "echoes input",
        inputSchema: { type: "object", properties: { text: { type: "string" } } },
        inputExamples: [{ input: {} }],
        strict: false,
        providerOptions: { function: {} },
      },
      {
        type: "provider",
        id: "capture.search",
        name: "providerSearch",
        args: { maxResults: 0, filters: {} },
      },
    ],
    toolChoice: { type: "tool", toolName: "echo" },
    includeRawChunks: false,
    headers: {
      "x-call": "call",
      "x-collision": "call-wins",
      "ai-language-model-id": "call-model-loses",
      "ai-language-model-specification-version": "call-version-loses",
      "ai-language-model-streaming": "call-streaming-loses",
      "ai-o11y-project-id": "call-project-loses",
    },
    reasoning: "provider-default",
    providerOptions: {
      gateway: {
        order: ["provider-a", "provider-b"],
        only: ["provider-a"],
        nullable: null,
      },
    },
    abortSignal: new AbortController().signal,
  };

  return recordScenario(
    "direct-do-generate-complete",
    [unaryResponse],
    async (baseURL) => {
      const gateway = newCaptureGateway(baseURL);
      await gateway("capture/model").doGenerate(options);
    },
    { "ai-o11y-project-id": "synthetic-capture-project" },
  );
}

async function captureDirectStream(): Promise<RequestCapture[]> {
  return recordScenario("direct-do-stream", [streamResponse], async (baseURL) => {
    const gateway = newCaptureGateway(baseURL);
    const result = await gateway("capture/model").doStream({
      prompt: [{ role: "user", content: [{ type: "text", text: "stream directly" }] }],
      includeRawChunks: true,
      tools: [],
      stopSequences: [],
      providerOptions: {},
      headers: {},
    });
    for await (const _part of result.stream) {
    }
  });
}

async function captureGenerateText(): Promise<RequestCapture[]> {
  return recordScenario("orchestration-generate-text", [unaryResponse], async (baseURL) => {
    const gateway = newCaptureGateway(baseURL);
    await generateText({
      model: gateway("capture/model"),
      maxRetries: 0,
      system: "orchestration system",
      messages: [{ role: "user", content: "orchestration generate" }],
      headers: { "x-call": "orchestration" },
      providerOptions: { gateway: { order: ["provider-a"] } },
      tools: {
        echo: tool({
          description: "echo",
          inputSchema: z.object({ text: z.string() }),
          execute: async ({ text }) => ({ echoed: text }),
        }),
      },
      toolChoice: { type: "tool", toolName: "echo" },
    });
  });
}

async function captureClientToolFlow(): Promise<RequestCapture[]> {
  const toolCallResponse: ScriptedResponse = {
    contentType: "application/json",
    body: JSON.stringify({
      content: [
        {
          type: "tool-call",
          toolCallId: "client-call",
          toolName: "echo",
          input: JSON.stringify({ text: "client execution" }),
        },
      ],
      finishReason: { unified: "tool-calls", raw: "tool_use" },
      usage: { inputTokens: { total: 1 }, outputTokens: { total: 1 } },
      warnings: [],
    }),
  };

  return recordScenario("orchestration-client-tool-flow", [toolCallResponse, unaryResponse], async (baseURL) => {
    const gateway = newCaptureGateway(baseURL);
    await generateText({
      model: gateway("capture/model"),
      maxRetries: 0,
      prompt: "execute the client tool",
      tools: {
        echo: tool({
          inputSchema: z.object({ text: z.string() }),
          execute: async ({ text }) => ({ echoed: text }),
        }),
      },
      stopWhen: stepCountIs(2),
    });
  });
}

async function captureStreamText(): Promise<RequestCapture[]> {
  return recordScenario("orchestration-stream-text", [streamResponse], async (baseURL) => {
    const gateway = newCaptureGateway(baseURL);
    const result = streamText({
      model: gateway("capture/model"),
      maxRetries: 0,
      system: "stream orchestration system",
      prompt: "orchestration stream",
      includeRawChunks: true,
      headers: { "x-call": "stream-orchestration" },
      providerOptions: { gateway: { order: [] } },
    });
    for await (const _part of result.fullStream) {
    }
  });
}

async function recordScenario(
  scenario: string,
  responses: ScriptedResponse[],
  run: (baseURL: string) => Promise<void>,
  expectedRawHeaders: Record<string, string> = {},
): Promise<RequestCapture[]> {
  const recorder = await startRecorder(responses);
  try {
    await run(recorder.baseURL);
    if (recorder.requests.length !== responses.length) {
      throw new Error(`${scenario}: expected ${responses.length} request(s), received ${recorder.requests.length}`);
    }
    for (const [name, expected] of Object.entries(expectedRawHeaders)) {
      const values = recorder.rawHeaderValues(name);
      if (values.some((value) => value !== expected)) {
        throw new Error(`${scenario}: expected raw ${name}=${expected}, received ${values.join(",")}`);
      }
    }
    return recorder.requests.map((request, sequence) => ({ scenario, sequence, request }));
  } finally {
    await recorder.close();
  }
}

function newCaptureGateway(baseURL: string) {
  return createGateway({
    apiKey: "capture-not-a-real-key",
    baseURL,
    teamIdOrSlug: "capture-team",
    headers: {
      "x-configured": "configured",
      "x-collision": "configured-loses",
      "ai-language-model-id": "configured-model-loses",
      "ai-language-model-specification-version": "configured-version-loses",
      "ai-language-model-streaming": "configured-streaming-loses",
      "ai-o11y-project-id": "configured-project-loses",
    },
  });
}

async function withSyntheticObservability<T>(run: () => Promise<T>): Promise<T> {
  const names = ["VERCEL_DEPLOYMENT_ID", "VERCEL_ENV", "VERCEL_REGION", "VERCEL_PROJECT_ID"] as const;
  const original = Object.fromEntries(names.map((name) => [name, process.env[name]]));
  for (const name of names) delete process.env[name];
  process.env.VERCEL_PROJECT_ID = "synthetic-capture-project";
  try {
    return await run();
  } finally {
    for (const name of names) {
      const value = original[name];
      if (value === undefined) delete process.env[name];
      else process.env[name] = value;
    }
  }
}
