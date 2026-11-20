import React, { ReactNode, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import { Alert, Button, Field, Input, LinkButton, Stack, Text, useStyles2 } from '@grafana/ui';
import { createModelCatalog } from '../api/modelCatalogs';
import { CatalogEditor } from '../components/Catalog/CatalogEditor';
import { useWrite, WriteError } from '../components/Catalog/FormModal';
import {
  CatalogChange,
  catalogNameError,
  createSecure,
  draftKeys,
  emptySpec,
  NewCatalogProblems,
  newCatalogProblems,
} from '../components/Catalog/catalogSpec';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';
import { catalogBaseURL, catalogPageURL, gatewayEndpoint, prefixRoute, ROUTES } from '../constants';
import { catalogPermissions } from '../permissions';

/**
 * NewCatalog creates a catalog in three steps: its name, its providers with
 * their keys, and the models they serve. Nothing is written until Create,
 * because the platform accepts a catalog only with a provider and a model;
 * the entered keys stay in memory until then.
 */
function NewCatalog() {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [spec, setSpec] = useState(emptySpec);
  const [keys, setKeys] = useState<Record<string, string>>({});
  const [problems, setProblems] = useState<NewCatalogProblems>({});
  const endpoint = gatewayEndpoint(config.appUrl, config.namespace);
  const nameError = name === '' ? problems.name : catalogNameError(name);

  const { saving, error, submit } = useWrite(async () => {
    const found = newCatalogProblems(name, spec);
    setProblems(found);
    if (found.name === undefined && found.content === undefined) {
      await createModelCatalog(name, spec, createSecure(keys));
      navigate(catalogPageURL(name));
    }
  });

  const change = async ({ spec: next, secure }: CatalogChange) => {
    setSpec(next);
    setKeys((current) => draftKeys(current, secure));
    setProblems((current) => ({ ...current, content: undefined }));
  };

  const body = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (!permissions.create) {
      return (
        <Alert severity="info" title="You can't create model catalogs">
          Creating model catalogs needs the Model catalog writer role, which Admins have by default.
        </Alert>
      );
    }
    return (
      <Stack direction="column" gap={4}>
        <Step
          number={1}
          title="Name the catalog"
          description="The name becomes part of the catalog's URL, which applications call to use its models."
        >
          <Field
            label="Catalog name"
            description="It can't be changed later. Letters, digits and '-', '_', '.' or ':'."
            invalid={nameError !== undefined}
            error={nameError}
          >
            <Input id="catalog-name" value={name} onChange={(event) => setName(event.currentTarget.value)} />
          </Field>
          {name !== '' && nameError === undefined && (
            <Stack direction="column" gap={0.5}>
              <Text color="secondary" variant="bodySmall">
                Applications use this URL as their base URL, with a Grafana service account token:
              </Text>
              <span className={s.url}>{catalogBaseURL(endpoint, name)}</span>
            </Stack>
          )}
        </Step>

        <Step
          number={2}
          title="Connect providers and add models"
          description="Each provider is an account with its own API key. Add the models applications can use by their provider model ID."
        >
          <Stack direction="column" gap={1}>
            <CatalogEditor spec={spec} keys={Object.keys(keys)} stored={false} readOnly={false} onChange={change} />
            {problems.content && (
              <Text color="error" variant="bodySmall">
                {problems.content}
              </Text>
            )}
          </Stack>
        </Step>

        <Step
          number={3}
          title="Create"
          description="Keys are stored as Grafana secrets and can't be viewed afterwards."
        >
          <Stack direction="column" gap={2}>
            <WriteError error={error} />
            <Stack gap={1}>
              <Button disabled={saving} onClick={() => void submit()}>
                {saving ? 'Creating...' : 'Create catalog'}
              </Button>
              <LinkButton variant="secondary" fill="outline" href={prefixRoute(ROUTES.Catalogs)}>
                Cancel
              </LinkButton>
            </Stack>
          </Stack>
        </Step>
      </Stack>
    );
  };

  return (
    <PluginPage pageNav={{ text: 'New catalog' }}>
      <div className={s.page}>{body()}</div>
    </PluginPage>
  );
}

export default NewCatalog;

interface StepProps {
  number: number;
  title: string;
  description: string;
  children: ReactNode;
}

function Step({ number, title, description, children }: StepProps) {
  const s = useStyles2(getStyles);

  return (
    <section className={s.step} aria-label={title}>
      <span className={s.number} aria-hidden>
        {number}
      </span>
      <Stack direction="column" gap={2}>
        <div>
          <Text element="h2" variant="h4">
            {title}
          </Text>
          <Text color="secondary">{description}</Text>
        </div>
        <div>{children}</div>
      </Stack>
    </section>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.lg,
  }),
  step: css({
    display: 'grid',
    gridTemplateColumns: `${theme.spacing(4)} minmax(0, 1fr)`,
    gap: theme.spacing(2),
  }),
  number: css({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: theme.spacing(4),
    height: theme.spacing(4),
    borderRadius: theme.shape.radius.circle,
    border: `1px solid ${theme.colors.border.medium}`,
    fontWeight: theme.typography.fontWeightMedium,
  }),
  url: css({
    fontFamily: theme.typography.fontFamilyMonospace,
    fontSize: theme.typography.sm.fontSize,
    color: theme.colors.text.secondary,
    wordBreak: 'break-all',
  }),
});
