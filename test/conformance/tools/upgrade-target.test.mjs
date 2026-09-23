import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it } from "node:test";
import { applyTarget, buildPackageMetadata, createTarget, readBaseline, saveTarget } from "./upgrade-baseline.mjs";

const oldCommit = "1".repeat(40);
const newCommit = "2".repeat(40);
const now = new Date("2026-09-22T00:00:00.000Z");
const publishedAt = "2026-09-18T00:00:00.000Z";
const repository = "https://github.com/vercel/ai";
const consumerPaths = [
  "test/conformance/tools/package.json",
  "test/integration/package.json",
  "test/cli/package.json",
  "ai-gateway/test/providerwire-v4/package.json",
];
const baselineYaml = `upstream:
  repository: ${repository}
  commit: ${oldCommit}
  verifiedAt: "2026-09-01"
packages:
  ai: 1.0.0
  "@ai-sdk/provider": 1.0.0
verification:
  status: enforced
knownGaps:
  - id: keep-this-gap
`;
const dependencies = (name) => name === "ai" ? { "@ai-sdk/provider": "2.0.0" } : {};
const getCommit = () => newCommit;
const getRelease = (name) => ({ publishedAt, dependencies: dependencies(name) });

function target() {
  return createTarget({
    baseline: readBaseline(baselineYaml),
    minimumReleaseAge: 4320,
    now,
    packageMetadata: ["ai", "@ai-sdk/provider"].map((name) => buildPackageMetadata(
      name, "2.0.0", ["1.0.0", "2.0.0"], { "1.0.0": "2026-09-01", "2.0.0": publishedAt },
    )),
    getDependencies: (_name, version) => version === "2.0.0" ? dependencies(_name) : {},
    getCommit,
  });
}

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "ai-sdk-upgrade-target-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const files = {
    "test/conformance/upstream.yaml": baselineYaml,
    "test/pnpm-workspace.yaml": "packages: []\nminimumReleaseAge: 4320\n",
    ...Object.fromEntries(consumerPaths.map((path) => [path, JSON.stringify({
      name: path,
      dependencies: { ai: "1.0.0", "@ai-sdk/provider": "1.0.0", other: "^3.0.0" },
    }, null, 2) + "\n"])),
  };
  for (const [path, content] of Object.entries(files)) {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), content);
  }
  const snapshot = () => Object.fromEntries(Object.keys(files).map((path) => [path, readFileSync(join(root, path), "utf8")]));
  return { root, snapshot, options: { root, now, getRelease, getCommit } };
}

