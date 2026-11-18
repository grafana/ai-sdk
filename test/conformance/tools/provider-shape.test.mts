import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test, { type TestContext } from "node:test";
import { fileURLToPath } from "node:url";

const version = "4.0.38";
const installed = "test/conformance/tools/node_modules/@ai-sdk/provider";
const v4 = `${installed}/src/language-model/v4`;
const warning = `${installed}/src/shared/v4/shared-v4-warning.ts`;
const shape = "test/conformance/tools/provider-shape.mts";
const providerManifest = { name: "@ai-sdk/provider", version, exports: { "./package.json": "./package.json" } };
const pingGap = {
  id: "test-ping",
  status: "gap",
  description: "Synthetic exception rationale.",
  providerShapeException: { version, check: "stream part", direction: "upstream-only", value: "ping" },
};

function fixture(t: TestContext) {
  const root = mkdtempSync(join(tmpdir(), "provider-shape-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  function file(path: string, body: string) {
    const target = join(root, path);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, body);
  }
  function baseline(...knownGaps: object[]) {
    file("test/conformance/upstream.yaml", JSON.stringify({ packages: { "@ai-sdk/provider": version }, knownGaps }));
  }
  function stream(...types: string[]) {
    file(`${v4}/language-model-v4-stream-part.ts`, `
      import type { Part as ImportedPart } from './part';
      import type { Metadata } from './metadata';
      export type LanguageModelV4StreamPart = ImportedPart | ({ type: 'metadata' } & Metadata)
        ${types.map((type) => `| { type: '${type}' }`).join(" ")};
    `);
  }
  function localStream(...types: string[]) {
    file("provider/stream_part.go", `package provider\ntype StreamPartType string\nconst (\n${types.map((type, i) => ` Part${i} StreamPartType = "${type}"`).join("\n")}\n)\n`);
  }
  baseline();
  file(`${installed}/package.json`, JSON.stringify(providerManifest));
  stream();
  file(`${v4}/part.ts`, `export type Part = { type: 'imported'; nested: { type: 'not-a-part' } };`);
  file(`${v4}/metadata.ts`, `export interface Metadata { id: string; }`);
  file(`${v4}/language-model-v4-content.ts`, `import type { Part } from './part'; export type LanguageModelV4Content = Part | { type: 'url' };`);
  file(`${v4}/language-model-v4-finish-reason.ts`, `export type LanguageModelV4FinishReason = { unified: 'stop'; raw: 'not-unified' };`);
  file(`${v4}/language-model-v4-prompt.ts`, `export type Unrelated = { type: 'unrelated' }; export type LanguageModelV4ToolResultOutput = { type: 'content'; value: { type: 'not-an-output' }[] };`);
  file(warning, `export type SharedV4Warning = { type: 'other' };`);
  localStream("imported", "metadata");
  file("provider/language_model.go", `package provider\ntype GenerateContentType string\nconst (\n Imported GenerateContentType = "imported"\n URL GenerateContentType = "url"\n)\n`);
  file("provider/types.go", `package provider\ntype UnifiedFinishReason string\ntype ToolResultOutputType string\ntype WarningType string\nconst (\n Stop UnifiedFinishReason = "stop"\n Content ToolResultOutputType = "content"\n Other WarningType = "other"\n)\n`);
  for (const src of ["provider-shape.mts", "gateway-contract-inventory/main.go"]) {
    const target = join(root, dirname(shape), src);
    mkdirSync(dirname(target), { recursive: true });
    copyFileSync(fileURLToPath(new URL(src, import.meta.url)), target);
  }
  const require = createRequire(import.meta.url);
  for (const dependency of ["yaml", "@babel/parser"]) {
    const link = join(root, dirname(shape), "node_modules", dependency);
    mkdirSync(dirname(link), { recursive: true });
    symlinkSync(dirname(require.resolve(`${dependency}/package.json`)), link, "dir");
  }
  return {
    file, baseline, stream, localStream,
    remove: (path: string) => rmSync(join(root, path), { recursive: true }),
    run: (...args: string[]) => {
      const env = { ...process.env };
      delete env.NODE_PATH;
      const result = spawnSync(process.execPath, [join(root, shape), ...args], { cwd: root, encoding: "utf8", env });
      assert.ifError(result.error);
      assert.equal(result.signal, null);
      return { status: result.status, output: result.stderr + result.stdout };
    },
  };
}

