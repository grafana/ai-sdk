import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { fixture, base } from './authorization-fixture.mjs';
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

test('Renovate final rule selects only registered libraries and tagged prereleases', () => {
  const config = JSON.parse(readFileSync(new URL('../../release-please-config.json', import.meta.url)));
  const renovate = JSON.parse(readFileSync(new URL('../../renovate.json', import.meta.url)));
  const rule = renovate.packageRules.at(-1);
  assert.deepEqual(rule.matchPackageNames, Object.keys(config.packages).filter(p => p !== 'ai-gateway').map(p => 'github.com/grafana/ai-sdk' + (p === '.' ? '' : '/' + p)));
  const versions = new RegExp(rule.allowedVersions.slice(1, -1));
  assert.ok(versions.test('v0.1.0-alpha.2'));
  assert.ok(versions.test('v1.2.3'));
  assert.equal(versions.test('v0.0.0-20261001000000-123456abcdef'), false);
  assert.equal(rule.automerge, false); assert.equal(rule.groupName, null);
  assert.equal(rule.enabled, true); assert.equal(rule.ignoreUnstable, false);
});

test('pinned preparation skips pending components and Gateway without a global veto', async () => {
  const { github } = fixture();
  github.releaseIterator = async function* () {};
  github.tagIterator = async function* () {};
  github.mergeCommitIterator = async function* () {
    yield { sha: 'e'.repeat(40), message: 'fix: root change', files: ['stream.go'] };
    yield { sha: 'f'.repeat(40), message: 'fix: provider change', files: ['providers/openai/model.go'] };
    yield { sha: '1'.repeat(40), message: 'fix: gateway change', files: ['ai-gateway/main.go'] };
  };
  const iterator = github.pullRequestIterator;
  github.pullRequestIterator = async function* (branch, state, ...args) {
    if (state === 'MERGED') yield* iterator(branch, state, ...args);
  };
  const created = [];
  github.createPullRequest = async pr => { created.push(pr); return { number: 999 }; };
  const result = await prepareLibraries(github, base);
  assert.deepEqual(result, [{ component: 'providers/openai', prNumber: 999 }]);
  assert.equal(created.length, 1);
  assert.match(created[0].headBranchName, /components--providers\/openai$/);
  assert.doesNotMatch(created[0].body, /gateway change|root change/);
  await assert.rejects(prepareLibraries(github, 'e'.repeat(40)), /advanced/);
});


test('pinned preparation refreshes an unchanged open release against the current base', async () => {
  const { github } = fixture();
  github.releaseIterator = async function* () {};
  github.tagIterator = async function* () {};
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
});
