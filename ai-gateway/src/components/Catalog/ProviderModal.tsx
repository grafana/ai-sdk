import React, { useState } from 'react';
import { FormModal } from './FormModal';
import { ProviderFields } from './ProviderFields';
import {
  FieldErrors,
  hasErrors,
  keyChange,
  newProviderDraft,
  ProviderDraft,
  providerErrors,
  StoredProvider,
  withProvider,
} from './catalogSpec';
import { SecureChanges } from '../../api/modelCatalogs';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  spec: Spec;
  storedKeys: string[];
  name?: string;
  onSave: (spec: Spec, secure: SecureChanges) => Promise<void>;
  onDismiss: () => void;
}

/** ProviderModal adds a provider with its key to a catalog, or edits the named one's type and key. */
export function ProviderModal({ spec, storedKeys, name, onSave, onDismiss }: Props) {
  const storedProvider = name === undefined ? undefined : spec.providers[name];
  const stored: StoredProvider | undefined =
    name === undefined || storedProvider === undefined
      ? undefined
      : { type: storedProvider.type, keySaved: storedKeys.includes(name) };
  const [draft, setDraft] = useState<ProviderDraft>(() =>
    name !== undefined && stored !== undefined
      ? { name, type: stored.type, key: '', replaceKey: false }
      : newProviderDraft()
  );
  const [errors, setErrors] = useState<FieldErrors>({});

  const submit = async () => {
    const found = providerErrors(draft, Object.keys(spec.providers), stored);
    setErrors(found);
    if (hasErrors(found)) {
      return;
    }
    await onSave(
      withProvider(spec, draft.name, { ...storedProvider, type: draft.type }),
      keyChange(draft.name, draft.key)
    );
  };

  return (
    <FormModal
      title={stored === undefined ? 'Add provider' : `Edit provider ${name}`}
      submitLabel={stored === undefined ? 'Add provider' : 'Save provider'}
      onSubmit={submit}
      onDismiss={onDismiss}
    >
      <ProviderFields draft={draft} errors={errors} onChange={setDraft} stored={stored} idPrefix="provider" />
    </FormModal>
  );
}
