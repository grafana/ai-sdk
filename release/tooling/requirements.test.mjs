import assert from 'node:assert/strict';
import test from 'node:test';
import { expectedVersions, rewriteRequirements } from './requirements.mjs';

const versions = { 'github.com/grafana/ai-sdk': 'v0.2.0', 'github.com/grafana/ai-sdk/providers/openai': 'v0.1.1' };

test('expected versions come from the manifest, then the registered initial version', () => {
  const config = { 'initial-version': '0.0.1', packages: { '.': {}, 'providers/bedrock': { 'initial-version': '0.1.0-alpha.1' }, 'middleware/new': {} } };
  assert.deepEqual(expectedVersions({ '.': '0.2.0' }, config), {
    'github.com/grafana/ai-sdk': 'v0.2.0',
    'github.com/grafana/ai-sdk/providers/bedrock': 'v0.1.0-alpha.1',
    'github.com/grafana/ai-sdk/middleware/new': 'v0.0.1',
  });
});

test('only first-party requirement versions are rewritten', () => {
  const before = [
    'module github.com/grafana/ai-sdk/providers/bedrock', '', 'go 1.27.3', '',
    'require github.com/grafana/ai-sdk v0.1.0', '',
    'require (',
    '\tgithub.com/grafana/ai-sdk/providers/openai v0.0.0-20260101000000-0123456789ab',
    '\tgithub.com/grafana/ai-sdk/ai-gateway v0.1.0',
    '\tgithub.com/grafana/ai-sdk-extra v1.0.0',
    '\texample.com/x v1.0.0 // indirect',
    ')', '',
    'replace github.com/grafana/ai-sdk => ../..',
    'replace (', '\tgithub.com/grafana/ai-sdk/providers/openai => ../openai', ')', '',
  ].join('\n');
  const after = rewriteRequirements(before, versions);
  assert.equal(after, before
    .replace('require github.com/grafana/ai-sdk v0.1.0', 'require github.com/grafana/ai-sdk v0.2.0')
    .replace('openai v0.0.0-20260101000000-0123456789ab', 'openai v0.1.1'));
  assert.equal(rewriteRequirements(after, versions), after);
});
