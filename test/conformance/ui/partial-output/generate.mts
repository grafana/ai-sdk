import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  jsonSchema,
  Output,
  stepCountIs,
  streamText,
  tool,
} from "../../tools/node_modules/ai/src/index.ts";

const nameSchema = jsonSchema({
  type: "object",
  properties: { name: { type: "string" } },
  required: ["name"],
  additionalProperties: false,
});

const cases = {
  "empty-array": Output.array({ element: nameSchema }),
  "multi-step-repeat": Output.object({ schema: nameSchema }),
};

function readParts(path: string) {
  return readFileSync(path, "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line));
}

const fixtureDir = dirname(fileURLToPath(import.meta.url));
for (const [name, output] of Object.entries(cases)) {
  const caseDir = join(fixtureDir, name);
  const multiStep = existsSync(join(caseDir, "input-1.jsonl"));
  const steps = multiStep
    ? [readParts(join(caseDir, "input-1.jsonl")), readParts(join(caseDir, "input-2.jsonl"))]
    : [readParts(join(caseDir, "input.jsonl"))];
  let call = 0;
  const result = streamText({
    model: {
      specificationVersion: "v4",
      provider: "test",
      modelId: "test-model",
      supportedUrls: {},
      doStream: async () => {
        const chunks = steps[call++];
        return {
          stream: new ReadableStream({
            start(controller) {
              for (const chunk of chunks) controller.enqueue(chunk);
              controller.close();
            },
          }),
        };
      },
    } as never,
    prompt: "test",
    maxRetries: 0,
    ...(multiStep && {
      tools: {
        lookup: tool({
          inputSchema: jsonSchema({ type: "object" }),
          execute: async () => ({}),
        }),
      },
      stopWhen: stepCountIs(3),
    }),
    output,
  });

  const partials: unknown[] = [];
  const ui: string[] = [];
  await Promise.all([
    (async () => {
      for await (const partial of result.partialOutputStream) partials.push(partial);
    })(),
    (async () => {
      for await (const chunk of result.toUIMessageStream({
        generateMessageId: () => "message-1",
      })) {
        ui.push(JSON.stringify(chunk));
      }
    })(),
  ]);

  writeFileSync(join(caseDir, "expected.jsonl"), `${ui.join("\n")}\n`);
  writeFileSync(join(caseDir, "expected-partials.json"), `${JSON.stringify(partials, null, 2)}\n`);
}
