import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { fixture, base, sha } from './authorization-fixture.mjs';
import { authorizeInventory } from './authorize.mjs';
import { main } from './publish.mjs';
import { publicationState, publicationTransport, publishInventory } from './publication.mjs';

function publisher() {
  const f = fixture();
  const tags = new Map(), releases = new Map(), requests = [], validations = [];
  const missing = () => { throw Object.assign(new Error('not found'), { status: 404 }); };
  f.github.octokit.git.getRef = async ({ ref }) => ({ data: tags.get(ref.slice(5)) ?? missing() });
  f.github.octokit.repos.getReleaseByTag = async ({ tag }) => ({ data: releases.get(tag) ?? missing() });
  const fetch = async (url, options = {}) => {
    const path = decodeURIComponent(new URL(url).pathname), method = options.method ?? 'GET';
    const body = options.body ? JSON.parse(options.body) : undefined;
    requests.push({ method, path, body });
    let data, status = 200;
    if (method === 'POST' && path.endsWith('/git/refs')) {
      const tag = body.ref.slice(10);
      if (tags.has(tag)) { status = 422; data = { message: 'exists' }; }
      else { data = { ref: body.ref, object: { type: 'commit', sha: body.sha } }; tags.set(tag, data); }
    } else if (method === 'POST' && path.endsWith('/releases')) {
      data = { ...body, id: 123, html_url: 'https://github.com/grafana/ai-sdk/releases/tag/' + body.tag_name };
      releases.set(body.tag_name, data);
    } else if (method === 'POST' && path.endsWith('/labels')) {
      f.candidate.pr.labels.push({ name: body.labels[0] });
      data = f.candidate.pr.labels;
    } else if (method === 'DELETE' && path.endsWith('/labels/autorelease: pending')) {
      f.candidate.pr.labels = f.candidate.pr.labels.filter(l => l.name !== 'autorelease: pending');
      data = f.candidate.pr.labels;
    } else throw new Error(`unexpected pinned writer request: ${method} ${path}`);
    return new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } });
  };
  const options = { github: f.github, inventory: f.inventory, toolingRevision: base,
    validate: async candidate => { validations.push(candidate.prNumber); return { baseSha: base }; }, fetchImpl: fetch };
  const run = () => publishInventory(options);
  return { ...f, tags, releases, requests, validations, options, fetch, run };
}

function existing(f, release = false) {
  f.tags.set(f.candidate.tag, { object: { type: 'commit', sha: f.candidate.mergeSha } });
  if (release) f.releases.set(f.candidate.tag, { id: 123, name: f.candidate.name, body: f.candidate.notes,
    tag_name: f.candidate.tag, target_commitish: f.candidate.mergeSha, draft: false, prerelease: true });
}

const status = report => report.candidates[0].status;

test('pinned writer creates exact tag and release then completes lifecycle labels', async () => {
  const f = publisher();
  const report = await f.run();
  assert.equal(status(report), 'published', report.candidates[0].reason);
  assert.deepEqual(f.validations, [10]);
  assert.deepEqual(f.requests.map(r => `${r.method} ${r.path}`), [
    'POST /repos/grafana/ai-sdk/git/refs', 'POST /repos/grafana/ai-sdk/releases',
    'POST /repos/grafana/ai-sdk/issues/10/labels', 'DELETE /repos/grafana/ai-sdk/issues/10/labels/autorelease: pending',
  ]);
  assert.equal(f.tags.get(f.candidate.tag).object.sha, f.candidate.mergeSha);
  assert.deepEqual(f.candidate.pr.labels, [{ name: 'autorelease: tagged' }]);
});

test('partial publication retries only missing operations after fresh validation', async () => {
  for (const failAt of ['/releases', '/issues/10/labels', '/labels/autorelease: pending']) {
    const f = publisher();
    let fail = true;
    f.options.fetchImpl = async (url, options) => {
      if (fail && decodeURIComponent(new URL(url).pathname).endsWith(failAt)) {
        fail = false; throw new Error('response lost');
      }
      return f.fetch(url, options);
    };
    assert.equal(status(await f.run()), 'blocked');
    assert.ok(f.tags.has(f.candidate.tag));
    assert.ok(f.candidate.pr.labels.some(l => l.name === 'autorelease: pending'));
    f.requests.length = 0;
    const result = await f.run();
    assert.notEqual(status(result), 'blocked', result.candidates[0].reason);
    assert.equal(f.requests.filter(r => r.path.endsWith('/git/refs')).length, 0);
    assert.equal(f.requests.filter(r => r.path.endsWith('/releases')).length, failAt === '/releases' ? 1 : 0);
    assert.equal(f.validations.length, 2);
  }
});

