import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, LinkButton, LoadingPlaceholder, Stack } from '@grafana/ui';
import { NoCatalogAccess } from './CatalogList';
import { createModelCatalog, errorMessage, getModelCatalog, updateModelCatalog } from '../api/modelCatalogs';
import { CatalogForm } from '../components/CatalogForm/CatalogForm';
import {
  CatalogForm as CatalogFormState,
  formFromSpec,
  formProblem,
  newModelRow,
  newProviderRow,
  secureChanges,
  specFromForm,
} from '../components/CatalogForm/catalogForm';
import { prefixRoute, ROUTES } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { catalogPermissions } from '../permissions';

function emptyForm(): CatalogFormState {
  return { name: '', providers: [newProviderRow()], models: [newModelRow()] };
}

interface Props {
  name?: string;
}

/**
 * CatalogEditor creates a catalog, or shows and edits the named one. Without
 * the write action for the operation it shows the catalog read-only.
 */
export function CatalogEditor({ name }: Props) {
  const creating = name === undefined;
  const permissions = catalogPermissions();
  const readOnly = creating ? !permissions.create : !permissions.update;
  const navigate = useNavigate();
  const [stored, setStored] = useState<ModelCatalog>();
  const [form, setForm] = useState<CatalogFormState | undefined>(creating ? emptyForm() : undefined);
  const [error, setError] = useState<string>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (name === undefined || !permissions.read) {
      return;
    }
    let current = true;
    getModelCatalog(name).then(
      (catalog) => {
        if (current) {
          setStored(catalog);
          setForm(formFromSpec(catalog.metadata.name, catalog.spec));
        }
      },
      (err) => current && setError(errorMessage(err))
    );
    return () => {
      current = false;
    };
  }, [name, permissions.read]);

  const storedKeys = useMemo(() => Object.keys(stored?.secure ?? {}), [stored]);

  const save = async () => {
    if (!form) {
      return;
    }
    const problem = formProblem(form);
    if (problem) {
      setError(problem);
      return;
    }
    setSaving(true);
    setError(undefined);
    const spec = specFromForm(form);
    const secure = secureChanges(storedKeys, form.providers);
    try {
      if (stored) {
        await updateModelCatalog(stored, spec, secure);
      } else {
        await createModelCatalog(form.name, spec, secure);
      }
      navigate(prefixRoute(ROUTES.Catalogs));
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  };

  const content = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (creating && readOnly) {
      return (
        <Alert severity="info" title="You can't create model catalogs">
          Creating model catalogs needs the Model catalog writer role, which Admins have by default.
        </Alert>
      );
    }
    if (!form) {
      return error ? null : <LoadingPlaceholder text="Loading catalog..." />;
    }
    return (
      <Stack direction="column" gap={3}>
        <CatalogForm form={form} onChange={setForm} storedKeys={storedKeys} creating={creating} readOnly={readOnly} />
        <Stack gap={1}>
          {!readOnly && (
            <Button onClick={save} disabled={saving}>
              {saving ? 'Saving...' : 'Save'}
            </Button>
          )}
          <LinkButton variant="secondary" href={prefixRoute(ROUTES.Catalogs)}>
            {readOnly ? 'Back to catalogs' : 'Cancel'}
          </LinkButton>
        </Stack>
      </Stack>
    );
  };

  return (
    <PluginPage pageNav={{ text: creating ? 'New catalog' : name }}>
      <Stack direction="column" gap={2}>
        {error && (
          <Alert severity="error" title="Model catalog request failed">
            {error}
          </Alert>
        )}
        {readOnly && !creating && permissions.read && (
          <Alert severity="info" title="Read-only">
            Changing model catalogs needs the Model catalog writer role, which Admins have by default.
          </Alert>
        )}
        {content()}
      </Stack>
    </PluginPage>
  );
}

/** CatalogEdit is the catalog page of the route's catalog name, or of a new catalog. */
function CatalogEdit() {
  const { name } = useParams();
  return <CatalogEditor key={name ?? ''} name={name} />;
}

export default CatalogEdit;
