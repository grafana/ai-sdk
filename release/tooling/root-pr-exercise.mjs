import assert from 'node:assert/strict';
import { appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { GitHub, setLogger } from 'release-please';
import { prepareCandidates, readOnlyFetch } from './preview.mjs';

export const ROOT_BRANCH = 'release-please--branches--main';
const repo = { owner: 'grafana', repo: 'ai-sdk' };
const prefix = '/repos/grafana/ai-sdk';

export function assertContext(env) {
  assert.equal(env.GITHUB_REPOSITORY, 'grafana/ai-sdk');
  assert.equal(env.GITHUB_EVENT_NAME, 'push');
  assert.equal(env.GITHUB_REF, 'refs/heads/main');
  assert.equal(env.RELEASE_APP_PR_TEST, 'root');
  assert.match(env.GITHUB_SHA ?? '', /^[a-f0-9]{40}$/);
}

export function assertCandidate(candidate, changes) {
  assert.equal(candidate.headRefName, ROOT_BRANCH);
  assert.equal(candidate.draft, false);
  assert.equal(candidate.title.toString(), `chore(main): release ${candidate.version}`);
  assert.deepEqual([...changes.keys()].sort(), ['.release-please-manifest.json', 'CHANGELOG.md']);
  assert.deepEqual(candidate.updates.map(update => update.path).sort(), [...changes.keys()].sort());
  const manifest = changes.get('.release-please-manifest.json');
  const before = JSON.parse(manifest.originalContent);
  const after = JSON.parse(manifest.content);
  assert.deepEqual(Object.keys(after).sort(), Object.keys(before).sort());
  assert.notEqual(before['.'], after['.']);
  assert.equal(after['.'], candidate.version.toString());
  for (const key of Object.keys(before)) if (key !== '.') assert.equal(before[key], after[key]);
  assert.ok(changes.get('CHANGELOG.md').content);
}

export function rootWriteTransport(fetchImpl = globalThis.fetch) {
  const read = readOnlyFetch(fetchImpl);
  let approved, createdBranch = false, treeSha, commitSha, prNumber;
  return {
    arm(candidate) { assert.equal(approved, undefined); approved = candidate; },
    async fetch(url, options = {}) {
      const method = (options.method ?? 'GET').toUpperCase();
      const target = new URL(url);
      if (method === 'GET' || target.pathname === '/graphql') return read(url, options);
      assert.ok(approved, 'writes require a frozen candidate');
      assert.equal(target.origin, 'https://api.github.com');
      const body = JSON.parse(options.body);
      const route = `${method} ${decodeURIComponent(target.pathname)}`;
      if (route === `POST ${prefix}/git/refs`) {
        assert.equal(createdBranch, false);
        assert.deepEqual(body, { ref: `refs/heads/${ROOT_BRANCH}`, sha: approved.sha });
      } else if (route === `POST ${prefix}/git/trees`) {
        assert.equal(createdBranch, true);
        assert.equal(treeSha, undefined);
        assert.equal(body.base_tree, approved.tree);
        assert.equal(body.tree.length, approved.changes.size);
        assert.equal(new Set(body.tree.map(entry => entry.path)).size, approved.changes.size);
        for (const entry of body.tree) {
          assert.deepEqual(entry, {
            path: entry.path, mode: '100644', type: 'blob',
            content: approved.changes.get(entry.path)?.content,
          });
          assert.ok(approved.changes.has(entry.path));
        }
      } else if (route === `POST ${prefix}/git/commits`) {
        assert.ok(treeSha);
        assert.equal(commitSha, undefined);
        assert.deepEqual(body, { message: approved.title, tree: treeSha, parents: [approved.sha] });
      } else if (route === `PATCH ${prefix}/git/refs/heads/${ROOT_BRANCH}`) {
        assert.ok(commitSha);
        assert.equal(body.sha, commitSha);
        body.force = false;
      } else if (route === `POST ${prefix}/pulls`) {
        assert.ok(commitSha);
        assert.equal(body.head, `grafana:${ROOT_BRANCH}`);
        assert.equal(body.base, 'main');
        assert.equal(body.title, approved.title);
        assert.equal(body.body, approved.body);
        assert.equal(body.draft, false);
      } else if (prNumber && route === `POST ${prefix}/issues/${prNumber}/labels`) {
        assert.deepEqual(body.labels, approved.labels);
      } else {
        throw new Error(`root PR test refused write: ${route}`);
      }
      const response = await fetchImpl(url, {
        ...options, body: JSON.stringify(body), redirect: 'error', signal: AbortSignal.timeout(30_000),
      });
      if (response.ok) {
        const result = await response.clone().json();
        if (route === `POST ${prefix}/git/refs`) createdBranch = true;
        if (route === `POST ${prefix}/git/trees`) treeSha = result.sha;
        if (route === `POST ${prefix}/git/commits`) {
          assert.equal(result.verification?.verified, true, 'App commit must be signed before updating the branch');
          assert.equal(result.verification.reason, 'valid');
          commitSha = result.sha;
        }
        if (route === `POST ${prefix}/pulls`) prNumber = result.number;
      }
      return response;
    },
  };
}

export async function createRootTest(github, transport, sha, diagnostics) {
  const head = async () => (await github.octokit.repos.getCommit({ ...repo, ref: 'main' })).data;
  const base = await head();
  assert.equal(base.sha, sha, 'workflow revision must still be main');
  try {
    await github.octokit.git.getRef({ ...repo, ref: `heads/${ROOT_BRANCH}` });
    throw new Error('root release branch already exists; inspect it instead of overwriting it');
  } catch (error) {
    if (error.status !== 404) throw error;
  }
  const candidates = await prepareCandidates(github, { path: '.', manifestOptions: { separatePullRequests: true } });
  assert.equal(candidates.length, 1, 'expected exactly one root candidate');
  assert.equal(diagnostics.length, 0, 'resolve commit parsing warnings before App writes');
  const [candidate] = candidates;
  const changes = await github.buildChangeSet(candidate.updates, 'main');
  assertCandidate(candidate, changes);
  assert.equal((await head()).sha, sha, 'main advanced during preparation');
  transport.arm({ sha, tree: base.commit.tree.sha, changes, title: candidate.title.toString(),
    body: candidate.body.toString(), labels: candidate.labels });
  github.buildChangeSet = async () => changes;
  const pr = await github.createPullRequest({
    title: candidate.title.toString(), body: candidate.body.toString(),
    headBranchName: ROOT_BRANCH, labels: candidate.labels,
  }, 'main', candidate.title.toString(), candidate.updates, { fork: false, draft: false });
  assert.ok(pr.number > 0);
  assert.equal((await head()).sha, sha, 'main advanced during creation; inspect the PR and rerun readiness');
  return pr.number;
}

async function main() {
  assertContext(process.env);
  assert.ok(process.env.GH_TOKEN, 'GH_TOKEN is required');
  const diagnostics = [];
  const logger = Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(level => [level, (...values) => {
    if (values.some(value => String(value).includes('commit could not be parsed:'))) diagnostics.push(values.join(' '));
    console.error(...values);
  }]));
  setLogger(logger);
  const transport = rootWriteTransport();
  const github = await GitHub.create({ ...repo, defaultBranch: 'main', token: process.env.GH_TOKEN,
    logger, fetch: transport.fetch });
  const scope = await github.octokit.request('GET /installation/repositories');
  assert.equal(scope.data.total_count, 1);
  assert.deepEqual(scope.data.repositories.map(item => [item.id, item.full_name]), [[1315458055, 'grafana/ai-sdk']]);
  const number = await createRootTest(github, transport, process.env.GITHUB_SHA, diagnostics);
  if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `pr_number=${number}\n`);
  console.log(`Created root test PR https://github.com/grafana/ai-sdk/pull/${number}; do not merge during the exercise.`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
