import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = join(packageRoot, "../..");
const deltaPath = join(packageRoot, "phase2-delta.md");
const contractTestPattern = /^TestProviderWireV4Contract_[A-Za-z0-9_]+$/;

export function resolvedContractTests(markdown: string): Set<string> {
  const tests = new Set<string>();
  for (const line of markdown.split("\n")) {
    if (!line.startsWith("| `") || !line.includes("(resolved)")) {
      continue;
    }
    const matches = line.matchAll(/`(TestProviderWireV4Contract_[A-Za-z0-9_]+)`/g);
    const names = [...matches].map((match) => match[1]);
    if (names.length !== 1) {
      throw new Error(`resolved delta row must name exactly one contract test: ${line}`);
    }
    tests.add(names[0]);
  }
  if (tests.size === 0) {
    throw new Error("resolved delta table contains no provider contract tests");
  }
  return tests;
}

export function listedContractTests(output: string): Set<string> {
  return new Set(
    output
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => contractTestPattern.test(line)),
  );
}

export function compareContractTestSets(expected: Set<string>, actual: Set<string>): string[] {
  const errors: string[] = [];
  for (const name of expected) {
    if (!actual.has(name)) {
      errors.push(`missing provider contract test ${name}`);
    }
  }
  for (const name of actual) {
    if (!expected.has(name)) {
      errors.push(`unexpected provider contract test ${name}`);
    }
  }
  return errors;
}

export function verifyProviderContractTests(): void {
  const expected = resolvedContractTests(readFileSync(deltaPath, "utf8"));
  const listed = execFileSync(
    "go",
    ["test", "./provider", "-list", "^TestProviderWireV4Contract_"],
    { cwd: repositoryRoot, encoding: "utf8" },
  );
  const errors = compareContractTestSets(expected, listedContractTests(listed));
  if (errors.length > 0) {
    throw new Error(errors.join("\n"));
  }
  execFileSync("go", ["test", "./provider"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: "inherit",
  });
}
