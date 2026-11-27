import {
  catalogNameError,
  defaultProviderName,
  fallbackSuggestions,
  fallbackUsers,
  keyChange,
  keyRemoval,
  modelDraft,
  modelErrors,
  modelFromDraft,
  modelsOf,
  providerNameError,
  publicIDError,
  quickAddError,
  quickModel,
  withModel,
  withoutModel,
  withoutProvider,
  withProvider,
} from './catalogSpec';
import { Model, Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

const balanced: Model = {
  name: 'Balanced',
  description: 'General-purpose model',
  aliases: ['default', 'fast'],
  primary: { provider: 'claude', model: 'claude-sonnet-4-6' },
  fallback: [{ provider: 'gpt', model: 'gpt-5' }],
};

const spec: Spec = {
  providers: { claude: { type: 'anthropic' }, gpt: { type: 'openai' }, spare: { type: 'openai' } },
  models: {
    balanced,
    'gpt-5': quickModel('gpt', 'gpt-5'),
  },
};

describe('components/Catalog/catalogSpec', () => {
  test('names a new provider after its type, numbering it when the name is taken', () => {
    expect(defaultProviderName('anthropic', ['openai'])).toBe('anthropic');
    expect(defaultProviderName('anthropic', ['anthropic', 'anthropic-2'])).toBe('anthropic-3');
  });

  test('groups models under their primary provider and finds fallbacks from other providers', () => {
    expect(modelsOf(spec, 'claude').map(([id]) => id)).toEqual(['balanced']);
    expect(modelsOf(spec, 'gpt').map(([id]) => id)).toEqual(['gpt-5']);
    expect(fallbackUsers(spec, 'gpt')).toEqual(['balanced']);
    expect(fallbackUsers(spec, 'claude')).toEqual([]);
  });

  test('serves a quick-added provider model under its own ID and name', () => {
    expect(quickModel('claude', 'claude-haiku-4-5')).toEqual({
      name: 'claude-haiku-4-5',
      primary: { provider: 'claude', model: 'claude-haiku-4-5' },
    });
    expect(quickAddError(spec, ' ')).toBe("Enter the provider's model ID.");
    expect(quickAddError(spec, 'gpt-5')).toContain('already has a model with ID "gpt-5"');
    expect(quickAddError(spec, 'claude-haiku-4-5')).toBeUndefined();
  });

  test('removes a provider together with the models it serves', () => {
    expect(withoutProvider(spec, 'claude')).toEqual({
      providers: { gpt: spec.providers.gpt, spare: spec.providers.spare },
      models: { 'gpt-5': spec.models['gpt-5'] },
    });
    expect(withProvider(spec, 'extra', { type: 'anthropic' }).providers.extra).toEqual({ type: 'anthropic' });
  });

  test('adds, renames and removes models', () => {
    expect(Object.keys(withoutModel(spec, 'gpt-5').models)).toEqual(['balanced']);
    const renamed = withModel(spec, 'general', balanced, 'balanced');
    expect(Object.keys(renamed.models).sort()).toEqual(['general', 'gpt-5']);
    expect(renamed.models.general).toEqual(balanced);
  });

  test('round-trips a stored model through its draft, keeping fields the draft does not edit', () => {
    const stored = { ...balanced, skills: ['search'] } as Model;
    expect(modelFromDraft(modelDraft('balanced', stored), stored)).toEqual(stored);
  });

  test('defaults the display name to the ID', () => {
    const draft = modelDraft('gpt-5', spec.models['gpt-5']);
    expect(draft.name).toBe('');
    expect(modelFromDraft({ ...draft, id: 'gpt-5-chat' }, spec.models['gpt-5']).name).toBe('gpt-5-chat');
  });

  test('drops empty optional model fields', () => {
    const draft = { ...modelDraft('gpt-5', spec.models['gpt-5']), description: '', aliases: [] };
    expect(withModel(spec, 'gpt-5', modelFromDraft(draft, spec.models['gpt-5'])).models['gpt-5']).toEqual(
      quickModel('gpt', 'gpt-5')
    );
  });

  test('reports missing model fields, including fallbacks, and IDs other models have', () => {
    const draft = { ...modelDraft('gpt-5', spec.models['gpt-5']), id: '', fallback: [{ provider: '', model: 'x' }] };
    draft.primary = { provider: 'gpt', model: '' };
    expect(modelErrors(draft, ['balanced'])).toEqual({
      id: 'Enter a model ID.',
      'primary.model': "Enter the provider's model ID.",
      'fallback.0.provider': 'Choose a provider.',
    });
    expect(modelErrors({ ...modelDraft('gpt-5', spec.models['gpt-5']), id: 'balanced' }, ['balanced'])).toEqual({
      id: 'A model with ID "balanced" already exists.',
    });
  });

  test('suggests other models as fallbacks, except the ones the draft already calls', () => {
    const draft = modelDraft('gpt-5', spec.models['gpt-5']);
    expect(fallbackSuggestions(spec, draft, 'gpt-5')).toEqual([balanced.primary]);
    expect(fallbackSuggestions(spec, { ...draft, fallback: [balanced.primary] }, 'gpt-5')).toEqual([]);
  });

  test('sends typed keys as create and removes a stored one', () => {
    expect(keyChange('claude', 'typed')).toEqual({ claude: { create: 'typed' } });
    expect(keyRemoval('claude', ['claude'])).toEqual({ claude: { remove: true } });
    expect(keyRemoval('claude', [])).toEqual({});
  });

  test.each([
    ['', 'Enter a name.'],
    ['team', undefined],
    ['Team_2.prod:eu', undefined],
    ['$%^&*', "Use only letters, digits and '-', '_', '.' or ':'."],
    ['a b', "Use only letters, digits and '-', '_', '.' or ':'."],
    ['trash', '"trash" is reserved. Choose another name.'],
    ['x'.repeat(254), 'Use at most 253 characters.'],
  ])('checks the catalog name %p as Grafana does', (name, expected) => {
    expect(catalogNameError(name)).toBe(expected);
  });

  test.each([
    ['claude-sonnet-4-6', true],
    ['anthropic/claude:latest', true],
    ['bad id', false],
    ['-leading', false],
    ['claude-sonnet-4@20250514', true],
    ['model+version', true],
    ['x'.repeat(320), true],
    ['x'.repeat(321), false],
    ['/model', false],
    ['modèle', false],
    ['line\n', false],
  ])('checks the public ID %p as the Gateway does', (id, valid) => {
    expect(publicIDError(id) === undefined).toBe(valid);
  });

  test('refuses provider models and aliases outside the public ID format', () => {
    expect(quickAddError(spec, 'bad id')).toContain('Applications request the model by this ID.');
    const draft = { ...modelDraft('gpt-5', spec.models['gpt-5']), aliases: ['ok', 'not ok'] };
    expect(modelErrors(draft, [])).toEqual({ aliases: expect.stringContaining('"not ok" can\'t be an alias.') });
  });

  test('checks a new provider name', () => {
    expect(providerNameError('', [])).toBe('Enter a name.');
    expect(providerNameError('Anthropic', [])).toContain('lowercase letters');
    expect(providerNameError('claude', ['claude'])).toBe('A provider named "claude" already exists.');
  });
});
