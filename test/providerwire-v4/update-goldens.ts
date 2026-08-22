import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { requestGoldenCases } from "./request-cases.ts";
import { assertValidRequest } from "./schema.ts";

const defaultGoldenDirectory = fileURLToPath(new URL("./goldens/", import.meta.url));

export async function updateGoldens(
  outputDirectory = defaultGoldenDirectory,
): Promise<string[]> {
  mkdirSync(outputDirectory, { recursive: true });
  const resolvedOutputDirectory = resolve(outputDirectory);
  const seen = new Set<string>();
  const updated: string[] = [];

  for (const testCase of requestGoldenCases) {
    const outputPath = resolve(resolvedOutputDirectory, testCase.fileName);
    if (!outputPath.startsWith(`${resolvedOutputDirectory}/`) || seen.has(outputPath)) {
      throw new Error(`invalid golden output path: ${testCase.fileName}`);
    }
    seen.add(outputPath);

    const requests = await testCase.capture();
    for (const [index, request] of requests.entries()) {
      assertValidRequest(request.body, `${testCase.name} request ${index + 1}`);
    }
    writeFileSync(outputPath, `${JSON.stringify(requests, null, 2)}\n`);
    updated.push(testCase.fileName);
  }

  return updated;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  for (const fileName of await updateGoldens()) {
    console.log(`updated ${fileName}`);
  }
}
