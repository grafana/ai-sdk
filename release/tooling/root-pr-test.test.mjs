import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { GitHub, setLogger } from 'release-please';
import { assertContext, assertCandidate, createRootTest, rootWriteTransport, ROOT_BRANCH } from './root-pr-exercise.mjs';
import { verifyIdentity, verifyChecks } from './verify-app-pr.mjs';

const sha = 'a'.repeat(40);
const tree = 'b'.repeat(40);
const commit = 'c'.repeat(40);
const repo = { owner: 'grafana', repo: 'ai-sdk', defaultBranch: 'main' };
const config = JSON.parse(readFileSync(new URL('../../release-please-config.json', import.meta.url)));
const silent = Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(key => [key, () => {}]));
setLogger(silent);

function fixture() {
  const github = {
    repository: repo,
    octokit: { repos: { getCommit: async () => ({ data: { sha, commit: { tree: { sha: tree } } } }) },
      git: { getRef: async () => { throw Object.assign(new Error('missing'), { status: 404 }); } } },
    getFileJson: async path => path === 'release-please-config.json' ? config : { '.': '0.1.0-alpha.1' },
    async *releaseIterator() {}, async *tagIterator() {},
    async *mergeCommitIterator() { yield { sha: 'd'.repeat(40), message: 'fix: behavior', files: ['stream.go'] }; },
    buildChangeSet: async updates => new Map(updates.map(update => {
      const originalContent = update.path === 'CHANGELOG.md' ? '# Changelog\n' : '{".":"0.1.0-alpha.1"}\n';
      return [update.path, { originalContent, content: update.updater.updateContent(originalContent), mode: '100644' }];
    })),
    createPullRequest: async () => ({ number: 99 }),
  };
  const transport = { arm(value) { this.candidate = value; } };
  return { github, transport };
}

const env = { GITHUB_REPOSITORY: 'grafana/ai-sdk', GITHUB_EVENT_NAME: 'push',
  GITHUB_REF: 'refs/heads/main', GITHUB_SHA: sha, RELEASE_APP_PR_TEST: 'root' };

test('write exercise requires canonical push context and explicit opt-in', () => {
  assertContext(env);
  for (const [key, value] of [['GITHUB_EVENT_NAME', 'workflow_dispatch'], ['GITHUB_REF', 'refs/heads/other'],
    ['GITHUB_REPOSITORY', 'other/repo'], ['RELEASE_APP_PR_TEST', ''], ['GITHUB_SHA', 'main']]) {
    assert.throws(() => assertContext({ ...env, [key]: value }));
  }
});

test('preparation freezes a single root proposal and generated file contents', async () => {
  const { github, transport } = fixture();
  assert.equal(await createRootTest(github, transport, sha, []), 99);
  assert.equal(transport.candidate.sha, sha);
  assert.equal(transport.candidate.title, 'chore(main): release 0.1.0-alpha.2');
  assert.deepEqual([...transport.candidate.changes.keys()].sort(), ['.release-please-manifest.json', 'CHANGELOG.md']);
});

test('existing branches, stale bases, and parser diagnostics stop before writes', async () => {
  for (const failure of ['branch', 'base', 'parser', 'advance']) {
    const { github, transport } = fixture();
    github.createPullRequest = async () => assert.fail('must not write');
    if (failure === 'branch') github.octokit.git.getRef = async () => ({});
    if (failure === 'base') github.octokit.repos.getCommit = async () => ({ data: { sha: 'wrong' } });
    if (failure === 'advance') {
      const get = github.octokit.repos.getCommit;
      let reads = 0;
      github.octokit.repos.getCommit = async () => reads++ ? { data: { sha: 'new' } } : get();
    }
    await assert.rejects(createRootTest(github, transport, sha, failure === 'parser' ? ['unparsed commit'] : []));
    assert.equal(transport.candidate, undefined);
  }
});

test('candidate validation rejects downstream, extra files, and wrong manifest versions', () => {
  const candidate = { headRefName: ROOT_BRANCH, draft: false, version: '0.1.0-alpha.2',
    title: 'chore(main): release 0.1.0-alpha.2', updates: [{ path: 'CHANGELOG.md' }, { path: '.release-please-manifest.json' }] };
  const changes = new Map([
    ['CHANGELOG.md', { content: '# Changelog' }],
    ['.release-please-manifest.json', { originalContent: '{".":"0.1.0-alpha.1"}', content: '{".":"0.1.0-alpha.2"}' }],
  ]);
  assertCandidate(candidate, changes);
  assert.throws(() => assertCandidate({ ...candidate, headRefName: `${ROOT_BRANCH}--components--providers/openai` }, changes));
  assert.throws(() => assertCandidate(candidate, new Map([...changes, ['go.mod', { content: 'bad' }]])));
  changes.get('.release-please-manifest.json').content = '{".":"0.1.0-alpha.3"}';
  assert.throws(() => assertCandidate(candidate, changes));
});

