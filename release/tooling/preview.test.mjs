import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { setLogger } from 'release-please';
import { parseConventionalCommits } from 'release-please/build/src/commit.js';
import { prepareCandidates, preview, readOnlyFetch } from './preview.mjs';

const config = JSON.parse(readFileSync(new URL('../../release-please-config.json', import.meta.url)));
const silent = Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(level => [level, () => {}]));
setLogger(silent);

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

test('default preview combines library proposals into one release pull request', async () => {
  const candidates = await prepareCandidates(repository());
  assert.deepEqual(candidates.map(candidate => candidate.headRefName), ['release-please--branches--main']);
  assert.equal(candidates[0].title.toString(), 'chore: release main');
  assert.deepEqual(candidates[0].updates.map(update => update.path).sort(), [
    '.release-please-manifest.json', 'CHANGELOG.md', 'middleware/logger/CHANGELOG.md', 'providers/openai/CHANGELOG.md',
  ]);
});

test('unknown component fails instead of returning a misleading empty preview', async () => {
  await assert.rejects(prepareCandidates(repository(), { path: 'providers/typo' }), /unregistered/);
});

test('preview refuses a branch that advances while candidates are built', async () => {
  const github = repository();
  let reads = 0;
  github.octokit.repos.getCommit = async () => ({ data: { sha: String(reads++) } });
  await assert.rejects(preview(github, { path: '.' }), /advanced/);
});

test('preview reports source revision and generated contents', async () => {
  const result = await preview(repository(), { path: '.' });
  assert.equal(result.sha, 'base');
  assert.equal(result.proposals.length, 1);
  assert.deepEqual(result.proposals[0].changes.map(change => change.path), [
    'CHANGELOG.md', '.release-please-manifest.json',
  ]);
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
