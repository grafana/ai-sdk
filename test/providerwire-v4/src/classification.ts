import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  firstDifference,
  serializeArtifact,
} from "./artifacts.ts";
import { baselineMetadata, type BaselineMetadata } from "./baseline.ts";
import { requestCoverage, type CoverageCategory } from "./request-coverage.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
export const classificationPath = join(packageRoot, "classification.json");

export interface ClassificationArtifact {
  schemaVersion: 1;
  baseline: BaselineMetadata;
  categories: readonly CoverageCategory[];
}

export function generateClassificationArtifact(): ClassificationArtifact {
  return {
    schemaVersion: 1,
    baseline: baselineMetadata(),
    categories: requestCoverage,
  };
}

export function readClassificationArtifact(): ClassificationArtifact {
  return JSON.parse(readFileSync(classificationPath, "utf8")) as ClassificationArtifact;
}

export function writeClassificationArtifact(artifact: ClassificationArtifact): void {
  writeFileSync(classificationPath, serializeArtifact(artifact));
}

export function compareClassificationArtifact(
  artifact: ClassificationArtifact,
  committedPath = classificationPath,
): string[] {
  const generated = serializeArtifact(artifact);
  const committed = readFileSync(committedPath, "utf8");
  if (generated === committed) {
    return [];
  }
  return [
    "ProviderWire V4 classification evidence is stale",
    firstDifference(committed, generated),
    "run mise run update-providerwire-v4-artifacts and review the generated diff",
  ];
}

export function valueAtPointer(root: unknown, pointer: string): { found: boolean; value?: unknown } {
  let value = root;
  for (const encodedSegment of pointer.slice(1).split("/")) {
    const segment = encodedSegment.replaceAll("~1", "/").replaceAll("~0", "~");
    if (Array.isArray(value)) {
      const index = Number.parseInt(segment, 10);
      if (!Number.isInteger(index) || String(index) !== segment || index >= value.length) {
        return { found: false };
      }
      value = value[index];
      continue;
    }
    if (typeof value !== "object" || value === null || !(segment in value)) {
      return { found: false };
    }
    value = (value as Record<string, unknown>)[segment];
  }
  return { found: true, value };
}
