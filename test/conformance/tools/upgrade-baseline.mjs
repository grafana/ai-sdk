#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(__dirname, "..", "..", "..");
const packagePaths = [
  join(__dirname, "package.json"),
  join(__dirname, "..", "..", "integration", "package.json"),
  join(__dirname, "..", "..", "cli", "package.json"),
  join(
    __dirname,
    "..",
    "..",
    "..",
    "ai-gateway",
    "test",
    "providerwire-v4",
    "package.json",
  ),
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

function updateBaseline(yaml, versions, commit) {
  let inPackages = false;

  return yaml
    .split("\n")
    .map((line) => {
      if (line.startsWith("  commit:")) {
        return `  commit: ${commit}`;
      }
      if (line.startsWith("  verifiedAt:")) {
        return "  verifiedAt: null";
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

function requireCommit(commit) {
  if (typeof commit !== "string" || !/^[0-9a-f]{40,64}$/.test(commit)) {
    throw new Error(`invalid upstream source commit: ${commit}`);
  }
  return commit;
}

function timestamp(value) {
  const date = new Date(value);
  if (typeof value !== "string" || Number.isNaN(date.getTime()) || date.toISOString() !== value) {
    throw new Error(`expected an ISO publication/selection timestamp, got ${value}`);
  }
  return date;
}

function samePackageNames(left, right) {
  return Object.keys(left).length === Object.keys(right).length &&
    Object.keys(left).every((name) => Object.hasOwn(right, name));
}

function samePackages(left, right) {
  return samePackageNames(left, right) &&
    Object.entries(left).every(([name, version]) => right[name] === version);
}

function sameBaseline(left, right) {
  return left.repository === right.repository && left.commit === right.commit &&
    samePackages(left.packages, right.packages);
}

export function readBaseline(yaml) {
  const repository = yaml.match(/^  repository:\s*(\S+)\s*$/m)?.[1];
  const commit = yaml.match(/^  commit:\s*(\S+)\s*$/m)?.[1];
  const packages = Object.fromEntries(packageVersionsFromBaseline(yaml));
  if (!repository || !packages.ai) {
    throw new Error("baseline must declare upstream.repository, commit and packages.ai");
  }
  requireCommit(commit);
  for (const version of Object.values(packages)) stableVersionParts(version);
  return { repository, commit, packages };
}

export function createTarget({ baseline, packageMetadata, minimumReleaseAge, now, getDependencies, getCommit }) {
  const cutoff = new Date(now.getTime() - minimumReleaseAge * 60_000);
  const { versions } = selectMaturePackageSet(
    packageMetadata, cutoff, getDependencies, new Map(Object.entries(baseline.packages)),
  );
  const packages = Object.fromEntries([...versions].map(([name, version]) => {
    const release = packageMetadata.find((metadata) => metadata.name === name)
      .releases.find((candidate) => candidate.version === version);
    return [name, {
      version,
      publishedAt: release.publishedAt.toISOString(),
      sourceCommit: requireCommit(getCommit(baseline.repository, `${name}@${version}`)),
    }];
  }));
  return { format: 1, selectedAt: now.toISOString(), minimumReleaseAge, baseline, packages };
}

export function saveTarget(path, target) {
  writeFileSync(path, `${JSON.stringify(target, null, 2)}\n`, { flag: "wx" });
}

function fetchRelease(name, version) {
  const times = npmView(name, "time");
  return { publishedAt: times[version], dependencies: npmView(`${name}@${version}`, "dependencies") };
}

export function applyTarget(target, {
  root = repositoryRoot,
  now = new Date(),
  getRelease = fetchRelease,
  getCommit = resolveTagCommit,
} = {}) {
  const baselinePath = join(root, "test/conformance/upstream.yaml");
  const baselineYaml = readFileSync(baselinePath, "utf8");
  const baseline = readBaseline(baselineYaml);
  const minimumReleaseAge = parseMinimumReleaseAge(readFileSync(join(root, "test/pnpm-workspace.yaml"), "utf8"));
  if (target.format !== 1 || target.minimumReleaseAge !== minimumReleaseAge) {
    throw new Error("unsupported target format or changed minimum release age; reassess the target");
  }
  const selectedAt = timestamp(target.selectedAt);
  if (selectedAt > now) throw new Error("target selection time is in the future");
  const source = target.baseline;
  requireCommit(source.commit);
  if (source.repository !== baseline.repository || !samePackageNames(target.packages, source.packages)) {
    throw new Error("target package inventory or repository does not match its source baseline");
  }
  const versions = new Map();
  const cutoff = selectedAt.getTime() - minimumReleaseAge * 60_000;
  for (const [name, release] of Object.entries(target.packages)) {
    if (compareStableVersions(release.version, source.packages[name]) < 0) {
      throw new Error(`target would downgrade ${name}`);
    }
    requireCommit(release.sourceCommit);
    if (timestamp(release.publishedAt).getTime() > cutoff) {
      throw new Error(`${name}@${release.version} was not mature at target selection`);
    }
    versions.set(name, release.version);
  }
  const destination = {
    repository: source.repository,
    commit: target.packages.ai.sourceCommit,
    packages: Object.fromEntries(versions),
  };
  const alreadyApplied = sameBaseline(baseline, destination);
  if (!alreadyApplied && !sameBaseline(baseline, source)) {
    throw new Error("registered baseline changed since target selection; reassessment required");
  }

  const manifests = packagePaths.map((path) => {
    const resolved = join(root, relative(repositoryRoot, path));
    const manifest = JSON.parse(readFileSync(resolved, "utf8"));
    for (const section of ["dependencies", "devDependencies"]) {
      for (const [name, version] of Object.entries(manifest[section] ?? {})) {
        if (name !== "ai" && !name.startsWith("@ai-sdk/")) continue;
        if (baseline.packages[name] !== version || !versions.has(name)) {
          throw new Error(`consumer ${resolved} pin ${name}@${version} differs from its current baseline`);
        }
        manifest[section][name] = versions.get(name);
      }
    }
    return { path: resolved, content: `${JSON.stringify(manifest, null, 2)}\n` };
  });

  const dependencies = new Map();
  for (const [name, release] of Object.entries(target.packages)) {
    const actual = getRelease(name, release.version);
    if (timestamp(actual.publishedAt).toISOString() !== release.publishedAt) {
      throw new Error(`${name}@${release.version} publication evidence changed`);
    }
    if (getCommit(source.repository, `${name}@${release.version}`) !== release.sourceCommit) {
      throw new Error(`${name}@${release.version} source tag changed`);
    }
    dependencies.set(name, actual.dependencies);
  }
  const errors = validatePackageSet(versions, (name) => dependencies.get(name));
  if (errors.length) throw new Error(errors.join("\n"));

  if (alreadyApplied) return;
  for (const { path, content } of manifests) writeFileSync(path, content);
  writeFileSync(baselinePath, updateBaseline(baselineYaml, versions, destination.commit));
}

function main(args) {
  const [command, argument, ...extra] = args;
  const flag = command === "select" ? "--output=" : "--target=";
  if (!["select", "apply"].includes(command) || !argument?.startsWith(flag) ||
    argument.length === flag.length || extra.length) {
    throw new Error("usage: upgrade-baseline.mjs select --output=target.json | apply --target=target.json");
  }
  const path = resolve(argument.slice(flag.length));
  if (command === "apply") {
    applyTarget(JSON.parse(readFileSync(path, "utf8")));
    console.log("Applied frozen target; review snapshots, attestation and verification before merge.");
    return;
  }
  if (existsSync(path)) throw new Error(`target already exists: ${path}; resume it or choose a new output`);
  const baseline = readBaseline(readFileSync(join(repositoryRoot, "test/conformance/upstream.yaml"), "utf8"));
  const minimumReleaseAge = parseMinimumReleaseAge(readFileSync(join(repositoryRoot, "test/pnpm-workspace.yaml"), "utf8"));
  const packageMetadata = Object.keys(baseline.packages).map(fetchPackageMetadata);
  const cache = new Map();
  const getDependencies = (name, version) => {
    const spec = `${name}@${version}`;
    if (!cache.has(spec)) cache.set(spec, npmView(spec, "dependencies"));
    return cache.get(spec);
  };
  const target = createTarget({ baseline, minimumReleaseAge, now: new Date(), packageMetadata,
    getDependencies, getCommit: resolveTagCommit });
  saveTarget(path, target);
  console.log(`Selected target saved to ${path}; canonical files are unchanged. Assess and approve before applying.`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
