import type {
  LanguageModelV4CallOptions,
  LanguageModelV4StreamPart,
} from "@ai-sdk/provider";
import { stepCountIs, streamText, tool } from "ai";
import { z } from "zod";
import { captureScenario, type ScenarioCapture } from "./capture.ts";

const emptyPrompt: LanguageModelV4CallOptions["prompt"] = [];
const simpleTool = {
  type: "function" as const,
  name: "lookup",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
};

export async function generateScenarioCaptures(): Promise<ScenarioCapture[]> {
  return withDeterministicGatewayEnvironment(async () => {
    const captures: ScenarioCapture[] = [];
    captures.push(
      await captureScenario("unary-settings", {
        callOptions: {
          prompt: [],
          maxOutputTokens: 0,
          temperature: 0,
          stopSequences: [],
          topP: 0,
          topK: 0,
          presencePenalty: 0,
          frequencyPenalty: 0,
          responseFormat: {
            type: "json",
            schema: {
              type: "object",
              properties: { value: { type: "string" } },
              required: [],
              additionalProperties: false,
            },
            name: "",
            description: "",
          },
          seed: 0,
          tools: [],
          includeRawChunks: false,
          headers: {},
          reasoning: "xhigh",
          providerOptions: {
            example: { nested: null, empty: [], false: false, zero: 0, text: "" },
          },
        },
      }),
    );
    captures.push(
      await captureScenario("response-format-text", {
        callOptions: { prompt: emptyPrompt, responseFormat: { type: "text" } },
      }),
    );

    for (const reasoning of [
      "provider-default",
      "none",
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
    ] as const) {
      captures.push(
        await captureScenario(`reasoning-${reasoning}`, {
          callOptions: { prompt: emptyPrompt, reasoning },
        }),
      );
    }

    captures.push(
      await captureScenario("streaming-prompt-tools", {
        mode: "stream",
        callOptions: comprehensivePromptOptions(),
      }),
    );
    captures.push(
      await captureScenario("presence-losses", {
        callOptions: presenceLossOptions(),
      }),
    );

    for (const toolChoice of ["auto", "none", "required"] as const) {
      captures.push(
        await captureScenario(`tool-choice-${toolChoice}`, {
          callOptions: {
            prompt: emptyPrompt,
            tools: [simpleTool],
            toolChoice: { type: toolChoice },
          },
        }),
      );
    }
    captures.push(
      await captureScenario("tool-choice-tool", {
        callOptions: {
          prompt: emptyPrompt,
          tools: [simpleTool],
          toolChoice: { type: "tool", toolName: "lookup" },
        },
      }),
    );

    const headerScenarios: Array<{
      name: string;
      gatewayHeaders?: Record<string, string>;
      headers: Record<string, string>;
    }> = [
      {
        name: "header-call",
        headers: { "x-call": "call-value", traceparent: "00-trace-parent" },
      },
      {
        name: "header-exact-custom",
        gatewayHeaders: { "x-exact": "configured" },
        headers: { "x-exact": "call" },
      },
      {
        name: "header-case-custom",
        gatewayHeaders: { "X-Case": "configured" },
        headers: { "x-case": "call" },
      },
      {
        name: "header-exact-protocol",
        headers: { "ai-language-model-specification-version": "caller" },
      },
      {
        name: "header-case-protocol",
        headers: { "Ai-Language-Model-Specification-Version": "caller" },
      },
      {
        name: "header-exact-content-type",
        headers: { "Content-Type": "text/plain" },
      },
      {
        name: "header-case-content-type",
        headers: { "content-type": "text/plain" },
      },
    ];
    for (const headerScenario of headerScenarios) {
      captures.push(
        await captureScenario(headerScenario.name, {
          gatewayHeaders: headerScenario.gatewayHeaders,
          callOptions: { prompt: emptyPrompt, headers: headerScenario.headers },
        }),
      );
    }
    process.env.VERCEL_DEPLOYMENT_ID = "deployment-evidence";
    try {
      captures.push(
        await captureScenario("header-observability", {
          callOptions: {
            prompt: emptyPrompt,
            headers: { "ai-o11y-deployment-id": "caller-observability" },
          },
        }),
      );
    } finally {
      delete process.env.VERCEL_DEPLOYMENT_ID;
    }
    captures.push(
      await captureScenario("multi-step-tool", {
        streamParts: multiStepParts,
        execute: async (model) => {
          let executedInput: unknown;
          const result = streamText({
            model,
            maxRetries: 0,
            stopWhen: stepCountIs(2),
            messages: [
              { role: "user", content: "continue after the approved provider tool" },
              {
                role: "assistant",
                content: [
                  {
                    type: "tool-call",
                    toolCallId: "provider-call-1",
                    toolName: "providerSearch",
                    input: { query: "grafana" },
                    providerExecuted: true,
                  },
                  {
                    type: "tool-approval-request",
                    approvalId: "approval-1",
                    toolCallId: "provider-call-1",
                  },
                ],
              },
              {
                role: "tool",
                content: [
                  {
                    type: "tool-approval-response",
                    approvalId: "approval-1",
                    approved: true,
                    reason: "approved",
                    providerExecuted: true,
                  },
                ],
              },
            ],
            tools: {
              echoTool: tool({
                description: "Echoes deterministic input.",
                inputSchema: z.object({ text: z.string() }),
                execute: async (input) => {
                  executedInput = input;
                  return { echoed: input.text };
                },
              }),
            },
          });
          for await (const _part of result.fullStream) {
            // Consume the complete flow so the second request is emitted.
          }
          await result.text;
          if (JSON.stringify(executedInput) !== JSON.stringify({ text: "hello" })) {
            throw new Error(`unexpected multi-step tool input ${JSON.stringify(executedInput)}`);
          }
        },
      }),
    );
    return captures;
  });
}

