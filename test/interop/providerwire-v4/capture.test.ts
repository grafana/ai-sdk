import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { captureAllRequests, type CaptureArtifact } from "./scenarios";

const CAPTURE_PATH = resolve(import.meta.dirname, "captures/requests.json");
const PROVENANCE_PATH = resolve(import.meta.dirname, "provenance.json");
const EXPECTED_PACKAGES = {
  "@ai-sdk/provider": "4.0.4",
  "@ai-sdk/gateway": "4.0.33",
  "@ai-sdk/provider-utils": "5.0.16",
  ai: "7.0.44",
} as const;

describe("ProviderWire V4 stock-client request captures", () => {
  it("runs the exact registered packages", () => {
    for (const [name, expected] of Object.entries(EXPECTED_PACKAGES)) {
      expect(installedPackageVersion(name)).toBe(expected);
    }
    const provenance = JSON.parse(readFileSync(PROVENANCE_PATH, "utf8")) as {
      authority: string;
      packages: Record<string, string>;
      nonClaims: string[];
    };
    expect(provenance.authority).toBe("pinned-stock-client-emission");
    expect(provenance.packages).toEqual(EXPECTED_PACKAGES);
    expect(provenance.nonClaims).toContain("Vercel private server acceptance");
    expect(provenance.nonClaims).toContain("live provider response recording");
  });

  it("recaptures semantically without mutating committed fixtures", async () => {
    const generated = await captureAllRequests();
    assertPrivateDataExcluded(generated);

    const directory = mkdtempSync(join(tmpdir(), "providerwire-v4-capture-"));
    try {
      const temporaryPath = join(directory, "requests.json");
      writeArtifact(temporaryPath, generated);
      const recaptured = readArtifact(temporaryPath);
      expect(recaptured).toEqual(generated);

      if (process.env.PROVIDERWIRE_V4_UPDATE === "1") {
        writeArtifact(CAPTURE_PATH, recaptured);
      } else {
        const committedRaw = readFileSync(CAPTURE_PATH, "utf8");
        assertPrivateText(committedRaw);
        expect(recaptured).toEqual(JSON.parse(committedRaw) as CaptureArtifact);
      }
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });
});

function installedPackageVersion(name: string): string {
  const path = resolve(import.meta.dirname, "../node_modules", name, "package.json");
  return (JSON.parse(readFileSync(path, "utf8")) as { version: string }).version;
}

function readArtifact(path: string): CaptureArtifact {
  return JSON.parse(readFileSync(path, "utf8")) as CaptureArtifact;
}

function writeArtifact(path: string, artifact: CaptureArtifact): void {
  writeFileSync(path, `${JSON.stringify(artifact, null, 2)}\n`, "utf8");
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
  expect(encoded).not.toContain("ai-sdk/gateway/4.0.33");
  expect(encoded).not.toContain("provider recording");
}
