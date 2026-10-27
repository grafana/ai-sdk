import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { childEnvironment } from './authorize.mjs';
import { SHA } from './merged-candidates.mjs';

export const MANIFEST = '.release-please-manifest.json';
export const LIBRARY_BRANCH = 'release-please--branches--main';
export const GATEWAY_BRANCH = 'release-please--branches--main--components--ai-gateway';

const tooling = resolve(fileURLToPath(new URL('../..', import.meta.url)));

const changelog = component => component === '.' ? 'CHANGELOG.md' : `${component}/CHANGELOG.md`;

export function libraryComponents(config) {
  return Object.keys(config.packages).filter(path => path !== 'ai-gateway');
}

export function assertCombinedConfig(config) {
  assert.equal(config['separate-pull-requests'], false, 'libraries must share one release pull request');
  assert.equal(config.packages['ai-gateway']?.['separate-pull-requests'], true, 'Gateway must keep its own release pull request');
  for (const component of libraryComponents(config)) {
    assert.notEqual(config.packages[component]['separate-pull-requests'], true, `${component} must join the combined release`);
  }
}

export function releasedComponents(config, updates) {
  const components = libraryComponents(config);
  const paths = updates.map(update => update.path);
  assert.ok(paths.includes(MANIFEST), 'combined release must update the version manifest');
  const released = [];
  for (const path of paths) {
    if (path === MANIFEST) continue;
    const component = components.find(candidate => changelog(candidate) === path);
    assert.ok(component !== undefined, `combined release may only update the manifest and library changelogs: ${path}`);
    released.push(component);
  }
  assert.ok(released.length > 0, 'combined release must release at least one library');
  return released.sort();
}

const GO_LOCATIONS = ['GOROOT', 'GOTOOLCHAIN', 'GOCACHE', 'GOMODCACHE', 'GOPATH'];

export function goEnvironment(env = process.env) {
  return { ...childEnvironment(env), ...Object.fromEntries(GO_LOCATIONS.filter(key => env[key]).map(key => [key, env[key]])),
    GOFLAGS: '-mod=readonly' };
}

function run(command, args, options = {}) {
  return execFileSync(command, args, { env: childEnvironment(), encoding: 'utf8', maxBuffer: 256 << 20,
    stdio: ['ignore', 'pipe', 'pipe'], ...options });
}

export function generateRequirements({ sha, changes, root = tooling, source = root, exec = run }) {
  assert.match(sha, SHA);
  const dir = mkdtempSync(join(tmpdir(), 'release-requirements-'));
  try {
    exec('bash', ['-c', 'git -C "$1" archive --format=tar "$2" | tar -x -C "$3"', 'bash', source, sha, dir]);
    const git = args => exec('git', ['-C', dir, '-c', 'user.name=release', '-c', 'user.email=release@localhost',
      '-c', 'commit.gpgsign=false', ...args]);
    git(['init', '-q']);
    git(['add', '-A', '-f', '.']);
    git(['commit', '-q', '-m', 'base']);
    for (const [path, change] of changes) {
      const target = join(dir, path);
      mkdirSync(dirname(target), { recursive: true });
      writeFileSync(target, change.content);
    }
    git(['add', '--', ...changes.keys()]);
    const output = exec('go', ['-C', join(root, 'release/candidate'), 'run', './cmd/candidate',
      'generate', '-dir', dir, '-base', 'HEAD'], { env: goEnvironment() });
    const result = JSON.parse(output);
    const files = new Map((result.Files ?? []).map(path => {
      assert.match(path, /(^|\/)go\.(mod|sum)$/, `generator returned an unexpected file: ${path}`);
      return [path, readFileSync(join(dir, path), 'utf8')];
    }));
    return { files, checksums: result.Checksums ?? {} };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

export async function withRequirements(github, candidate, { sha, config, generate = generateRequirements }) {
  const components = releasedComponents(config, candidate.updates);
  const changes = await github.buildChangeSet(candidate.updates, 'main');
  const { files, checksums } = await generate({ sha, changes });
  for (const [path, content] of files) {
    candidate.updates.push({ path, createIfMissing: true, updater: { updateContent: () => content } });
  }
  return { components, files: [...files.keys()].sort(), checksums };
}
