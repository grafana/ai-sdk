import React, { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { PluginPage } from '@grafana/runtime';
import { Alert, Button, ConfirmModal, EmptyState, InteractiveTable, LinkButton, Stack, useStyles2 } from '@grafana/ui';
import { deleteModelCatalog, errorMessage, listModelCatalogs } from '../api/modelCatalogs';
import { editCatalogURL, prefixRoute, ROUTES } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { catalogPermissions } from '../permissions';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';

interface CatalogRow {
  name: string;
  providers: string;
  models: string;
}

function catalogRow(catalog: ModelCatalog): CatalogRow {
  return {
    name: catalog.metadata.name,
    providers: Object.entries(catalog.spec.providers ?? {})
      .map(([name, provider]) => `${name} (${provider.type})`)
      .join(', '),
    models: Object.keys(catalog.spec.models ?? {}).join(', '),
  };
}

function CatalogList() {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const [catalogs, setCatalogs] = useState<ModelCatalog[]>();
  const [error, setError] = useState<string>();
  const [pendingDelete, setPendingDelete] = useState<string>();
  const [loads, setLoads] = useState(0);

  useEffect(() => {
    if (!permissions.read) {
      return;
    }
    let current = true;
    listModelCatalogs().then(
      (items) => current && setCatalogs(items),
      (err) => current && setError(errorMessage(err))
    );
    return () => {
      current = false;
    };
  }, [permissions.read, loads]);

  const confirmDelete = async () => {
    const name = pendingDelete;
    setPendingDelete(undefined);
    if (name === undefined) {
      return;
    }
    try {
      await deleteModelCatalog(name);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
    setLoads((count) => count + 1);
  };

  const rows = useMemo(() => (catalogs ?? []).map(catalogRow), [catalogs]);
  const columns = useMemo(
    () => [
      {
        id: 'name',
        header: 'Name',
        cell: ({ row }: { row: { original: CatalogRow } }) => (
          <Link className={s.link} to={editCatalogURL(row.original.name)}>
            {row.original.name}
          </Link>
        ),
      },
      { id: 'providers', header: 'Providers' },
      { id: 'models', header: 'Models' },
      {
        id: 'actions',
        visible: () => permissions.delete,
        cell: ({ row }: { row: { original: CatalogRow } }) => (
          <Button
            variant="destructive"
            fill="text"
            icon="trash-alt"
            aria-label={`Delete ${row.original.name}`}
            onClick={() => setPendingDelete(row.original.name)}
          >
            Delete
          </Button>
        ),
      },
    ],
    [permissions.delete, s.link]
  );

  const newCatalog = permissions.create ? (
    <LinkButton icon="plus" href={prefixRoute(ROUTES.NewCatalog)}>
      New catalog
    </LinkButton>
  ) : undefined;

  return (
    <PluginPage subTitle="Providers, keys and models this stack serves through its own catalog routes.">
      {!permissions.read && <NoCatalogAccess />}
      {error && (
        <Alert severity="error" title="Model catalog request failed">
          {error}
        </Alert>
      )}
      {permissions.read && catalogs?.length === 0 && (
        <EmptyState variant="call-to-action" message="No model catalogs yet" button={newCatalog} />
      )}
      {permissions.read && rows.length > 0 && (
        <Stack direction="column" gap={2}>
          {newCatalog && <Stack justifyContent="flex-end">{newCatalog}</Stack>}
          <InteractiveTable columns={columns} data={rows} getRowId={(row) => row.name} />
        </Stack>
      )}
      <ConfirmModal
        isOpen={pendingDelete !== undefined}
        title="Delete model catalog"
        body={`Delete the catalog "${pendingDelete}"? Applications that call its URL stop working.`}
        confirmText="Delete"
        onConfirm={confirmDelete}
        onDismiss={() => setPendingDelete(undefined)}
      />
    </PluginPage>
  );
}

export default CatalogList;

const getStyles = (theme: GrafanaTheme2) => ({
  link: css({
    color: theme.colors.text.link,
  }),
});