test('duplicate completed publication revalidates but does not repeat writes', async () => {
  const f = publisher();
  assert.equal(status(await f.run()), 'published');
  f.requests.length = 0;
  assert.equal(status(await f.run()), 'recovered');
  assert.equal(f.requests.length, 0);
  assert.equal(f.validations.length, 2);
});

test('conflicting state and unavailable evidence prevent all publication writes', async () => {
  for (const mutate of [
    f => { existing(f); f.tags.get(f.candidate.tag).object.sha = sha('e'); },
    f => { existing(f, true); f.releases.get(f.candidate.tag).body = 'edited'; },
    f => { existing(f, true); f.tags.clear(); },
    f => { existing(f, true); f.options.validate = async () => { throw new Error('proxy unavailable'); }; },
    f => { f.github.octokit.actions.getWorkflow = async () => { throw new Error('missing evidence'); }; },
    f => { f.options.validate = async () => { f.candidate.pr.title = 'edited'; return {}; }; },
    f => { f.options.validate = async () => { existing(f); return {}; }; },
  ]) {
    const f = publisher(); mutate(f);
    assert.equal(status(await f.run()), 'blocked');
    assert.equal(f.requests.length, 0);
  }
});

test('recovery verifies annotated tags and rejects cycles and wrong targets', async () => {
  const f = publisher(); existing(f);
  f.tags.get(f.candidate.tag).object = { type: 'tag', sha: sha('e') };
  f.github.octokit.git.getTag = async () => ({ data: { object: { type: 'commit', sha: f.candidate.mergeSha } } });
  assert.ok((await publicationState(f.github, f.candidate.tag, f.candidate)).tagObject);
  f.github.octokit.git.getTag = async () => ({ data: { object: { type: 'tag', sha: sha('e') } } });
  await assert.rejects(publicationState(f.github, f.candidate.tag, f.candidate), /chain/);
});

test('read-only authorization still blocks matching published state', async () => {
  const f = publisher(); existing(f, true);
  const report = await authorizeInventory(f.github, f.inventory, f.options.validate, base);
  assert.equal(status(report), 'blocked');
  assert.equal(f.validations.length, 0);
});

test('exact-request guard rejects writes outside one candidate and repeat attempts', async () => {
  const f = publisher();
  const transport = publicationTransport(f.candidate, async () => {}, async () => {}, f.fetch);
  for (const [method, path, body] of [
    ['PATCH', `/git/refs/tags/${f.candidate.tag}`, { sha: sha('e'), force: true }],
    ['DELETE', `/git/refs/tags/${f.candidate.tag}`],
    ['POST', '/git/refs', { ref: 'refs/tags/other', sha: base }],
    ['POST', '/releases', { tag_name: f.candidate.tag, target_commitish: 'main' }],
    ['POST', '/issues/11/labels', { labels: ['autorelease: tagged'] }],
    ['POST', '/issues/10/comments', { body: 'done' }],
  ]) await assert.rejects(transport('https://api.github.com/repos/grafana/ai-sdk' + path,
    { method, body: body && JSON.stringify(body) }));
  assert.equal(f.requests.length, 0);
  const options = { method: 'POST', body: JSON.stringify({ ref: `refs/tags/${f.candidate.tag}`, sha: f.candidate.mergeSha }) };
  await transport('https://api.github.com/repos/grafana/ai-sdk/git/refs', options);
  await assert.rejects(transport('https://api.github.com/repos/grafana/ai-sdk/git/refs', options), /already attempted/);
});

test('frozen batch excludes arrivals and overlapping invocations fail without writes', async () => {
  const f = publisher();
  let release, entered;
  const started = new Promise(resolve => { entered = resolve; });
  f.options.validate = async () => { entered(); await new Promise(resolve => { release = resolve; }); return {}; };
  const first = f.run(); await started;
  f.inventory.candidates.push({ ...f.candidate, prNumber: 99 });
  await assert.rejects(f.run(), /already running/);
  release();
  const result = await first;
  assert.equal(result.candidates.length, 1);
  assert.equal(status(result), 'published', result.candidates[0].reason);
});

