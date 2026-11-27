import React, { useState } from 'react';
import { Field, Input, RadioButtonGroup, SecretInput, Stack, Text } from '@grafana/ui';
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
    const provider: Provider = { type, ...(baseURL ? { baseURL } : {}), serveAllModels: true };
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
            if (next !== type) {
              setType(next);
              setCustomName(undefined);
              setEndpoint('');
              setKey('');
            }
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
          onChange={(event) => {
            const next = event.currentTarget.value;
            if (next.trim() !== endpoint.trim()) {
              setKey('');
            }
            setEndpoint(next);
          }}
          onBlur={() => checks.leave('endpoint')}
        />
      </Field>
      <Field
        label="API key"
        description={
          endpoint.trim() || type === ProviderType.OpenAICompatible
            ? "Use a key for the custom endpoint above. Grafana stores it as a secret and doesn't show it after saving."
            : `Use a key for ${providerTypeLabel(type)}'s native endpoint. Grafana stores it as a secret and doesn't show it after saving.`
        }
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
      <Text color="secondary">
        This provider will serve every model its account can use. You can change this on the saved provider card.
      </Text>
    </Stack>
  );
  return { fields, submit };
}
