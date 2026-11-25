import React, { useState } from 'react';
import { Field, Input, RadioButtonGroup, SecretInput, Stack } from '@grafana/ui';
import { useFieldErrors } from './useFieldErrors';
import {
  defaultProviderName,
  fieldErrors,
  keyError,
  PROVIDER_TYPES,
  providerNameError,
  providerTypeLabel,
  ProviderType,
} from './catalogSpec';
import { Provider, Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

/** useProviderFields collects the same provider account details for create and connect. */
export function useProviderFields(providers: Spec['providers']) {
  const taken = Object.keys(providers);
  const connected = Object.values(providers).map((provider) => provider.type);
  const [type, setType] = useState(
    () => PROVIDER_TYPES.find((option) => !connected.includes(option.value))?.value ?? ProviderType.Anthropic
  );
  const [customName, setCustomName] = useState<string>();
  const [key, setKey] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const askName = taken.includes(type);
  const name = askName ? (customName ?? defaultProviderName(type, taken)) : type;
  const checks = useFieldErrors(
    fieldErrors({
      name: askName ? providerNameError(name, taken) : undefined,
      key: keyError(key),
      endpoint:
        type === ProviderType.OpenAICompatible && endpoint.trim() === '' ? 'Enter the provider endpoint.' : undefined,
    }),
    { name, key, endpoint }
  );
  const { errors } = checks;

  const submit = () => {
    if (!checks.submit()) {
      return;
    }
    const baseURL = endpoint.trim();
    const provider: Provider = { type, ...(baseURL ? { baseURL } : {}) };
    return { name, provider, key };
  };

  const fields = (
    <Stack direction="column" gap={0}>
      <Field label="Provider">
        <RadioButtonGroup
          aria-label="Provider"
          options={PROVIDER_TYPES}
          value={type}
          onChange={(next) => {
            setType(next);
            setCustomName(undefined);
            setEndpoint('');
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
        label="Provider endpoint"
        description={
          type === ProviderType.OpenAICompatible
            ? 'Use a custom endpoint allowed by your Gateway operator.'
            : 'Optional. Leave blank for the native endpoint. Custom endpoints must be allowed by your Gateway operator.'
        }
        invalid={errors.endpoint !== undefined}
        error={errors.endpoint}
      >
        <Input
          id="provider-endpoint"
          value={endpoint}
          onChange={(event) => setEndpoint(event.currentTarget.value)}
          onBlur={() => checks.leave('endpoint')}
        />
      </Field>
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
  );
  return { fields, submit };
}
