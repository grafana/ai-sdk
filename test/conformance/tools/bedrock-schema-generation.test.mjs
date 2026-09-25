import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { readSchemaBaseline, readAnthropicZodVersion, selectSupportedSchemas } from '../../../scripts/generate-bedrock-provider-tool-schemas.mjs';

const commit = 'a'.repeat(40);
const baseline = `upstream:
  repository: https://github.com/vercel/ai
  commit: ${commit}
packages:
  ai: 7.0.107
  "@ai-sdk/anthropic": 4.0.58
  "@ai-sdk/provider": 4.0.17
  "@ai-sdk/provider-utils": 5.0.45
`;
const lockfile = `lockfileVersion: '9.0'
importers:
  packages/anthropic:
    dependencies:
      '@ai-sdk/provider':
        specifier: workspace:*
    devDependencies:
      zod:
        specifier: 3.25.76
        version: 3.25.76
  packages/anthropic-aws:
    devDependencies:
      zod:
        specifier: 4.4.3
        version: 4.4.3
`;

describe('Bedrock provider-tool schema regeneration', () => {
  it('selects package versions from the registered baseline', () => {
    assert.deepEqual(readSchemaBaseline(baseline), {
      commit,
      anthropic: '4.0.58',
      provider: '4.0.17',
      providerUtils: '5.0.45',
    });
    assert.deepEqual(readSchemaBaseline(baseline.replace('4.0.58', '4.1.0').replace(commit, 'b'.repeat(40))), {
      commit: 'b'.repeat(40),
      anthropic: '4.1.0',
      provider: '4.0.17',
      providerUtils: '5.0.45',
    });
    assert.throws(() => readSchemaBaseline(baseline.replace('  "@ai-sdk/provider-utils": 5.0.45\n', '')), /provider-utils/);
  });

  it('uses the Anthropic importer Zod version from the pinned upstream lockfile', () => {
    assert.equal(readAnthropicZodVersion(lockfile), '3.25.76');
    assert.equal(readAnthropicZodVersion(lockfile.replaceAll('3.25.76', '4.4.3')), '4.4.3');
    assert.throws(() => readAnthropicZodVersion(lockfile.replace('  packages/anthropic:', '  packages/other:')), /anthropic/);
    assert.throws(() => readAnthropicZodVersion(lockfile.replace('        version: 3.25.76', '        version: latest')), /zod/);
  });

  it('allows new upstream tools but requires every Go-supported schema', () => {
    const source = {
      'anthropic.bash_20241022': { type: 'object' },
      'anthropic.new_tool_20260924': { type: 'string' },
      'anthropic.web_search_20250305': { type: 'object' },
    };
    const selected = selectSupportedSchemas(source, new Set(['anthropic.bash_20241022']));
    assert.deepEqual(selected, {
      schemas: { 'anthropic.bash_20241022': { type: 'object' } },
      newToolIds: ['anthropic.new_tool_20260924'],
    });
    assert.deepEqual(Object.keys(source), [
      'anthropic.bash_20241022', 'anthropic.new_tool_20260924', 'anthropic.web_search_20250305',
    ]);
    assert.throws(() => selectSupportedSchemas(source, new Set(['anthropic.removed_tool_20250522'])), /removed_tool/);
  });
});
