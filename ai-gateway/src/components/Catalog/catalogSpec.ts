import { SecureChanges } from '../../api/modelCatalogs';
import { Model, Provider, Spec, Target } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

/** ProviderType is a provider type a stack catalog accepts. */
export enum ProviderType {
  Anthropic = 'anthropic',
  OpenAI = 'openai',
}

/** PROVIDER_TYPES are the provider types a writer can connect, with their labels. */
export const PROVIDER_TYPES: Array<{ label: string; value: ProviderType }> = [
  { label: 'Anthropic', value: ProviderType.Anthropic },
  { label: 'OpenAI', value: ProviderType.OpenAI },
];

/** MAX_FALLBACKS is the most fallbacks a model can have: the Gateway tries at most eight providers per call. */
export const MAX_FALLBACKS = 7;

const MAX_CATALOG_NAME_LENGTH = 253;
const CATALOG_NAME_PATTERN = /^[A-Za-z0-9:_.-]+$/;
const RESERVED_CATALOG_NAMES = ['search', 'trash', 'history', 'query', 'list-keys'];
const PROVIDER_NAME_PATTERN = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/;
const PUBLIC_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$/;

/** FieldErrors maps a field to the reason its value can't be saved. */
export type FieldErrors = Record<string, string>;

/** CatalogChange is one change to a catalog: its whole new spec and the secure values the write sends. */
export interface CatalogChange {
  spec: Spec;
  secure: SecureChanges;
}

/** emptySpec returns a spec without providers or models. */
export function emptySpec(): Spec {
  return { providers: {}, models: {} };
}

/** providerTypeLabel returns the label of a provider type. */
export function providerTypeLabel(type: string): string {
  return PROVIDER_TYPES.find((option) => option.value === type)?.label ?? type;
}

/**
 * defaultProviderName returns the name a new provider of a type gets: the
 * type itself, or the type with the first free number when it's taken.
 */
export function defaultProviderName(type: string, taken: string[]): string {
  if (!taken.includes(type)) {
    return type;
  }
  let n = 2;
  while (taken.includes(`${type}-${n}`)) {
    n += 1;
  }
  return `${type}-${n}`;
}

/** modelsOf returns the models whose primary is the provider, by ID. */
export function modelsOf(spec: Spec, provider: string): Array<[string, Model]> {
  return Object.entries(spec.models).filter(([, model]) => model.primary.provider === provider);
}

/** fallbackUsers returns the IDs of other providers' models that fall back to the provider. */
export function fallbackUsers(spec: Spec, provider: string): string[] {
  return Object.entries(spec.models)
    .filter(
      ([, model]) =>
        model.primary.provider !== provider && (model.fallback ?? []).some((target) => target.provider === provider)
    )
    .map(([id]) => id);
}

/** withProvider returns the spec with the named provider added or replaced. */
export function withProvider(spec: Spec, name: string, provider: Provider): Spec {
  return { ...spec, providers: { ...spec.providers, [name]: provider } };
}

/** withoutProvider returns the spec without the named provider and the models whose primary it is. */
export function withoutProvider(spec: Spec, name: string): Spec {
  return {
    providers: without(spec.providers, name),
    models: Object.fromEntries(Object.entries(spec.models).filter(([, model]) => model.primary.provider !== name)),
  };
}

/**
 * withModel returns the spec with the model added or replaced, without its
 * empty optional fields. Given the model's previous ID, it replaces that
 * model, so the model can change ID.
 */
export function withModel(spec: Spec, id: string, model: Model, previousId?: string): Spec {
  const models = previousId === undefined ? spec.models : without(spec.models, previousId);
  return { ...spec, models: { ...models, [id]: withoutEmptyOptionals(model) } };
}

/** withoutModel returns the spec without the model. */
export function withoutModel(spec: Spec, id: string): Spec {
  return { ...spec, models: without(spec.models, id) };
}

