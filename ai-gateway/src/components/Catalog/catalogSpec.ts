import { SecureChanges } from '../../api/modelCatalogs';
import { Model, Provider, Spec, Target } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

/** ProviderType is a provider type a stack catalog accepts. */
export enum ProviderType {
  Anthropic = 'anthropic',
  OpenAI = 'openai',
}

/** PROVIDER_TYPES are the provider types a writer can choose, with their labels. */
export const PROVIDER_TYPES: Array<{ label: string; value: ProviderType }> = [
  { label: 'Anthropic', value: ProviderType.Anthropic },
  { label: 'OpenAI', value: ProviderType.OpenAI },
];

/** MAX_FALLBACKS is the most fallbacks a model can have: the Gateway tries at most eight providers per call. */
export const MAX_FALLBACKS = 7;

/** FieldErrors maps a field to the reason its value can't be saved. */
export type FieldErrors = Record<string, string>;

/** providerTypeLabel returns the label of a provider type. */
export function providerTypeLabel(type: string): string {
  return PROVIDER_TYPES.find((option) => option.value === type)?.label ?? type;
}

/** modelsUsing returns the IDs of the models that route calls to a provider. */
export function modelsUsing(spec: Spec, provider: string): string[] {
  return Object.entries(spec.models)
    .filter(([, model]) => targets(model).some((target) => target.provider === provider))
    .map(([id]) => id);
}

function targets(model: Model): Target[] {
  return [model.primary, ...(model.fallback ?? [])];
}

/** withProvider returns the spec with the named provider added or replaced. */
export function withProvider(spec: Spec, name: string, provider: Provider): Spec {
  return { ...spec, providers: { ...spec.providers, [name]: provider } };
}

/** withoutProvider returns the spec without the named provider. */
export function withoutProvider(spec: Spec, name: string): Spec {
  return { ...spec, providers: without(spec.providers, name) };
}

/** withModel returns the spec with the model added or replaced, without its empty optional fields. */
export function withModel(spec: Spec, id: string, model: Model): Spec {
  return { ...spec, models: { ...spec.models, [id]: withoutEmptyOptionals(model) } };
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

/** keyChange returns the secure values that store a typed key, or none for an empty one, so the stored key stays. */
export function keyChange(provider: string, key: string): SecureChanges {
  return key === '' ? {} : { [provider]: { create: key } };
}

/** keyRemoval returns the secure values that delete a removed provider's stored key. */
export function keyRemoval(provider: string, storedKeys: string[]): SecureChanges {
  return storedKeys.includes(provider) ? { [provider]: { remove: true } } : {};
}

/** ProviderDraft is the provider a writer is adding or editing. It holds a key only while the writer types it. */
export interface ProviderDraft {
  name: string;
  type: string;
  key: string;
  replaceKey: boolean;
}

/** StoredProvider is the saved state of a provider being edited. */
export interface StoredProvider {
  type: string;
  keySaved: boolean;
}

/** newProviderDraft returns the draft of a provider to add. */
export function newProviderDraft(): ProviderDraft {
  return { name: ProviderType.Anthropic, type: ProviderType.Anthropic, key: '', replaceKey: false };
}

/** needsKey says whether saving the draft needs a typed key. */
export function needsKey(draft: ProviderDraft, stored?: StoredProvider): boolean {
  return stored === undefined || !stored.keySaved || draft.replaceKey || draft.type !== stored.type;
}

/**
 * providerErrors returns what keeps a provider draft from being saved: a
 * missing value, a name another provider has, or a missing key. Name rules
 * are the platform's to check.
 */
export function providerErrors(draft: ProviderDraft, taken: string[], stored?: StoredProvider): FieldErrors {
  const errors: FieldErrors = {};
  if (draft.name.trim() === '') {
    errors.name = 'Enter a name.';
  } else if (stored === undefined && taken.includes(draft.name)) {
    errors.name = `A provider named "${draft.name}" already exists.`;
  }
  if (draft.key === '' && needsKey(draft, stored)) {
    errors.key =
      stored !== undefined && draft.type !== stored.type
        ? 'Enter a new API key: the saved key belongs to the previous type.'
        : 'Enter the API key.';
  }
  return errors;
}

/** ModelDraft is the model a writer is adding or editing. */
export interface ModelDraft {
  id: string;
  name: string;
  description: string;
  aliases: string[];
  primary: Target;
  fallback: Target[];
}

/** newModelDraft returns the draft of a model to add, routed to the given provider. */
export function newModelDraft(provider = ''): ModelDraft {
  return { id: '', name: '', description: '', aliases: [], primary: { provider, model: '' }, fallback: [] };
}

/** modelDraft returns the draft of a stored model. */
export function modelDraft(id: string, model: Model): ModelDraft {
  return {
    id,
    name: model.name,
    description: model.description ?? '',
    aliases: model.aliases ?? [],
    primary: model.primary,
    fallback: model.fallback ?? [],
  };
}

/** modelFromDraft returns the model a draft describes, keeping the stored model's other fields. */
export function modelFromDraft(draft: ModelDraft, stored?: Model): Model {
  return {
    ...stored,
    name: draft.name,
    description: draft.description,
    aliases: draft.aliases,
    primary: draft.primary,
    fallback: draft.fallback,
  };
}

/**
 * modelErrors returns what keeps a model draft from being saved: a missing
 * value, or an ID another model has. ID and alias rules are the platform's to
 * check.
 */
export function modelErrors(draft: ModelDraft, taken: string[], editing: boolean): FieldErrors {
  const errors: FieldErrors = {};
  if (draft.id.trim() === '') {
    errors.id = 'Enter a model ID.';
  } else if (!editing && taken.includes(draft.id)) {
    errors.id = `A model with ID "${draft.id}" already exists.`;
  }
  if (draft.name.trim() === '') {
    errors.name = 'Enter a display name.';
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

/** targetField names a model's primary, at index 0, or one of its fallbacks in field errors. */
export function targetField(index: number): string {
  return index === 0 ? 'primary' : `fallback.${index - 1}`;
}

/** hasErrors says whether any field has an error. */
export function hasErrors(errors: FieldErrors): boolean {
  return Object.keys(errors).length > 0;
}
