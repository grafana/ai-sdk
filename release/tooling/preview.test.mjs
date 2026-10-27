import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { setLogger } from 'release-please';
import { parseConventionalCommits } from 'release-please/build/src/commit.js';
import { prepareCandidates, preview, readOnlyFetch } from './preview.mjs';

const config = JSON.parse(readFileSync(new URL('../../release-please-config.json', import.meta.url)));
const silent = Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(level => [level, () => {}]));
setLogger(silent);

const generate = async () => ({ files: new Map([['providers/openai/go.mod', 'module openai\n']]), checksums: {} });

function repository() {
  return {
    repository: { owner: 'grafana', repo: 'ai-sdk', defaultBranch: 'main' },
    octokit: { repos: { getCommit: async () => ({ data: { sha: 'base' } }) } },
    getFileJson: async path => path === 'release-please-config.json' ? config : {},
    async *releaseIterator() {},
    async *tagIterator() {},
    async *mergeCommitIterator() {
      yield { sha: 'a'.repeat(40), message: 'fix: root behavior', files: ['stream.go'] };
      yield { sha: 'b'.repeat(40), message: 'fix: provider behavior', files: ['providers/openai/model.go'] };
      yield { sha: 'c'.repeat(40), message: 'fix: middleware behavior', files: ['middleware/logger/logger.go'] };
    },
    buildChangeSet: async updates => new Map(updates.map(update => [
      update.path, { originalContent: '', content: 'generated' },
    ])),
  };
}

test('pinned release-please selects root without downstream proposals', async () => {
  const candidates = await prepareCandidates(repository(), { path: '.' });
  assert.equal(candidates.length, 1);
  assert.equal(candidates[0].headRefName, 'release-please--branches--main');
  assert.deepEqual(candidates[0].updates.map(update => update.path), [
    'CHANGELOG.md', '.release-please-manifest.json',
  ]);
  assert.match(candidates[0].body.toString(), /root behavior/);
  assert.doesNotMatch(candidates[0].body.toString(), /provider behavior|middleware behavior/);
});

test('default preview preserves independent component proposals', async () => {
  const candidates = await prepareCandidates(repository());
  assert.deepEqual(candidates.map(candidate => candidate.headRefName), [
    'release-please--branches--main',
    'release-please--branches--main--components--providers/openai',
    'release-please--branches--main--components--middleware/logger',
  ]);
});

test('unknown component fails instead of returning a misleading empty preview', async () => {
  await assert.rejects(prepareCandidates(repository(), { path: 'providers/typo' }), /unregistered/);
});

test('preview refuses a branch that advances while candidates are built', async () => {
  const github = repository();
  let reads = 0;
  github.octokit.repos.getCommit = async () => ({ data: { sha: String(reads++) } });
  await assert.rejects(preview(github, { path: '.', generate }), /advanced/);
});

test('preview reports source revision and generated contents', async () => {
  const result = await preview(repository(), { path: '.', generate });
  assert.equal(result.sha, 'base');
  assert.equal(result.proposals.length, 1);
  assert.deepEqual(result.proposals[0].changes.map(change => change.path), [
    'CHANGELOG.md', '.release-please-manifest.json', 'providers/openai/go.mod',
  ]);
  assert.equal(result.proposals[0].changes.at(-1).after, 'generated');
});

test('combined preview shows one library release with generated requirement files and a separate Gateway release', async () => {
  const combined = { ...config, 'separate-pull-requests': false, packages: { ...config.packages,
    'ai-gateway': { ...config.packages['ai-gateway'], 'separate-pull-requests': true } } };
  const github = repository();
  github.getFileJson = async path => path === 'release-please-config.json' ? combined : {};
  const iterator = github.mergeCommitIterator;
  github.mergeCommitIterator = async function* () {
    yield* iterator();
    yield { sha: 'e'.repeat(40), message: 'fix: gateway behavior', files: ['ai-gateway/main.go'] };
  };
  const calls = [];
  const result = await preview(github, { generate: async options => { calls.push(options.sha); return generate(); } });
  assert.deepEqual(result.proposals.map(proposal => proposal.branch), [
    'release-please--branches--main', 'release-please--branches--main--components--ai-gateway',
  ]);
  assert.deepEqual(calls, ['base']);
  assert.ok(result.proposals[0].changes.some(change => change.path === 'providers/openai/go.mod'));
  assert.ok(result.proposals[1].changes.every(change => !change.path.endsWith('go.mod')));
  assert.match(result.proposals[0].body, /provider behavior/);
  assert.doesNotMatch(result.proposals[0].body, /gateway behavior/);
});

test('read-only transport blocks writes, GraphQL mutations, and other hosts', async () => {
  const sent = [];
  const fetch = readOnlyFetch(async (url, options) => { sent.push({ url, options }); });
  await fetch('https://api.github.com/repos/grafana/ai-sdk');
  await fetch('https://api.github.com/graphql', {
    method: 'POST', body: JSON.stringify({ query: 'query { viewer { login } }' }),
  });
  for (const [url, options] of [
    ['https://api.github.com/repos/grafana/ai-sdk/pulls', { method: 'POST' }],
    ['https://api.github.com/graphql', { method: 'POST', body: JSON.stringify({ query: 'mutation { createPullRequest { id } }' }) }],
    ['https://api.github.com/graphql', { method: 'POST', body: JSON.stringify({ query: 'query { viewer { login } } mutation { deleteRef { id } }' }) }],
    ['https://example.com/', {}],
    ['https://api.github.com/repos/grafana/ai-sdk/git/refs/tags/v1', { method: 'DELETE' }],
  ]) {
    await assert.rejects(fetch(url, options), /refused/);
  }
  assert.equal(sent.length, 2);
  assert.ok(sent.every(request => request.options.redirect === 'error'));
});

test('supported commit override recovers intent from an unparsable dependency body', () => {
  const commit = {
    sha: 'd'.repeat(40), files: ['package.json'],
    message: 'fix(deps): update dependencies\n\n[link](https://example.com/\n',
  };
  assert.deepEqual(parseConventionalCommits([commit], silent), []);
  commit.pullRequest = {
    body: 'Original description\n\nBEGIN_COMMIT_OVERRIDE\nfix(deps): update dependencies\nEND_COMMIT_OVERRIDE',
  };
  const parsed = parseConventionalCommits([commit], silent);
  assert.equal(parsed.length, 1);
  assert.equal(parsed[0].type, 'fix');
  assert.equal(parsed[0].scope, 'deps');
});
