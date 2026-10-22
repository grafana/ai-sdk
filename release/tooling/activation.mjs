import assert from 'node:assert/strict';
import { setTimeout as sleep } from 'node:timers/promises';
import { Manifest } from 'release-please';
import { SOURCE_JOBS, successfulRun } from './authorization-evidence.mjs';
import { REPO, REPOSITORY_ID, SHA, discoverMerged } from './merged-candidates.mjs';
import { readOnlyFetch } from './preview.mjs';

export function activationContext(env) {
  assert.ok(['root', 'libraries'].includes(env.RELEASE_LIBRARY_MODE), 'library publication is disabled');
  assert.equal(env.GITHUB_REPOSITORY, 'grafana/ai-sdk');
  assert.equal(env.GITHUB_EVENT_NAME, 'push');
  assert.equal(env.GITHUB_REF, 'refs/heads/main');
  assert.match(env.GITHUB_SHA ?? '', SHA);
  return env.RELEASE_LIBRARY_MODE;
}

export function selectInventory(inventory, mode) {
  assert.ok(['root', 'libraries'].includes(mode));
  const candidates = inventory.candidates.filter(candidate => candidate.component !== 'ai-gateway');
  if (mode === 'libraries') return { ...inventory, candidates };
  return { ...inventory, candidates: candidates.filter(candidate => candidate.component === '.').map(candidate => {
    assert.equal(candidate.prNumber, 490, 'root bootstrap only permits PR #490');
    assert.equal(candidate.mergeSha, 'c8e497f3ca45571ef57c3556ebde32a836e8b918');
    assert.equal(candidate.tag, 'v0.1.0-alpha.2');
    return candidate;
  }) };
}

export async function verifyProtections(github, approval) {
  assert.ok(approval && Number.isSafeInteger(approval.id) && approval.id > 0 &&
    typeof approval.updatedAt === 'string' && Number.isFinite(Date.parse(approval.updatedAt)),
  'maintainer-approved tag ruleset ID and revision are required');
  const { data: repository } = await github.octokit.repos.get(REPO);
  assert.equal(repository.id, REPOSITORY_ID);
  assert.equal(repository.allow_squash_merge, true);
  assert.equal(repository.allow_merge_commit, false);
  assert.equal(repository.allow_rebase_merge, false);
  assert.equal(repository.squash_merge_commit_title, 'PR_TITLE');
  assert.equal(repository.squash_merge_commit_message, 'PR_BODY');
  const rules = await github.octokit.paginate('GET /repos/{owner}/{repo}/rules/branches/{branch}', { ...REPO, branch: 'main' });
  assert.ok(rules.some(rule => rule.type === 'pull_request' && rule.parameters.required_approving_review_count >= 1),
    'main must require PR approval');
  const checks = rules.filter(rule => rule.type === 'required_status_checks');
  assert.ok(checks.some(rule => rule.parameters.strict_required_status_checks_policy), 'main must require current-base checks');
  for (const context of [...SOURCE_JOBS, 'release-readiness', 'check', 'Validate Changes']) {
    assert.ok(checks.some(rule => rule.parameters.required_status_checks.some(check =>
      check.context === context && check.integration_id === 15368)), `missing required GitHub Actions check: ${context}`);
  }
  const sets = await github.octokit.paginate('GET /repos/{owner}/{repo}/rulesets', { ...REPO, includes_parents: true });
  let immutable = false;
  for (const set of sets.filter(set => set.target === 'tag' && set.enforcement === 'active')) {
    const { data } = await github.octokit.request('GET /repos/{owner}/{repo}/rulesets/{ruleset_id}', { ...REPO, ruleset_id: set.id });
    if (data.id === approval.id && data.updated_at === approval.updatedAt &&
      data.conditions?.ref_name?.include?.includes('~ALL') && data.conditions.ref_name.exclude?.length === 0 &&
      (!Object.hasOwn(data, 'bypass_actors') || data.bypass_actors.length === 0) && ['update', 'deletion'].every(type => data.rules.some(rule => rule.type === type))) immutable = true;
  }
  assert.ok(immutable, 'approved immutable tag ruleset is absent, changed, or conflicts with its approval');
}

