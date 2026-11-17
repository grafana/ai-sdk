import React, { ReactNode, useState } from 'react';
import { Alert, Button, Modal } from '@grafana/ui';
import { errorMessage } from '../../api/modelCatalogs';

interface Props {
  title: string;
  submitLabel: string;
  onSubmit: () => Promise<void>;
  onDismiss: () => void;
  children: ReactNode;
}

/**
 * FormModal is a modal form whose submit writes to the platform. A refused
 * write shows the message the platform returned and keeps the input; the
 * caller closes the modal once the write succeeds.
 */
export function FormModal({ title, submitLabel, onSubmit, onDismiss, children }: Props) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  const submit = async () => {
    setSaving(true);
    setError(undefined);
    try {
      await onSubmit();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal title={title} isOpen onDismiss={onDismiss}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        {children}
        {error && (
          <Alert severity="error" title="The change wasn't saved">
            {error}
          </Alert>
        )}
        <Modal.ButtonRow>
          <Button variant="secondary" fill="outline" onClick={onDismiss}>
            Cancel
          </Button>
          <Button type="submit" disabled={saving}>
            {saving ? 'Saving...' : submitLabel}
          </Button>
        </Modal.ButtonRow>
      </form>
    </Modal>
  );
}
