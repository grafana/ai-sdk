import React, { useState } from 'react';
import { Field, SecretInput } from '@grafana/ui';
import { FormModal } from './FormModal';
import { useFieldErrors } from './useFieldErrors';
import { CatalogChange, fieldErrors, keyChange, keyError, providerTypeLabel } from './catalogSpec';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  spec: Spec;
  provider: string;
  onSave: (change: CatalogChange) => Promise<void>;
  onDismiss: () => void;
}

/** ReplaceKeyModal replaces a provider's API key. Only the new key is sent. */
export function ReplaceKeyModal({ spec, provider, onSave, onDismiss }: Props) {
  const [key, setKey] = useState('');
  const checks = useFieldErrors(fieldErrors({ key: keyError(key) }), { key });
  const error = checks.errors.key;
  const { baseURL, type } = spec.providers[provider];
  const destination = baseURL || `${providerTypeLabel(type)}'s native endpoint`;

  const submit = async () => {
    if (checks.submit()) {
      await onSave({ spec, secure: keyChange(provider, key) });
    }
  };

  return (
    <FormModal title={`Replace the ${provider} key`} submitLabel="Save key" onSubmit={submit} onDismiss={onDismiss}>
      <Field
        label="New API key"
        description={`Use a key for ${destination}. It replaces the saved key and applies on the next request.`}
        invalid={error !== undefined}
        error={error}
      >
        <SecretInput
          id="provider-key"
          autoComplete="off"
          isConfigured={false}
          value={key}
          onReset={() => setKey('')}
          onChange={(event) => setKey(event.currentTarget.value)}
        />
      </Field>
    </FormModal>
  );
}
