import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { discoverMatrix, reconcile, runMatrix, type RowResult } from "./gateway-matrix.mts";

test("matrix discovers all providers and categories without a capabilities list", () => {
  const root = mkdtempSync(join(tmpdir(), "gateway-inventory-"));
  try {
    for (const name of ["anthropic/recorded/text", "future/upstream/new", "ui/core/text"]) {
      const dir = join(root, name);
      mkdirSync(dir, { recursive: true });
      writeFileSync(join(dir, "config.yaml"), "model: test\n");
    }
    const matrix = discoverMatrix(root);
    assert.equal(matrix.rows.length, 4);
    assert.equal(new Set(matrix.rows.map(row => row.id)).size, 4);
    assert.deepEqual(matrix.rows.map(row => row.client), ["typescript", "go", "typescript", "go"]);
    assert.equal(matrix.scope, "full");
    const selected = discoverMatrix(root, { scenario: "future", client: "go" });
    assert.equal(selected.scope, "filtered");
    assert.equal(selected.rows.length, 1);
    assert.equal(selected.rows[0].provider, "future");
    assert.throws(() => discoverMatrix(root, { scenario: "missing" }), /no fixtures/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("matrix rejects empty, missing, duplicate and foreign results", () => {
  const root = mkdtempSync(join(tmpdir(), "gateway-inventory-"));
  try {
    assert.throws(() => discoverMatrix(root), /no fixtures/);
    const dir = join(root, "p/recorded/text");
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, "config.yaml"), "model: test\n");
    const { rows } = discoverMatrix(root);
    const results: RowResult[] = rows.map(row => ({ ...row, outcome: "passed", stage: "comparison", invoked: true, errors: [] }));
    assert.deepEqual(reconcile(rows, results), []);
    assert.match(reconcile(rows, results.slice(1)).join(), /missing/);
    assert.match(reconcile(rows, [...results, results[0]]).join(), /duplicate/);
    assert.match(reconcile(rows, [{ ...results[0], id: "foreign" }]).join(), /unexpected/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("failed attempts do not hide later rows and aborts are unexecuted", async () => {
  const rows = ["a", "b", "c"].map(id => ({ id, name: id, dir: id, provider: "p", client: "go" as const }));
  const controller = new AbortController();
  const visited: string[] = [];
  const results = await runMatrix(rows, async row => {
    visited.push(row.id);
    if (row.id === "a") throw new Error("broken executor");
    controller.abort();
    return { ...row, invoked: true, outcome: "failed", stage: "comparison", errors: ["text differs"] };
  }, controller.signal);
  assert.deepEqual(visited, ["a", "b"]);
  assert.deepEqual(results.map(row => row.outcome), ["failed", "failed", "not-executed"]);
  assert.equal(results[0].stage, "harness");
  assert.deepEqual(reconcile(rows, results), []);
});
