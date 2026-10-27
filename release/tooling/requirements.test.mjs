import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { assertCombinedConfig, generateRequirements, goEnvironment, releasedComponents, withRequirements } from './requirements.mjs';
import { config } from './authorization-fixture.mjs';

const core = 'github.com/grafana/ai-sdk';
const combined = { ...config, 'separate-pull-requests': false, packages: { ...config.packages,
  'ai-gateway': { ...config.packages['ai-gateway'], 'separate-pull-requests': true } } };

function repository() {
  const dir = mkdtempSync(join(tmpdir(), 'requirements-fixture-'));
  const write = (path, content) => { mkdirSync(dirname(join(dir, path)), { recursive: true }); writeFileSync(join(dir, path), content); };
  write('LICENSE', 'license\n');
  write('go.mod', `module ${core}\n\ngo 1.27.3\n`);
  write('root.go', 'package aisdk\n\nconst Value = "a"\n');
  write('providers/openai/go.mod', `module ${core}/providers/openai\n\ngo 1.27.3\n\nrequire ${core} v0.1.0\n`);
  write('providers/openai/openai.go', `package openai\n\nimport aisdk "${core}"\n\nconst Value = aisdk.Value\n`);
  write('.release-please-manifest.json', JSON.stringify({ '.': '0.1.0', 'providers/openai': '0.1.0' }));
  const git = args => execFileSync('git', ['-C', dir, '-c', 'user.name=t', '-c', 'user.email=t@localhost',
    '-c', 'commit.gpgsign=false', ...args], { encoding: 'utf8' }).trim();
  git(['init', '-q']); git(['add', '.']); git(['commit', '-qm', 'base']);
  return { dir, sha: git(['rev-parse', 'HEAD']) };
}

test('combined configuration shape', () => {
  assertCombinedConfig(config);
  assert.throws(() => assertCombinedConfig({ ...config, 'separate-pull-requests': true }), /share one release pull request/);
  assertCombinedConfig(combined);
  assert.throws(() => assertCombinedConfig({ ...combined, packages: { ...combined.packages, 'ai-gateway': { component: 'ai-gateway' } } }),
    /Gateway must keep its own/);
});

test('released components come from library changelogs only', () => {
  const update = path => ({ path });
  assert.deepEqual(releasedComponents(combined, [update('.release-please-manifest.json'), update('providers/openai/CHANGELOG.md'), update('CHANGELOG.md')]),
    ['.', 'providers/openai']);
  assert.throws(() => releasedComponents(combined, [update('providers/openai/CHANGELOG.md')]), /must update the version manifest/);
  assert.throws(() => releasedComponents(combined, [update('.release-please-manifest.json')]), /at least one library/);
  assert.throws(() => releasedComponents(combined, [update('.release-please-manifest.json'), update('ai-gateway/CHANGELOG.md')]),
    /ai-gateway\/CHANGELOG.md/);
});

test('generated requirement files come from the Go candidate generator', () => {
  const { dir, sha } = repository();
  try {
    const changes = new Map([['.release-please-manifest.json',
      { content: JSON.stringify({ '.': '0.2.0', 'providers/openai': '0.2.0' }) }]]);
    const { files, checksums } = generateRequirements({ sha, changes, source: dir });
    assert.deepEqual([...files.keys()], ['providers/openai/go.mod', 'providers/openai/go.sum']);
    assert.match(files.get('providers/openai/go.mod'), new RegExp(`require ${core} v0\\.2\\.0`));
    assert.match(files.get('providers/openai/go.sum'), new RegExp(`^${core} v0\\.2\\.0 h1:`, 'm'));
    assert.deepEqual(Object.keys(checksums).sort(), [`${core}/providers/openai@v0.2.0`, `${core}@v0.2.0`]);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('requirement updates are appended to the release candidate', async () => {
  const candidate = { updates: [{ path: '.release-please-manifest.json' }, { path: 'providers/openai/CHANGELOG.md' }] };
  const github = { buildChangeSet: async updates => new Map(updates.map(update => [update.path, { content: 'x' }])) };
  const generate = async ({ sha, changes }) => {
    assert.equal(sha, 'a'.repeat(40));
    assert.deepEqual([...changes.keys()], ['.release-please-manifest.json', 'providers/openai/CHANGELOG.md']);
    return { files: new Map([['ai-gateway/go.mod', 'gateway\n']]), checksums: {} };
  };
  const result = await withRequirements(github, candidate, { sha: 'a'.repeat(40), config: combined, generate });
  assert.deepEqual(result, { components: ['providers/openai'], files: ['ai-gateway/go.mod'], checksums: {} });
  const added = candidate.updates.at(-1);
  assert.equal(added.path, 'ai-gateway/go.mod');
  assert.equal(added.createIfMissing, true);
  assert.equal(added.updater.updateContent('old'), 'gateway\n');
});

test('generator builds see only Go cache and toolchain locations', () => {
  const env = goEnvironment({ PATH: '/bin', GOCACHE: '/cache', GOMODCACHE: '/mod', GOPROXY: 'https://evil.example',
    GOFLAGS: '-mod=mod', GONOSUMDB: '*', GOWORK: '/elsewhere/go.work' });
  assert.equal(env.GOCACHE, '/cache');
  assert.equal(env.GOMODCACHE, '/mod');
  assert.equal(env.GOFLAGS, '-mod=readonly');
  assert.equal(env.GOWORK, 'off');
  assert.equal(env.GOPROXY, undefined);
  assert.equal(env.GONOSUMDB, undefined);
});
