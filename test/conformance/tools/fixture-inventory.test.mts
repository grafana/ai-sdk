import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { checkProviderInventories } from "./coverage-inventory.mts";
import { listProviderInputFiles } from "./fixture-inventory.mts";

test("lists provider inputs recursively, including orphan directories", () => {
  const root = mkdtempSync(join(tmpdir(), "fixture-inventory-"));
  try {
    const mapped = join(root, "mapped");
    const orphan = join(root, "orphan", "nested");
    mkdirSync(mapped);
    mkdirSync(orphan, { recursive: true });
    writeFileSync(join(mapped, "input.chunks.txt"), "mapped");
    writeFileSync(join(orphan, "input-1.chunks.txt"), "orphan");
    writeFileSync(join(orphan, "input.response.json"), "unary");
    writeFileSync(join(orphan, "expected.jsonl"), "ignored");

    assert.deepEqual(listProviderInputFiles(root), [
      join(mapped, "input.chunks.txt"),
      join(orphan, "input-1.chunks.txt"),
      join(orphan, "input.response.json"),
    ]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("checks the inventory and bytes at each registered provider release", async t => {
  const root = mkdtempSync(join(tmpdir(), "coverage-inventory-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const upstream = join(root, "upstream");
  const conformance = join(root, "conformance");
  mkdirSync(upstream);
  const git = (...args: string[]) => execFileSync("git", [
    "-C", upstream,
    "-c", "user.name=Fixture Test", "-c", "user.email=fixture@example.test",
    "-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false",
    "-c", "core.hooksPath=/dev/null", "-c", "core.autocrlf=false",
    ...args,
  ], { encoding: "utf8" }).trim();
  const put = (root: string, path: string, value: string) => {
    const destination = join(root, path);
    mkdirSync(dirname(destination), { recursive: true });
    writeFileSync(destination, value);
  };
  const commit = (message: string) => {
    git("add", ".");
    git("commit", "--quiet", "-m", message);
    return git("rev-parse", "HEAD");
  };

  const packages = {
    "@ai-sdk/anthropic": "4.0.1",
    "@ai-sdk/amazon-bedrock": "5.0.1",
    "@ai-sdk/openai": "4.0.2",
    "@ai-sdk/openai-compatible": "3.0.1",
  };
  const compatibleTag = "@ai-sdk/openai-compatible@3.0.1";
  const compatibleFixtures = "packages/openai-compatible/src/chat/__fixtures__";
  const compatibleIndex = "openai-compatible/upstream/INDEX.yaml";
  const importedInput = "openai-compatible/upstream/sample/input.response.json";
  const exactBytes = '"release bytes"\n';
  const sharedBytes = '"shared revision bytes"\n';

  git("init", "--quiet");
  for (const [name, version] of Object.entries(packages)) {
    put(upstream, `packages/${name.split("/")[1]}/package.json`, JSON.stringify({ name, version }));
  }
  put(upstream, `${compatibleFixtures}/sample.json`, exactBytes);
  put(upstream, `${compatibleFixtures}/release-only.json`, "null\n");
  const compatibleCommit = commit("compatible release");
  git("tag", "-a", compatibleTag, "-m", "compatible release");

  put(upstream, `${compatibleFixtures}/sample.json`, sharedBytes);
  git("rm", "--quiet", `${compatibleFixtures}/release-only.json`);
  put(upstream, `${compatibleFixtures}/shared-only.json`, "null\n");
  put(upstream, "packages/openai/src/responses/__fixtures__/openai-only.json", "null\n");
  const sharedCommit = commit("shared revision and other provider releases");
  for (const [name, version] of Object.entries(packages)) {
    const tag = `${name}@${version}`;
    if (tag !== compatibleTag) git("tag", tag);
  }
  put(conformance, "upstream.yaml", JSON.stringify({ upstream: { commit: sharedCommit }, packages }));
  put(conformance, "openai/upstream/INDEX.yaml", "openai-only: null\n");
  put(conformance, "openai-compatible/upstream/sample/config.yaml", "operation: generate\n");
  put(conformance, compatibleIndex, "sample: sample\nrelease-only: null\n");
  put(conformance, importedInput, exactBytes);

  await t.test("accepts imports and indexes matching independent releases, not the shared commit", () => {
    assert.deepEqual(checkProviderInventories(conformance, upstream), []);
  });

  await t.test("checks the tagged tree even when the checkout has a local replacement", () => {
    git("replace", compatibleCommit, sharedCommit);
    try {
      assert.deepEqual(checkProviderInventories(conformance, upstream), []);
    } finally {
      git("replace", "-d", compatibleCommit);
    }
  });

  await t.test("rejects bytes and index entries that only match the shared commit", () => {
    put(conformance, compatibleIndex, "sample: sample\nshared-only: null\n");
    put(conformance, importedInput, sharedBytes);
    assert.deepEqual(checkProviderInventories(conformance, upstream), [
      "openai-compatible/upstream INDEX is missing pinned fixture release-only",
      "openai-compatible/upstream sample is not byte-identical to " +
        `${compatibleFixtures}/sample.json at ${compatibleCommit}`,
      "openai-compatible/upstream INDEX references nonexistent pinned fixture shared-only",
    ]);
  });

  await t.test("requires the exact tag even when the shared commit has a matching manifest", () => {
    git("tag", "-d", compatibleTag);
    assert.throws(() => checkProviderInventories(conformance, upstream), /missing.*@ai-sdk\/openai-compatible@3\.0\.1/);
  });

  await t.test("rejects a tag whose package manifest has a different name or version", () => {
    for (const manifest of [
      { name: "@ai-sdk/openai-compatible", version: "3.0.0" },
      { name: "@ai-sdk/other", version: "3.0.1" },
    ]) {
      put(upstream, "packages/openai-compatible/package.json", JSON.stringify(manifest));
      const wrongCommit = commit("mismatched package manifest");
      git("tag", "-f", compatibleTag, wrongCommit);
      assert.throws(() => checkProviderInventories(conformance, upstream), /expected @ai-sdk\/openai-compatible@3\.0\.1/);
    }
  });
});
