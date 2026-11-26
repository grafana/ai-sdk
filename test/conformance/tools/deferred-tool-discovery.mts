import assert from "node:assert/strict";
import { mkdirSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  experimental_toolCaller,
  jsonSchema,
  stepCountIs,
  streamText,
  tool,
  toolSearch,
} from "ai";
import { MockLanguageModelV4 } from "ai/test";
import type {
  LanguageModelV4CallOptions,
  LanguageModelV4StreamPart,
} from "@ai-sdk/provider";

const objectSchema = {
  type: "object",
  properties: {},
  additionalProperties: false,
} as const;
const usage = {
  inputTokens: { total: 1, noCache: 1, cacheRead: undefined, cacheWrite: undefined },
  outputTokens: { total: 1, text: 1, reasoning: undefined },
};

for (const mode of ["direct", "nested", "direct-ranked", "nested-ranked"] as const) {
  const nested = mode.startsWith("nested");
  const ranking = mode.endsWith("-ranked") ? {
    maxResults: 2,
    names: ["unknown", "rainRisk", "rainRisk", "search", "getWeather", "unrelated"],
  } : undefined;
  const query = ranking ? "meteorology" : "weather";
  const steps = !nested
    ? [
        [
          { id: "search", name: "search", input: { query } },
          { id: "too-early", name: "getWeather", input: {} },
        ],
        [{ id: "weather", name: "getWeather", input: {} }],
        [],
      ]
    : [
        [{
          id: "search", name: "code",
          input: { name: "search", input: { query } },
        }],
        [{
          id: "weather", name: "code",
          input: { name: "getWeather", input: {} },
        }],
        [],
      ];
  const requests: unknown[] = [];
  let executions = 0;
  const model = new MockLanguageModelV4({
    doStream: async (options: LanguageModelV4CallOptions) => {
      const step = requests.length;
      if (step === 1) assert.equal(executions, 0);
      requests.push({
        tools: [...(options.tools ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
        prompt: options.prompt,
        toolChoice: options.toolChoice,
      });
      const parts: LanguageModelV4StreamPart[] = steps[step].map(call => ({
        type: "tool-call",
        toolCallId: call.id,
        toolName: call.name,
        input: JSON.stringify(call.input),
      }));
      if (step === 2) parts.push(
        { type: "text-start", id: "text" },
        { type: "text-delta", id: "text", delta: "sunny" },
        { type: "text-end", id: "text" },
      );
      parts.push({
        type: "finish", usage,
        finishReason: { unified: step < 2 ? "tool-calls" : "stop", raw: undefined },
      });
      return {
        stream: new ReadableStream({
          start(controller) {
            for (const part of parts) controller.enqueue(part);
            controller.close();
          },
        }),
      };
    },
  });
  const callerSchema = jsonSchema<{ name: string; input: Record<string, unknown> }>({
    type: "object",
    properties: { name: { type: "string" }, input: { type: "object" } },
    required: ["name", "input"],
    additionalProperties: false,
  });
  const tools = {
    search: toolSearch(ranking && {
      maxResults: ranking.maxResults,
      search: ({ query, tools }) => {
        assert.equal(query, "meteorology");
        assert.deepEqual(tools, [
          { name: "getWeather", description: "Weather forecast" },
          { name: "unrelated", description: "Send email" },
          { name: "rainRisk", description: "Chance of rain" },
        ]);
        return ranking.names;
      },
    }),
    getWeather: tool({
      deferLoading: true,
      description: "Weather forecast",
      inputSchema: jsonSchema(objectSchema),
      execute: () => { executions++; return "sunny"; },
    }),
    unrelated: tool({
      deferLoading: true,
      description: "Send email",
      inputSchema: jsonSchema(objectSchema),
    }),
    ...(ranking && { rainRisk: tool({
      deferLoading: true,
      description: "Chance of rain",
      inputSchema: jsonSchema(objectSchema),
    }) }),
  };
  const code = experimental_toolCaller(
    tool({ description: "Stable code tool.", inputSchema: callerSchema }),
    {
      type: "local",
      bind: registry => tool({
        description: "Bound tool",
        inputSchema: callerSchema,
        execute: async ({ name, input }, options) =>
          await registry[name].execute!(input, options),
      }),
      prepareModelMessage: registry =>
        `Catalog: ${Object.keys(registry).sort().join(", ")}`,
    },
  );
  const settings = { model, prompt: "Find the weather.", stopWhen: stepCountIs(3) };
  const result = nested
    ? streamText({
        ...settings,
        tools: { ...tools, code },
        experimental_toolCallers: {
          search: ["code"], getWeather: ["code"], unrelated: ["code"],
          ...(ranking && { rainRisk: ["code"] }),
        },
      })
    : streamText({ ...settings, tools });
  const chunks = [];
  for await (const chunk of result.toUIMessageStream({
    generateMessageId: () => "message-1",
  })) chunks.push(chunk);
  assert.equal(await result.text, "sunny");
  assert.equal(executions, 1);
  assert.equal(requests.length, 3);
  const dir = fileURLToPath(new URL(
    `../ui/deferred-tool-discovery/${mode}/`, import.meta.url,
  ));
  mkdirSync(dir, { recursive: true });
  writeFileSync(dir + "scenario.json", JSON.stringify({ mode, steps, ranking }, null, 2) + "\n");
  writeFileSync(dir + "expected.jsonl", chunks.map(chunk => JSON.stringify(chunk)).join("\n") + "\n");
  writeFileSync(dir + "expected-requests.jsonl", requests.map(request => JSON.stringify(request)).join("\n") + "\n");
}
