import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { ScriptedResponse } from "./recorder";

interface PositiveCase {
  name: string;
  status?: number;
  document: unknown;
}

interface PositiveCorpus {
  cases: PositiveCase[];
}

const PROJECTION_DIR = resolve(import.meta.dirname, "projections");
const POSITIVE_CORPUS_PATH = resolve(import.meta.dirname, "../../../gateway/providerwire/v4/testdata/positive.json");

export function unaryProjection(): string {
  return readFileSync(resolve(PROJECTION_DIR, "unary.json"), "utf8");
}

export function cleanStreamProjection(): string {
  return readFileSync(resolve(PROJECTION_DIR, "stream-clean.sse"), "utf8");
}

export function doneStreamProjection(): string {
  return `${cleanStreamProjection()}data: [DONE]\n\n`;
}

export function positiveProjection(name: string): { status?: number; body: string } {
  const corpus = JSON.parse(readFileSync(POSITIVE_CORPUS_PATH, "utf8")) as PositiveCorpus;
  const fixture = corpus.cases.find((candidate) => candidate.name === name);
  if (fixture === undefined) {
    throw new Error(`unknown positive ProviderWire fixture ${JSON.stringify(name)}`);
  }
  return { status: fixture.status, body: JSON.stringify(fixture.document) };
}

export function unaryScriptedResponse(): ScriptedResponse {
  return { contentType: "application/json", body: unaryProjection() };
}

export function streamScriptedResponse(): ScriptedResponse {
  return { contentType: "text/event-stream", body: cleanStreamProjection() };
}
