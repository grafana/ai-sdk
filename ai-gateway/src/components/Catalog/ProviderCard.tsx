import React, { useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Badge, Button, Field, IconButton, Input, Stack, Text, useStyles2 } from '@grafana/ui';
import { errorMessage } from '../../api/modelCatalogs';
import { providerTypeLabel, targetLabel } from './catalogSpec';
import { Model, Provider } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

/** KeyState is what the page knows of a provider's key: saved in Grafana, entered for a new catalog, or missing. */
export type KeyState = 'saved' | 'entered' | 'missing';

const KEY_BADGES: Record<KeyState, { text: string; color: 'green' | 'blue' | 'orange' }> = {
  saved: { text: 'Key saved', color: 'green' },
  entered: { text: 'Key entered', color: 'blue' },
  missing: { text: 'No key', color: 'orange' },
};

interface Props {
  name: string;
  provider: Provider;
  models: Array<[string, Model]>;
  keyState: KeyState;
  readOnly: boolean;
  removeBlocked?: string;
  lastModel: boolean;
  onOpenModel: (id: string) => void;
  onRemoveModel: (id: string) => void;
  onAddModel: (providerModel: string) => Promise<void>;
  addError: (providerModel: string) => string | undefined;
  onReplaceKey: () => void;
  onRemove: () => void;
}

/**
 * ProviderCard shows one provider account with its key state and the models
 * it serves, and lets a writer add a model by its provider model ID alone.
 */
export function ProviderCard(props: Props) {
  const { name, provider, models, keyState, readOnly, removeBlocked, lastModel } = props;
  const s = useStyles2(getStyles);
  const label = providerTypeLabel(provider.type);
  const badge = KEY_BADGES[keyState];

  return (
    <section className={s.card} aria-label={name}>
      <div className={s.header}>
        <Stack alignItems="center" gap={1} wrap="wrap">
          <Text element="h3" variant="h4">
            {label}
          </Text>
          {name !== provider.type && <code className={s.name}>{name}</code>}
          <Badge text={badge.text} icon="lock" color={badge.color} />
        </Stack>
        {!readOnly && (
          <Stack alignItems="center" gap={1}>
            <Button variant="secondary" size="sm" onClick={props.onReplaceKey}>
              Replace key
            </Button>
            <IconButton
              name="trash-alt"
              tooltip={removeBlocked ?? `Remove ${name}`}
              disabled={removeBlocked !== undefined}
              onClick={props.onRemove}
            />
          </Stack>
        )}
      </div>
      <ul className={s.models}>
        {models.length === 0 && (
          <li className={s.model}>
            <Text color="secondary">No models yet.</Text>
          </li>
        )}
        {models.map(([id, model]) => (
          <li key={id} className={s.model} aria-label={id}>
            <div className={s.modelContent}>
              <Stack alignItems="baseline" gap={1} wrap="wrap">
                <button type="button" className={s.modelID} onClick={() => props.onOpenModel(id)}>
                  {id}
                </button>
                {model.name !== id && <Text color="secondary">{model.name}</Text>}
                {model.primary.model !== id && (
                  <Text color="secondary" variant="bodySmall">
                    serves {model.primary.model}
                  </Text>
                )}
              </Stack>
              {(model.fallback ?? []).length > 0 && (
                <Text color="secondary" variant="bodySmall">
                  Falls back to {(model.fallback ?? []).map(targetLabel).join(', then ')}
                </Text>
              )}
              {(model.aliases ?? []).length > 0 && (
                <Text color="secondary" variant="bodySmall">
                  Also requested as {(model.aliases ?? []).join(', ')}
                </Text>
              )}
            </div>
            {!readOnly && (
              <IconButton
                name="times"
                tooltip={lastModel ? 'A catalog needs at least one model' : `Remove ${id}`}
                disabled={lastModel}
                onClick={() => props.onRemoveModel(id)}
              />
            )}
          </li>
        ))}
      </ul>
      {!readOnly && <QuickAdd name={name} label={label} onAdd={props.onAddModel} addError={props.addError} />}
    </section>
  );
}

interface QuickAddProps {
  name: string;
  label: string;
  onAdd: (providerModel: string) => Promise<void>;
  addError: (providerModel: string) => string | undefined;
}

function QuickAdd({ name, label, onAdd, addError }: QuickAddProps) {
  const s = useStyles2(getStyles);
  const [value, setValue] = useState('');
  const [error, setError] = useState<string>();
  const [saving, setSaving] = useState(false);

  const add = async () => {
    const found = addError(value);
    setError(found);
    if (found !== undefined) {
      return;
    }
    setSaving(true);
    try {
      await onAdd(value);
      setValue('');
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      className={s.add}
      onSubmit={(event) => {
        event.preventDefault();
        void add();
      }}
    >
      <Field
        className={s.addField}
        description={`The model ID in ${label}'s API. Applications request it by the same ID.`}
        invalid={error !== undefined}
        error={error}
        noMargin
      >
        <Input
          aria-label={`Add a model to ${name}`}
          value={value}
          placeholder="Provider model ID"
          onChange={(event) => setValue(event.currentTarget.value)}
        />
      </Field>
      <Button type="submit" variant="secondary" icon="plus" disabled={saving}>
        Add model
      </Button>
    </form>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  card: css({
    background: theme.colors.background.primary,
    border: `1px solid ${theme.colors.border.weak}`,
    borderRadius: theme.shape.radius.default,
  }),
  header: css({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: theme.spacing(2),
    padding: theme.spacing(1.5, 2),
    borderBottom: `1px solid ${theme.colors.border.weak}`,
  }),
  name: css({
    fontFamily: theme.typography.fontFamilyMonospace,
    color: theme.colors.text.secondary,
  }),
  models: css({
    listStyle: 'none',
    margin: 0,
    padding: 0,
  }),
  model: css({
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2),
    padding: theme.spacing(1, 2),
    borderBottom: `1px solid ${theme.colors.border.weak}`,
  }),
  modelContent: css({
    flex: 1,
    minWidth: 0,
  }),
  modelID: css({
    padding: 0,
    border: 'none',
    background: 'none',
    color: theme.colors.text.link,
    fontFamily: theme.typography.fontFamilyMonospace,
    fontWeight: theme.typography.fontWeightMedium,
    cursor: 'pointer',
    '&:hover': {
      textDecoration: 'underline',
    },
  }),
  add: css({
    display: 'flex',
    alignItems: 'flex-start',
    gap: theme.spacing(1),
    padding: theme.spacing(1.5, 2),
  }),
  addField: css({
    flex: 1,
    maxWidth: theme.spacing(60),
  }),
});
