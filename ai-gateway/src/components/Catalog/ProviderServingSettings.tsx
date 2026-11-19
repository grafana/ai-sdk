import React, { useId, useState } from 'react';
import { Alert, Button, Field, Stack, Switch, Text, TextArea } from '@grafana/ui';
import { errorMessage } from '../../api/modelCatalogs';
import { Provider } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

interface Props {
  name: string;
  provider: Provider;
  readOnly: boolean;
  onSave: (provider: Provider) => Promise<void>;
}

/** ProviderServingSettings edits one saved provider's serving mode and blocklist together. */
export function ProviderServingSettings({ name, provider, readOnly, onSave }: Props) {
  const switchID = useId();
  const blocklistID = useId();
  const [enabled, setEnabled] = useState<boolean>();
  const [text, setText] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const savedEnabled = provider.serveAllModels === true;
  const savedBlocklist = (provider.modelBlocklist ?? []).join('\n');
  const serveAllModels = enabled ?? savedEnabled;
  const blocklist = text ?? savedBlocklist;
  const dirty = serveAllModels !== savedEnabled || blocklist !== savedBlocklist;

  if (readOnly) {
    return (
      <Stack direction="column" gap={0.5}>
        <Text>{savedEnabled ? 'Serves unlisted models' : 'Serves listed models only'}</Text>
        <Text color="secondary">Blocked model IDs: {(provider.modelBlocklist ?? []).join(', ') || 'None'}</Text>
      </Stack>
    );
  }

  const save = async () => {
    if (saving || !dirty) {
      return;
    }
    setError(undefined);
    setSaving(true);
    try {
      const modelBlocklist = blocklist.split('\n').filter((id) => id !== '');
      await onSave({ ...provider, serveAllModels, modelBlocklist });
      setEnabled(undefined);
      setText(undefined);
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
        void save();
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
            setEnabled(event.currentTarget.checked);
            setError(undefined);
          }}
        />
      </Field>
      <Field
        label="Blocked model IDs"
        htmlFor={blocklistID}
        description="Enter one exact provider model ID per line. The blocklist is kept when the switch is off."
      >
        <TextArea
          id={blocklistID}
          aria-label="Blocked model IDs"
          rows={2}
          value={blocklist}
          disabled={saving}
          onChange={(event) => {
            setText(event.currentTarget.value);
            setError(undefined);
          }}
        />
      </Field>
      <Button type="submit" variant="secondary" size="sm" disabled={saving || !dirty}>
        Save serving settings
      </Button>
    </form>
  );
}
