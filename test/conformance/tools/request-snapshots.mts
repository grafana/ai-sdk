import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { normalizeRequestSnapshot, writeRequestSnapshots, type RequestSnapshot } from "./common.mts";

export const requestTargetCases: { name: string; url: string; expectedPath: string }[] = JSON.parse(
  readFileSync(new URL("../testdata/request-snapshots/cases.json", import.meta.url), "utf8"),
);

const expectedPath = fileURLToPath(
  new URL("../testdata/request-snapshots/expected-requests.jsonl", import.meta.url),
);

export function requestTargetSnapshots(): RequestSnapshot[] {
  return requestTargetCases.map((tc) => {
    const snapshot = normalizeRequestSnapshot(
      "anthropic",
      { method: "POST", url: tc.url, headers: { "content-type": "application/json" } },
      "{}",
    );
    assert.equal(snapshot.path, tc.expectedPath, tc.name);
    return snapshot;
  });
}

export function checkRequestSnapshots(path = expectedPath): void {
  const dir = mkdtempSync(join(tmpdir(), "aisdk-request-snapshots-"));
  try {
    const actualPath = join(dir, "actual.jsonl");
    writeRequestSnapshots(actualPath, requestTargetSnapshots());
    assert.deepEqual(readFileSync(actualPath), readFileSync(path), "stale request snapshots");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const args = process.argv.slice(2);
  assert.ok(
    args.length === 0 || (args.length === 1 && args[0] === "--write"),
    "usage: tsx request-snapshots.mts [--write]",
  );
  if (args[0] === "--write") {
    writeRequestSnapshots(expectedPath, requestTargetSnapshots());
  } else {
    checkRequestSnapshots();
  }
}
