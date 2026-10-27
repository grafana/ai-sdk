import assert from 'node:assert/strict';
import { Manifest } from 'release-please';

export const REPO = { owner: 'grafana', repo: 'ai-sdk' };
export const REPOSITORY_ID = 1315458055;
export const BOT_ID = 332654929;
export const SHA = /^[a-f0-9]{40}$/;

function freeze(value) {
  if (value && typeof value === 'object') {
    Object.values(value).forEach(freeze);
    Object.freeze(value);
  }
  return value;
}

export async function discoverMerged(github, component) {
  const before = (await github.octokit.repos.getCommit({ ...REPO, ref: 'main' })).data.sha;
  assert.match(before, SHA);
  const pulls = [];
  for await (const pr of github.pullRequestIterator('main', 'MERGED', Number.MAX_SAFE_INTEGER, false)) {
    pulls.push(structuredClone(pr));
  }
  const snapshot = Object.create(github);
  snapshot.pullRequestIterator = async function* () { yield* pulls; };
  const config = await github.getFileJson('release-please-config.json', 'main');
  if (component !== undefined) assert.ok(Object.hasOwn(config.packages, component), 'unregistered component');
  const manifest = await Manifest.fromManifest(snapshot, 'main', 'release-please-config.json',
    '.release-please-manifest.json', {}, component);
  const releases = await manifest.buildReleases();
  const candidates = [];
  for (const release of releases) {
    const { data: pr } = await github.octokit.pulls.get({ ...REPO, pull_number: release.pullRequest.number });
    assert.equal(pr.merge_commit_sha, release.sha, 'discovered merge changed');
    const { data: commit } = await github.octokit.repos.getCommit({ ...REPO, ref: release.sha });
    candidates.push({
      repositoryId: REPOSITORY_ID, component: release.path, version: release.tag.version.toString(),
      tag: release.tag.toString(), notes: release.notes, name: release.name,
      draft: !!release.draft, prerelease: !!release.prerelease,
      prNumber: pr.number, headSha: pr.head.sha, mergeSha: release.sha,
      parents: commit.parents.map(parent => parent.sha), treeSha: commit.commit.tree.sha,
      config, pr,
    });
  }
  const after = (await github.octokit.repos.getCommit({ ...REPO, ref: 'main' })).data.sha;
  assert.equal(after, before, 'main advanced during discovery; discard the incomplete inventory');
  assert.deepEqual(await github.getFileJson('release-please-config.json', 'main'), config, 'release config changed');
  return freeze(JSON.parse(JSON.stringify({ mainSha: before, candidates })));
}

export function duplicateCandidates(candidates) {
  return new Set(candidates.filter(candidate => candidates.some(other => other !== candidate &&
    (other.component === candidate.component || other.tag === candidate.tag))));
}

export function modulePath(component) {
  return component === '.' ? 'github.com/grafana/ai-sdk' : `github.com/grafana/ai-sdk/${component}`;
}

export function coReleasedPrerequisites(candidate, candidates, prerequisites) {
  const required = new Set(prerequisites.map(module => `${module.path}@${module.version}`));
  return candidates.filter(other => other !== candidate && other.mergeSha === candidate.mergeSha &&
    required.has(`${modulePath(other.component)}@v${other.version}`));
}