export async function waitForToolingCI(github, sha, { attempts = 60, wait = () => sleep(20_000) } = {}) {
  for (let attempt = 0; attempt < attempts; attempt++) {
    try { return await successfulRun(github, 'ci.yml', 'push', sha, 'main', SOURCE_JOBS); }
    catch (error) {
      if (attempt === attempts - 1) throw error;
      await wait();
    }
  }
  throw new Error('source CI wait exhausted');
}

export function preparationTransport(beforeWrite, fetchImpl = globalThis.fetch) {
  const read = readOnlyFetch(fetchImpl);
  return async (url, options = {}) => {
    const target = new URL(url), method = (options.method ?? 'GET').toUpperCase();
    if (method === 'GET' || target.pathname === '/graphql') return read(url, options);
    assert.equal(target.origin, 'https://api.github.com');
    assert.equal(target.search, '');
    const route = decodeURIComponent(target.pathname).replace(/^\/repos\/grafana\/ai-sdk/, '');
    assert.ok(target.pathname.startsWith('/repos/grafana/ai-sdk/'));
    const body = options.body ? JSON.parse(options.body) : undefined;
    const branch = /^refs\/heads\/release-please--branches--main(?:--components--(?:providers|middleware)\/[a-z-]+)?$/;
    let allowed = method === 'POST' && ['/git/trees', '/git/commits', '/pulls'].includes(route);
    if (method === 'POST' && route === '/git/refs') allowed = branch.test(body.ref);
    if (method === 'PATCH' && route.startsWith('/git/refs/')) allowed = branch.test('refs/' + route.slice('/git/refs/'.length));
    if (method === 'PATCH' && /^\/pulls\/\d+$/.test(route)) allowed = true;
    if (method === 'POST' && /^\/issues\/\d+\/labels$/.test(route)) allowed = true;
    if (method === 'DELETE' && /^\/issues\/\d+\/labels\/autorelease: (?:pending|snoozed)$/.test(route)) allowed = true;
    assert.ok(allowed, `preparation refused write: ${method} ${route}`);
    if (method === 'POST' && route === '/pulls') {
      assert.equal(body.base, 'main');
      assert.ok(branch.test('refs/heads/' + body.head.replace(/^grafana:/, '')));
    }
    if (route === '/git/commits') assert.ok(!body.author && !body.committer && !body.signature);
    await beforeWrite();
    const headers = new Headers(options.headers); headers.set('connection', 'close');
    const response = await fetchImpl(url, { ...options, headers, redirect: 'error', signal: AbortSignal.timeout(30_000) });
    if (response.ok && route === '/git/commits') {
      const commit = await response.clone().json();
      assert.equal(commit.verification?.verified, true, 'release App commit must be verified');
      assert.equal(commit.verification.reason, 'valid');
    }
    return response;
  };
}

export async function prepareLibraries(github, expectedSha) {
  const inventory = await discoverMerged(github);
  assert.equal(inventory.mainSha, expectedSha, 'main advanced before preparation');
  const pending = new Set(inventory.candidates.map(candidate => candidate.component));
  const config = await github.getFileJson('release-please-config.json', 'main');
  const results = [];
  for (const component of Object.keys(config.packages).filter(path => path !== 'ai-gateway' && !pending.has(path))) {
    const manifest = await Manifest.fromManifest(github, 'main', 'release-please-config.json',
      '.release-please-manifest.json', {}, component);
    manifest.lastReleaseSha = undefined;
    manifest.alwaysUpdate = true;
    const candidates = await manifest.buildPullRequests();
    const open = await manifest.findOpenReleasePullRequests();
    const snoozed = await manifest.findSnoozedReleasePullRequests();
    for (const candidate of candidates) {
      const changelog = component === '.' ? 'CHANGELOG.md' : `${component}/CHANGELOG.md`;
      assert.deepEqual(candidate.updates.map(update => update.path).sort(), ['.release-please-manifest.json', changelog].sort());
      const result = await manifest.createOrUpdatePullRequest(candidate, open, snoozed);
      if (result) results.push({ component, prNumber: result.number });
    }
  }
  return results;
}
