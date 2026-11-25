import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import { Alert, Badge, Button, ConfirmModal, LoadingPlaceholder, Stack, Text, useStyles2 } from '@grafana/ui';
import {
  CatalogHealth,
  catalogHealthErrorMessage,
  checkModelCatalogHealth,
  deleteModelCatalog,
  errorMessage,
  getModelCatalog,
  listProviderModels,
  updateModelCatalog,
} from '../api/modelCatalogs';
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

function countHealthProblems(health: CatalogHealth): number {
  return [
    ...Object.values(health.providers),
    ...Object.values(health.models).flatMap(({ primary, fallback }) => [primary, ...fallback]),
  ].filter((status) => status !== 'ok').length;
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
  const location = useLocation();
  const [stored, setStored] = useState<ModelCatalog>();
  const [error, setError] = useState<string>();
  const [deleting, setDeleting] = useState(false);
  const [health, setHealth] = useState<CatalogHealth>();
  const [healthError, setHealthError] = useState<string>();
  const [checking, setChecking] = useState(false);
  const healthGeneration = useRef(0);
  const inFlight = useRef(false);
  const consumedArrival = useRef(false);
  const canCheckHealth = permissions.read && permissions.checkHealth && !!stored;
  const problems = canCheckHealth && health ? countHealthProblems(health) : undefined;

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
      healthGeneration.current++;
      inFlight.current = false;
      setHealth(undefined);
      setHealthError(undefined);
      setChecking(false);
    }
  };

  const checkHealth = useCallback(async () => {
    if (inFlight.current) {
      return;
    }
    inFlight.current = true;
    const generation = ++healthGeneration.current;
    setHealth(undefined);
    setHealthError(undefined);
    setChecking(true);
    try {
      const result = await checkModelCatalogHealth(name);
      if (generation === healthGeneration.current) {
        setHealth(result);
      }
    } catch (err) {
      if (generation === healthGeneration.current) {
        setHealthError(catalogHealthErrorMessage(err));
      }
    } finally {
      if (generation === healthGeneration.current) {
        inFlight.current = false;
        setChecking(false);
      }
    }
  }, [name]);

  useEffect(() => {
    if (!location.state?.checkHealthAfterCreate || consumedArrival.current || (permissions.read && !stored && !error)) {
      return;
    }
    consumedArrival.current = true;
    navigate(
      { pathname: location.pathname, search: location.search, hash: location.hash },
      { replace: true, state: null }
    );
    if (canCheckHealth) {
      queueMicrotask(() => void checkHealth());
    }
  }, [canCheckHealth, checkHealth, error, location, navigate, permissions.read, stored]);

  const remove = async () => {
    setDeleting(false);
    try {
      await deleteModelCatalog(name);
      navigate(prefixRoute(ROUTES.Catalogs));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const canBrowse = permissions.read && permissions.update && permissions.listProviderModels;
  const body = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (!stored) {
      return error ? null : <LoadingPlaceholder text="Loading catalog..." />;
    }
    return (
      <section aria-labelledby="catalog-providers-heading">
        <Stack direction="column" gap={2}>
          <Stack alignItems="center" gap={1}>
            <Text element="h2" id="catalog-providers-heading" variant="h3">
              Providers
            </Text>
            {canCheckHealth && (
              <div role="status">
                {problems !== undefined && (
                  <Badge
                    color={problems === 0 ? 'darkgrey' : 'orange'}
                    text={
                      problems === 0 ? 'No problems found' : `${problems} ${problems === 1 ? 'problem' : 'problems'}`
                    }
                  />
                )}
              </div>
            )}
          </Stack>
          <CatalogEditor
            spec={stored.spec}
            keys={Object.keys(stored.secure ?? {})}
            health={canCheckHealth ? health : undefined}
            readOnly={!permissions.update}
            listingRevision={stored.metadata.resourceVersion}
            listModels={canBrowse ? (provider) => listProviderModels(name, provider) : undefined}
            onChange={save}
          />
        </Stack>
      </section>
    );
  };

  return (
    <PluginPage
      pageNav={{ text: name }}
      subTitle={
        stored && (
          <Stack direction="column" gap={0.5}>
            <span>Applications call this catalog&apos;s models with its URL as their base URL.</span>
            <CatalogURL name={name} url={catalogBaseURL(gatewayEndpoint(config.appUrl, config.namespace), name)} />
          </Stack>
        )
      }
      actions={
        stored && (
          <Stack gap={1} alignItems="center">
            {canCheckHealth && (
              <Button
                variant="secondary"
                disabled={checking}
                onClick={checkHealth}
                tooltip="Checks provider model listings without calling a model."
              >
                {checking ? 'Checking health…' : 'Check health'}
              </Button>
            )}
            {permissions.delete && (
              <Button variant="destructive" fill="outline" icon="trash-alt" onClick={() => setDeleting(true)}>
                Delete catalog
              </Button>
            )}
          </Stack>
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
          {canCheckHealth && healthError && (
            <Alert severity="warning" title="Catalog health check failed">
              {healthError}
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
