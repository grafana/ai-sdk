import assert from 'node:assert/strict';
import { GitHub } from 'release-please';
import { TagName } from 'release-please/build/src/util/tag-name.js';
import { readOnlyFetch } from './preview.mjs';
import { authorizeInventory, recheckCandidate } from './authorize.mjs';
import { coReleasedPrerequisites, REPO, SHA } from './merged-candidates.mjs';
import { verifyPublishedVersion } from './proxy-verification.mjs';

const prefix = '/repos/grafana/ai-sdk';
const pending = 'autorelease: pending', tagged = 'autorelease: tagged';
let running = false;

async function optional(read) {
  try { return (await read()).data; } catch (error) {
    if (error.status === 404) return null;
    throw error;
  }
}

export async function publicationState(github, tag, candidate) {
  assert.equal(tag, candidate.tag);
  assert.equal(candidate.draft, false, 'draft publication is unsupported');
  assert.equal(typeof candidate.prerelease, 'boolean');
  const ref = await optional(() => github.octokit.git.getRef({ ...REPO, ref: `tags/${tag}` }));
  let target = ref?.object;
  const seen = new Set();
  while (target?.type === 'tag') {
    assert.match(target.sha, SHA);
    assert.ok(seen.size < 8 && !seen.has(target.sha), 'invalid annotated tag chain');
    seen.add(target.sha);
    target = (await github.octokit.git.getTag({ ...REPO, tag_sha: target.sha })).data.object;
  }
  if (ref) {
    assert.equal(target?.type, 'commit');
    assert.equal(target.sha, candidate.mergeSha, 'existing tag targets a different commit');
  }
  const release = await optional(() => github.octokit.repos.getReleaseByTag({ ...REPO, tag }));
  if (release) {
    assert.ok(ref, 'release exists without verifiable tag');
    assert.ok(Number.isSafeInteger(release.id) && release.id > 0);
    for (const [key, value] of Object.entries({ tag_name: tag, target_commitish: candidate.mergeSha,
      name: candidate.name, body: candidate.notes, draft: false, prerelease: candidate.prerelease })) {
      assert.equal(release[key], value, `existing release differs: ${key}`);
    }
  }
  return { tagObject: ref?.object ?? null, releaseId: release?.id ?? null };
}

export function publicationTransport(candidate, beforeWrite, afterWrite, fetchImpl = globalThis.fetch) {
  const approved = structuredClone(candidate);
  const read = readOnlyFetch(fetchImpl);
  const attempted = new Set();
  return async (url, options = {}) => {
    const target = new URL(url);
    const method = (options.method ?? 'GET').toUpperCase();
    if (method === 'GET' || target.pathname === '/graphql') return read(url, options);
    assert.equal(target.origin, 'https://api.github.com');
    assert.equal(target.search, '');
    const route = `${method} ${decodeURIComponent(target.pathname)}`;
    const body = options.body ? JSON.parse(options.body) : undefined;
    let operation;
    if (route === `POST ${prefix}/git/refs`) {
      operation = 'tag';
      assert.deepEqual(body, { ref: `refs/tags/${approved.tag}`, sha: approved.mergeSha });
    } else if (route === `POST ${prefix}/releases`) {
      operation = 'release';
      assert.deepEqual(body, { name: approved.name, tag_name: approved.tag, body: approved.notes,
        draft: false, prerelease: approved.prerelease, target_commitish: approved.mergeSha });
    } else if (route === `POST ${prefix}/issues/${approved.prNumber}/labels`) {
      operation = 'label-add';
      assert.deepEqual(body, { labels: [tagged] });
    } else if (route === `DELETE ${prefix}/issues/${approved.prNumber}/labels/${pending}`) {
      operation = 'label-remove';
      assert.equal(body, undefined);
    } else {
      throw new Error(`publication refused write: ${route}`);
    }
    assert.equal(attempted.has(operation), false, 'mutation already attempted; inspect state in a new run');
    attempted.add(operation);
    await beforeWrite(operation);
    const headers = new Headers(options.headers);
    headers.set('connection', 'close');
    const timeout = AbortSignal.timeout(30_000);
    const response = await fetchImpl(url, { ...options, headers, redirect: 'error',
      signal: options.signal ? AbortSignal.any([options.signal, timeout]) : timeout });
    await afterWrite(operation, response);
    return response;
  };
}

function publicationOrder(candidates, evidence) {
  const order = [], state = new Map();
  const visit = candidate => {
    if (state.get(candidate) === 'done') return;
    assert.notEqual(state.get(candidate), 'visiting', 'co-released prerequisite cycle');
    state.set(candidate, 'visiting');
    const result = evidence[candidates.indexOf(candidate)];
    for (const prerequisite of coReleasedPrerequisites(candidate, candidates, result.library?.prerequisites ?? [])) {
      visit(prerequisite);
    }
    state.set(candidate, 'done');
    order.push(candidate);
  };
  candidates.forEach(visit);
  return order;
}

const published = result => ['published', 'recovered'].includes(result?.status);

