import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { setLogger } from 'release-please';
import { discoverMerged, REPOSITORY_ID, BOT_ID } from './merged-candidates.mjs';
import { successfulRun, readinessArtifact, downloadArtifact, verifyMergedIdentity, SOURCE_JOBS } from './authorization-evidence.mjs';
import { authorizeInventory, childEnvironment } from './authorize.mjs';

const sha = letter => letter.repeat(40);
const base = sha('a'), headSha = sha('b'), mergeSha = sha('c'), treeSha = sha('d');
const config = JSON.parse(readFileSync(new URL('../../release-please-config.json', import.meta.url)));
const notes = '## 0.1.0-alpha.2 (2026-10-22)\n\n* Fix';
const title = 'chore(main): release 0.1.0-alpha.2';
const body = `Header\n---\n\n${notes}\n\n---\nFooter`;
const branch = 'release-please--branches--main';
setLogger(Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(key => [key, () => {}])));

function fixture() {
  const repo = { id: REPOSITORY_ID, full_name: 'grafana/ai-sdk' };
  const pr = { number: 10, title, body, state: 'closed', merged: true, draft: false,
    merge_commit_sha: mergeSha, user: { id: BOT_ID, login: 'grafana-ai-sdk-release[bot]', type: 'Bot' },
    head: { sha: headSha, ref: branch, repo }, base: { sha: mergeSha, ref: 'main', repo } };
  const candidate = { repositoryId: REPOSITORY_ID, component: '.', version: '0.1.0-alpha.2',
    tag: 'v0.1.0-alpha.2', notes, name: 'v0.1.0-alpha.2', prNumber: 10, headSha, mergeSha,
    parents: [base], treeSha, config, pr };
  const head = { sha: headSha, author: { id: BOT_ID }, commit: { verification: { verified: true, reason: 'valid' } } };
  const missing = async () => { throw Object.assign(new Error('missing'), { status: 404 }); };
  const github = { repository: { owner: 'grafana', repo: 'ai-sdk', defaultBranch: 'main' },
    getFileJson: async path => path === 'release-please-config.json' ? config : { '.': '0.1.0-alpha.1' },
    async *pullRequestIterator(target, state, max, files) {
      assert.equal(target, 'main'); assert.equal(state, 'MERGED');
      assert.equal(max, Number.MAX_SAFE_INTEGER); assert.equal(files, false);
      for (let i = 0; i < 200; i++) yield { number: i + 100, labels: [], title: 'fix: ordinary', body: '', sha: base };
      yield { number: 10, title, body, sha: mergeSha, labels: ['autorelease: pending'],
        headBranchName: branch, baseBranchName: 'main', files: [] };
    },
    octokit: {
      repos: { getCommit: async ({ ref }) => ({ data: ref === headSha ? head :
        { sha: ref === 'main' ? base : mergeSha, parents: [{ sha: base }], commit: { tree: { sha: treeSha } } } }),
      getReleaseByTag: missing, compareCommits: async () => ({ data: { status: 'ahead' } }) },
      pulls: { get: async () => ({ data: structuredClone(pr) }) },
      git: { getRef: missing },
      actions: { getWorkflow: async ({ workflow_id }) => ({ data: { id: workflow_id === 'ci.yml' ? 100 : 200 } }),
        listWorkflowRuns() {}, listWorkflowRunArtifacts() {} },
      checks: { getSuite: async () => ({ data: { app: { slug: 'github-actions', id: 15368 } } }) },
      paginate: async (route, params) => {
        if (route === github.octokit.actions.listWorkflowRuns) {
          return [{ id: params.workflow_id, run_attempt: 1, workflow_id: params.workflow_id,
            path: `.github/workflows/${params.workflow_id === 100 ? 'ci.yml' : 'release-readiness.yml'}`,
            event: params.event, head_sha: params.head_sha, head_branch: params.event === 'push' ? 'main' : branch,
            repository: repo, head_repository: repo, status: 'completed', conclusion: 'success', check_suite_id: 99 }];
        }
        if (route === github.octokit.actions.listWorkflowRunArtifacts) {
          return [{ id: 77, name: 'library-readiness-200-1', expired: false, size_in_bytes: 500,
            workflow_run: { id: 200, head_sha: headSha, repository_id: REPOSITORY_ID, head_repository_id: REPOSITORY_ID } }];
        }
        assert.match(route, /attempts/);
        return (params.run_id === 100 ? SOURCE_JOBS : ['release-readiness']).map(name => ({ name,
          run_id: params.run_id, head_sha: params.run_id === 100 ? mergeSha : headSha,
          status: 'completed', conclusion: 'success' }));
      },
    },
  };
  return { github, candidate, head, inventory: { mainSha: base, candidates: [candidate] } };
}

