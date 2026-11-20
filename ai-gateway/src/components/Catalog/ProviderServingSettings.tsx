import React, { useId, useState } from 'react';
import { Alert, Button, Field, IconButton, Input, Stack, Switch, Text } from '@grafana/ui';
import { errorMessage, ProviderModelListing } from '../../api/modelCatalogs';
import { Provider } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';
import { ModelPicker } from './ModelPicker';

interface Props {
  name: string;
  provider: Provider;
  readOnly: boolean;
  showBlocklist: boolean;
  onEditBlockedIDs: (editing: boolean) => void;
  listModels?: () => Promise<ProviderModelListing>;
  listingRevision?: string;
  onSave: (provider: Provider) => Promise<void>;
}

/** ProviderServingSettings saves changes to a provider's serving mode and blocklist. */
export function ProviderServingSettings({
  name,
  provider,
  readOnly,
  showBlocklist,
  onEditBlockedIDs,
  listModels,
  listingRevision,
  onSave,
}: Props) {
  const switchID = useId();
  const [modelID, setModelID] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const serveAllModels = provider.serveAllModels === true;
  const blocklist = provider.modelBlocklist ?? [];
  const duplicate = blocklist.includes(modelID);
  const canAdd = modelID.trim() !== '' && !duplicate;

  if (readOnly) {
    return (
      <Stack direction="column" gap={0.5}>
        <Text>{serveAllModels ? 'Serves unlisted models' : 'Serves listed models only'}</Text>
        <Text color="secondary">Blocked model IDs: {blocklist.join(', ') || 'None'}</Text>
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
      onEditBlockedIDs(false);
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
        description={`Allows all models this provider account can use, except blocked IDs. Apps request unlisted models as ${name}/<model-id>.`}
      >
        <Switch
          id={switchID}
          aria-label="Serve every model"
          value={serveAllModels}
          disabled={saving}
          onChange={(event) => void changeMode(event.currentTarget.checked)}
        />
      </Field>
      {!serveAllModels && (
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={saving}
          aria-expanded={showBlocklist}
          onClick={() => {
            onEditBlockedIDs(!showBlocklist);
            setModelID('');
            setError(undefined);
          }}
        >
          {showBlocklist ? 'Close blocked IDs' : 'Edit blocked IDs'}
        </Button>
      )}
      {showBlocklist && (
        <Field
          label="Blocked model IDs"
          description="Choose or enter an exact provider model ID. Adding or removing an ID saves immediately."
          invalid={duplicate}
          error={duplicate ? 'This ID is already blocked.' : undefined}
        >
          <Stack direction="column" gap={1}>
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
