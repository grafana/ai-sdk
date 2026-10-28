import assert from 'node:assert/strict';
import test from 'node:test';
import { GitHub, Manifest } from 'release-please';
import { readFileSync } from 'node:fs';
import { fixture, base, config } from './authorization-fixture.mjs';
import { SOURCE_JOBS } from './authorization-evidence.mjs';
import { activationContext, selectInventory, verifyProtections, waitForToolingCI, preparationTransport, prepareLibraries } from './activation.mjs';

const approval = { id: 1, updatedAt: '2026-10-22T00:00:00Z' };
const env = { RELEASE_LIBRARY_MODE: 'root', GITHUB_REPOSITORY: 'grafana/ai-sdk',
  GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/heads/main', GITHUB_SHA: base };

test('activation rejects unapproved modes and noncanonical events', () => {
  assert.equal(activationContext(env), 'root');
  for (const change of [{ RELEASE_LIBRARY_MODE: '' }, { RELEASE_LIBRARY_MODE: 'all' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch' }, { GITHUB_REF: 'refs/heads/other' },
    { GITHUB_REPOSITORY: 'fork/ai-sdk' }, { GITHUB_SHA: 'main' }]) {
    assert.throws(() => activationContext({ ...env, ...change }));
  }
  const root = { component: '.', prNumber: 490, mergeSha: 'c8e497f3ca45571ef57c3556ebde32a836e8b918', tag: 'v0.1.0-alpha.2' };
  const inventory = { candidates: [root, { component: 'providers/openai' }, { component: 'ai-gateway' }] };
  assert.deepEqual(selectInventory(inventory, 'root').candidates, [root]);
  assert.equal(selectInventory(inventory, 'libraries').candidates.length, 2);
  assert.throws(() => selectInventory({ candidates: [{ ...root, prNumber: 491 }] }, 'root'));
});

function protectionFixture() {
  const repository = { id: 1315458055, allow_squash_merge: true, allow_merge_commit: false, allow_rebase_merge: false,
    squash_merge_commit_title: 'PR_TITLE', squash_merge_commit_message: 'PR_BODY' };
  const rules = [{ type: 'pull_request', parameters: { required_approving_review_count: 1 } },
    { type: 'required_status_checks', parameters: { strict_required_status_checks_policy: true,
      required_status_checks: [...SOURCE_JOBS, 'release-readiness', 'check', 'Validate Changes'].map(context => ({ context, integration_id: 15368 })) } }];
  const tag = { target: 'tag', enforcement: 'active', id: 1, updated_at: approval.updatedAt, bypass_actors: [],
    conditions: { ref_name: { include: ['~ALL'], exclude: [] } }, rules: [{ type: 'update' }, { type: 'deletion' }] };
  const github = { octokit: { repos: { get: async () => ({ data: repository }) },
    paginate: async route => route.endsWith('/rulesets') ? [tag] : rules,
    request: async () => ({ data: tag }) } };
  return { repository, rules, tag, github };
}

test('activation requires strict source/readiness checks and immutable tags', async () => {
  await verifyProtections(protectionFixture().github, approval);
  for (const mutate of [
    f => { f.repository.allow_merge_commit = true; },
    f => { f.repository.squash_merge_commit_title = 'COMMIT_OR_PR_TITLE'; },
    f => { f.rules[0].parameters.required_approving_review_count = 0; },
    f => { f.rules[1].parameters.strict_required_status_checks_policy = false; },
    f => { f.rules[1].parameters.required_status_checks = f.rules[1].parameters.required_status_checks.filter(c => c.context !== 'release-readiness'); },
    f => { f.rules[1].parameters.required_status_checks[0].integration_id = null; },
    f => { f.tag.enforcement = 'evaluate'; },
    f => { f.tag.bypass_actors.push({ actor_id: 1 }); },
    f => { f.tag.conditions.ref_name.exclude.push('v*'); },
    f => { f.tag.rules.pop(); },
    f => { f.tag.updated_at = '2026-10-23T00:00:00Z'; },
  ]) { const f = protectionFixture(); mutate(f); await assert.rejects(verifyProtections(f.github, approval)); }
});

test('repository settings use the App reader while rules use the workflow reader', async () => {
  const f = protectionFixture();
  const settings = { ...f.repository };
  for (const key of Object.keys(f.repository)) if (key !== 'id') delete f.repository[key];
  let reads = 0;
  const app = { octokit: { repos: { get: async () => { reads++; return { data: settings }; } } } };
  await assert.rejects(verifyProtections(f.github, approval));
  await verifyProtections(f.github, approval, app);
  await verifyProtections(f.github, approval, app);
  assert.equal(reads, 2);
  for (const key of ['allow_squash_merge', 'allow_merge_commit', 'allow_rebase_merge',
    'squash_merge_commit_title', 'squash_merge_commit_message']) {
    const value = settings[key];
    delete settings[key];
    await assert.rejects(verifyProtections(f.github, approval, app));
    settings[key] = value;
  }
  settings.allow_merge_commit = true;
  await assert.rejects(verifyProtections(f.github, approval, app));
  settings.allow_merge_commit = false;
  settings.id = 1;
  await assert.rejects(verifyProtections(f.github, approval, app));
  settings.id = 1315458055;
  f.rules[1].parameters.strict_required_status_checks_policy = false;
  await assert.rejects(verifyProtections(f.github, approval, app));
});

test('redacted bypass lists require exact maintainer approval rather than admin credentials', async () => {
  const f = protectionFixture(); delete f.tag.bypass_actors;
  await assert.rejects(verifyProtections(f.github));
  await verifyProtections(f.github, approval);
  await assert.rejects(verifyProtections(f.github, { ...approval, id: 2 }));
  await assert.rejects(verifyProtections(f.github, { ...approval, updatedAt: '2026-10-21T00:00:00Z' }));
});

test('tooling source CI waits boundedly for exact successful evidence', async () => {
  const { github } = fixture();
  const original = github.octokit.paginate;
  let ready = false, waits = 0;
  github.octokit.paginate = async (route, params) => {
    const result = await original(route, params);
    if (!ready && route === github.octokit.actions.listWorkflowRuns) result[0].status = 'in_progress';
    return result;
  };
  await waitForToolingCI(github, 'c'.repeat(40), { attempts: 2, wait: async () => { waits++; ready = true; } });
  assert.equal(waits, 1);
  ready = false;
  await assert.rejects(waitForToolingCI(github, 'c'.repeat(40), { attempts: 1 }));
});

test('preparation cannot publish, target Gateway, or advance unsigned commits', async () => {
  let writes = 0, checks = 0;
  const send = preparationTransport(async () => { checks++; }, async () => {
    writes++; return new Response(JSON.stringify({ verification: { verified: false } }));
  });
  for (const [method, route, body] of [
    ['POST', '/releases', {}], ['POST', '/git/refs', { ref: 'refs/tags/v1' }],
    ['PATCH', '/git/refs/heads/main', {}],
    ['POST', '/git/refs', { ref: 'refs/heads/release-please--branches--main--components--ai-gateway' }],
    ['POST', '/pulls', { base: 'main', head: 'grafana:other' }],
    ['POST', '/issues/490/comments', {}],
  ]) await assert.rejects(send('https://api.github.com/repos/grafana/ai-sdk' + route, { method, body: JSON.stringify(body) }));
  assert.equal(writes, 0);
  await assert.rejects(send('https://api.github.com/repos/grafana/ai-sdk/git/commits', { method: 'POST', body: JSON.stringify({ message: 'release', tree: base, parents: [base] }) }), /verified/);
  assert.equal(writes, 1); assert.equal(checks, 1);
});

test('Renovate never proposes first-party requirement updates', () => {
  const renovate = JSON.parse(readFileSync(new URL('../../renovate.json', import.meta.url)));
  const firstParty = renovate.packageRules.filter(rule => rule.matchPackageNames?.some(name => name.startsWith('github.com/grafana/ai-sdk')));
  assert.deepEqual(firstParty, [renovate.packageRules.at(-1)]);
  assert.deepEqual(firstParty[0].matchPackageNames, ['github.com/grafana/ai-sdk', 'github.com/grafana/ai-sdk/**']);
  assert.equal(firstParty[0].enabled, false);
  assert.equal(firstParty[0].matchUpdateTypes, undefined);
});


const combined = { ...config, 'separate-pull-requests': false, packages: { ...config.packages,
  'ai-gateway': { ...config.packages['ai-gateway'], 'separate-pull-requests': true } } };

const modules = {
  'go.mod': 'module github.com/grafana/ai-sdk\n\ngo 1.27.3\n',
  'providers/openai/go.mod': 'module github.com/grafana/ai-sdk/providers/openai\n\ngo 1.27.3\n\nrequire github.com/grafana/ai-sdk v0.1.0-alpha.1\n\nreplace github.com/grafana/ai-sdk => ../..\n',
  'providers/bedrock/go.mod': 'module github.com/grafana/ai-sdk/providers/bedrock\n\ngo 1.27.3\n\nrequire (\n\tgithub.com/grafana/ai-sdk v0.1.0-alpha.1 // indirect\n\tgithub.com/grafana/ai-sdk/providers/openai v0.1.0-alpha.1\n)\n',
};

function combinedFixture(settings = combined) {
  const source = fixture();
  const { github } = source;
  github.getFileJson = async path => path === 'release-please-config.json' ? structuredClone(settings) : { '.': '0.1.0-alpha.1', 'providers/openai': '0.1.0-alpha.1' };
  github.releaseIterator = async function* () {};
  github.tagIterator = async function* () {};
  github.findFilesByFilenameAndRef = async (name, ref) => { assert.equal(name, 'go.mod'); assert.equal(ref, 'main'); return Object.keys(modules); };
  github.getFileContentsOnBranch = async path => ({ parsedContent: modules[path] });
  return source;
}

const libraryChanges = async function* () {
  yield { sha: 'e'.repeat(40), message: 'fix: root change', files: ['stream.go'] };
  yield { sha: 'f'.repeat(40), message: 'fix: provider change', files: ['providers/openai/model.go'] };
  yield { sha: '1'.repeat(40), message: 'fix: logger change', files: ['middleware/logger/logger.go'] };
  yield { sha: '2'.repeat(40), message: 'fix: gateway change', files: ['ai-gateway/main.go'] };
};

function mergedOnly(github) {
  const iterator = github.pullRequestIterator;
  github.pullRequestIterator = async function* (branch, state, ...args) {
    if (state === 'MERGED') yield* iterator(branch, state, ...args);
  };
}

test('preparation combines pending libraries and skips merged pending components and Gateway', async () => {
  const { github } = combinedFixture();
  github.mergeCommitIterator = libraryChanges;
  mergedOnly(github);
  const created = [], updates = [];
  github.createPullRequest = async (pr, target, message, files) => { created.push(pr); updates.push(files); return { number: 999 }; };
  const result = await prepareLibraries(github, base);
  assert.deepEqual(result, [{ components: ['middleware/logger', 'providers/openai'],
    requirements: ['providers/bedrock/go.mod'], prNumber: 999 }]);
  assert.equal(created.length, 1);
  const bedrock = updates[0].find(update => update.path === 'providers/bedrock/go.mod');
  assert.equal(bedrock.updater.updateContent(modules['providers/bedrock/go.mod']),
    modules['providers/bedrock/go.mod'].replace('openai v0.1.0-alpha.1', 'openai v0.1.0-alpha.2'));
  assert.equal(created[0].headBranchName, 'release-please--branches--main');
  assert.equal(created[0].title, 'chore: release main');
  assert.match(created[0].body, /provider change[\s\S]*logger change|logger change[\s\S]*provider change/);
  assert.doesNotMatch(created[0].body, /gateway change|root change/);
  await assert.rejects(prepareLibraries(github, 'e'.repeat(40)), /advanced/);
});

test('a separately configured library keeps its own release pull request', async () => {
  const settings = structuredClone(combined);
  settings.packages['providers/openai']['separate-pull-requests'] = true;
  const { github } = combinedFixture(settings);
  github.mergeCommitIterator = libraryChanges;
  mergedOnly(github);
  const created = [];
  github.createPullRequest = async pr => { created.push(pr); return { number: 900 + created.length }; };
  const result = await prepareLibraries(github, base);
  assert.deepEqual(result.map(entry => entry.components).sort(), [['middleware/logger'], ['providers/openai']]);
  assert.deepEqual(result.find(entry => entry.components[0] === 'providers/openai').requirements, ['providers/bedrock/go.mod']);
  assert.deepEqual(created.map(pr => pr.headBranchName).sort(), [
    'release-please--branches--main', 'release-please--branches--main--components--providers/openai']);
});

test('preparation refreshes an unchanged open release against the current base', async () => {
  const { github } = combinedFixture();
  github.mergeCommitIterator = async function* () {
    yield { sha: 'e'.repeat(40), message: 'fix: root change', files: ['stream.go'] };
  };
  let openPR;
  github.pullRequestIterator = async function* (branch, state) { if (state === 'OPEN' && openPR) yield openPR; };
  github.createPullRequest = async pr => {
    openPR = { ...pr, number: 99 };
    return { number: 99 };
  };
  await prepareLibraries(github, base);
  let updated = 0;
  github.updatePullRequest = async (number, candidate) => {
    assert.equal(number, 99);
    assert.equal(candidate.body.toString(), openPR.body);
    updated++;
    return { number };
  };
  await prepareLibraries(github, base);
  assert.equal(updated, 1);
  openPR.labels = ['autorelease: snooze'];
  github.pullRequestIterator = async function* (branch, state) { if (state === 'CLOSED') yield openPR; };
  await prepareLibraries(github, base);
  assert.equal(updated, 1, 'unchanged closed snoozed PR must stay snoozed');
});

test('preparation rejects candidate files outside the manifest and library changelogs', async () => {
  const { github } = combinedFixture();
  github.mergeCommitIterator = async function* () {
    yield { sha: 'e'.repeat(40), message: 'fix: root change', files: ['stream.go'] };
  };
  github.pullRequestIterator = async function* () {};
  const original = Manifest.prototype.buildPullRequests;
  Manifest.prototype.buildPullRequests = async function () {
    const candidates = await original.call(this);
    candidates[0].updates.push({ path: 'providers/openai/go.mod', createIfMissing: false, updater: { updateContent: () => '' } });
    return candidates;
  };
  try {
    await assert.rejects(prepareLibraries(github, base), /only update the manifest and library changelogs: providers\/openai\/go.mod/);
  } finally {
    Manifest.prototype.buildPullRequests = original;
  }
});


test('preparation permits the exact pinned snooze lifecycle label', async () => {
  const requests = [];
  const send = preparationTransport(async () => {}, async (url, options) => {
    requests.push([url, options.method]); return new Response('[]');
  });
  await send('https://api.github.com/repos/grafana/ai-sdk/issues/99/labels/autorelease%3A%20snooze', { method: 'DELETE' });
  assert.equal(requests.length, 1);
});


test('changed snoozed release uses the pinned labeled creation path and actual replacement number', async () => {
  const source = fixture();
  const branch = source.candidate.pr.head.ref;
  const closed = { number: 99, headBranchName: branch, baseBranchName: 'main',
    labels: ['autorelease: snooze'], title: source.candidate.pr.title, body: source.candidate.pr.body };
  const requests = [];
  let replacement;
  const transport = preparationTransport(async () => {}, async (url, options = {}) => {
    const path = decodeURIComponent(new URL(url).pathname), method = options.method ?? 'GET';
    const body = options.body ? JSON.parse(options.body) : undefined;
    requests.push({ method, path, body });
    let data, status = 200;
    if (method === 'GET' && path.endsWith('/branches/main')) data = { commit: { sha: base } };
    else if (method === 'GET' && path.includes('/git/ref/')) data = { object: { sha: base } };
    else if (method === 'GET' && path.endsWith(`/git/commits/${base}`)) data = { tree: { sha: 'd'.repeat(40) } };
    else if (method === 'POST' && path.endsWith('/git/refs')) data = { ref: body.ref };
    else if (method === 'POST' && path.endsWith('/git/trees')) data = { sha: 'e'.repeat(40) };
    else if (method === 'POST' && path.endsWith('/git/commits')) data = { sha: 'f'.repeat(40), verification: { verified: true, reason: 'valid' } };
    else if (method === 'PATCH' && path.includes('/git/refs/')) data = {};
    else if (method === 'GET' && path.endsWith('/pulls')) data = replacement ? [replacement] : [];
    else if (method === 'POST' && path.endsWith('/pulls')) {
      replacement = { number: 100, title: body.title, body: body.body, state: 'open',
        head: { ref: branch }, base: { ref: 'main' }, labels: [] };
      data = replacement;
    } else if (method === 'POST' && path.endsWith('/issues/100/labels')) {
      replacement.labels = body.labels.map(name => ({ name })); data = replacement.labels;
    } else if (method === 'GET' && path.endsWith('/pulls/100')) data = replacement;
    else if (method === 'PATCH' && path.endsWith('/pulls/99')) {
      data = { ...body, number: 99, state: 'closed', head: { ref: branch }, base: { ref: 'main' }, labels: closed.labels.map(name => ({ name })) };
    } else if (method === 'DELETE' && path.endsWith('/issues/99/labels/autorelease: snooze')) data = [];
    else throw new Error(`unexpected pinned request: ${method} ${path}`);
    return new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } });
  });
  const github = await GitHub.create({ owner: 'grafana', repo: 'ai-sdk', defaultBranch: 'main', fetch: transport });
  github.getFileJson = source.github.getFileJson;
  github.findFilesByFilenameAndRef = async () => [];
  github.octokit.repos.getCommit = source.github.octokit.repos.getCommit;
  github.releaseIterator = async function* () {};
  github.tagIterator = async function* () {};
  github.pullRequestIterator = async function* (target, state) { if (state === 'CLOSED') yield closed; };
  github.mergeCommitIterator = async function* () {
    yield { sha: 'e'.repeat(40), message: 'fix: new root change', files: ['stream.go'] };
  };
  github.buildChangeSet = async updates => new Map(updates.map(update => [update.path, { content: 'generated content' }]));
  const result = await prepareLibraries(github, base);
  assert.deepEqual(result, [{ components: ['.'], requirements: [], prNumber: 100 }]);
  assert.deepEqual(replacement.labels, [{ name: 'autorelease: pending' }]);
  assert.equal(requests.some(r => r.method === 'PATCH' && r.path.endsWith('/pulls/99')), false);
  const label = requests.findIndex(r => r.method === 'POST' && r.path.endsWith('/issues/100/labels'));
  const unsnooze = requests.findIndex(r => r.method === 'DELETE');
  assert.ok(label >= 0 && unsnooze > label, 'retire old snooze only after labeling the replacement');
});
