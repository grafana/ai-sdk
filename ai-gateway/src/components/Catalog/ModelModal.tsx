import React, { useState } from 'react';
import { Stack, Text } from '@grafana/ui';
import { FormModal } from './FormModal';
import { ModelIdentityFields, RoutingFields } from './ModelFields';
import {
  FieldErrors,
  hasErrors,
  modelDraft,
  ModelDraft,
  modelErrors,
  modelFromDraft,
  newModelDraft,
  withModel,
} from './catalogSpec';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  spec: Spec;
  id?: string;
  onSave: (spec: Spec) => Promise<void>;
  onDismiss: () => void;
}

/** ModelModal adds a model to a catalog, or edits the one with the given ID. */
export function ModelModal({ spec, id, onSave, onDismiss }: Props) {
  const stored = id === undefined ? undefined : spec.models[id];
  const [draft, setDraft] = useState<ModelDraft>(() => {
    if (id !== undefined && stored !== undefined) {
      return modelDraft(id, stored);
    }
    const providers = Object.keys(spec.providers);
    return newModelDraft(providers.length === 1 ? providers[0] : '');
  });
  const [errors, setErrors] = useState<FieldErrors>({});
  const editing = stored !== undefined;

  const submit = async () => {
    const found = modelErrors(draft, Object.keys(spec.models), editing);
    setErrors(found);
    if (hasErrors(found)) {
      return;
    }
    await onSave(withModel(spec, draft.id, modelFromDraft(draft, stored)));
  };

  return (
    <FormModal
      title={editing ? `Edit model ${id}` : 'Add model'}
      submitLabel={editing ? 'Save model' : 'Add model'}
      onSubmit={submit}
      onDismiss={onDismiss}
    >
      <Stack direction="column" gap={2}>
        <section>
          <Text element="h3" variant="h5">
            Model
          </Text>
          <ModelIdentityFields
            draft={draft}
            errors={errors}
            onChange={setDraft}
            idPrefix="model"
            editing={editing}
            optional
          />
        </section>
        <section>
          <Stack direction="column" gap={1}>
            <Text element="h3" variant="h5">
              Routing
            </Text>
            <RoutingFields draft={draft} errors={errors} onChange={setDraft} providers={spec.providers} />
          </Stack>
        </section>
      </Stack>
    </FormModal>
  );
}