function presenceLossOptions(): LanguageModelV4CallOptions {
  const emptyProviderOptions = {};
  return {
    prompt: [
      { role: "system", content: "", providerOptions: emptyProviderOptions },
      {
        role: "user",
        providerOptions: emptyProviderOptions,
        content: [
          { type: "text", text: "", providerOptions: emptyProviderOptions },
          {
            type: "file",
            data: { type: "text", text: "" },
            mediaType: "",
            filename: "",
            providerOptions: emptyProviderOptions,
          },
        ],
      },
      {
        role: "assistant",
        providerOptions: emptyProviderOptions,
        content: [
          {
            type: "custom",
            kind: ".",
            providerOptions: emptyProviderOptions,
          },
          {
            type: "tool-call",
            toolCallId: "",
            toolName: "",
            input: {},
            providerExecuted: false,
            providerOptions: emptyProviderOptions,
          },
          {
            type: "tool-result",
            toolCallId: "",
            toolName: "",
            output: {
              type: "text",
              value: "",
              providerOptions: emptyProviderOptions,
            },
            providerOptions: emptyProviderOptions,
          },
        ],
      },
      {
        role: "tool",
        providerOptions: emptyProviderOptions,
        content: [
          {
            type: "tool-approval-response",
            approvalId: "",
            approved: false,
            reason: "",
            providerOptions: emptyProviderOptions,
          },
          {
            type: "tool-result",
            toolCallId: "nested-file-result",
            toolName: "lookup",
            output: {
              type: "content",
              value: [
                {
                  type: "file",
                  data: { type: "text", text: "" },
                  mediaType: "",
                  filename: "",
                  providerOptions: emptyProviderOptions,
                },
              ],
            },
          },
        ],
      },
    ],
    maxOutputTokens: 1.5,
    topK: 2.5,
    seed: 3.5,
    responseFormat: {
      type: "json",
      schema: {
        $defs: { value: { type: "string" } },
        $ref: "#/$defs/value",
      },
      name: "",
      description: "",
    },
    tools: [
      {
        type: "function",
        name: "",
        description: "",
        inputSchema: {
          $defs: { input: { type: "object", additionalProperties: false } },
          $ref: "#/$defs/input",
        },
        inputExamples: [],
        strict: false,
        providerOptions: emptyProviderOptions,
      },
      { type: "provider", id: ".", name: "", args: {} },
    ],
    toolChoice: { type: "tool", toolName: "" },
    providerOptions: emptyProviderOptions,
  };
}

function multiStepParts(sequence: number): LanguageModelV4StreamPart[] {
  if (sequence === 1) {
    const input = JSON.stringify({ text: "hello" });
    return [
      { type: "stream-start", warnings: [] },
      { type: "tool-input-start", id: "call-echo-1", toolName: "echoTool" },
      { type: "tool-input-delta", id: "call-echo-1", delta: input },
      { type: "tool-input-end", id: "call-echo-1" },
      {
        type: "tool-call",
        toolCallId: "call-echo-1",
        toolName: "echoTool",
        input,
      },
      finishPart("tool-calls"),
    ];
  }
  return [
    { type: "stream-start", warnings: [] },
    { type: "text-start", id: "text-1" },
    { type: "text-delta", id: "text-1", delta: "done" },
    { type: "text-end", id: "text-1" },
    finishPart("stop"),
  ];
}

function finishPart(
  unified: "stop" | "tool-calls",
): Extract<LanguageModelV4StreamPart, { type: "finish" }> {
  return {
    type: "finish",
    finishReason: { unified, raw: unified },
    usage: {
      inputTokens: { total: 1, noCache: 1, cacheRead: 0, cacheWrite: 0 },
      outputTokens: { total: 1, text: 1, reasoning: 0 },
    },
  };
}

