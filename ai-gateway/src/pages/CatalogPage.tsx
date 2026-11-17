import React, { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import { Alert, Button, ConfirmModal, LoadingPlaceholder, Stack, useStyles2 } from '@grafana/ui';
import { deleteModelCatalog, errorMessage, getModelCatalog, updateModelCatalog } from '../api/modelCatalogs';
import { CatalogEditor } from '../components/Catalog/CatalogEditor';
import { CatalogURL } from '../components/Catalog/CatalogURL';
import { CatalogChange } from '../components/Catalog/catalogSpec';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';
import { catalogBaseURL, gatewayEndpoint, prefixRoute, ROUTES } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { catalogPermissions } from '../permissions';

interface Props {
  name: string;
}

/**
 * CatalogView shows a stored catalog's URL and its providers with their
 * models. Each change a writer confirms is saved at once, as an update of the
 * stored object, and the page continues from the object the update returns.
 */
function CatalogView({ name }: Props) {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const navigate = useNavigate();
  const [stored, setStored] = useState<ModelCatalog>();
  const [error, setError] = useState<string>();
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (!permissions.read) {
      return;
    }
    let current = true;
    getModelCatalog(name).then(
      (catalog) => current && setStored(catalog),
      (err) => current && setError(errorMessage(err))
    );
    return () => {
      current = false;
    };
  }, [name, permissions.read]);

  const save = async ({ spec, secure }: CatalogChange) => {
    if (stored) {
      setStored(await updateModelCatalog(stored, spec, secure));
    }
  };

  const remove = async () => {
    setDeleting(false);
    try {
      await deleteModelCatalog(name);
      navigate(prefixRoute(ROUTES.Catalogs));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const body = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (!stored) {
      return error ? null : <LoadingPlaceholder text="Loading catalog..." />;
    }
    return (
      <CatalogEditor
        spec={stored.spec}
        keys={Object.keys(stored.secure ?? {})}
        keyState="saved"
        stored
        readOnly={!permissions.update}
        onChange={save}
      />
    );
  };

  return (
    <PluginPage
      pageNav={{ text: name }}
      subTitle={
        stored && (
          <CatalogURL name={name} url={catalogBaseURL(gatewayEndpoint(config.appUrl, config.namespace), name)} />
        )
      }
      actions={
        stored &&
        permissions.delete && (
          <Button variant="destructive" fill="outline" icon="trash-alt" onClick={() => setDeleting(true)}>
            Delete catalog
          </Button>
        )
      }
    >
      <div className={s.page}>
        <Stack direction="column" gap={2}>
          {error && (
            <Alert severity="error" title="Model catalog request failed">
              {error}
            </Alert>
          )}
          {stored && !permissions.update && (
            <Alert severity="info" title="Read-only">
              You can view this catalog. Changing it needs the Model catalog writer role, which Admins have by default.
            </Alert>
          )}
          {body()}
        </Stack>
      </div>
      <ConfirmModal
        isOpen={deleting}
        title="Delete model catalog"
        body={`Delete the catalog "${name}"? Applications that call its URL stop working, and its saved keys are deleted.`}
        confirmText="Delete"
        onConfirm={remove}
        onDismiss={() => setDeleting(false)}
      />
    </PluginPage>
  );
}

/** CatalogPage is the page of the route's catalog. */
function CatalogPage() {
  const { name = '' } = useParams();
  return <CatalogView key={name} name={name} />;
}

export default CatalogPage;

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.lg,
  }),
});
