import {
  CatalogForm,
  formFromSpec,
  formProblem,
  isKeyConfigured,
  newModelRow,
  newProviderRow,
  ProviderRow,
  ProviderType,
  secureChanges,
  specFromForm,
} from './catalogForm';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

const fullSpec: Spec = {
  providers: {
    claude: { type: 'anthropic' },
    gpt: { type: 'openai' },
  },
  models: {
    balanced: {
      name: 'Balanced',
      description: 'General-purpose model',
      aliases: ['default', 'fast'],
      primary: { provider: 'claude', model: 'claude-sonnet-4-6' },
      fallback: [{ provider: 'gpt', model: 'gpt-5' }],
    },
    plain: { name: 'Plain', primary: { provider: 'gpt', model: 'gpt-5-mini' } },
  },
};

function row(name: string, key = '', reset = false): ProviderRow {
  return { name, provider: { type: ProviderType.Anthropic }, key, reset };
}

describe('components/CatalogForm/catalogForm', () => {
  test('round-trips a full spec through the form unchanged', () => {
    const form = formFromSpec('team', fullSpec);
    expect(form.name).toBe('team');
    expect(form.providers.map((provider) => [provider.name, provider.key, provider.reset])).toEqual([
      ['claude', '', false],
      ['gpt', '', false],
    ]);
    expect(specFromForm(form)).toEqual(fullSpec);
  });

  test('keeps provider fields the form does not edit', () => {
    const spec = { providers: { claude: { type: 'anthropic', extra: 'kept' } }, models: {} } as unknown as Spec;
    expect(specFromForm(formFromSpec('team', spec))).toEqual(spec);
  });

  test('drops empty optional model fields', () => {
    const form: CatalogForm = {
      name: 'team',
      providers: [],
      models: [
        {
          id: 'm',
          model: { name: 'M', description: '', aliases: [], fallback: [], primary: { provider: 'p', model: 'x' } },
        },
      ],
    };
    expect(specFromForm(form).models.m).toEqual({ name: 'M', primary: { provider: 'p', model: 'x' } });
  });

  test('starts new rows empty, with an Anthropic provider', () => {
    expect(newProviderRow()).toEqual({ name: '', provider: { type: 'anthropic' }, key: '', reset: false });
    expect(newModelRow()).toEqual({ id: '', model: { name: '', primary: { provider: '', model: '' } } });
  });

  test.each([
    ['a stored provider untouched', ['claude'], [row('claude')], {}],
    ['a stored provider with a replaced key', ['claude'], [row('claude', 'k2', true)], { claude: { create: 'k2' } }],
    ['a stored provider reset but left empty', ['claude'], [row('claude', '', true)], {}],
    ['a new provider with a key', [], [row('claude', 'k1')], { claude: { create: 'k1' } }],
    ['a new provider without a key', [], [row('claude')], {}],
    ['a deleted provider', ['claude', 'gpt'], [row('claude')], { gpt: { remove: true } }],
    [
      'a renamed provider with its new key',
      ['claude'],
      [row('anthropic', 'k2')],
      { anthropic: { create: 'k2' }, claude: { remove: true } },
    ],
    ['a renamed provider without a key', ['claude'], [row('anthropic')], { claude: { remove: true } }],
    ['a deleted provider added again', ['claude'], [row('claude', 'k3')], { claude: { create: 'k3' } }],
  ])('sends the secure values of %s', (_, stored, providers, expected) => {
    expect(secureChanges(stored, providers)).toEqual(expected);
  });

  test.each([
    ['stored and untouched', ['claude'], row('claude'), true],
    ['stored and reset', ['claude'], row('claude', '', true), false],
    ['stored with a typed key', ['claude'], row('claude', 'k2'), false],
    ['not stored', [], row('claude'), false],
  ])('shows a key %s as configured: %s', (_, stored, provider, expected) => {
    expect(isKeyConfigured(provider, stored)).toBe(expected);
  });

  test.each([
    ['unique names', formFromSpec('team', fullSpec), undefined],
    [
      'a duplicate provider name',
      { name: 'team', providers: [row('claude'), row('claude')], models: [] },
      'Provider names must be unique: "claude" is used more than once.',
    ],
    [
      'a duplicate model ID',
      { name: 'team', providers: [], models: [newModelRow(), newModelRow()] },
      'Model IDs must be unique: "" is used more than once.',
    ],
  ])('reports %s', (_, form, expected) => {
    expect(formProblem(form)).toBe(expected);
  });
});
