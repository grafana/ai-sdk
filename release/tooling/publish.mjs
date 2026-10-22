import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { GitHub, setLogger } from 'release-please';
import { readOnlyFetch } from './preview.mjs';
import { discoverMerged, REPO, REPOSITORY_ID } from './merged-candidates.mjs';
import { childEnvironment, libraryValidator } from './authorize.mjs';
import { publishInventory } from './publication.mjs';
import { activationContext, selectInventory, verifyProtections, waitForToolingCI,
  preparationTransport, prepareLibraries } from './activation.mjs';

export async function main(args, env = process.env) {
  const mode = activationContext(env);
  const approval = JSON.parse(env.RELEASE_TAG_RULESET ?? 'null');
  assert.equal(args.length, 0, 'usage: node publish.mjs (selection comes from RELEASE_LIBRARY_MODE)');
  assert.ok(env.GH_TOKEN && env.GH_READ_TOKEN, 'App write and Actions read credentials are required');
  const cwd = new URL('../..', import.meta.url);
  const git = args => execFileSync('git', args, { cwd, env: childEnvironment(env), encoding: 'utf8' }).trim();
  assert.equal(git(['status', '--porcelain', '--untracked-files=normal']), '');
  const toolingRevision = git(['rev-parse', 'HEAD']);
  assert.equal(toolingRevision, env.GITHUB_SHA);
  const diagnostics = [];
  const logger = Object.fromEntries(['debug', 'info', 'warn', 'error', 'trace'].map(level =>
    [level, (...values) => {
      if (values.some(value => String(value).includes('commit could not be parsed:'))) diagnostics.push(values.join(' '));
      console.error(...values);
    }]));
  setLogger(logger);
  const github = await GitHub.create({ ...REPO, defaultBranch: 'main', token: env.GH_READ_TOKEN, logger, fetch: readOnlyFetch() });
  await verifyProtections(github, approval);
  await waitForToolingCI(github, toolingRevision);
  const app = await GitHub.create({ ...REPO, defaultBranch: 'main', token: env.GH_TOKEN, logger, fetch: readOnlyFetch() });
  const { data: scope } = await app.octokit.request('GET /installation/repositories');
  assert.equal(scope.total_count, 1);
  assert.deepEqual(scope.repositories.map(repo => [repo.id, repo.full_name]), [[REPOSITORY_ID, 'grafana/ai-sdk']]);
  const inventory = selectInventory(await discoverMerged(github, mode === 'root' ? '.' : undefined), mode);
  const report = await publishInventory({ github, inventory, toolingRevision, token: env.GH_TOKEN,
    validate: libraryValidator(inventory, env.GH_READ_TOKEN),
    fetchImpl: async (url, options) => { await verifyProtections(github, approval); return fetch(url, options); } });
  console.log(JSON.stringify(report, null, 2));
  if (mode === 'libraries') {
    const beforeWrite = async () => {
      assert.equal(diagnostics.length, 0, 'resolve commit parsing diagnostics before preparation writes');
      await verifyProtections(github, approval);
      const { data: main } = await github.octokit.repos.getCommit({ ...REPO, ref: 'main' });
      assert.equal(main.sha, toolingRevision, 'main advanced; preparation requires a new run');
    };
    const writer = await GitHub.create({ ...REPO, defaultBranch: 'main', token: env.GH_TOKEN, logger,
      fetch: preparationTransport(beforeWrite) });
    console.log(JSON.stringify({ prepared: await prepareLibraries(writer, toolingRevision) }, null, 2));
  }
  if (report.candidates.some(candidate => candidate.status === 'blocked')) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
}
