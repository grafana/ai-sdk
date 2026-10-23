import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { jsonSchema, Output, streamText } from "../../tools/node_modules/ai/src/index.ts";

const cases: Record<string, { minItems?: number; maxItems?: number }> = {
  within: { minItems: 1, maxItems: 2 },
  overflow: { maxItems: 2 },
  "below-minimum": { minItems: 2 },
};

function innermostCause(error: unknown): string {
  let current = error as { cause?: unknown; message?: string };
  while (current?.cause != null) {
    if (typeof current.cause === "string") return current.cause;
    current = current.cause as typeof current;
  }
  return String(current?.message ?? current);
}

const fixtureDir = dirname(fileURLToPath(import.meta.url));
for (const [name, bounds] of Object.entries(cases)) {
  const caseDir = join(fixtureDir, name);
  const chunks = readFileSync(join(caseDir, "input.jsonl"), "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line));
  let responseFormat: unknown;
  const result = streamText({
    model: {
      specificationVersion: "v4",
      provider: "test",
      modelId: "test-model",
      supportedUrls: {},
      doStream: async (options: { responseFormat: unknown }) => {
        responseFormat = options.responseFormat;
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
    output: Output.array({
      element: jsonSchema({
        type: "object",
        properties: { name: { type: "string" } },
        required: ["name"],
        additionalProperties: false,
      }),
      ...bounds,
    }),
  });

  const elements: unknown[] = [];
  let elementStreamError: string | null = null;
  let outputError: string | null = null;
  const ui: string[] = [];
  await Promise.all([
    (async () => {
      try {
        for await (const element of result.elementStream) elements.push(element);
      } catch (error) {
        elementStreamError = innermostCause(error);
      }
    })(),
    result.output.then(
      () => {},
      (error) => {
        outputError = innermostCause(error);
      },
    ),
    (async () => {
      for await (const chunk of result.toUIMessageStream({
        generateMessageId: () => "message-1",
      })) {
        ui.push(JSON.stringify(chunk));
      }
    })(),
  ]);

  writeFileSync(join(caseDir, "expected.jsonl"), `${ui.join("\n")}\n`);
  writeFileSync(
    join(caseDir, "expected-output.json"),
    `${JSON.stringify({ responseFormat, elements, elementStreamError, outputError }, null, 2)}\n`,
  );
}