describe("frozen upgrade target", () => {
  it("selects a coherent mature set with exact source evidence without changing the baseline", (t) => {
    const { root, snapshot } = fixture(t);
    const before = snapshot();
    const selected = target();
    assert.equal(selected.format, 1);
    assert.equal(selected.selectedAt, now.toISOString());
    assert.equal(selected.baseline.commit, oldCommit);
    assert.deepEqual(selected.packages.ai, { version: "2.0.0", publishedAt, sourceCommit: newCommit });
    assert.deepEqual(snapshot(), before);
    const path = join(root, "target.json");
    saveTarget(path, selected);
    assert.throws(() => saveTarget(path, { changed: true }), /exist/i);
    assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), selected);
  });

  it("applies the saved set after time passes without selecting latest; updates every consumer", (t) => {
    const { root, options } = fixture(t);
    const calls = [];
    applyTarget(target(), { ...options, now: new Date("2026-10-22"), getRelease: (name, version) => {
      calls.push([name, version]);
      assert.equal(version, "2.0.0");
      return getRelease(name);
    } });
    assert.equal(calls.length, 2);
    for (const path of consumerPaths) {
      const manifest = JSON.parse(readFileSync(join(root, path), "utf8"));
      assert.deepEqual(manifest.dependencies, { ai: "2.0.0", "@ai-sdk/provider": "2.0.0", other: "^3.0.0" });
    }
    const yaml = readFileSync(join(root, "test/conformance/upstream.yaml"), "utf8");
    assert.match(yaml, /verifiedAt: null/);
    assert.match(yaml, /keep-this-gap/);
    assert.equal(readBaseline(yaml).commit, newCommit);
  });

  it("resumes idempotently without changing verified date or gap decisions", (t) => {
    const { root, options, snapshot } = fixture(t);
    const selected = target();
    applyTarget(selected, options);
    const path = join(root, "test/conformance/upstream.yaml");
    writeFileSync(path, readFileSync(path, "utf8").replace("verifiedAt: null", 'verifiedAt: "2026-09-22"'));
    const before = snapshot();
    applyTarget(selected, options);
    assert.deepEqual(snapshot(), before);
  });

  const invalidCases = [
    ["unsupported format", (record) => { record.format = 2; }],
    ["missing package", (record) => { delete record.packages["@ai-sdk/provider"]; }],
    ["extra package", (record) => { record.packages.extra = record.packages.ai; }],
    ["prerelease", (record) => { record.packages.ai.version = "2.0.0-beta.1"; }],
    ["downgrade", (record) => { record.packages.ai.version = "0.9.0"; }],
    ["wrong source baseline", (record) => { record.baseline.commit = "3".repeat(40); }],
    ["invalid source commit", (record) => { record.packages.ai.sourceCommit = "main"; }],
    ["moved target tag", (record) => { record.packages.ai.sourceCommit = "3".repeat(40); }],
    ["changed maturity policy", (record) => { record.minimumReleaseAge = 1; }],
    ["future selection", (record) => { record.selectedAt = "2030-01-01T00:00:00.000Z"; }],
    ["invalid selection date", (record) => { record.selectedAt = "not-a-date"; }],
    ["immature at selection", (record) => { record.packages.ai.publishedAt = "2026-09-21T00:00:00.000Z"; }],
    ["invalid publication date", (record) => { record.packages.ai.publishedAt = "bad-date"; }],
    ["publication evidence mismatch", (record) => { record.packages.ai.publishedAt = "2026-09-17T00:00:00.000Z"; }],
  ];
  for (const [name, mutate] of invalidCases) {
    it(`rejects ${name} before writing`, (t) => {
      const { options, snapshot } = fixture(t);
      const selected = target();
      mutate(selected);
      const before = snapshot();
      assert.throws(() => applyTarget(selected, options));
      assert.deepEqual(snapshot(), before);
    });
  }

  it("rejects changed exact dependencies before writing", (t) => {
    const { options, snapshot } = fixture(t);
    const before = snapshot();
    assert.throws(() => applyTarget(target(), { ...options, getRelease: () => ({
      publishedAt, dependencies: { "@ai-sdk/provider": "3.0.0" },
    }) }), /requires/);
    assert.deepEqual(snapshot(), before);
  });

  it("rejects mixed baseline and consumer drift before writing", (t) => {
    const { root, options, snapshot } = fixture(t);
    const selected = target();
    const path = join(root, consumerPaths[3]);
    const original = readFileSync(path, "utf8");
    writeFileSync(path, original.replaceAll("1.0.0", "9.0.0"));
    const before = snapshot();
    assert.throws(() => applyTarget(selected, options), /consumer|pin/i);
    assert.deepEqual(snapshot(), before);
    writeFileSync(path, original);
    const baselinePath = join(root, "test/conformance/upstream.yaml");
    writeFileSync(baselinePath, baselineYaml.replace("ai: 1.0.0", "ai: 2.0.0"));
    const mixed = snapshot();
    assert.throws(() => applyTarget(selected, options), /baseline/i);
    assert.deepEqual(snapshot(), mixed);
  });

  it("checks all consumers before writing and leaves them intact on unavailable evidence", (t) => {
    const { root, options, snapshot } = fixture(t);
    const selected = target();
    const last = join(root, consumerPaths[3]);
    const original = readFileSync(last, "utf8");
    writeFileSync(last, "invalid JSON");
    const invalid = snapshot();
    assert.throws(() => applyTarget(selected, options));
    assert.deepEqual(snapshot(), invalid);
    writeFileSync(last, original);
    const before = snapshot();
    assert.throws(() => applyTarget(selected, { ...options, getCommit: () => { throw new Error("source unavailable"); } }), /source unavailable/);
    assert.deepEqual(snapshot(), before);
  });

  it("updates devDependencies and preserves unrelated dependency declarations", (t) => {
    const { root, options } = fixture(t);
    const path = join(root, consumerPaths[1]);
    writeFileSync(path, JSON.stringify({ devDependencies: { ai: "1.0.0", other: "^3.0.0" } }));
    applyTarget(target(), options);
    assert.deepEqual(JSON.parse(readFileSync(path, "utf8")), {
      devDependencies: { ai: "2.0.0", other: "^3.0.0" },
    });
  });

  it("refuses the old implicit CLI before accessing npm or modifying files", () => {
    const script = fileURLToPath(new URL("./upgrade-baseline.mjs", import.meta.url));
    assert.throws(() => execFileSync(process.execPath, [script], { env: { ...process.env, PATH: "" }, stdio: "pipe" }), (error) => {
      assert.match(error.stderr.toString(), /select.*output|apply.*target/);
      return true;
    });
  });
});