test("provider shape reads all five named declarations from the installed provider", (t) => {
  const f = fixture(t);
  const result = f.run();
  assert.equal(result.status, 0, result.output);
  for (const name of ["stream part", "content part", "finish reason", "tool result output", "warning"]) {
    assert.match(result.output, new RegExp(`${name} values match`));
  }
});

test("provider shape fails when the installed provider or its required source is unavailable", async (t) => {
  for (const path of [installed, `${v4}/part.ts`]) {
    await t.test(path, (t) => {
      const f = fixture(t);
      f.remove(path);
      const result = f.run();
      assert.equal(result.status, 1, result.output);
      assert.match(result.output, /@ai-sdk\/provider@4\.0\.38/);
      assert.match(result.output, path.endsWith(".ts") ? /part\.ts/ : /mise deps install --only test/);
    });
  }
});

test("provider shape rejects a different installed or explicitly supplied version", (t) => {
  const f = fixture(t);
  f.file(`${installed}/package.json`, JSON.stringify({ ...providerManifest, version: "4.0.37" }));
  for (const args of [[], [`--upstream-root=${installed}`]]) {
    const result = f.run(...args);
    assert.equal(result.status, 1, result.output);
    assert.match(result.output, /expected @ai-sdk\/provider@4\.0\.38.*4\.0\.37/);
  }
});

test("provider shape reports imported upstream and Go-only stream drift together", (t) => {
  const f = fixture(t);
  f.file(`${v4}/part.ts`, `export type Part = { type: 'imported' } | { type: 'added-imported' };`);
  f.file("provider/another.go", `package provider\nconst Extra = StreamPartType("go-extra")\n`);
  f.file(warning, `export type SharedV4Warning = { type: 'other' } | { type: 'warning-extra' };`);
  f.file(`${v4}/language-model-v4-prompt.ts`, `export type LanguageModelV4ToolResultOutput = { type: 'content' } | { type: 'output-extra' };`);
  const result = f.run();
  assert.equal(result.status, 1, result.output);
  assert.match(result.output, /stream part upstream-only: added-imported/);
  assert.match(result.output, /stream part Go-only: go-extra/);
  assert.match(result.output, /warning upstream-only: warning-extra/);
  assert.match(result.output, /tool result output upstream-only: output-extra/);
});

test("provider shape reports an exact allowed difference but still rejects other drift", (t) => {
  const f = fixture(t);
  f.baseline(pingGap);
  f.stream("ping");
  const allowed = f.run();
  assert.equal(allowed.status, 0, allowed.output);
  assert.match(allowed.output, /allowed stream part upstream-only: ping .*test-ping/);
  f.stream("ping", "another");
  const drift = f.run();
  assert.equal(drift.status, 1, drift.output);
  assert.match(drift.output, /stream part upstream-only: another/);
});

test("provider shape rejects stale exceptions, including when the direction flips", async (t) => {
  for (const kind of ["matched", "wrong direction", "wrong version"]) {
    await t.test(kind, (t) => {
      const f = fixture(t);
      if (kind !== "wrong direction") f.stream("ping");
      if (kind !== "wrong version") f.localStream("imported", "metadata", "ping");
      f.baseline(kind === "wrong version" ? {
        ...pingGap,
        providerShapeException: { ...pingGap.providerShapeException, version: "4.0.37" },
      } : pingGap);
      const result = f.run();
      assert.equal(result.status, 1, result.output);
      assert.match(result.output, /stale.*test-ping/);
      if (kind === "wrong direction") assert.match(result.output, /stream part Go-only: ping/);
    });
  }
});

test("provider shape rejects an unextractable variant rather than comparing partial sets", (t) => {
  const f = fixture(t);
  f.file(`${v4}/part.ts`, `export type Part = { type: 'imported' } | { type: string };`);
  const result = f.run();
  assert.equal(result.status, 1, result.output);
  assert.match(result.output, /stream part.*part\.ts/);
});
