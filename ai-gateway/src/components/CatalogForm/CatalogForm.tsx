import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import {
  Button,
  Combobox,
  ComboboxOption,
  Field,
  IconButton,
  Input,
  RadioButtonGroup,
  SecretInput,
  Stack,
  TagsInput,
  Text,
  useStyles2,
} from '@grafana/ui';
import {
  CatalogForm as CatalogFormState,
  isKeyConfigured,
  ModelRow,
  newModelRow,
  newProviderRow,
  ProviderRow,
  ProviderType,
} from './catalogForm';
import { Model, Target } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

const PROVIDER_TYPES: Array<{ label: string; value: ProviderType }> = [
  { label: 'Anthropic', value: ProviderType.Anthropic },
  { label: 'OpenAI', value: ProviderType.OpenAI },
];

const MAX_ALIAS_LENGTH = 128;
const CONFIGURED = 'configured';
const NOT_CONFIGURED = 'not configured';

interface Props {
  form: CatalogFormState;
  onChange: (form: CatalogFormState) => void;
  storedKeys: string[];
  creating: boolean;
  readOnly: boolean;
}

/** CatalogForm edits a catalog's name, providers with their keys, and models. */
export function CatalogForm({ form, onChange, storedKeys, creating, readOnly }: Props) {
  const s = useStyles2(getStyles);
  const providerNames = form.providers.map((row) => row.name).filter((name) => name !== '');

  const setProvider = (index: number, row: ProviderRow) =>
    onChange({ ...form, providers: form.providers.map((current, i) => (i === index ? row : current)) });
  const setModel = (index: number, row: ModelRow) =>
    onChange({ ...form, models: form.models.map((current, i) => (i === index ? row : current)) });

  return (
    <Stack direction="column" gap={3}>
      <Field label="Catalog name" description="Part of the catalog's URL. It can't be changed later.">
        <Input
          id="catalog-name"
          aria-label="Catalog name"
          value={form.name}
          disabled={!creating || readOnly}
          onChange={(event) => onChange({ ...form, name: event.currentTarget.value })}
        />
      </Field>

      <Stack direction="column" gap={1}>
        <Text element="h2" variant="h4">
          Providers
        </Text>
        <Text color="secondary">
          Each provider needs its API key. Saved keys are never shown again; reset one to replace it.
        </Text>
        {form.providers.map((row, index) => (
          <ProviderFields
            key={index}
            index={index}
            row={row}
            configured={isKeyConfigured(row, storedKeys)}
            readOnly={readOnly}
            onChange={(next) => setProvider(index, next)}
            onRemove={() => onChange({ ...form, providers: form.providers.filter((_, i) => i !== index) })}
          />
        ))}
        {!readOnly && (
          <div>
            <Button
              variant="secondary"
              icon="plus"
              onClick={() => onChange({ ...form, providers: [...form.providers, newProviderRow()] })}
            >
              Add provider
            </Button>
          </div>
        )}
      </Stack>

      <Stack direction="column" gap={1}>
        <Text element="h2" variant="h4">
          Models
        </Text>
        {form.models.map((row, index) => (
          <div key={index} className={s.model}>
            <ModelFields
              index={index}
              row={row}
              providerNames={providerNames}
              readOnly={readOnly}
              onChange={(next) => setModel(index, next)}
              onRemove={() => onChange({ ...form, models: form.models.filter((_, i) => i !== index) })}
            />
          </div>
        ))}
        {!readOnly && (
          <div>
            <Button
              variant="secondary"
              icon="plus"
              onClick={() => onChange({ ...form, models: [...form.models, newModelRow()] })}
            >
              Add model
            </Button>
          </div>
        )}
      </Stack>
    </Stack>
  );
}

interface ProviderFieldsProps {
  index: number;
  row: ProviderRow;
  configured: boolean;
  readOnly: boolean;
  onChange: (row: ProviderRow) => void;
  onRemove: () => void;
}

function ProviderFields({ index, row, configured, readOnly, onChange, onRemove }: ProviderFieldsProps) {
  const id = `provider-${index}`;
  const label = `Provider ${index + 1}`;

  return (
    <Stack alignItems="flex-end" gap={2}>
      <Field label="Name" noMargin>
        <Input
          id={`${id}-name`}
          aria-label={`${label} name`}
          value={row.name}
          disabled={readOnly}
          onChange={(event) => onChange({ ...row, name: event.currentTarget.value })}
        />
      </Field>
      <Field label="Type" noMargin>
        <RadioButtonGroup
          aria-label={`${label} type`}
          options={PROVIDER_TYPES}
          value={row.provider.type as ProviderType}
          disabled={readOnly}
          onChange={(type) => onChange({ ...row, provider: { ...row.provider, type }, reset: true })}
        />
      </Field>
      <Field label="API key" noMargin>
        {readOnly ? (
          <Input aria-label={`${label} API key`} value={configured ? CONFIGURED : NOT_CONFIGURED} disabled />
        ) : (
          <SecretInput
            id={`${id}-key`}
            aria-label={`${label} API key`}
            autoComplete="off"
            isConfigured={configured}
            value={row.key}
            onReset={() => onChange({ ...row, key: '', reset: true })}
            onChange={(event) => onChange({ ...row, key: event.currentTarget.value })}
          />
        )}
      </Field>
      {!readOnly && <IconButton name="trash-alt" tooltip={`Remove ${label.toLowerCase()}`} onClick={onRemove} />}
    </Stack>
  );
}