test('pinned merged release discovery includes the 201st PR and freezes root-only records', async () => {
  const { github } = fixture();
  const inventory = await discoverMerged(github, '.');
  assert.equal(inventory.candidates.length, 1);
  assert.equal(inventory.candidates[0].mergeSha, mergeSha);
  assert.equal(inventory.candidates[0].tag, 'v0.1.0-alpha.2');
  assert.throws(() => { inventory.candidates[0].pr.title = 'edited'; });
  github.pullRequestIterator = async function* () {};
  assert.equal((await discoverMerged(github)).candidates.length, 0);
  await assert.rejects(discoverMerged(github, 'unknown'), /unregistered/);
});

test('partial discovery and main movement never return an inventory', async () => {
  const { github } = fixture();
  github.pullRequestIterator = async function* () { yield {}; throw new Error('API outage'); };
  await assert.rejects(discoverMerged(github), /API outage/);
  const changed = fixture().github;
  const get = changed.octokit.repos.getCommit;
  let reads = 0;
  changed.octokit.repos.getCommit = async params => params.ref === 'main' && reads++ ? { data: { sha: sha('e') } } : get(params);
  await assert.rejects(discoverMerged(changed), /main advanced/);
});

test('authorization keeps ready root independent of blocked provider and Gateway', async () => {
  const { github, candidate, inventory } = fixture();
  inventory.candidates.push({ ...candidate, component: 'providers/bedrock', tag: 'providers/bedrock/v0.1.0', prNumber: 11,
    pr: { ...candidate.pr, number: 11 } }, { ...candidate, component: 'ai-gateway', tag: 'ai-gateway/v0.1.0', prNumber: 12 });
  const calls = [];
  const report = await authorizeInventory(github, inventory, async candidate => {
    calls.push(candidate.component);
    if (candidate.component !== '.') throw new Error('unpublished prerequisite');
    return { baseSha: base };
  }, sha('f'));
  assert.deepEqual(report.candidates.map(item => item.status), ['approved', 'blocked', 'blocked']);
  assert.deepEqual(calls, ['.', 'providers/bedrock']);
  assert.equal(report.publicationEnabled, false);
});

test('duplicate claims block before validation and new arrivals cannot join the frozen batch', async () => {
  const { github, candidate, inventory } = fixture();
  inventory.candidates.push({ ...candidate, prNumber: 11 });
  const report = await authorizeInventory(github, inventory, () => assert.fail('duplicate cannot validate'), base);
  assert.ok(report.candidates.every(item => item.status === 'blocked'));
  const fresh = fixture();
  let discoveries = 0;
  fresh.github.pullRequestIterator = async function* () { discoveries++; yield candidate; };
  const result = await authorizeInventory(fresh.github, fresh.inventory, async () => ({}), base);
  assert.equal(result.candidates.length, 1);
  assert.equal(result.candidates[0].status, 'approved');
  assert.equal(discoveries, 0);
  assert.deepEqual((await authorizeInventory(github, { mainSha: base, candidates: [] }, () => assert.fail(), base)).candidates, []);
});

test('unsigned, spoofed and unsupported identities fail before library validation', () => {
  for (const mutate of [
    ({ candidate }) => { candidate.pr.user.id = 1; },
    ({ candidate }) => { candidate.pr.head.repo.id = 1; },
    ({ candidate }) => { candidate.parents.push(base); },
    ({ head }) => { head.commit.verification.verified = false; },
    ({ head }) => { head.author.id = 1; },
  ]) {
    const data = fixture(); mutate(data);
    assert.throws(() => verifyMergedIdentity(data.candidate, data.head));
  }
});

test('source checks bind workflow, repository, revision, event and latest attempts', async () => {
  for (const [field, value] of [['status', 'queued'], ['conclusion', 'cancelled'],
    ['head_sha', base], ['event', 'workflow_dispatch'], ['path', '.github/workflows/other.yml'],
    ['workflow_id', 999], ['repository', { id: 1 }], ['head_repository', { id: 1 }]]) {
    const { github } = fixture();
    const paginate = github.octokit.paginate;
    github.octokit.paginate = async (route, params) => {
      const data = await paginate(route, params);
      if (route === github.octokit.actions.listWorkflowRuns) data[0][field] = value;
      return data;
    };
    await assert.rejects(successfulRun(github, 'ci.yml', 'push', mergeSha, 'main', SOURCE_JOBS), field);
  }
  for (const failure of ['skipped', 'missing', 'wrong-sha', 'spoof', 'newer-pending']) {
    const { github } = fixture();
    const paginate = github.octokit.paginate;
    github.octokit.paginate = async (route, params) => {
      const data = await paginate(route, params);
      if (typeof route === 'string') {
        if (failure === 'skipped') data[0].conclusion = 'skipped';
        if (failure === 'missing') data.pop();
        if (failure === 'wrong-sha') data[0].head_sha = base;
      } else if (failure === 'newer-pending') data.push({ ...data[0], id: 999, status: 'in_progress' });
      return data;
    };
    if (failure === 'spoof') github.octokit.checks.getSuite = async () => ({ data: { app: { id: 1, slug: 'github-actions' } } });
    await assert.rejects(successfulRun(github, 'ci.yml', 'push', mergeSha, 'main', SOURCE_JOBS), failure);
  }
});

