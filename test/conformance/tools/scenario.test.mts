import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { command } from "./gateway-runtime.mts";
import { discoverCases } from "./gateway-matrix.mts";
import { loadConfig } from "./common.mts";
import { createModel } from "./generate.mts";
import { startReplay } from "./replay.mts";
import { executeScenario } from "./scenario.mts";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

test("extracted TypeScript execution reproduces all direct goldens without writing fixtures", async t => {
  const directory = mkdtempSync(join(tmpdir(), "conformance-scenario-test-"));
  try {
    const binary = join(directory, "compare");
    await command("go", ["build", "-mod=readonly", "-tags", "conformance", "-o", binary, "./cmd/gateway-client"], { cwd: root, timeout: 180_000 });
    for (const tc of discoverCases(root)) await t.test(tc.name, async () => {
      const digest = () => createHash("sha256").update(readdirSync(tc.dir).sort().map(name => readFileSync(join(tc.dir, name))).reduce((a, b) => Buffer.concat([a, b]), Buffer.alloc(0))).digest("hex");
      const before = digest();
      const replay = await startReplay(tc);
      try {
        const cfg = loadConfig(tc.dir);
        const result = await executeScenario(tc, createModel(tc.provider, cfg.model, replay.url), AbortSignal.timeout(15_000));
        const errors = JSON.parse(await command(binary, [], { input: { operation: "compare", directory: tc.dir, result, requests: replay.requests } }));
        assert.deepEqual(errors, []);
        assert.deepEqual(replay.errors, []);
        assert.equal(digest(), before);
      } finally { await replay.close(); }
    });
  } finally { rmSync(directory, { recursive: true, force: true }); }
});