test('CLI and workflow require explicit activation before credentials or networking', async () => {
  await assert.rejects(main([], {}), /disabled/);
  const workflow = readFileSync(new URL('../../.github/workflows/release-please.yml', import.meta.url), 'utf8');
  assert.match(workflow, /vars\.RELEASE_LIBRARY_MODE == 'root'/);
  assert.match(workflow, /vars\.RELEASE_LIBRARY_MODE == 'libraries'/);
  assert.match(workflow, /cancel-in-progress: false/);
  assert.match(workflow, /node release\/tooling\/publish\.mjs/);
});

test('root publishes independently of blocked provider and unsupported Gateway', async () => {
  const f = publisher();
  f.inventory.candidates.unshift({ ...f.candidate, component: 'providers/bedrock',
    tag: 'providers/bedrock/v0.1.0', prNumber: 11, pr: { ...f.candidate.pr, number: 11 } });
  f.inventory.candidates.push({ ...f.candidate, component: 'ai-gateway', tag: 'ai-gateway/v0.1.0', prNumber: 12 });
  f.options.validate = async candidate => {
    f.validations.push(candidate.component);
    if (candidate.component !== '.') throw new Error('unpublished prerequisite');
    return {};
  };
  const report = await f.run();
  assert.deepEqual(report.candidates.map(c => c.status), ['blocked', 'published', 'blocked']);
  assert.deepEqual(f.validations, ['providers/bedrock', '.']);
  assert.deepEqual([...f.releases.keys()], [f.candidate.tag]);
});

test('ambiguous component claims cannot reach validation or writes', async () => {
  const f = publisher();
  f.inventory.candidates.push({ ...f.candidate, prNumber: 11 });
  assert.deepEqual((await f.run()).candidates.map(c => c.status), ['blocked', 'blocked']);
  assert.deepEqual(f.validations, []);
  assert.deepEqual(f.requests, []);
});

test('lost responses after successful mutations recover without duplicate creation', async () => {
  for (const suffix of ['/git/refs', '/releases', '/issues/10/labels', '/labels/autorelease: pending']) {
    const f = publisher();
    let fail = true;
    f.options.fetchImpl = async (url, options) => {
      const response = await f.fetch(url, options);
      if (fail && decodeURIComponent(new URL(url).pathname).endsWith(suffix)) {
        fail = false; throw new Error('response lost after mutation');
      }
      return response;
    };
    assert.equal(status(await f.run()), 'blocked');
    const tags = f.requests.filter(r => r.path.endsWith('/git/refs')).length;
    const releases = f.requests.filter(r => r.path.endsWith('/releases')).length;
    assert.notEqual(status(await f.run()), 'blocked');
    assert.equal(f.requests.filter(r => r.path.endsWith('/git/refs')).length, tags);
    assert.equal(f.requests.filter(r => r.path.endsWith('/releases')).length, releases || 1);
    assert.deepEqual(f.candidate.pr.labels, [{ name: 'autorelease: tagged' }]);
  }
});

test('changes between mutations stop remaining operations', async () => {
  for (const change of ['metadata', 'source attempt', 'tag target']) {
    const f = publisher();
    f.options.fetchImpl = async (url, options) => {
      const response = await f.fetch(url, options);
      if (new URL(url).pathname.endsWith('/git/refs')) {
        if (change === 'metadata') f.candidate.pr.body = 'changed';
        if (change === 'tag target') f.tags.get(f.candidate.tag).object.sha = sha('e');
        if (change === 'source attempt') {
          const paginate = f.github.octokit.paginate;
          f.github.octokit.paginate = async (route, params) => {
            const result = await paginate(route, params);
            if (route === f.github.octokit.actions.listWorkflowRuns && params.workflow_id === 100) result[0].run_attempt++;
            return result;
          };
        }
      }
      return response;
    };
    assert.equal(status(await f.run()), 'blocked', change);
    assert.equal(f.requests.length, 1, change);
    assert.equal(f.releases.size, 0);
  }
});

test('concurrent tag creation accepts only the exact immutable target', async () => {
  for (const matching of [true, false]) {
    const f = publisher();
    f.options.fetchImpl = async (url, options) => {
      if (new URL(url).pathname.endsWith('/git/refs')) {
        existing(f);
        if (!matching) f.tags.get(f.candidate.tag).object.sha = sha('e');
      }
      return f.fetch(url, options);
    };
    const report = await f.run();
    assert.equal(status(report), matching ? 'published' : 'blocked');
    assert.equal(f.releases.size, matching ? 1 : 0);
  }
});
