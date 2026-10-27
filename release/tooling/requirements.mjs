import assert from 'node:assert/strict';

export const MANIFEST = '.release-please-manifest.json';
const PREFIX = 'github.com/grafana/ai-sdk';
const REQUIREMENT = /^(\s*(?:require\s+)?)(github\.com\/grafana\/ai-sdk(?:\/\S+)?)(\s+)(v\S+)/gm;

export function expectedVersions(manifest, config) {
  return Object.fromEntries(Object.entries(config.packages).flatMap(([root, settings]) => {
    const version = manifest[root] ?? settings['initial-version'] ?? config['initial-version'];
    return version ? [[root === '.' ? PREFIX : `${PREFIX}/${root}`, `v${version}`]] : [];
  }));
}

export function rewriteRequirements(content, versions) {
  return content.replace(REQUIREMENT, (line, lead, path, space) =>
    Object.hasOwn(versions, path) ? `${lead}${path}${space}${versions[path]}` : line);
}

export async function withRequirementUpdates(github, candidate, { branch = 'main', config }) {
  const update = candidate.updates.find(entry => entry.path === MANIFEST);
  assert.ok(update, 'release must update the version manifest');
  const manifest = JSON.parse(update.updater.updateContent(JSON.stringify(await github.getFileJson(MANIFEST, branch))));
  const versions = expectedVersions(manifest, config);
  const paths = [];
  for (const path of [...await github.findFilesByFilenameAndRef('go.mod', branch)].sort()) {
    const { parsedContent } = await github.getFileContentsOnBranch(path, branch);
    if (rewriteRequirements(parsedContent, versions) === parsedContent) continue;
    candidate.updates.push({ path, createIfMissing: false,
      updater: { updateContent: content => rewriteRequirements(content, versions) } });
    paths.push(path);
  }
  return paths;
}