function comprehensivePromptOptions(): LanguageModelV4CallOptions {
  const providerOptions = { example: { trace: "prompt", nested: null } };
  return {
    prompt: [
      { role: "system", content: "system", providerOptions },
      {
        role: "user",
        providerOptions,
        content: [
          { type: "text", text: "user", providerOptions },
          {
            type: "file",
            data: { type: "data", data: new Uint8Array([0, 1, 2, 255]) },
            mediaType: "application/octet-stream",
            filename: "",
            providerOptions,
          },
          {
            type: "file",
            data: { type: "url", url: new URL("https://example.test/file") },
            mediaType: "text/plain",
            providerOptions,
          },
          {
            type: "file",
            data: { type: "reference", reference: { example: "file-1" } },
            mediaType: "image/png",
            providerOptions,
          },
          {
            type: "file",
            data: { type: "text", text: "inline" },
            mediaType: "text/plain",
            providerOptions,
          },
        ],
      },
      {
        role: "assistant",
        providerOptions,
        content: [
          { type: "text", text: "assistant", providerOptions },
          {
            type: "file",
            data: { type: "data", data: "AQI=" },
            mediaType: "application/octet-stream",
            providerOptions,
          },
          { type: "custom", kind: "example.custom", providerOptions },
          { type: "reasoning", text: "reasoning", providerOptions },
          {
            type: "reasoning-file",
            data: { type: "data", data: new Uint8Array([3, 4]) },
            mediaType: "application/octet-stream",
            providerOptions,
          },
          {
            type: "reasoning-file",
            data: { type: "url", url: new URL("https://example.test/reasoning") },
            mediaType: "text/plain",
            providerOptions,
          },
          {
            type: "tool-call",
            toolCallId: "call-1",
            toolName: "lookup",
            input: { city: "Berlin" },
            providerExecuted: false,
            providerOptions,
          },
          toolResult("result-text", { type: "text", value: "ok", providerOptions }),
          toolResult("result-json", { type: "json", value: null, providerOptions }),
          toolResult("result-denied", {
            type: "execution-denied",
            reason: "",
            providerOptions,
          }),
          toolResult("result-error-text", {
            type: "error-text",
            value: "failed",
            providerOptions,
          }),
          toolResult("result-error-json", {
            type: "error-json",
            value: { code: 0, detail: null },
            providerOptions,
          }),
          toolResult("result-content", {
            type: "content",
            value: [
              { type: "text", text: "content", providerOptions },
              {
                type: "file",
                data: { type: "data", data: new Uint8Array([5, 6]) },
                mediaType: "application/octet-stream",
                filename: "",
                providerOptions,
              },
              { type: "custom", providerOptions },
            ],
          }),
        ],
      },
      {
        role: "tool",
        providerOptions,
        content: [
          toolResult("tool-role-result", { type: "json", value: { ok: true } }),
          {
            type: "tool-approval-response",
            approvalId: "approval-1",
            approved: false,
            reason: "",
            providerOptions,
          },
        ],
      },
    ],
    tools: [
      {
        type: "function",
        name: "lookup",
        description: "",
        inputSchema: {
          type: "object",
          properties: { city: { type: "string" } },
          required: ["city"],
          additionalProperties: false,
        },
        inputExamples: [{ input: { city: "Berlin" } }],
        strict: false,
        providerOptions,
      },
      {
        type: "provider",
        id: "example.search",
        name: "providerSearch",
        args: {},
      },
    ],
    toolChoice: { type: "auto" },
    providerOptions,
  };
}

function toolResult(
  toolCallId: string,
  output: Extract<
    Extract<LanguageModelV4CallOptions["prompt"][number], { role: "assistant" }>["content"][number],
    { type: "tool-result" }
  >["output"],
) {
  return {
    type: "tool-result" as const,
    toolCallId,
    toolName: "lookup",
    output,
    providerOptions: { example: { result: toolCallId } },
  };
}

async function withDeterministicGatewayEnvironment<T>(action: () => Promise<T>): Promise<T> {
  const names = [
    "VERCEL_DEPLOYMENT_ID",
    "VERCEL_ENV",
    "VERCEL_REGION",
    "VERCEL_PROJECT_ID",
    "VERCEL_REQUEST_ID",
  ] as const;
  const original = new Map(names.map((name) => [name, process.env[name]]));
  for (const name of names) {
    delete process.env[name];
  }
  try {
    return await action();
  } finally {
    for (const name of names) {
      const value = original.get(name);
      if (value === undefined) {
        delete process.env[name];
      } else {
        process.env[name] = value;
      }
    }
  }
}
