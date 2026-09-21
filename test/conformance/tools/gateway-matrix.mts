import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import type { TestCase } from "./common.mts";

export type Client = "typescript" | "go";
export type Stage = "provider-setup" | "execution" | "comparison" | "harness";
export interface Row extends TestCase {
  id: string;
  client: Client;
}
export interface RowResult extends Row {
  outcome: "passed" | "failed" | "not-executed";
  stage: Stage;
  invoked: boolean;
  errors: string[];
  artifact?: string;
}

export function discoverCases(root: string): TestCase[] {
  const cases: TestCase[] = [];
  for (const provider of readdirSync(root, { withFileTypes: true })) {
    if (!provider.isDirectory() || provider.name === "ui") continue;
    for (const category of ["upstream", "recorded"]) {
      const directory = join(root, provider.name, category);
      if (!existsSync(directory)) continue;
      for (const entry of readdirSync(directory, { withFileTypes: true })) {
        const dir = join(directory, entry.name);
        if (entry.isDirectory() && existsSync(join(dir, "config.yaml"))) {
          cases.push({ name: `${provider.name}/${category}/${entry.name}`, dir, provider: provider.name });
        }
      }
    }
  }
  return cases.sort((a, b) => a.name.localeCompare(b.name));
}

export function discoverMatrix(root: string, filter: { scenario?: string; client?: Client } = {}) {
  const cases = discoverCases(root).filter(tc => !filter.scenario || tc.name.includes(filter.scenario));
  if (!cases.length) throw new Error("no fixtures matched gateway inventory");
  const clients: Client[] = filter.client ? [filter.client] : ["typescript", "go"];
  return {
    scope: filter.scenario || filter.client ? "filtered" : "full",
    filter,
    rows: cases.flatMap(tc => clients.map(client => ({ ...tc, client, id: `${tc.name}/${client}` }))),
  };
}

export function reconcile(rows: Row[], results: RowResult[]): string[] {
  const expected = new Set(rows.map(row => row.id));
  const seen = new Set<string>();
  const errors: string[] = [];
  if (!rows.length) errors.push("empty inventory");
  if (expected.size !== rows.length) errors.push("duplicate inventory rows");
  for (const result of results) {
    if (!expected.has(result.id)) errors.push(`unexpected result: ${result.id}`);
    if (seen.has(result.id)) errors.push(`duplicate result: ${result.id}`);
    seen.add(result.id);
  }
  for (const id of expected) if (!seen.has(id)) errors.push(`missing result: ${id}`);
  return errors;
}

export async function runMatrix(rows: Row[], attempt: (row: Row) => Promise<RowResult>, signal: AbortSignal, record: (row: RowResult) => void = () => {}) {
  const results: RowResult[] = [];
  for (const row of rows) {
    let result: RowResult;
    if (signal.aborted) {
      result = { ...row, outcome: "not-executed", stage: "harness", invoked: false, errors: ["run canceled before invocation"] };
    } else {
      try {
        result = await attempt(row);
      } catch (error) {
        result = { ...row, outcome: "failed", stage: "harness", invoked: false, errors: [String(error)] };
      }
    }
    results.push(result);
    record(result);
  }
  return results;
}

export function summarize(results: RowResult[]): string {
  const groups = new Map<string, RowResult[]>();
  for (const row of results) {
    const key = `${row.provider} / ${row.client}`;
    groups.set(key, [...(groups.get(key) ?? []), row]);
  }
  const lines = ["# Gateway conformance (advisory)", "", "| Provider / client | Passed | Failed | Not executed | Invoked |", "| --- | ---: | ---: | ---: | ---: |"];
  for (const [key, rows] of groups) {
    lines.push(`| ${key} | ${rows.filter(r => r.outcome === "passed").length} | ${rows.filter(r => r.outcome === "failed").length} | ${rows.filter(r => r.outcome === "not-executed").length} | ${rows.filter(r => r.invoked).length} |`);
  }
  lines.push("", `${results.length} rows; ${results.filter(r => r.outcome === "passed").length} passed.`, "", "## Failures", "");
  for (const row of results.filter(r => r.outcome !== "passed")) {
    lines.push(`- ${row.id}: ${row.stage}; ${row.invoked ? "client invoked" : "client not invoked"}${row.artifact ? ` (evidence: \`${row.artifact}\`)` : ""}`);
  }
  return lines.join("\n") + "\n";
}
