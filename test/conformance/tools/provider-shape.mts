#!/usr/bin/env tsx

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";
import { parse as parseYaml } from "yaml";

const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const providerPackage = "@ai-sdk/provider";
const v4 = "language-model/v4";
const checks = [
  ["stream part", `${v4}/language-model-v4-stream-part.ts`, "LanguageModelV4StreamPart", "type", "StreamPartType"],
  ["content part", `${v4}/language-model-v4-content.ts`, "LanguageModelV4Content", "type", "GenerateContentType"],
  ["finish reason", `${v4}/language-model-v4-finish-reason.ts`, "LanguageModelV4FinishReason", "unified", "UnifiedFinishReason"],
  ["tool result output", `${v4}/language-model-v4-prompt.ts`, "LanguageModelV4ToolResultOutput", "type", "ToolResultOutputType"],
  ["warning", "shared/v4/shared-v4-warning.ts", "SharedV4Warning", "type", "WarningType"],
] as const;

type Check = typeof checks[number][0];
type Direction = "upstream-only" | "Go-only";
type ShapeException = { id: string; rationale: string; version: string; check: Check; direction: Direction; value: string };
type Statement = ReturnType<typeof parse>["program"]["body"][number];
type Declaration = Extract<Statement, { type: "TSTypeAliasDeclaration" | "TSInterfaceDeclaration" }>;
type TypeNode = Extract<Declaration, { type: "TSTypeAliasDeclaration" }>["typeAnnotation"];
type Members = Extract<TypeNode, { type: "TSTypeLiteral" }>["members"];
type Source = { declarations: Map<string, Declaration>; imports: Map<string, { specifier: string; name: string }> };

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function providerSource(version: string): string {
  const override = process.argv.find((arg) => arg.startsWith("--upstream-root="));
  let manifestPath: string;
  if (override !== undefined) {
    const explicit = override.slice("--upstream-root=".length);
    if (!explicit) throw new Error("--upstream-root requires a provider source location");
    const root = resolve(explicit.replace(/^~(?=$|\/)/, homedir()));
    const packageRoot = basename(root) === "src" ? dirname(root) : root;
    const direct = join(packageRoot, "package.json");
    const checkout = join(root, "packages", "provider", "package.json");
    manifestPath = existsSync(checkout) ? checkout : direct;
  } else {
    try {
      manifestPath = fileURLToPath(import.meta.resolve(`${providerPackage}/package.json`));
    } catch {
      throw new Error(`registered ${providerPackage}@${version} is not installed; run mise deps install --only test`);
    }
  }
  const manifest = record(JSON.parse(readFileSync(manifestPath, "utf8")));
  if (manifest?.name !== providerPackage || manifest.version !== version) {
    throw new Error(`expected ${providerPackage}@${version} at ${manifestPath}, found ${manifest?.name}@${manifest?.version}`);
  }
  return join(dirname(manifestPath), "src");
}

function shapeExceptions(gaps: unknown): ShapeException[] {
  if (gaps === undefined) return [];
  if (!Array.isArray(gaps)) throw new Error("upstream baseline knownGaps must be a list");
  const seen = new Set<string>();
  return gaps.flatMap((item) => {
    const gap = record(item);
    if (!gap || !Object.hasOwn(gap, "providerShapeException")) return [];
    const entry = record(gap.providerShapeException);
    const { id, description } = gap;
    if (!entry || typeof id !== "string" || !id.trim() || typeof description !== "string" || !description.trim()) {
      throw new Error(`invalid provider shape exception in known gap ${id}; expected an ID and rationale`);
    }
    const { version, check, direction, value } = entry;
    const checkName = checks.find(([name]) => name === check)?.[0];
    if (typeof version !== "string" || !version.trim() || !checkName ||
        (direction !== "upstream-only" && direction !== "Go-only") || typeof value !== "string" || !value.trim()) {
      throw new Error(`invalid provider shape exception in known gap ${id}; expected version, check, direction and discriminator`);
    }
    const key = JSON.stringify([version, checkName, direction, value]);
    if (seen.has(key)) throw new Error(`duplicate provider shape exception in known gap ${id}`);
    seen.add(key);
    return [{ id, rationale: description, version, check: checkName, direction, value }];
  });
}

