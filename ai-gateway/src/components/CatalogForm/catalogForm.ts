import { SecureChanges } from '../../api/modelCatalogs';
import { Model, Provider, Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

/** ProviderType is a provider type a stack catalog accepts. */
export enum ProviderType {
  Anthropic = 'anthropic',
  OpenAI = 'openai',
}

/**
 * ProviderRow is one provider in the form: its name, its spec as stored, the
 * key the writer typed, and whether the writer reset a stored key to replace it.
 */
export interface ProviderRow {
  name: string;
  provider: Provider;
  key: string;
  reset: boolean;
}

/** ModelRow is one model in the form: its public ID and its spec as stored. */
export interface ModelRow {
  id: string;
  model: Model;
}

/** CatalogForm is the editable state of one catalog. It never holds a stored key. */
export interface CatalogForm {
  name: string;
  providers: ProviderRow[];
  models: ModelRow[];
}

/** newProviderRow returns an empty provider row. */
export function newProviderRow(): ProviderRow {
  return { name: '', provider: { type: ProviderType.Anthropic }, key: '', reset: false };
}

/** newModelRow returns an empty model row. */
export function newModelRow(): ModelRow {
  return { id: '', model: { name: '', primary: { provider: '', model: '' } } };
}

/** formFromSpec returns the form state of a catalog's name and spec. */
export function formFromSpec(name: string, spec: Spec): CatalogForm {
  return {
    name,
    providers: Object.entries(spec.providers ?? {}).map(([providerName, provider]) => ({
      name: providerName,
      provider,
      key: '',
      reset: false,
    })),
    models: Object.entries(spec.models ?? {}).map(([id, model]) => ({ id, model })),
  };
}

/** specFromForm returns the spec the form describes, without empty optional model fields. */
export function specFromForm(form: CatalogForm): Spec {
  return {
    providers: Object.fromEntries(form.providers.map((row) => [row.name, row.provider])),
    models: Object.fromEntries(form.models.map((row) => [row.id, withoutEmptyOptionals(row.model)])),
  };
}

function withoutEmptyOptionals(model: Model): Model {
  const { description, aliases, fallback, ...required } = model;
  return {
    ...required,
    ...(description ? { description } : {}),
    ...(aliases?.length ? { aliases } : {}),
    ...(fallback?.length ? { fallback } : {}),
  };
}

/** isKeyConfigured says whether a row's provider has a stored key the write keeps. */
export function isKeyConfigured(row: ProviderRow, storedKeys: string[]): boolean {
  return storedKeys.includes(row.name) && !row.reset && row.key === '';
}

/**
 * secureChanges returns the secure values a write sends: `create` for each
 * typed key, `remove` for each stored key whose provider left the form, and
 * nothing for a stored key the writer kept, so the platform keeps it.
 */
export function secureChanges(storedKeys: string[], providers: ProviderRow[]): SecureChanges {
  const changes: SecureChanges = {};
  for (const row of providers) {
    if (row.key !== '') {
      changes[row.name] = { create: row.key };
    }
  }
  for (const key of storedKeys) {
    if (!providers.some((row) => row.name === key)) {
      changes[key] = { remove: true };
    }
  }
  return changes;
}

/**
 * formProblem returns why the form can't be sent, or undefined. It covers only
 * what a catalog body can't express, and a stored key that would be kept for a
 * provider of another type; the platform validates everything else.
 */
export function formProblem(form: CatalogForm, storedProviders: Record<string, Provider> = {}): string | undefined {
  const provider = firstDuplicate(form.providers.map((row) => row.name));
  if (provider !== undefined) {
    return `Provider names must be unique: "${provider}" is used more than once.`;
  }
  const model = firstDuplicate(form.models.map((row) => row.id));
  if (model !== undefined) {
    return `Model IDs must be unique: "${model}" is used more than once.`;
  }
  const retyped = form.providers.find((row) => {
    const stored = storedProviders[row.name];
    return stored !== undefined && stored.type !== row.provider.type && row.key === '';
  });
  if (retyped !== undefined) {
    return `Enter a new API key for "${retyped.name}": its saved key belongs to another provider type.`;
  }
  return undefined;
}

function firstDuplicate(names: string[]): string | undefined {
  return names.find((name, index) => names.indexOf(name) !== index);
}
