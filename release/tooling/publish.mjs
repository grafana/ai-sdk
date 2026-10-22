import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { GitHub } from 'release-please';
import { readOnlyFetch } from './preview.mjs';
import { discoverMerged, REPO, SHA } from './merged-candidates.mjs';
import { childEnvironment, libraryValidator } from './authorize.mjs';
import { publishInventory } from './publication.mjs';

const publicationEnabled = false;

export async function main(args, env = process.env) {
  assert.ok(publicationEnabled, 'library publication is disabled pending #245 activation');
  assert.ok(args.length <= 1 && args.every(arg => !arg.startsWith('-')), 'usage: node publish.mjs [component]');
  assert.equal(env.GITHUB_REPOSITORY, 'grafana/ai-sdk');
  assert.equal(env.GITHUB_EVENT_NAME, 'push');
  assert.equal(env.GITHUB_REF, 'refs/heads/main');
  assert.match(env.GITHUB_SHA ?? '', SHA);
  assert.ok(env.GH_TOKEN);
  const cwd = new URL('../..', import.meta.url);
  const git = args => execFileSync('git', args, { cwd, env: childEnvironment(env), encoding: 'utf8' }).trim();
  assert.equal(git(['status', '--porcelain', '--untracked-files=normal']), '');
  const toolingRevision = git(['rev-parse', 'HEAD']);
  assert.equal(toolingRevision, env.GITHUB_SHA);
  const github = await GitHub.create({ ...REPO, defaultBranch: 'main', token: env.GH_TOKEN, fetch: readOnlyFetch() });
  const inventory = await discoverMerged(github, args[0]);
  const report = await publishInventory({ github, inventory, toolingRevision, token: env.GH_TOKEN,
    validate: libraryValidator(inventory, env.GH_TOKEN) });
  console.log(JSON.stringify(report, null, 2));
  if (report.candidates.some(candidate => candidate.status === 'blocked')) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
}
