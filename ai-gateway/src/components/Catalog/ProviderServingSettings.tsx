import React, { useId, useState } from 'react';
import { Alert, Button, Field, IconButton, Input, Stack, Switch, Text } from '@grafana/ui';
import { errorMessage, ProviderModelListing } from '../../api/modelCatalogs';
import { Provider } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';
import { ModelPicker } from './ModelPicker';
import { publicIDError } from './catalogSpec';

interface Props {
  name: string;
  provider: Provider;
  readOnly: boolean;
  listModels?: () => Promise<ProviderModelListing>;
  listingRevision?: string;
  onSave: (provider: Provider) => Promise<void>;
}

/** ProviderServingSettings saves changes to a provider's serving mode and blocklist. */
export function ProviderServingSettings({ name, provider, readOnly, listModels, listingRevision, onSave }: Props) {
  const switchID = useId();
  const [modelID, setModelID] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const serveAllModels = provider.serveAllModels === true;
  const blocklist = provider.modelBlocklist ?? [];
  const inputError = blocklist.includes(modelID)
    ? 'This ID is already blocked.'
    : modelID === ''
      ? undefined
      : publicIDError(modelID);
  const canAdd = serveAllModels && modelID !== '' && !inputError;

  if (readOnly) {
    return (
      <Stack direction="column" gap={0.5}>
        <Text>{serveAllModels ? 'Serves unlisted models' : 'Serves listed models only'}</Text>
        {(serveAllModels || blocklist.length > 0) && (
          <Text color="secondary">Blocked model IDs: {blocklist.join(', ') || 'None'}</Text>
        )}
      </Stack>
    );
  }

  const save = async (next: Provider): Promise<boolean> => {
    if (saving) {
      return false;
    }
    setError(undefined);
    setSaving(true);
    try {
      await onSave(next);
      return true;
    } catch (err) {
      setError(errorMessage(err));
      return false;
    } finally {
      setSaving(false);
    }
  };

  const changeMode = async (enabled: boolean) => {
    if (await save({ ...provider, serveAllModels: enabled })) {
      setModelID('');
    }
  };

  const addBlockedID = async () => {
    if (canAdd && (await save({ ...provider, modelBlocklist: [...blocklist, modelID] }))) {
      setModelID('');
    }
  };

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void addBlockedID();
      }}
    >
      {error && (
        <Alert severity="error" title="Provider serving settings weren’t saved">
          {error}
        </Alert>
      )}
      <Field
        label="Serve every model"
        htmlFor={switchID}
        description={`Apps can request unlisted models as ${name}/<model-id>. The selector and model ID must fit the model ID format and the model must not be blocked.`}
      >
        <Switch
          id={switchID}
          aria-label="Serve every model"
          value={serveAllModels}
          disabled={saving}
          onChange={(event) => void changeMode(event.currentTarget.checked)}
        />
      </Field>
      {(serveAllModels || blocklist.length > 0) && (
        <Field
          label="Blocked model IDs"
          description={
            serveAllModels
              ? 'Choose or enter an exact provider model ID. Adding or removing an ID saves immediately.'
              : 'These IDs still cannot be added as listed models. Remove an ID here to allow it.'
          }
          invalid={serveAllModels && inputError !== undefined}
          error={serveAllModels ? inputError : undefined}
        >
          <Stack direction="column" gap={1}>
            {serveAllModels && (
              <Stack alignItems="flex-start" gap={1}>
                {listModels ? (
                  <ModelPicker
                    key={listingRevision}
                    label="Add a blocked model ID"
                    value={modelID}
                    onChange={setModelID}
                    disabled={saving}
                    listModels={listModels}
                  />
                ) : (
                  <Input
                    aria-label="Add a blocked model ID"
                    placeholder="Provider model ID"
                    value={modelID}
                    disabled={saving}
                    onChange={(event) => setModelID(event.currentTarget.value)}
                  />
                )}
                <Button type="submit" variant="secondary" disabled={saving || !canAdd}>
                  Add blocked ID
                </Button>
              </Stack>
            )}
            {blocklist.length > 0 && (
              <ul aria-label="Blocked model IDs">
                {blocklist.map((id) => (
                  <li key={id}>
                    <Stack alignItems="center" gap={1}>
                      <Text>{id}</Text>
                      <IconButton
                        type="button"
                        name="times"
                        tooltip={`Remove blocked ID ${id}`}
                        disabled={saving}
                        onClick={() =>
                          void save({ ...provider, modelBlocklist: blocklist.filter((blocked) => blocked !== id) })
                        }
                      />
                    </Stack>
                  </li>
                ))}
              </ul>
            )}
          </Stack>
        </Field>
      )}
    </form>
  );
}