function providerTypes(root: string) {
  const sources = new Map<string, Source>();
  function source(path: string): Source {
    const prior = sources.get(path);
    if (prior) return prior;
    const declarations = new Map<string, Declaration>();
    const imports: Source["imports"] = new Map();
    const { program } = parse(readFileSync(path, "utf8"), { sourceType: "module", plugins: ["typescript"] });
    for (const statement of program.body) {
      if (statement.type === "ImportDeclaration") {
        for (const spec of statement.specifiers) {
          if (spec.type === "ImportSpecifier") {
            const name = spec.imported.type === "Identifier" ? spec.imported.name : spec.imported.value;
            imports.set(spec.local.name, { specifier: statement.source.value, name });
          }
        }
      }
      const declaration = statement.type === "ExportNamedDeclaration" ? statement.declaration : statement;
      if (declaration?.type === "TSTypeAliasDeclaration" || declaration?.type === "TSInterfaceDeclaration") {
        if (declarations.has(declaration.id.name)) throw new Error(`${path}: duplicate type ${declaration.id.name}`);
        declarations.set(declaration.id.name, declaration);
      }
    }
    const result = { declarations, imports };
    sources.set(path, result);
    return result;
  }

  function literalValues(type: TypeNode, path: string): string[] {
    if (type.type === "TSUnionType") return type.types.flatMap((arm) => literalValues(arm, path));
    if (type.type === "TSParenthesizedType") return literalValues(type.typeAnnotation, path);
    if (type.type === "TSLiteralType" && type.literal.type === "StringLiteral") return [type.literal.value];
    throw new Error(`${path}: expected a string-literal discriminator, found ${type.type}`);
  }

  return (file: string, name: string, property: string): Set<string> => {
    const active = new Set<string>();
    function members(nodes: Members, path: string): string[] {
      let values: string[] = [];
      for (const node of nodes) {
        if (node.type !== "TSPropertySignature") continue;
        if (node.computed && node.key.type !== "StringLiteral") throw new Error(`${path}: unsupported computed property`);
        if ((node.key.type === "Identifier" && node.key.name === property) ||
            (node.key.type === "StringLiteral" && node.key.value === property)) {
          if (values.length || node.optional || !node.typeAnnotation) {
            throw new Error(`${path}: expected a single required ${property} discriminator`);
          }
          values = literalValues(node.typeAnnotation.typeAnnotation, path);
        }
      }
      return values;
    }
    function named(path: string, name: string): string[] {
      const key = `${path}:${name}`;
      if (active.has(key)) throw new Error(`${path}: circular discriminator reference to ${name}`);
      active.add(key);
      try {
        const { imports, declarations } = source(path);
        const declaration = declarations.get(name);
        if (declaration?.type === "TSTypeAliasDeclaration" && !declaration.typeParameters) {
          return collect(declaration.typeAnnotation, path);
        }
        if (declaration?.type === "TSInterfaceDeclaration" && !declaration.typeParameters && !declaration.extends?.length) {
          return members(declaration.body.body, path);
        }
        const imported = imports.get(name);
        if (imported?.specifier.startsWith(".")) {
          const target = resolve(dirname(path), `${imported.specifier.replace(/\.(?:ts|js)$/, "")}.ts`);
          const fromRoot = relative(root, target);
          if (fromRoot === ".." || fromRoot.startsWith(`..${sep}`)) {
            throw new Error(`${path}: ${name} resolves outside the provider source`);
          }
          return named(target, imported.name);
        }
        throw new Error(`${path}: cannot resolve discriminator type ${name}`);
      } finally {
        active.delete(key);
      }
    }
    function collect(type: TypeNode, path: string): string[] {
      switch (type.type) {
        case "TSTypeLiteral": return members(type.members, path);
        case "TSParenthesizedType": return collect(type.typeAnnotation, path);
        case "TSUnionType": {
          const arms = type.types.map((arm) => collect(arm, path));
          if (arms.some((arm) => arm.length === 0)) throw new Error(`${path}: union member does not declare ${property}`);
          return arms.flat();
        }
        case "TSIntersectionType": {
          const tagged = type.types.map((arm) => collect(arm, path)).filter((arm) => arm.length > 0);
          if (tagged.length > 1) throw new Error(`${path}: multiple intersection members declare ${property}`);
          return tagged[0] ?? [];
        }
        case "TSTypeReference":
          if (type.typeName.type === "Identifier" && !type.typeParameters) return named(path, type.typeName.name);
      }
      throw new Error(`${path}: unsupported ${property} discriminator declaration (${type.type})`);
    }
    const values = named(join(root, file), name);
    if (!values.length) throw new Error(`${file}: no discriminator values found for ${name}.${property}`);
    return new Set(values);
  };
}