test('artifact lookup rejects absent, expired, other-run and other-head evidence', async () => {
  for (const mutate of [data => [], data => [{ ...data[0], expired: true }],
    data => [{ ...data[0], name: 'library-readiness-200-2' }],
    data => [{ ...data[0], workflow_run: { ...data[0].workflow_run, head_sha: base } }]]) {
    const { github } = fixture();
    const paginate = github.octokit.paginate;
    github.octokit.paginate = async (...args) => mutate(await paginate(...args));
    await assert.rejects(readinessArtifact(github, { id: 200, attempt: 1, headSha }));
  }
});

test('artifact downloader follows only a bounded trusted storage GET without credentials', async () => {
  const requests = [];
  const fetch = async (url, options) => {
    requests.push({ url, options });
    return requests.length === 1 ? new Response(null, { status: 302,
      headers: { location: 'https://example.blob.core.windows.net/artifact?signature=example' } }) : new Response('zip');
  };
  assert.equal((await downloadArtifact(77, 'secret', fetch)).toString(), 'zip');
  assert.equal(requests[0].options.headers.authorization, 'Bearer secret');
  assert.equal(requests[1].options.headers, undefined);
  assert.equal(requests[1].options.redirect, 'error');
  for (const url of ['http://example.blob.core.windows.net/x', 'https://evil.test/x',
    'https://user:password@example.blob.core.windows.net/x', 'https://example.blob.core.windows.net:8443/x']) {
    await assert.rejects(downloadArtifact(77, 'secret', async () => new Response(null, { status: 302, headers: { location: url } })));
  }
  let request = 0;
  await assert.rejects(downloadArtifact(77, 'secret', async () => ++request === 1
    ? new Response(null, { status: 302, headers: { location: 'https://example.blob.core.windows.net/x' } })
    : new Response(Buffer.alloc(1024 * 1024 + 1))), /limit/);
});

test('final checks reject metadata changes, reruns, tag appearance and lost ancestry', async () => {
  for (const failure of ['body', 'rerun', 'tag', 'release', 'ancestry', 'proxy']) {
    const { github, inventory } = fixture();
    const report = await authorizeInventory(github, inventory, async () => {
      if (failure === 'body') github.octokit.pulls.get = async () => ({ data: { ...inventory.candidates[0].pr, body: 'edited' } });
      if (failure === 'rerun') {
        const paginate = github.octokit.paginate;
        github.octokit.paginate = async (route, params) => {
          const data = await paginate(route, params);
          if (route === github.octokit.actions.listWorkflowRuns) data[0].run_attempt = 2;
          return data;
        };
      }
      if (failure === 'tag') github.octokit.git.getRef = async () => ({ data: { object: { sha: mergeSha } } });
      if (failure === 'release') github.octokit.repos.getReleaseByTag = async () => ({ data: {} });
      if (failure === 'ancestry') github.octokit.repos.compareCommits = async () => ({ data: { status: 'diverged' } });
      if (failure === 'proxy') throw new Error('proxy unavailable');
      return {};
    }, base);
    assert.equal(report.candidates[0].status, 'blocked', failure);
  }
});

test('candidate child processes receive no API credentials or executable injection environment', () => {
  const env = childEnvironment({ PATH: '/bin', HOME: '/home/test', GH_TOKEN: 'secret', GITHUB_TOKEN: 'secret',
    PYTHONPATH: '/candidate', NODE_OPTIONS: '--require=evil', GIT_CONFIG_COUNT: '1', GOROOT: '/wrong', GOFLAGS: '-modfile=evil' });
  assert.equal(env.PATH, '/bin');
  for (const key of ['GH_TOKEN', 'GITHUB_TOKEN', 'PYTHONPATH', 'NODE_OPTIONS', 'GIT_CONFIG_COUNT', 'GOROOT', 'GOFLAGS']) assert.equal(env[key], undefined);
});

test('unrelated image failure does not replace successful source-job evidence', async () => {
  const { github } = fixture();
  const paginate = github.octokit.paginate;
  github.octokit.paginate = async (route, params) => {
    const data = await paginate(route, params);
    if (route === github.octokit.actions.listWorkflowRuns) data[0].conclusion = 'failure';
    return data;
  };
  await successfulRun(github, 'ci.yml', 'push', mergeSha, 'main', SOURCE_JOBS);
  await assert.rejects(successfulRun(github, 'release-readiness.yml', 'pull_request', headSha, branch, ['release-readiness']));
});
