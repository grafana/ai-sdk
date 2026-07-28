import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  buildPackageMetadata,
  compareStableVersions,
  parseMinimumReleaseAge,
  selectMaturePackageSet,
} from "./upgrade-baseline.mjs";

function dependencies(entries) {
  return (packageName, version) => entries[`${packageName}@${version}`] ?? {};
}

describe("parseMinimumReleaseAge", () => {
  it("reads the workspace age in minutes", () => {
    assert.equal(parseMinimumReleaseAge("packages: []\nminimumReleaseAge: 4320\n"), 4320);
  });

  it("rejects a missing age", () => {
    assert.throws(
      () => parseMinimumReleaseAge("packages: []\n"),
      /must declare an integer minimumReleaseAge/,
    );
  });
});

describe("buildPackageMetadata", () => {
  it("keeps stable releases on or below the npm latest dist-tag", () => {
    const metadata = buildPackageMetadata(
      "ai",
      "2.0.0",
      ["1.0.0", "2.0.0-beta.1", "2.0.0", "3.0.0"],
      {
        "1.0.0": "2026-01-01T00:00:00.000Z",
        "2.0.0-beta.1": "2026-01-02T00:00:00.000Z",
        "2.0.0": "2026-01-03T00:00:00.000Z",
        "3.0.0": "2026-01-04T00:00:00.000Z",
      },
    );

    assert.deepEqual(
      metadata.releases.map((release) => release.version),
      ["1.0.0", "2.0.0"],
    );
  });
});

describe("compareStableVersions", () => {
  it("orders stable semantic versions numerically", () => {
    assert.ok(compareStableVersions("10.0.0", "2.9.9") > 0);
    assert.ok(compareStableVersions("1.10.0", "1.9.9") > 0);
    assert.equal(compareStableVersions("1.2.3", "1.2.3"), 0);
  });
});

describe("selectMaturePackageSet", () => {
  it("falls back to the newest coherent mature release set", () => {
    const metadata = [
      buildPackageMetadata("ai", "2.0.0", ["1.0.0", "2.0.0"], {
        "1.0.0": "2026-01-01T00:00:00.000Z",
        "2.0.0": "2026-01-09T00:00:00.000Z",
      }),
      buildPackageMetadata("@ai-sdk/provider-utils", "2.0.0", ["1.0.0", "2.0.0"], {
        "1.0.0": "2026-01-01T00:00:00.000Z",
        "2.0.0": "2026-01-11T00:00:00.000Z",
      }),
    ];

    const result = selectMaturePackageSet(
      metadata,
      new Date("2026-01-10T00:00:00.000Z"),
      dependencies({
        "ai@1.0.0": { "@ai-sdk/provider-utils": "1.0.0" },
        "ai@2.0.0": { "@ai-sdk/provider-utils": "2.0.0" },
      }),
    );

    assert.deepEqual([...result.versions], [
      ["ai", "1.0.0"],
      ["@ai-sdk/provider-utils", "1.0.0"],
    ]);
  });

  it("selects compatible versions independently of publication order", () => {
    const metadata = [
      buildPackageMetadata("a", "2.0.0", ["1.0.0", "2.0.0"], {
        "1.0.0": "2026-01-01T00:00:00.000Z",
        "2.0.0": "2026-01-09T00:00:00.000Z",
      }),
      buildPackageMetadata("b", "2.0.0", ["1.0.0", "2.0.0"], {
        "1.0.0": "2026-01-01T00:00:00.000Z",
        "2.0.0": "2026-01-08T00:00:00.000Z",
      }),
    ];

    const result = selectMaturePackageSet(
      metadata,
      new Date("2026-01-10T00:00:00.000Z"),
      dependencies({
        "a@2.0.0": { b: "1.0.0" },
      }),
    );

    assert.deepEqual([...result.versions], [
      ["a", "2.0.0"],
      ["b", "1.0.0"],
    ]);
  });

  it("rejects package sets with no coherent mature release", () => {
    const metadata = [
      buildPackageMetadata("ai", "2.0.0", ["2.0.0"], {
        "2.0.0": "2026-01-01T00:00:00.000Z",
      }),
      buildPackageMetadata("@ai-sdk/provider-utils", "1.0.0", ["1.0.0"], {
        "1.0.0": "2026-01-01T00:00:00.000Z",
      }),
    ];

    assert.throws(
      () =>
        selectMaturePackageSet(
          metadata,
          new Date("2026-01-10T00:00:00.000Z"),
          dependencies({
            "ai@2.0.0": { "@ai-sdk/provider-utils": "2.0.0" },
          }),
        ),
      /no coherent stable package set .* satisfies the minimum release age/,
    );
  });

  it("does not downgrade the current baseline to find a mature set", () => {
    const metadata = [
      buildPackageMetadata("ai", "2.0.0", ["1.0.0", "2.0.0"], {
        "1.0.0": "2026-01-01T00:00:00.000Z",
        "2.0.0": "2026-01-11T00:00:00.000Z",
      }),
    ];

    assert.throws(
      () =>
        selectMaturePackageSet(
          metadata,
          new Date("2026-01-10T00:00:00.000Z"),
          dependencies({}),
          new Map([["ai", "2.0.0"]]),
        ),
      /no coherent stable package set .* satisfies the minimum release age/,
    );
  });

  it("rejects a package set when no release is mature", () => {
    const metadata = [
      buildPackageMetadata("ai", "1.0.0", ["1.0.0"], {
        "1.0.0": "2026-01-11T00:00:00.000Z",
      }),
    ];

    assert.throws(
      () =>
        selectMaturePackageSet(
          metadata,
          new Date("2026-01-10T00:00:00.000Z"),
          dependencies({}),
        ),
      /no coherent stable package set .* satisfies the minimum release age/,
    );
  });
});