function goConstants(): Record<string, Record<string, string> | undefined> {
  const inventory = fileURLToPath(new URL("gateway-contract-inventory/main.go", import.meta.url));
  const { constants } = JSON.parse(execFileSync("go", [
    "run", inventory, join(repoRoot, "provider"), ...checks.map(([, , , , type]) => type),
  ], { cwd: repoRoot, encoding: "utf8" }));
  return constants;
}

function goConstValues(constants: Record<string, string> | undefined, typeName: string): Set<string> {
  const values = new Set<string>();
  for (const [name, expression] of Object.entries(constants ?? {})) {
    const literal = /^(?:"([^"\\]+)"|\w+\("([^"\\]+)"\))$/.exec(expression);
    if (!literal) throw new Error(`provider.${name}: expected a plain string discriminator for ${typeName}, found ${expression}`);
    values.add(literal[1] ?? literal[2]);
  }
  if (!values.size) throw new Error(`no Go discriminator constants found for ${typeName} in provider/`);
  return values;
}

function run(): boolean {
  const baseline = record(parseYaml(readFileSync(join(repoRoot, "test/conformance/upstream.yaml"), "utf8")));
  const version = record(baseline?.packages)?.[providerPackage];
  if (typeof version !== "string" || !version.trim()) throw new Error(`upstream baseline must declare ${providerPackage}`);
  const exceptions = shapeExceptions(baseline?.knownGaps);
  const readDiscriminators = providerTypes(providerSource(version));
  const constants = goConstants();
  const used = new Set<ShapeException>();
  let ok = true;
  for (const [check, file, alias, property, goType] of checks) {
    let upstream: Set<string>;
    try {
      upstream = readDiscriminators(file, alias, property);
    } catch (error) {
      throw new Error(`${providerPackage}@${version} ${check}: ${error instanceof Error ? error.message : error}`);
    }
    const go = goConstValues(constants[goType], goType);
    const differences = [
      ["upstream-only", [...upstream].filter((value) => !go.has(value)).sort()],
      ["Go-only", [...go].filter((value) => !upstream.has(value)).sort()],
    ] as const;
    if (differences.every(([, values]) => values.length === 0)) console.log(`provider shape: ${check} values match`);
    for (const [direction, values] of differences) {
      for (const value of values) {
        const exception = exceptions.find((entry) =>
          entry.version === version && entry.check === check && entry.direction === direction && entry.value === value);
        if (exception) {
          used.add(exception);
          console.log(`provider shape: allowed ${check} ${direction}: ${value} (${exception.id}: ${exception.rationale})`);
        } else {
          console.error(`provider shape: ${check} ${direction}: ${value}`);
          ok = false;
        }
      }
    }
  }
  for (const entry of exceptions) {
    if (used.has(entry)) continue;
    console.error(`provider shape: stale exception ${entry.id} for ${providerPackage}@${entry.version}: ${entry.check} ${entry.direction}: ${entry.value}; remove or review against ${version}`);
    ok = false;
  }
  return ok;
}

try {
  if (!run()) process.exitCode = 1;
} catch (error) {
  console.error(`provider shape: ${error instanceof Error ? error.message : error}`);
  process.exitCode = 1;
}
