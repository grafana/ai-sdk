import assert from "node:assert/strict";
import { mkdtempSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { it } from "node:test";
import { requestGoldenCases } from "./request-cases.ts";
import { updateGoldens } from "./update-goldens.ts";

it("writes only the explicit request golden allowlist", async () => {
  const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-goldens-"));
  try {
    const updated = await updateGoldens(directory);
    const expected = requestGoldenCases.map((testCase) => testCase.fileName).sort();

    assert.deepEqual([...updated].sort(), expected);
    assert.deepEqual(readdirSync(directory).sort(), expected);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
