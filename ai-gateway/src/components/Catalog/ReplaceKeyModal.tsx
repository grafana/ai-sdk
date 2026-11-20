import React, { useState } from 'react';
import { Field, SecretInput } from '@grafana/ui';
import { FormModal } from './FormModal';
import { useFieldErrors } from './useFieldErrors';
import { CatalogChange, fieldErrors, keyChange, keyError } from './catalogSpec';
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

  const submit = async () => {
    if (checks.submit()) {
      await onSave({ spec, secure: keyChange(provider, key) });
    }
  };

  return (
    <FormModal title={`Replace the ${provider} key`} submitLabel="Save key" onSubmit={submit} onDismiss={onDismiss}>
      <Field
        label="New API key"
        description="Replaces the saved key, which can't be viewed. Calls use the new key from the next request."
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
