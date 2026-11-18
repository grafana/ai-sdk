import React, { useState } from 'react';
import { Field, Input, RadioButtonGroup, SecretInput, Stack } from '@grafana/ui';
import { FormModal } from './FormModal';
import { useFieldErrors } from './useFieldErrors';
import {
  CatalogChange,
  defaultProviderName,
  fieldErrors,
  keyChange,
  keyError,
  PROVIDER_TYPES,
  providerNameError,
  providerTypeLabel,
  ProviderType,
  withProvider,
} from './catalogSpec';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  spec: Spec;
  onSave: (change: CatalogChange) => Promise<void>;
  onDismiss: () => void;
}

/**
 * ConnectProviderModal adds a provider account with its key. The provider is
 * named after its type; a name is asked for only when another provider of
 * the catalog already has that one.
 */
export function ConnectProviderModal({ spec, onSave, onDismiss }: Props) {
  const taken = Object.keys(spec.providers);
  const connected = Object.values(spec.providers).map((provider) => provider.type);
  const [type, setType] = useState(
    () => PROVIDER_TYPES.find((option) => !connected.includes(option.value))?.value ?? ProviderType.Anthropic
  );
  const [customName, setCustomName] = useState<string>();
  const [key, setKey] = useState('');
  const askName = taken.includes(type);
  const name = askName ? (customName ?? defaultProviderName(type, taken)) : type;
  const checks = useFieldErrors(
    fieldErrors({ name: askName ? providerNameError(name, taken) : undefined, key: keyError(key) }),
    { name, key }
  );
  const { errors } = checks;

  const submit = async () => {
    if (!checks.submit()) {
      return;
    }
    await onSave({ spec: withProvider(spec, name, { type }), secure: keyChange(name, key) });
  };

  return (
    <FormModal title="Connect a provider" submitLabel="Connect" onSubmit={submit} onDismiss={onDismiss}>
      <Stack direction="column" gap={0}>
        <Field label="Provider">
          <RadioButtonGroup
            aria-label="Provider"
            options={PROVIDER_TYPES}
            value={type}
            onChange={(next) => {
              setType(next);
              setCustomName(undefined);
            }}
          />
        </Field>
        {askName && (
          <Field
            label="Account name"
            description={`This catalog already has an ${providerTypeLabel(type)} account. Name this one so models can tell them apart. Lowercase letters, digits and '-'.`}
            invalid={errors.name !== undefined}
            error={errors.name}
          >
            <Input
              id="provider-name"
              value={name}
              onChange={(event) => setCustomName(event.currentTarget.value)}
              onBlur={() => checks.leave('name')}
            />
          </Field>
        )}
        <Field
          label="API key"
          description="Stored as a Grafana secret. It can't be viewed after saving."
          invalid={errors.key !== undefined}
          error={errors.key}
        >
          <SecretInput
            id="provider-key"
            autoComplete="off"
            isConfigured={false}
            value={key}
            onReset={() => setKey('')}
            onChange={(event) => setKey(event.currentTarget.value)}
            onBlur={() => checks.leave('key')}
          />
        </Field>
      </Stack>
    </FormModal>
  );
}