async function realWriter({ signed = true } = {}) {
  const { github: source, transport: captured } = fixture();
  await createRootTest(source, captured, sha, []);
  const approved = captured.candidate;
  const requests = [];
  const transport = rootWriteTransport(async (url, options) => {
    const path = new URL(url).pathname;
    const method = options.method ?? 'GET';
    const body = options.body ? JSON.parse(options.body) : undefined;
    requests.push({ method, path, body });
    let data, status = 200;
    if (method === 'GET' && path.endsWith('/branches/main')) data = { commit: { sha } };
    else if (method === 'GET' && path.includes('/git/ref/')) { data = { message: 'Not Found' }; status = 404; }
    else if (method === 'GET' && path.endsWith(`/git/commits/${sha}`)) data = { tree: { sha: tree } };
    else if (method === 'POST' && path.endsWith('/git/refs')) data = { ref: body.ref };
    else if (method === 'POST' && path.endsWith('/git/trees')) data = { sha: 'new-tree' };
    else if (method === 'POST' && path.endsWith('/git/commits')) data = { sha: commit, verification: { verified: signed, reason: signed ? 'valid' : 'unsigned' } };
    else if (method === 'PATCH' && path.includes('/git/refs/')) data = {};
    else if (method === 'GET' && path.endsWith('/pulls')) data = [];
    else if (method === 'POST' && path.endsWith('/pulls')) data = { number: 99 };
    else if (method === 'POST' && path.endsWith('/labels')) data = body.labels.map(name => ({ name }));
    else if (method === 'GET' && path.endsWith('/pulls/99')) data = {
      number: 99, head: { ref: ROOT_BRANCH }, base: { ref: 'main' }, labels: [], title: approved.title, body: approved.body,
    };
    else throw new Error(`unexpected upstream request: ${method} ${path}`);
    return new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } });
  });
  transport.arm(approved);
  const github = await GitHub.create({ ...repo, defaultBranch: 'main', logger: silent, fetch: transport.fetch });
  github.buildChangeSet = async () => approved.changes;
  const run = () => github.createPullRequest({ title: approved.title, body: approved.body,
    headBranchName: ROOT_BRANCH, labels: approved.labels }, 'main', approved.title, [], { fork: false, draft: false });
  return { run, requests, transport };
}

test('pinned release-please writer passes only the bounded signed root PR requests', async () => {
  const { run, requests, transport } = await realWriter();
  assert.equal((await run()).number, 99);
  assert.equal(requests.find(request => request.method === 'PATCH').body.force, false);
  const write = (route, body) => transport.fetch(`https://api.github.com/repos/grafana/ai-sdk/${route}`,
    { method: 'POST', body: JSON.stringify(body) });
  await assert.rejects(write('releases', { tag_name: 'v1' }), /refused/);
  await assert.rejects(write('git/tags', { tag: 'v1' }), /refused/);
  await assert.rejects(write('git/refs', { ref: 'refs/tags/v1', sha }));
});

test('unsigned App commit cannot advance the root branch or open a PR', async () => {
  const { run, requests } = await realWriter({ signed: false });
  await assert.rejects(run());
  assert.ok(!requests.some(request => request.method === 'PATCH' || request.method === 'POST' && request.path.endsWith('/pulls')));
});

function evidence() {
  const repository = { id: 1315458055, full_name: 'grafana/ai-sdk' };
  const pr = { number: 99, state: 'open', draft: false,
    user: { id: 332654929, login: 'grafana-ai-sdk-release[bot]', type: 'Bot' },
    head: { sha: commit, ref: ROOT_BRANCH, repo: repository }, base: { ref: 'main', repo: repository } };
  const result = { sha: commit, author: { id: 332654929 }, parents: [{ sha }],
    commit: { verification: { verified: true, reason: 'valid' } },
    files: [{ filename: 'CHANGELOG.md' }, { filename: '.release-please-manifest.json' }] };
  return { pr, result };
}

test('verification requires the immutable bot identity, signed commit and expected parent', () => {
  const { pr, result } = evidence();
  verifyIdentity(pr, result, sha);
  assert.throws(() => verifyIdentity({ ...pr, user: { ...pr.user, id: 1 } }, result, sha));
  assert.throws(() => verifyIdentity(pr, { ...result, parents: [{ sha: 'new-main' }] }, sha));
  assert.throws(() => verifyIdentity(pr, { ...result, files: [...result.files, { filename: 'go.mod' }] }, sha));
});

test('verification requires PR-triggered source and readiness jobs at the exact head', async () => {
  const { pr } = evidence();
  const github = { octokit: { actions: {
    listWorkflowRuns: async () => ({ data: { workflow_runs: [{ id: 1, event: 'pull_request', head_sha: commit,
      head_branch: ROOT_BRANCH, head_repository: { id: 1315458055 }, actor: { id: 332654929 },
      pull_requests: [{ number: 99 }], status: 'completed', conclusion: 'success' }] } }),
    listJobsForWorkflowRun() {},
  }, paginate: async () => [{ name: 'ci', conclusion: 'success' }, { name: 'release-readiness', conclusion: 'success' }] } };
  assert.equal(await verifyChecks(github, pr), true);
  github.octokit.paginate = async () => [{ name: 'ci', conclusion: 'success' }, { name: 'release-readiness', conclusion: 'skipped' }];
  await assert.rejects(verifyChecks(github, pr));
  github.octokit.actions.listWorkflowRuns = async () => ({ data: { workflow_runs: [] } });
  assert.equal(await verifyChecks(github, pr), false);
});
