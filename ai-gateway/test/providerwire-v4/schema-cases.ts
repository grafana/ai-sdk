type SchemaCase = {
  name: string;
  value: unknown;
};

const options = {
  provider: {
    nested: { nullValue: null, falseValue: false, zero: 0, empty: "" },
    array: [null, false, 0, "", [], {}],
  },
};

const richRequest = {
  prompt: [
    { role: "system", content: "", providerOptions: options },
    {
      role: "user",
      providerOptions: options,
      content: [
        { type: "text", text: "", providerOptions: options },
        {
          type: "file",
          filename: "",
          data: { type: "data", data: "" },
          mediaType: "application/octet-stream",
          providerOptions: options,
        },
        {
          type: "file",
          data: { type: "url", url: "https://example.com/%" },
          mediaType: "text/plain",
        },
        {
          type: "file",
          data: { type: "reference", reference: { openai: "file-1" } },
          mediaType: "application/pdf",
        },
        {
          type: "file",
          data: { type: "text", text: "" },
          mediaType: "text/plain",
        },
      ],
    },
    {
      role: "assistant",
      providerOptions: options,
      content: [
        { type: "text", text: "assistant", providerOptions: options },
        {
          type: "file",
          data: { type: "data", data: "AAE=" },
          mediaType: "image/png",
        },
        { type: "custom", kind: "provider.kind", providerOptions: options },
        { type: "reasoning", text: "", providerOptions: options },
        {
          type: "reasoning-file",
          data: { type: "data", data: "" },
          mediaType: "application/octet-stream",
          providerOptions: options,
        },
        {
          type: "reasoning-file",
          data: { type: "url", url: "https://example.test/reasoning" },
          mediaType: "application/pdf",
        },
        {
          type: "tool-call",
          toolCallId: "call-1",
          toolName: "lookup",
          input: { empty: "", nullValue: null },
          providerExecuted: false,
          providerOptions: options,
        },
        {
          type: "tool-result",
          toolCallId: "call-text",
          toolName: "lookup",
          output: { type: "text", value: "", providerOptions: options },
          providerOptions: options,
        },
        {
          type: "tool-result",
          toolCallId: "call-json",
          toolName: "lookup",
          output: { type: "json", value: null, providerOptions: options },
        },
        {
          type: "tool-result",
          toolCallId: "call-denied",
          toolName: "lookup",
          output: { type: "execution-denied", reason: "", providerOptions: options },
        },
        {
          type: "tool-result",
          toolCallId: "call-error-text",
          toolName: "lookup",
          output: { type: "error-text", value: "", providerOptions: options },
        },
        {
          type: "tool-result",
          toolCallId: "call-error-json",
          toolName: "lookup",
          output: { type: "error-json", value: { code: 0 }, providerOptions: options },
        },
        {
          type: "tool-result",
          toolCallId: "call-content",
          toolName: "lookup",
          output: {
            type: "content",
            value: [
              { type: "text", text: "", providerOptions: options },
              {
                type: "file",
                filename: "",
                data: { type: "reference", reference: { provider: "file-2" } },
                mediaType: "application/pdf",
                providerOptions: options,
              },
              { type: "custom", providerOptions: options },
            ],
          },
        },
      ],
    },
    {
      role: "tool",
      providerOptions: options,
      content: [
        {
          type: "tool-result",
          toolCallId: "call-tool",
          toolName: "lookup",
          output: {
            type: "content",
            value: [
              {
                type: "file",
                data: { type: "text", text: "" },
                mediaType: "text/plain",
              },
              {
                type: "file",
                data: { type: "url", url: "https://example.test/result" },
                mediaType: "text/plain",
              },
              {
                type: "file",
                data: { type: "data", data: "" },
                mediaType: "application/octet-stream",
              },
            ],
          },
        },
        {
          type: "tool-approval-response",
          approvalId: "approval-1",
          approved: false,
          reason: "",
          providerOptions: options,
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
    schema: {
      type: "object",
      properties: { value: { type: "string" } },
      required: ["value"],
    },
    name: "",
    description: "",
  },
  seed: 0,
  tools: [
    {
      type: "function",
      name: "lookup",
      description: "",
      inputSchema: {
        type: "object",
        properties: { value: { type: "string" } },
      },
      inputExamples: [{ input: { value: "", nested: { nullValue: null } } }],
      strict: false,
      providerOptions: options,
    },
    {
      type: "provider",
      id: "provider.search",
      name: "search",
      args: { limit: 0, enabled: false, nested: { value: null } },
    },
  ],
  toolChoice: { type: "auto" },
  includeRawChunks: false,
  headers: { "x-empty": "" },
  reasoning: "provider-default",
  providerOptions: options,
};

function withValue(mutator: (value: Record<string, any>) => void): unknown {
  const value = structuredClone(richRequest) as Record<string, any>;
  mutator(value);
  return value;
}

export const validRequests: SchemaCase[] = [
  { name: "complete request", value: richRequest },
  { name: "minimal request", value: { prompt: [] } },
  {
    name: "explicit empty collections",
    value: { prompt: [], tools: [], stopSequences: [], headers: {}, providerOptions: {} },
  },
  {
    name: "empty provider reference",
    value: {
      prompt: [
        {
          role: "user",
          content: [
            {
              type: "file",
              data: { type: "reference", reference: {} },
              mediaType: "application/pdf",
            },
          ],
        },
      ],
    },
  },
  { name: "text response format", value: withValue((value) => (value.responseFormat = { type: "text" })) },
  { name: "none tool choice", value: withValue((value) => (value.toolChoice = { type: "none" })) },
  {
    name: "required tool choice",
    value: withValue((value) => (value.toolChoice = { type: "required" })),
  },
  {
    name: "named tool choice",
    value: withValue((value) => (value.toolChoice = { type: "tool", toolName: "lookup" })),
  },
  ...["none", "minimal", "low", "medium", "high", "xhigh"].map((reasoning) => ({
    name: `${reasoning} reasoning`,
    value: withValue((value) => (value.reasoning = reasoning)),
  })),
  {
    name: "minimal dotted identifiers",
    value: {
      prompt: [{ role: "assistant", content: [{ type: "custom", kind: "." }] }],
      tools: [{ type: "provider", id: ".", name: "tool", args: {} }],
    },
  },
];

export const invalidRequests: SchemaCase[] = [
  { name: "missing prompt", value: {} },
  { name: "unknown root member", value: { prompt: [], unknown: true } },
  { name: "abort signal member", value: { prompt: [], abortSignal: {} } },
  { name: "typed null", value: { prompt: [], temperature: null } },
  { name: "fractional max output tokens", value: { prompt: [], maxOutputTokens: 1.5 } },
  { name: "fractional top k", value: { prompt: [], topK: 1.5 } },
  { name: "fractional seed", value: { prompt: [], seed: 1.5 } },
  { name: "array provider namespace", value: { prompt: [], providerOptions: { provider: [] } } },
  { name: "null provider namespace", value: { prompt: [], providerOptions: { provider: null } } },
  { name: "scalar provider namespace", value: { prompt: [], providerOptions: { provider: 1 } } },
  {
    name: "unknown message member",
    value: { prompt: [{ role: "system", content: "", unknown: true }] },
  },
  {
    name: "role incompatible content",
    value: { prompt: [{ role: "user", content: [{ type: "reasoning", text: "" }] }] },
  },
  {
    name: "unknown content discriminator",
    value: { prompt: [{ role: "assistant", content: [{ type: "future" }] }] },
  },
  {
    name: "mixed file data arms",
    value: {
      prompt: [
        {
          role: "user",
          content: [
            {
              type: "file",
              data: { type: "data", data: "", url: "https://example.test" },
              mediaType: "text/plain",
            },
          ],
        },
      ],
    },
  },
  {
    name: "reasoning file reference arm",
    value: {
      prompt: [
        {
          role: "assistant",
          content: [
            {
              type: "reasoning-file",
              data: { type: "reference", reference: { provider: "file" } },
              mediaType: "application/pdf",
            },
          ],
        },
      ],
    },
  },
  {
    name: "provider reference type property",
    value: {
      prompt: [
        {
          role: "user",
          content: [
            {
              type: "file",
              data: { type: "reference", reference: { type: "file", provider: "id" } },
              mediaType: "application/pdf",
            },
          ],
        },
      ],
    },
  },
  {
    name: "undotted provider tool id",
    value: {
      prompt: [],
      tools: [{ type: "provider", id: "provider", name: "search", args: {} }],
    },
  },
  {
    name: "undotted custom part kind",
    value: {
      prompt: [{ role: "assistant", content: [{ type: "custom", kind: "provider" }] }],
    },
  },
  {
    name: "inactive tool result member",
    value: {
      prompt: [
        {
          role: "tool",
          content: [
            {
              type: "tool-result",
              toolCallId: "call",
              toolName: "lookup",
              output: { type: "execution-denied", value: "not allowed" },
            },
          ],
        },
      ],
    },
  },
  {
    name: "text response format with json member",
    value: { prompt: [], responseFormat: { type: "text", schema: {} } },
  },
  {
    name: "missing approval decision",
    value: {
      prompt: [
        {
          role: "tool",
          content: [{ type: "tool-approval-response", approvalId: "approval" }],
        },
      ],
    },
  },
  {
    name: "provider tool provider options",
    value: {
      prompt: [],
      tools: [
        {
          type: "provider",
          id: "provider.search",
          name: "search",
          args: {},
          providerOptions: {},
        },
      ],
    },
  },
  { name: "unknown tool choice", value: { prompt: [], toolChoice: { type: "future" } } },
  { name: "named tool choice missing name", value: { prompt: [], toolChoice: { type: "tool" } } },
  {
    name: "inactive named tool choice member",
    value: { prompt: [], toolChoice: { type: "auto", toolName: "tool" } },
  },
  { name: "system array content", value: { prompt: [{ role: "system", content: [] }] } },
  { name: "boolean response schema", value: { prompt: [], responseFormat: { type: "json", schema: true } } },
  {
    name: "unknown json response format member",
    value: { prompt: [], responseFormat: { type: "json", unknown: true } },
  },
  { name: "null input schema", value: { prompt: [], tools: [{ type: "function", name: "x", inputSchema: null }] } },
  { name: "non-string body header", value: { prompt: [], headers: { "x-value": 1 } } },
  {
    name: "tool result custom kind",
    value: {
      prompt: [
        {
          role: "tool",
          content: [
            {
              type: "tool-result",
              toolCallId: "call",
              toolName: "lookup",
              output: { type: "content", value: [{ type: "custom", kind: "provider.kind" }] },
            },
          ],
        },
      ],
    },
  },
];
