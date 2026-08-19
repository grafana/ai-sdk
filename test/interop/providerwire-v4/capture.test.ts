import { mkdtempSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { captureAllRequests, type CaptureArtifact } from "./scenarios";

const CAPTURE_PATH = resolve(import.meta.dirname, "captures/requests.json");

describe("ProviderWire V4 stock-client request captures", () => {
  it("recaptures semantically without mutating committed fixtures", async () => {
    const committedBefore = readFileSync(CAPTURE_PATH, "utf8");
    const generated = await captureAllRequests();
    assertPrivateDataExcluded(generated);
    expect(readFileSync(CAPTURE_PATH, "utf8")).toBe(committedBefore);

    const recaptured = JSON.parse(serializeArtifact(generated)) as CaptureArtifact;
    expect(recaptured).toEqual(generated);

    if (process.env.PROVIDERWIRE_V4_UPDATE === "1") {
      replaceArtifactAtomically(CAPTURE_PATH, recaptured);
    } else {
      assertPrivateText(committedBefore);
      expect(recaptured).toEqual(JSON.parse(committedBefore) as CaptureArtifact);
    }
  });

  it("preserves the destination when staged artifact validation fails", () => {
    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-update-failure-"));
    try {
      const destination = join(directory, "requests.json");
      const original = "original artifact\n";
      writeFileSync(destination, original, "utf8");
      const invalid: CaptureArtifact = {
        formatVersion: 1,
        captures: [{
          scenario: "capture-not-a-real-key",
          sequence: 1,
          request: { method: "POST", path: "/language-model", headers: {}, body: {} },
        }],
      };

      expect(() => replaceArtifactAtomically(destination, invalid)).toThrow();
      expect(readFileSync(destination, "utf8")).toBe(original);
      expect(readdirSync(directory)).toEqual(["requests.json"]);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });
});

function readArtifact(path: string): CaptureArtifact {
  return JSON.parse(readFileSync(path, "utf8")) as CaptureArtifact;
}

function serializeArtifact(artifact: CaptureArtifact): string {
  return `${JSON.stringify(artifact, null, 2)}\n`;
}

function writeArtifact(path: string, artifact: CaptureArtifact): void {
  writeFileSync(path, serializeArtifact(artifact), "utf8");
}

function replaceArtifactAtomically(path: string, artifact: CaptureArtifact): void {
  const directory = mkdtempSync(join(dirname(path), ".providerwire-v4-update-"));
  try {
    const stagedPath = join(directory, "requests.json");
    writeArtifact(stagedPath, artifact);
    assertPrivateDataExcluded(readArtifact(stagedPath));
    renameSync(stagedPath, path);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

function assertPrivateDataExcluded(artifact: CaptureArtifact): void {
  assertPrivateText(JSON.stringify(artifact));
}

function assertPrivateText(encoded: string): void {
  expect(encoded).not.toContain("capture-not-a-real-key");
  expect(encoded).not.toContain(process.cwd());
  expect(encoded).not.toMatch(/\/(?:home|Users)\//);
  expect(encoded).not.toMatch(/Bearer (?!<redacted>)/);
  expect(encoded).not.toContain("synthetic-capture-project");
  expect(encoded).not.toContain("ai-sdk/gateway/");
  expect(encoded).not.toContain("provider recording");
}
