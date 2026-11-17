import React, { useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Field, Input, Stack, Text, useStyles2 } from '@grafana/ui';
import { FormModal } from './FormModal';
import { ModelIdentityFields } from './ModelFields';
import { ProviderFields } from './ProviderFields';
import {
  FieldErrors,
  hasErrors,
  keyChange,
  modelErrors,
  modelFromDraft,
  newModelDraft,
  newProviderDraft,
  providerErrors,
  targetField,
  withModel,
  withProvider,
} from './catalogSpec';
import { SecureChanges } from '../../api/modelCatalogs';
import { catalogBaseURL } from '../../constants';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

const NAME_PLACEHOLDER = '<name>';

interface Props {
  endpoint: string;
  onCreate: (name: string, spec: Spec, secure: SecureChanges) => Promise<void>;
  onDismiss: () => void;
}

interface Errors {
  name?: string;
  provider: FieldErrors;
  model: FieldErrors;
}

/**
 * NewCatalogModal creates a catalog with its first provider and model, the
 * least the platform accepts. More providers, models and fallbacks are added
 * on the catalog's page.
 */
export function NewCatalogModal({ endpoint, onCreate, onDismiss }: Props) {
  const s = useStyles2(getStyles);
  const [name, setName] = useState('');
  const [provider, setProvider] = useState(newProviderDraft);
  const [model, setModel] = useState(() => newModelDraft());
  const [errors, setErrors] = useState<Errors>({ provider: {}, model: {} });
  const primary = targetField(0);
  const routed = { ...model, primary: { ...model.primary, provider: provider.name } };

  const submit = async () => {
    const found = {
      name: name.trim() === '' ? 'Enter a name.' : undefined,
      provider: providerErrors(provider, []),
      model: modelErrors(routed, [], false),
    };
    setErrors(found);
    if (found.name !== undefined || hasErrors(found.provider) || hasErrors(found.model)) {
      return;
    }
    const spec = withModel(
      withProvider({ providers: {}, models: {} }, provider.name, { type: provider.type }),
      routed.id,
      modelFromDraft(routed)
    );
    await onCreate(name, spec, keyChange(provider.name, provider.key));
  };

  return (
    <FormModal title="New catalog" submitLabel="Create catalog" onSubmit={submit} onDismiss={onDismiss}>
      <Stack direction="column" gap={2}>
        <Field
          label="Catalog name"
          description={
            <>
              Part of the catalog&apos;s URL, so it can&apos;t be changed. Lowercase letters, digits, &apos;-&apos; and
              &apos;.&apos;.
              <span className={s.url}>{catalogBaseURL(endpoint, name || NAME_PLACEHOLDER)}</span>
            </>
          }
          invalid={errors.name !== undefined}
          error={errors.name}
          noMargin
        >
          <Input id="catalog-name" value={name} onChange={(event) => setName(event.currentTarget.value)} />
        </Field>
        <section>
          <Text element="h3" variant="h5">
            Provider
          </Text>
          <ProviderFields draft={provider} errors={errors.provider} onChange={setProvider} idPrefix="provider" />
        </section>
        <section>
          <Text element="h3" variant="h5">
            First model
          </Text>
          <ModelIdentityFields
            draft={model}
            errors={errors.model}
            onChange={setModel}
            idPrefix="model"
            editing={false}
            optional={false}
          />
          <Field
            label="Provider model"
            description="The model ID at the provider, as its API expects it. Add fallbacks and more models later."
            invalid={errors.model[`${primary}.model`] !== undefined}
            error={errors.model[`${primary}.model`]}
          >
            <Input
              id="model-target"
              value={model.primary.model}
              onChange={(event) =>
                setModel({ ...model, primary: { ...model.primary, model: event.currentTarget.value } })
              }
            />
          </Field>
        </section>
      </Stack>
    </FormModal>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  url: css({
    display: 'block',
    marginTop: theme.spacing(0.5),
    fontFamily: theme.typography.fontFamilyMonospace,
    overflowWrap: 'anywhere',
  }),
});