export async function publishInventory({ github, inventory, validate, toolingRevision, token,
  fetchImpl = globalThis.fetch, verifyProxy = verifyPublishedVersion }) {
  assert.equal(running, false, 'publication already running in this process');
  running = true;
  try {
    const frozen = structuredClone(inventory);
    const initialStates = new Map();
    const inspect = async (reader, tag, candidate) => {
      const current = await publicationState(reader, tag, candidate);
      if (!initialStates.has(candidate)) initialStates.set(candidate, current);
      return current;
    };
    const report = await authorizeInventory(github, frozen, validate, toolingRevision, inspect);
    const results = new Map(), contexts = new Map();
    for (const candidate of publicationOrder(frozen.candidates, report.candidates)) {
      const evidence = report.candidates[frozen.candidates.indexOf(candidate)];
      const identity = { prNumber: candidate.prNumber, component: candidate.component,
        tag: candidate.tag, mergeSha: candidate.mergeSha };
      try {
        assert.equal(evidence.status, 'approved', evidence.reason);
        const missing = coReleasedPrerequisites(candidate, frozen.candidates, evidence.library.prerequisites ?? [])
          .filter(prerequisite => !published(results.get(prerequisite)));
        assert.equal(missing.length, 0, `co-released prerequisite was not published: ${missing.map(m => m.component).join(', ')}`);
        let state = initialStates.get(candidate);
        const initial = structuredClone(state);
        const recheck = async operation => {
          await recheckCandidate(github, candidate, evidence);
          assert.deepEqual(await publicationState(github, candidate.tag, candidate), state,
            'publication state changed before mutation');
          if (operation === 'tag') assert.equal(state.releaseId, null);
          if (operation === 'release') {
            assert.ok(state.tagObject, 'create the immutable tag before release');
            assert.equal(state.releaseId, null);
          }
          if (operation.startsWith('label')) assert.ok(state.releaseId, 'verify publication before labels');
          if (operation === 'label-remove') {
            const { data: pr } = await github.octokit.pulls.get({ ...REPO, pull_number: candidate.prNumber });
            assert.ok(pr.labels.some(label => label.name === tagged), 'retain pending until tagged label exists');
          }
        };
        const after = async (operation, response) => {
          if (!response.ok && !(operation === 'tag' && response.status === 422)) return;
          const current = await publicationState(github, candidate.tag, candidate);
          if (operation === 'tag') {
            assert.ok(current.tagObject, 'tag create did not establish the expected target');
            assert.equal(current.releaseId, null);
            if (state.tagObject) assert.deepEqual(current.tagObject, state.tagObject, 'tag changed during recovery');
          } else if (operation === 'release') {
            assert.deepEqual(current.tagObject, state.tagObject, 'tag changed during release creation');
            assert.ok(current.releaseId, 'release creation did not establish expected metadata');
          } else assert.deepEqual(current, state);
          state = current;
        };
        const writer = await GitHub.create({ ...REPO, defaultBranch: 'main', token,
          fetch: publicationTransport(candidate, recheck, after, fetchImpl) });
        if (!state.releaseId) {
          const tag = TagName.parse(candidate.tag);
          assert.ok(tag, 'invalid release-please tag');
          await writer.createRelease({ tag, sha: candidate.mergeSha, name: candidate.name, notes: candidate.notes },
            { draft: false, prerelease: candidate.prerelease, forceTag: !state.tagObject });
        }
        await recheckCandidate(github, candidate, evidence);
        assert.deepEqual(await publicationState(github, candidate.tag, candidate), state);
        const proxy = await verifyProxy(candidate);
        contexts.set(candidate, { writer, recheck });
        results.set(candidate, { ...identity, status: initial.releaseId ? 'recovered' : 'published',
          releaseId: state.releaseId, proxy, evidence });
      } catch (error) {
        results.set(candidate, { ...identity, status: 'blocked', reason: error.message });
      }
    }
    for (const prNumber of new Set(frozen.candidates.map(candidate => candidate.prNumber))) {
      const members = frozen.candidates.filter(candidate => candidate.prNumber === prNumber);
      if (!members.every(candidate => published(results.get(candidate)))) continue;
      const { writer, recheck } = contexts.get(members.at(-1));
      try {
        await recheck('label-add');
        const { data: pr } = await github.octokit.pulls.get({ ...REPO, pull_number: prNumber });
        if (!pr.labels.some(label => label.name === tagged)) await writer.addIssueLabels([tagged], prNumber);
        if (pr.labels.some(label => label.name === pending)) await writer.removeIssueLabels([pending], prNumber);
      } catch (error) {
        for (const candidate of members) {
          const { prNumber: number, component, tag, mergeSha } = results.get(candidate);
          results.set(candidate, { prNumber: number, component, tag, mergeSha, status: 'blocked',
            reason: `release lifecycle labels incomplete: ${error.message}` });
        }
      }
    }
    return { schemaVersion: 1, toolingRevision, mainSha: frozen.mainSha,
      candidates: frozen.candidates.map(candidate => results.get(candidate)) };
  } finally {
    running = false;
  }
}
