import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { baselineMetadata, type BaselineMetadata } from "./baseline.ts";
import { generateScenarioCaptures } from "./scenarios.ts";
import type { ScenarioCapture } from "./capture.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
export const semanticRequestsPath = join(packageRoot, "artifacts", "semantic-requests.json");

export interface SemanticRequestsArtifact {
  schemaVersion: 1;
  baseline: BaselineMetadata;
  capturePolicy: {
    path: string;
    normalizedHeaders: string[];
    excludedTransportHeaders: string[];
    preserved: string[];
  };
  scenarios: ScenarioCapture[];
}

export async function generateSemanticRequestsArtifact(): Promise<SemanticRequestsArtifact> {
  return {
    schemaVersion: 1,
    baseline: baselineMetadata(),
    capturePolicy: {
      path: "relative to configured baseURL; requestPath retains /api/v1/aisdk composition",
      normalizedHeaders: ["authorization", "user-agent"],
      excludedTransportHeaders: [
        "accept",
        "accept-encoding",
        "accept-language",
        "connection",
        "content-length",
        "host",
        "sec-fetch-mode",
      ],
      preserved: [
        "array and request order",
        "absence and null",
        "empty strings, arrays, and objects",
        "zero and false",
        "union selection",
        "non-client-owned outer header values and lower-case last-value outcomes",
      ],
    },
    scenarios: await generateScenarioCaptures(),
  };
}

export function serializeArtifact(artifact: unknown): string {
  return `${JSON.stringify(sortObjectKeys(artifact), null, 2)}\n`;
}

export function writeSemanticRequestsArtifact(artifact: SemanticRequestsArtifact): void {
  writeFileSync(semanticRequestsPath, serializeArtifact(artifact));
}

export function compareSemanticRequestsArtifact(
  artifact: SemanticRequestsArtifact,
  committedPath = semanticRequestsPath,
): string[] {
  const generated = serializeArtifact(artifact);
  const committed = readFileSync(committedPath, "utf8");
  if (generated === committed) {
    return [];
  }
  return [
    "ProviderWire V4 semantic request evidence is stale",
    firstDifference(committed, generated),
    "run mise run update-providerwire-v4-artifacts and review the generated diff",
  ];
}

function sortObjectKeys(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sortObjectKeys);
  }
  if (typeof value !== "object" || value === null) {
    return value;
  }
  return Object.fromEntries(
    Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, child]) => [key, sortObjectKeys(child)]),
  );
}

export function firstDifference(left: string, right: string): string {
  const leftLines = left.split("\n");
  const rightLines = right.split("\n");
  const length = Math.max(leftLines.length, rightLines.length);
  for (let index = 0; index < length; index += 1) {
    if (leftLines[index] !== rightLines[index]) {
      return `first difference at line ${index + 1}: committed=${JSON.stringify(leftLines[index])} generated=${JSON.stringify(rightLines[index])}`;
    }
  }
  return "artifact byte length differs";
}