interface ModelFieldsProps {
  index: number;
  row: ModelRow;
  providerNames: string[];
  readOnly: boolean;
  onChange: (row: ModelRow) => void;
  onRemove: () => void;
}

function ModelFields({ index, row, providerNames, readOnly, onChange, onRemove }: ModelFieldsProps) {
  const id = `model-${index}`;
  const label = `Model ${index + 1}`;
  const { model } = row;
  const setModel = (next: Partial<Model>) => onChange({ ...row, model: { ...model, ...next } });
  const fallback = model.fallback ?? [];

  return (
    <Stack direction="column" gap={1}>
      <Stack justifyContent="space-between">
        <Text variant="h5">{label}</Text>
        {!readOnly && <IconButton name="trash-alt" tooltip={`Remove ${label.toLowerCase()}`} onClick={onRemove} />}
      </Stack>
      <Stack gap={2} wrap="wrap">
        <Field label="ID" description="The name applications request." noMargin>
          <Input
            id={`${id}-id`}
            aria-label={`${label} ID`}
            value={row.id}
            disabled={readOnly}
            onChange={(event) => onChange({ ...row, id: event.currentTarget.value })}
          />
        </Field>
        <Field label="Display name" noMargin>
          <Input
            id={`${id}-name`}
            aria-label={`${label} display name`}
            value={model.name}
            disabled={readOnly}
            onChange={(event) => setModel({ name: event.currentTarget.value })}
          />
        </Field>
        <Field label="Description" noMargin>
          <Input
            id={`${id}-description`}
            aria-label={`${label} description`}
            value={model.description ?? ''}
            disabled={readOnly}
            onChange={(event) => setModel({ description: event.currentTarget.value })}
          />
        </Field>
      </Stack>
      <Field label="Aliases" description="Other names applications can request." noMargin>
        <TagsInput
          id={`${id}-aliases`}
          tags={model.aliases ?? []}
          disabled={readOnly}
          maxLength={MAX_ALIAS_LENGTH}
          addOnBlur
          onChange={(aliases) => setModel({ aliases })}
        />
      </Field>
      <TargetFields
        label={`${label} primary`}
        title="Primary"
        target={model.primary}
        providerNames={providerNames}
        readOnly={readOnly}
        onChange={(primary) => setModel({ primary })}
      />
      {fallback.map((target, i) => (
        <TargetFields
          key={i}
          label={`${label} fallback ${i + 1}`}
          title={`Fallback ${i + 1}`}
          target={target}
          providerNames={providerNames}
          readOnly={readOnly}
          onChange={(next) => setModel({ fallback: fallback.map((current, j) => (j === i ? next : current)) })}
          onRemove={() => setModel({ fallback: fallback.filter((_, j) => j !== i) })}
        />
      ))}
      {!readOnly && (
        <div>
          <Button
            variant="secondary"
            size="sm"
            icon="plus"
            onClick={() => setModel({ fallback: [...fallback, { provider: '', model: '' }] })}
          >
            Add fallback
          </Button>
        </div>
      )}
    </Stack>
  );
}

interface TargetFieldsProps {
  label: string;
  title: string;
  target: Target;
  providerNames: string[];
  readOnly: boolean;
  onChange: (target: Target) => void;
  onRemove?: () => void;
}

function TargetFields({ label, title, target, providerNames, readOnly, onChange, onRemove }: TargetFieldsProps) {
  const names = new Set(target.provider ? [...providerNames, target.provider] : providerNames);
  const options: Array<ComboboxOption<string>> = [...names].map((name) => ({ value: name }));

  return (
    <Stack alignItems="flex-end" gap={2}>
      <Field label={`${title} provider`} noMargin>
        <Combobox
          aria-label={`${label} provider`}
          options={options}
          value={target.provider || null}
          disabled={readOnly}
          width={30}
          onChange={(option) => onChange({ ...target, provider: option.value })}
        />
      </Field>
      <Field label={`${title} provider model`} noMargin>
        <Input
          aria-label={`${label} model`}
          value={target.model}
          disabled={readOnly}
          width={40}
          onChange={(event) => onChange({ ...target, model: event.currentTarget.value })}
        />
      </Field>
      {!readOnly && onRemove && (
        <IconButton name="trash-alt" tooltip={`Remove ${title.toLowerCase()}`} onClick={onRemove} />
      )}
    </Stack>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  model: css({
    padding: theme.spacing(2),
    border: `1px solid ${theme.colors.border.weak}`,
    borderRadius: theme.shape.radius.default,
  }),
});
