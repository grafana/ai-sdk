import React, { useId, useState } from 'react';
import { Alert, Button, Field, IconButton, Input, Stack, Switch, Text } from '@grafana/ui';
import { errorMessage, ProviderModelListing } from '../../api/modelCatalogs';
import { Provider } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';
import { ModelPicker } from './ModelPicker';

interface Props {
  name: string;
  provider: Provider;
  readOnly: boolean;
  serveAllModels: boolean;
  onModeChange: (enabled: boolean) => void;
  listModels?: () => Promise<ProviderModelListing>;
  listingRevision?: string;
  onSave: (provider: Provider) => Promise<void>;
}

/** ProviderServingSettings edits one saved provider's serving mode and blocklist together. */
export function ProviderServingSettings({
  name,
  provider,
  readOnly,
  serveAllModels,
  onModeChange,
  listModels,
  listingRevision,
  onSave,
}: Props) {
  const switchID = useId();
  const [draftBlocklist, setDraftBlocklist] = useState<string[]>();
  const [modelID, setModelID] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const savedEnabled = provider.serveAllModels === true;
  const savedBlocklist = provider.modelBlocklist ?? [];
  const blocklist = draftBlocklist ?? savedBlocklist;
  const dirty =
    serveAllModels !== savedEnabled ||
    blocklist.length !== savedBlocklist.length ||
    blocklist.some((id, index) => id !== savedBlocklist[index]);
  const duplicate = blocklist.includes(modelID);
  const canAdd = modelID.trim() !== '' && !duplicate;

  if (readOnly) {
    return (
      <Stack direction="column" gap={0.5}>
        <Text>{savedEnabled ? 'Serves unlisted models' : 'Serves listed models only'}</Text>
        <Text color="secondary">Blocked model IDs: {savedBlocklist.join(', ') || 'None'}</Text>
      </Stack>
    );
  }

  const changeBlocklist = (ids: string[]) => {
    setDraftBlocklist(ids);
    setError(undefined);
  };

  const addBlockedID = () => {
    if (!saving && canAdd) {
      changeBlocklist([...blocklist, modelID]);
      setModelID('');
    }
  };

  const save = async () => {
    if (saving || !dirty || modelID !== '') {
      return;
    }
    setError(undefined);
    setSaving(true);
    try {
      await onSave({ ...provider, serveAllModels, modelBlocklist: blocklist });
      setDraftBlocklist(undefined);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        addBlockedID();
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
        description={`Apps request unlisted models as ${name}/<model-id>.`}
      >
        <Switch
          id={switchID}
          aria-label="Serve every model"
          value={serveAllModels}
          disabled={saving}
          onChange={(event) => {
            onModeChange(event.currentTarget.checked);
            setModelID('');
            setError(undefined);
          }}
        />
      </Field>
      {serveAllModels && (
        <Field
          label="Blocked model IDs"
          description="Choose or enter an exact provider model ID, then add it to the blocklist. The blocklist is kept when the switch is off."
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
                        onClick={() => changeBlocklist(blocklist.filter((blocked) => blocked !== id))}
                      />
                    </Stack>
                  </li>
                ))}
              </ul>
            )}
          </Stack>
        </Field>
      )}
      <Button
        type="button"
        variant="secondary"
        size="sm"
        disabled={saving || !dirty || modelID !== ''}
        onClick={() => void save()}
      >
        Save serving settings
      </Button>
    </form>
  );
}
