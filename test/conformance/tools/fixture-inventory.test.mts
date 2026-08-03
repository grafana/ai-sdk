import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
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
