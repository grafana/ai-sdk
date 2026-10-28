import assert from 'node:assert/strict';

export const MANIFEST = '.release-please-manifest.json';
export const PREFIX = 'github.com/grafana/ai-sdk';
const ENTRY = /^(\s*)(github\.com\/grafana\/ai-sdk(?:\/\S+)?)(\s+)(v\S+)(.*)$/;

export function expectedVersions(manifest, config) {
  return Object.fromEntries(Object.entries(config.packages).flatMap(([root, settings]) => {
    const version = manifest[root] ?? settings['initial-version'] ?? config['initial-version'];
    return version ? [[root === '.' ? PREFIX : `${PREFIX}/${root}`, `v${version}`]] : [];
  }));
}

function rewriteEntry(entry, versions) {
  const match = ENTRY.exec(entry);
  if (!match || !Object.hasOwn(versions, match[2]) || match[5].trimStart().startsWith('=>')) return entry;
  const [, lead, path, space, , rest] = match;
  return `${lead}${path}${space}${versions[path]}${rest}`;
}

export function rewriteRequirements(content, versions) {
  let block;
  return content.split('\n').map(line => {
    const trimmed = line.trim();
    if (block) {
      if (trimmed.startsWith(')')) block = undefined;
      else if (block === 'require') return rewriteEntry(line, versions);
      return line;
    }
    const directive = /^(\w+)\s*\((?:\s*\/\/.*)?$/.exec(trimmed);
    if (directive) { block = directive[1]; return line; }
    const single = /^(require\s+)(.*)$/.exec(line);
    return single ? single[1] + rewriteEntry(single[2], versions) : line;
  }).join('\n');
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
