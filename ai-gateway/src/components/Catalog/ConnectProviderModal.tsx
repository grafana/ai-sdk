import React from 'react';
import { FormModal } from './FormModal';
import { useProviderFields } from './useProviderFields';
import { CatalogChange, keyChange, withProvider } from './catalogSpec';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  spec: Spec;
  onSave: (change: CatalogChange) => Promise<void>;
  onDismiss: () => void;
}

/** ConnectProviderModal adds a provider account with its key. */
export function ConnectProviderModal({ spec, onSave, onDismiss }: Props) {
  const provider = useProviderFields(spec.providers);
  const submit = async () => {
    const account = provider.submit();
    if (account) {
      await onSave({
        spec: withProvider(spec, account.name, account.provider),
        secure: keyChange(account.name, account.key),
      });
    }
  };

  return (
    <FormModal title="Connect a provider" submitLabel="Connect" onSubmit={submit} onDismiss={onDismiss}>
      {provider.fields}
    </FormModal>
  );
}
