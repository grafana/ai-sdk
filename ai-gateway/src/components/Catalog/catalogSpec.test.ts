import {
  keyChange,
  keyRemoval,
  modelDraft,
  modelErrors,
  modelFromDraft,
  modelsUsing,
  newModelDraft,
  newProviderDraft,
  ProviderDraft,
  providerErrors,
  ProviderType,
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
    plain: { name: 'Plain', primary: { provider: 'gpt', model: 'gpt-5-mini' } },
  },
};

function draft(overrides: Partial<ProviderDraft>): ProviderDraft {
  return { name: 'claude', type: ProviderType.Anthropic, key: '', replaceKey: false, ...overrides };
}

describe('components/Catalog/catalogSpec', () => {
  test('finds the models that call a provider as primary or fallback', () => {
    expect(modelsUsing(spec, 'claude')).toEqual(['balanced']);
    expect(modelsUsing(spec, 'gpt')).toEqual(['balanced', 'plain']);
    expect(modelsUsing(spec, 'spare')).toEqual([]);
  });

  test('adds, replaces and removes providers and models without touching the rest', () => {
    expect(withProvider(spec, 'extra', { type: 'anthropic' }).providers.extra).toEqual({ type: 'anthropic' });
    expect(Object.keys(withoutProvider(spec, 'spare').providers)).toEqual(['claude', 'gpt']);
    expect(withoutProvider(spec, 'spare').models).toBe(spec.models);
    expect(Object.keys(withoutModel(spec, 'plain').models)).toEqual(['balanced']);
    expect(withModel(spec, 'plain', { name: 'Renamed', primary: spec.models.plain.primary }).models.plain.name).toBe(
      'Renamed'
    );
  });

  test('round-trips a stored model through its draft, keeping fields the draft does not edit', () => {
    const stored = { ...balanced, skills: ['search'] } as Model;
    expect(modelFromDraft(modelDraft('balanced', stored), stored)).toEqual(stored);
  });

  test('drops empty optional model fields', () => {
    const model = modelFromDraft({ ...newModelDraft('claude'), id: 'x', name: 'X', primary: balanced.primary });
    expect(withModel(spec, 'x', model).models.x).toEqual({ name: 'X', primary: balanced.primary });
  });

  test('sends a typed key as create, nothing for an untyped one, and removes a stored key', () => {
    expect(keyChange('claude', 'typed')).toEqual({ claude: { create: 'typed' } });
    expect(keyChange('claude', '')).toEqual({});
    expect(keyRemoval('claude', ['claude'])).toEqual({ claude: { remove: true } });
    expect(keyRemoval('claude', [])).toEqual({});
  });

  test.each([
    ['a new provider without a name', draft({ name: ' ', key: 'k' }), undefined, { name: 'Enter a name.' }],
    [
      'a new provider with a taken name',
      draft({ key: 'k' }),
      undefined,
      { name: 'A provider named "claude" already exists.' },
    ],
    ['a new provider without a key', draft({ name: 'new' }), undefined, { key: 'Enter the API key.' }],
    ['a new provider with a key', draft({ name: 'new', key: 'k' }), undefined, {}],
    ['a stored provider keeping its key', draft({}), { type: 'anthropic', keySaved: true }, {}],
    [
      'a stored provider whose key was reset',
      draft({ replaceKey: true }),
      { type: 'anthropic', keySaved: true },
      { key: 'Enter the API key.' },
    ],
    [
      'a stored provider with a new type',
      draft({ type: ProviderType.OpenAI }),
      { type: 'anthropic', keySaved: true },
      { key: 'Enter a new API key: the saved key belongs to the previous type.' },
    ],
    [
      'a stored provider with a new type and key',
      draft({ type: ProviderType.OpenAI, key: 'k' }),
      { type: 'anthropic', keySaved: true },
      {},
    ],
  ])('checks %s', (_, provider, stored, expected) => {
    expect(providerErrors(provider, ['claude'], stored)).toEqual(expected);
  });

  test('starts a new provider as Anthropic, named after its type', () => {
    expect(newProviderDraft()).toEqual({ name: 'anthropic', type: 'anthropic', key: '', replaceKey: false });
  });

  test('reports each missing model field, including fallbacks', () => {
    const empty = { ...newModelDraft(), fallback: [{ provider: 'gpt', model: '' }] };
    expect(modelErrors(empty, [], false)).toEqual({
      id: 'Enter a model ID.',
      name: 'Enter a display name.',
      'primary.provider': 'Choose a provider.',
      'primary.model': "Enter the provider's model ID.",
      'fallback.0.model': "Enter the provider's model ID.",
    });
  });

  test('refuses a taken model ID only when adding', () => {
    const stored = modelDraft('balanced', balanced);
    expect(modelErrors(stored, ['balanced'], false)).toEqual({ id: 'A model with ID "balanced" already exists.' });
    expect(modelErrors(stored, ['balanced'], true)).toEqual({});
  });
});
