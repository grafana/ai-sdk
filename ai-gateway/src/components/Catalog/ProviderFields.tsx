import React from 'react';
import { Field, Input, RadioButtonGroup, SecretInput, Stack } from '@grafana/ui';
import { FieldErrors, PROVIDER_TYPES, ProviderDraft, ProviderType, StoredProvider, needsKey } from './catalogSpec';

interface Props {
  draft: ProviderDraft;
  errors: FieldErrors;
  onChange: (draft: ProviderDraft) => void;
  stored?: StoredProvider;
  idPrefix: string;
}

/**
 * ProviderFields edits a provider's type, name and API key. A new provider's
 * name follows its type until the writer changes it; a stored provider's name
 * is fixed, because models and its saved key refer to it.
 */
export function ProviderFields({ draft, errors, onChange, stored, idPrefix }: Props) {
  const keyConfigured = !needsKey(draft, stored);

  const setType = (type: ProviderType) =>
    onChange({ ...draft, type, name: stored === undefined && draft.name === draft.type ? type : draft.name });

  return (
    <Stack direction="column" gap={0}>
      <Field label="Type">
        <RadioButtonGroup
          aria-label="Type"
          options={PROVIDER_TYPES}
          value={draft.type as ProviderType}
          onChange={setType}
        />
      </Field>
      <Field
        label="Name"
        description={
          stored === undefined
            ? "Models route calls to the provider by this name. Lowercase letters, digits and '-'."
            : "Models and the saved key refer to this name, so it can't be changed."
        }
        invalid={errors.name !== undefined}
        error={errors.name}
      >
        <Input
          id={`${idPrefix}-name`}
          value={draft.name}
          disabled={stored !== undefined}
          onChange={(event) => onChange({ ...draft, name: event.currentTarget.value })}
        />
      </Field>
      <Field
        label="API key"
        description={
          keyConfigured
            ? 'The saved key is hidden. Reset it to enter a replacement.'
            : "Stored as a Grafana secret. It can't be viewed after saving."
        }
        invalid={errors.key !== undefined}
        error={errors.key}
      >
        <SecretInput
          id={`${idPrefix}-key`}
          autoComplete="off"
          isConfigured={keyConfigured}
          value={draft.key}
          onReset={() => onChange({ ...draft, key: '', replaceKey: true })}
          onChange={(event) => onChange({ ...draft, key: event.currentTarget.value })}
        />
      </Field>
    </Stack>
  );
}
