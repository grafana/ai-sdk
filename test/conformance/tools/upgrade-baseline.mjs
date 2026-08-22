#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const baselinePath = join(__dirname, "..", "upstream.yaml");
const workspacePath = join(__dirname, "..", "..", "pnpm-workspace.yaml");
const packagePaths = [
  join(__dirname, "package.json"),
  join(__dirname, "..", "..", "integration", "package.json"),
  join(__dirname, "..", "..", "cli", "package.json"),
  join(__dirname, "..", "..", "providerwire-v4", "package.json"),
];
const stableVersionPattern = /^(\d+)\.(\d+)\.(\d+)$/;

export function parseTagCommit(output, tag) {
  const directRef = `refs/tags/${tag}`;
  const peeledRef = `${directRef}^{}`;
  const commits = new Map(
    output
      .trim()
      .split("\n")
      .filter(Boolean)
      .map((line) => line.split(/\s+/, 2).reverse()),
  );
  const commit = commits.get(peeledRef) ?? commits.get(directRef);
  if (!commit || !/^[0-9a-f]{40,64}$/.test(commit)) {
    throw new Error(`unable to resolve upstream tag ${tag}`);
  }
  return commit;
}

export function parseMinimumReleaseAge(yaml) {
  const match = yaml.match(/^minimumReleaseAge:\s*(\d+)\s*(?:#.*)?$/m);
  if (!match) {
    throw new Error("test/pnpm-workspace.yaml must declare an integer minimumReleaseAge");
  }
  return Number.parseInt(match[1], 10);
}

function stableVersionParts(version) {
  const match = version.match(stableVersionPattern);
  if (!match) {
    throw new Error(`expected a stable semantic version, got ${version}`);
  }
  return match.slice(1).map((part) => Number.parseInt(part, 10));
}

export function compareStableVersions(left, right) {
  const leftParts = stableVersionParts(left);
  const rightParts = stableVersionParts(right);

  for (let index = 0; index < leftParts.length; index += 1) {
    if (leftParts[index] !== rightParts[index]) {
      return leftParts[index] - rightParts[index];
    }
  }
  return 0;
}

export function buildPackageMetadata(name, latestVersion, versions, publicationTimes) {
  stableVersionParts(latestVersion);

  const releases = versions
    .filter((version) => stableVersionPattern.test(version))
    .filter((version) => compareStableVersions(version, latestVersion) <= 0)
    .map((version) => {
      const publishedAt = new Date(publicationTimes[version]);
      if (Number.isNaN(publishedAt.getTime())) {
        throw new Error(`${name}@${version} is missing a valid npm publication time`);
      }
      return { version, publishedAt };
    });

  return { name, latestVersion, releases };
}

export function validatePackageSet(versions, getDependencies) {
  const errors = [];

  for (const [packageName, version] of versions) {
    const dependencies = getDependencies(packageName, version);
    for (const [dependencyName, dependencyVersion] of Object.entries(dependencies)) {
      const selectedVersion = versions.get(dependencyName);
      if (!selectedVersion || !stableVersionPattern.test(dependencyVersion)) {
        continue;
      }
      if (selectedVersion !== dependencyVersion) {
        errors.push(
          `${packageName}@${version} requires ${dependencyName}@${dependencyVersion}, ` +
            `but the candidate set selects ${selectedVersion}`,
        );
      }
    }
  }

  return errors;
}

export function selectMaturePackageSet(
  packageMetadata,
  maturityCutoff,
  getDependencies,
  minimumVersions = new Map(),
) {
  const maturityCutoffTime = maturityCutoff.getTime();
  if (Number.isNaN(maturityCutoffTime)) {
    throw new Error("maturity cutoff must be a valid date");
  }

  const packageNames = packageMetadata.map((metadata) => metadata.name);
  const trackedPackages = new Set(packageNames);
  const candidates = new Map();

  for (const metadata of packageMetadata) {
    const minimumVersion = minimumVersions.get(metadata.name);
    const releases = metadata.releases
      .filter((candidate) => candidate.publishedAt <= maturityCutoff)
      .filter(
        (candidate) =>
          !minimumVersion || compareStableVersions(candidate.version, minimumVersion) >= 0,
      )
      .sort((left, right) => compareStableVersions(right.version, left.version));
    candidates.set(metadata.name, releases);
  }

  let best;

  function search(index, versions, requirements, score) {
    if (best && score >= best.score) {
      return;
    }
    if (index === packageNames.length) {
      const errors = validatePackageSet(versions, getDependencies);
      if (errors.length === 0) {
        best = { versions: new Map(versions), score };
      }
      return;
    }

    const packageName = packageNames[index];
    const requiredVersion = requirements.get(packageName);
    const packageCandidates = candidates.get(packageName);

    for (let rank = 0; rank < packageCandidates.length; rank += 1) {
      const candidate = packageCandidates[rank];
      if (requiredVersion && candidate.version !== requiredVersion) {
        continue;
      }

      const nextScore = score + rank;
      if (best && nextScore >= best.score) {
        continue;
      }

      const nextRequirements = new Map(requirements);
      let compatible = true;
      for (const [dependencyName, dependencyVersion] of Object.entries(
        getDependencies(packageName, candidate.version),
      )) {
        if (!trackedPackages.has(dependencyName) || !stableVersionPattern.test(dependencyVersion)) {
          continue;
        }

        const selectedDependency = versions.get(dependencyName);
        const requiredDependency = nextRequirements.get(dependencyName);
        if (
          (selectedDependency && selectedDependency !== dependencyVersion) ||
          (requiredDependency && requiredDependency !== dependencyVersion)
        ) {
          compatible = false;
          break;
        }
        nextRequirements.set(dependencyName, dependencyVersion);
      }
      if (!compatible) {
        continue;
      }

      const nextVersions = new Map(versions);
      nextVersions.set(packageName, candidate.version);
      search(index + 1, nextVersions, nextRequirements, nextScore);
      if (best?.score === 0) {
        return;
      }
    }
  }

  search(0, new Map(), new Map(), 0);
  if (best) {
    return { versions: best.versions };
  }

  throw new Error(
    `no coherent stable package set at or above the current baseline satisfies ` +
      `the minimum release age`,
  );
}

function npmView(packageSpec, field) {
  const output = execFileSync("npm", ["view", packageSpec, field, "--json"], {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "inherit"],
  }).trim();
  return output === "" ? {} : JSON.parse(output);
}

function fetchPackageMetadata(packageName) {
  const latestVersion = npmView(packageName, "version");
  const versionsValue = npmView(packageName, "versions");
  const versions = Array.isArray(versionsValue) ? versionsValue : [versionsValue];
  const publicationTimes = npmView(packageName, "time");
  return buildPackageMetadata(packageName, latestVersion, versions, publicationTimes);
}

function packageVersionsFromBaseline(yaml) {
  const versions = new Map();
  let inPackages = false;

  for (const line of yaml.split("\n")) {
    if (line === "packages:") {
      inPackages = true;
      continue;
    }
    if (inPackages && /^\S/.test(line)) {
      break;
    }
    if (!inPackages) {
      continue;
    }

    const match = line.match(/^(\s*)(?:"([^"]+)"|([^:\s]+)):\s*(.+)$/);
    if (match) {
      versions.set(match[2] ?? match[3], match[4]);
    }
  }

  return versions;
}

function updateBaseline(yaml, versions, verifiedAt, commit) {
  let inPackages = false;

  return yaml
    .split("\n")
    .map((line) => {
      if (line.startsWith("  commit:")) {
        return `  commit: ${commit}`;
      }
      if (line.startsWith("  verifiedAt:")) {
        return `  verifiedAt: "${verifiedAt.toISOString().slice(0, 10)}"`;
      }
      if (line === "packages:") {
        inPackages = true;
        return line;
      }
      if (inPackages && /^\S/.test(line)) {
        inPackages = false;
      }
      if (!inPackages) {
        return line;
      }

      const match = line.match(/^(\s*)("[^"]+"|[^:\s]+):\s*(.+)$/);
      if (!match) {
        return line;
      }
      const packageName = match[2].replaceAll('"', "");
      const version = versions.get(packageName);
      if (!version) {
        return line;
      }
      return `${match[1]}${match[2]}: ${version}`;
    })
    .join("\n");
}

function resolveTagCommit(repository, tag) {
  const output = execFileSync(
    "git",
    ["ls-remote", "--tags", repository, `refs/tags/${tag}`, `refs/tags/${tag}^{}`],
    { encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] },
  );
  return parseTagCommit(output, tag);
}

function main() {
  const now = new Date();
  const baselineYaml = readFileSync(baselinePath, "utf8");
  const minimumReleaseAge = parseMinimumReleaseAge(readFileSync(workspacePath, "utf8"));
  const maturityCutoff = new Date(now.getTime() - minimumReleaseAge * 60_000);
  const baselineVersions = packageVersionsFromBaseline(baselineYaml);
  const packageMetadata = [...baselineVersions.keys()].map(fetchPackageMetadata);
  const dependencyCache = new Map();
  const getDependencies = (packageName, version) => {
    const packageSpec = `${packageName}@${version}`;
    if (!dependencyCache.has(packageSpec)) {
      dependencyCache.set(packageSpec, npmView(packageSpec, "dependencies"));
    }
    return dependencyCache.get(packageSpec);
  };

  console.log(
    `minimum release age: ${minimumReleaseAge} minutes; ` +
      `publication cutoff: ${maturityCutoff.toISOString()}`,
  );

  const { versions } = selectMaturePackageSet(
    packageMetadata,
    maturityCutoff,
    getDependencies,
    baselineVersions,
  );

  for (const [packageName, version] of versions) {
    const metadata = packageMetadata.find((candidate) => candidate.name === packageName);
    const release = metadata.releases.find((candidate) => candidate.version === version);
    console.log(`${packageName}@${version} (published ${release.publishedAt.toISOString()})`);
  }

  const repository = baselineYaml.match(/^\s*repository:\s*(\S+)\s*$/m)?.[1];
  if (!repository) {
    throw new Error("test/conformance/upstream.yaml must declare upstream.repository");
  }
  const aiVersion = versions.get("ai");
  if (!aiVersion) {
    throw new Error("test/conformance/upstream.yaml must track the ai package");
  }
  const commit = resolveTagCommit(repository, `ai@${aiVersion}`);
  console.log(`upstream commit: ${commit}`);

  const packageManifests = packagePaths.map((path) => ({
    path,
    manifest: JSON.parse(readFileSync(path, "utf8")),
  }));
  for (const { path, manifest } of packageManifests) {
    for (const section of ["dependencies", "devDependencies"]) {
      if (!manifest[section]) {
        continue;
      }
      for (const [packageName, version] of versions) {
        if (manifest[section][packageName] !== undefined) {
          manifest[section][packageName] = version;
        }
      }
    }
    writeFileSync(path, `${JSON.stringify(manifest, null, 2)}\n`);
  }

  writeFileSync(baselinePath, updateBaseline(baselineYaml, versions, now, commit));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