function without<T>(entries: Record<string, T>, name: string): Record<string, T> {
  return Object.fromEntries(Object.entries(entries).filter(([key]) => key !== name));
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

/** quickModel returns the model a provider model is served as by default: under its own ID and name. */
export function quickModel(provider: string, providerModel: string): Model {
  return { name: providerModel, primary: { provider, model: providerModel } };
}

/** keyChange returns the secure values that store a typed key. */
export function keyChange(provider: string, key: string): SecureChanges {
  return { [provider]: { create: key } };
}

/** keyRemoval returns the secure values that delete a removed provider's key, if it has one. */
export function keyRemoval(provider: string, keys: string[]): SecureChanges {
  return keys.includes(provider) ? { [provider]: { remove: true } } : {};
}

/** keyError returns why a typed key can't be saved, or undefined. */
export function keyError(key: string): string | undefined {
  return key === '' ? 'Enter the API key.' : undefined;
}

/**
 * catalogNameError returns why a catalog name can't be saved, or undefined.
 * It follows Grafana's rules for resource names.
 */
export function catalogNameError(name: string): string | undefined {
  if (name === '') {
    return 'Enter a name.';
  }
  if (name.length > MAX_CATALOG_NAME_LENGTH) {
    return `Use at most ${MAX_CATALOG_NAME_LENGTH} characters.`;
  }
  if (!CATALOG_NAME_PATTERN.test(name)) {
    return "Use only letters, digits and '-', '_', '.' or ':'.";
  }
  return RESERVED_CATALOG_NAMES.includes(name) ? `"${name}" is reserved. Choose another name.` : undefined;
}

/**
 * providerNameError returns why a new provider name can't be saved, or
 * undefined. Provider names are DNS labels, because they also key the
 * provider's secure value.
 */
export function providerNameError(name: string, taken: string[]): string | undefined {
  if (name === '') {
    return 'Enter a name.';
  }
  if (!PROVIDER_NAME_PATTERN.test(name)) {
    return "Use 1 to 63 lowercase letters, digits and '-', starting and ending with a letter or digit.";
  }
  return taken.includes(name) ? `A provider named "${name}" already exists.` : undefined;
}

/** publicIDError returns why a model ID or alias can't be served, or undefined. */
export function publicIDError(id: string): string | undefined {
  return PUBLIC_ID_PATTERN.test(id)
    ? undefined
    : "Use up to 128 letters, digits and '.', '_', ':', '/' or '-', starting with a letter or digit.";
}

/** quickAddError returns why a provider model can't be added under its own ID, or undefined. */
export function quickAddError(spec: Spec, providerModel: string): string | undefined {
  if (providerModel.trim() === '') {
    return "Enter the provider's model ID.";
  }
  const format = publicIDError(providerModel);
  if (format !== undefined) {
    return `Applications request the model by this ID. ${format}`;
  }
  return Object.hasOwn(spec.models, providerModel)
    ? `The catalog already has a model with ID "${providerModel}". Open it to route it here, or give this one another ID.`
    : undefined;
}

/**
 * ModelDraft is the model a writer is editing. An empty name means the
 * display name defaults to the ID.
 */
export interface ModelDraft {
  id: string;
  name: string;
  description: string;
  aliases: string[];
  primary: Target;
  fallback: Target[];
}

/** modelDraft returns the draft of a stored model. */
export function modelDraft(id: string, model: Model): ModelDraft {
  return {
    id,
    name: model.name === id ? '' : model.name,
    description: model.description ?? '',
    aliases: model.aliases ?? [],
    primary: model.primary,
    fallback: model.fallback ?? [],
  };
}

/** modelFromDraft returns the model a draft describes, keeping the stored model's other fields. */
export function modelFromDraft(draft: ModelDraft, stored: Model): Model {
  return {
    ...stored,
    name: draft.name.trim() === '' ? draft.id : draft.name,
    description: draft.description,
    aliases: draft.aliases,
    primary: draft.primary,
    fallback: draft.fallback,
  };
}

/**
 * modelErrors returns what keeps a model draft from being saved: a missing
 * value, an ID or alias outside the Gateway's format, or an ID another model
 * has. Collisions between aliases are the platform's to report.
 */
export function modelErrors(draft: ModelDraft, otherIds: string[]): FieldErrors {
  const errors: FieldErrors = {};
  const idFormat = publicIDError(draft.id);
  if (draft.id.trim() === '') {
    errors.id = 'Enter a model ID.';
  } else if (idFormat !== undefined) {
    errors.id = idFormat;
  } else if (otherIds.includes(draft.id)) {
    errors.id = `A model with ID "${draft.id}" already exists.`;
  }
  const badAlias = draft.aliases.find((alias) => publicIDError(alias) !== undefined);
  if (badAlias !== undefined) {
    errors.aliases = `"${badAlias}" can't be an alias. ${publicIDError(badAlias)}`;
  }
  [draft.primary, ...draft.fallback].forEach((target, index) => {
    const field = targetField(index);
    if (target.provider === '') {
      errors[`${field}.provider`] = 'Choose a provider.';
    }
    if (target.model.trim() === '') {
      errors[`${field}.model`] = "Enter the provider's model ID.";
    }
  });
  return errors;
}

/** modelFieldValues returns a draft's text values by field, as modelErrors names them. */
export function modelFieldValues(draft: ModelDraft): Record<string, string> {
  const values: Record<string, string> = { id: draft.id };
  [draft.primary, ...draft.fallback].forEach((target, index) => {
    values[`${targetField(index)}.provider`] = target.provider;
    values[`${targetField(index)}.model`] = target.model;
  });
  return values;
}

/** targetField names a model's primary, at index 0, or one of its fallbacks in field errors. */
export function targetField(index: number): string {
  return index === 0 ? 'primary' : `fallback.${index - 1}`;
}

/** fieldErrors returns the fields that have an error, from a check per field. */
export function fieldErrors(checks: Record<string, string | undefined>): FieldErrors {
  return Object.fromEntries(
    Object.entries(checks).filter((entry): entry is [string, string] => entry[1] !== undefined)
  );
}

/** hasErrors says whether any field has an error. */
export function hasErrors(errors: FieldErrors): boolean {
  return Object.keys(errors).length > 0;
}

/** sameTarget says whether two targets call the same provider model. */
export function sameTarget(a: Target, b: Target): boolean {
  return a.provider === b.provider && a.model === b.model;
}

/**
 * fallbackSuggestions returns the primaries of the catalog's other models
 * that the draft doesn't call yet, as quick fallback choices.
 */
export function fallbackSuggestions(spec: Spec, draft: ModelDraft, previousId?: string): Target[] {
  const suggestions: Target[] = [];
  for (const [id, model] of Object.entries(spec.models)) {
    const target = model.primary;
    const called = [draft.primary, ...draft.fallback, ...suggestions].some((existing) => sameTarget(existing, target));
    if (id !== previousId && !called) {
      suggestions.push(target);
    }
  }
  return suggestions;
}

/** targetLabel describes a target as its model on its provider. */
export function targetLabel(target: Target): string {
  return `${target.model} on ${target.provider}`;
}

/**
 * draftKeys applies a change's secure values to the keys a new catalog's
 * writer has entered, which stay in memory until the catalog is created.
 */
export function draftKeys(keys: Record<string, string>, secure: SecureChanges): Record<string, string> {
  const next = { ...keys };
  for (const [provider, change] of Object.entries(secure)) {
    if (change.create !== undefined) {
      next[provider] = change.create;
    } else if (change.remove) {
      delete next[provider];
    }
  }
  return next;
}

/** createSecure returns the secure values that store a new catalog's entered keys. */
export function createSecure(keys: Record<string, string>): SecureChanges {
  return Object.fromEntries(Object.entries(keys).map(([provider, key]) => [provider, { create: key }]));
}

/** NewCatalogProblems are what keep a new catalog from being created. */
export interface NewCatalogProblems {
  name?: string;
  content?: string;
}

/** newCatalogProblems returns what keeps a new catalog from being created: no name, provider or model. */
export function newCatalogProblems(name: string, spec: Spec): NewCatalogProblems {
  const problems: NewCatalogProblems = { name: catalogNameError(name) };
  if (Object.keys(spec.providers).length === 0) {
    problems.content = 'Connect a provider and add a model.';
  } else if (Object.keys(spec.models).length === 0) {
    problems.content = 'Add at least one model.';
  }
  return problems;
}
