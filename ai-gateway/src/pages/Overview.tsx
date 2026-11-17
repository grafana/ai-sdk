import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import {
  Alert,
  CollapsableSection,
  Grid,
  LinkButton,
  Stack,
  Tab,
  TabContent,
  TabsBar,
  Text,
  TextLink,
  useStyles2,
} from '@grafana/ui';
import { errorMessage, listModelCatalogs } from '../api/modelCatalogs';
import { CodeSnippet } from '../components/CodeSnippet/CodeSnippet';
import { LinkCard } from '../components/LinkCard/LinkCard';
import { SetupStep } from '../components/SetupStep/SetupStep';
import { catalogBaseURL, catalogPageURL, gatewayEndpoint, prefixRoute, ROUTES } from '../constants';
import { catalogPermissions } from '../permissions';
import { CALLERS, LINKS, RESOURCES, SERVICE_ACCOUNT_PATH } from './overview/content';
import { callExample, ENVIRONMENT, INSTALL, Language, LANGUAGES } from './overview/examples';

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Stack direction="column" gap={2}>
      <Text element="h2" variant="h3">
        {title}
      </Text>
      {children}
    </Stack>
  );
}

function QuickStart({ endpoint }: { endpoint: string }) {
  const [language, setLanguage] = useState(Language.TypeScript);

  return (
    <Stack direction="column" gap={1}>
      <TabsBar>
        {LANGUAGES.map(({ value, label }) => (
          <Tab key={value} label={label} active={language === value} onChangeTab={() => setLanguage(value)} />
        ))}
      </TabsBar>
      <TabContent>
        <Stack direction="column" gap={1}>
          <CodeSnippet code={INSTALL[language]} />
          <CodeSnippet code={ENVIRONMENT} />
          <CodeSnippet code={callExample(language, endpoint)} />
        </Stack>
      </TabContent>
    </Stack>
  );
}

function GetStarted({ endpoint }: { endpoint: string }) {
  const s = useStyles2(getStyles);

  return (
    <Section title="Get started in Grafana Cloud">
      <ol className={s.steps}>
        <SetupStep index={1} title="Create a service account token">
          <Text element="p" color="secondary">
            Create a service account for your application and add a token to it. No extra role is needed. Keep the token
            in your server&apos;s secret store, never in browser code.
          </Text>
          <Stack gap={2} alignItems="center" wrap="wrap">
            <LinkButton href={`${config.appSubUrl}${SERVICE_ACCOUNT_PATH}`} icon="plus" variant="secondary" size="sm">
              Create service account
            </LinkButton>
            <TextLink href={LINKS.serviceAccountDocs} external>
              About service accounts
            </TextLink>
          </Stack>
        </SetupStep>

        <SetupStep index={2} title="Copy your Gateway URL">
          <CodeSnippet code={endpoint} />
          <Text element="p" color="secondary">
            Use it as the client&apos;s base URL. Clients that exchange tokens use the API root, without the namespace.{' '}
            <TextLink href={LINKS.findGatewayURL} external inline>
              Find your Gateway URL
            </TextLink>
          </Text>
        </SetupStep>

        <SetupStep index={3} title="Bring your provider key">
          <Text element="p" color="secondary">
            In Grafana Cloud, the Gateway calls Anthropic or OpenAI with your own account. Send your key with every call
            under <code>providerOptions.gateway.byok</code>, and select the provider&apos;s model as{' '}
            <code>provider/model</code>. Don&apos;t log request bodies, because they contain the key.{' '}
            <TextLink href={LINKS.providerCredentials} external inline>
              Use your own provider credentials
            </TextLink>
          </Text>
        </SetupStep>

        <SetupStep index={4} title="Make a call">
          <QuickStart endpoint={endpoint} />
        </SetupStep>
      </ol>
    </Section>
  );
}

function CatalogURLs({ endpoint }: { endpoint: string }) {
  const s = useStyles2(getStyles);
  const [names, setNames] = useState<string[]>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    let current = true;
    listModelCatalogs().then(
      (catalogs) => current && setNames(catalogs.map((catalog) => catalog.metadata.name)),
      (err) => current && setError(errorMessage(err))
    );
    return () => {
      current = false;
    };
  }, []);

  return (
    <Section title="Model catalogs">
      <Text element="p" color="secondary">
        Use a catalog&apos;s URL as the base URL to call only that catalog&apos;s models, with its own provider keys.
        Editors and Admins can call catalogs.
      </Text>
      {error && (
        <Alert severity="warning" title="Couldn't load model catalogs">
          {error}
        </Alert>
      )}
      {names?.length === 0 && <Text element="p">This stack has no model catalogs yet.</Text>}
      {names?.map((name) => (
        <Stack key={name} direction="column" gap={0.5}>
          <Link className={s.link} to={catalogPageURL(name)}>
            {name}
          </Link>
          <CodeSnippet code={catalogBaseURL(endpoint, name)} />
        </Stack>
      ))}
      <div>
        <LinkButton variant="secondary" fill="text" icon="arrow-right" href={prefixRoute(ROUTES.Catalogs)}>
          Manage model catalogs
        </LinkButton>
      </div>
    </Section>
  );
}

function Callers() {
  return (
    <Section title="Choose how you call">
      <Text element="p" color="secondary">
        Your credential decides which models you get.
      </Text>
      <Grid gap={2} columns={{ xs: 1, lg: 3 }}>
        {CALLERS.map((caller) => (
          <LinkCard key={caller.title} title={caller.title} href={caller.href}>
            <Text color="secondary">
              <Text weight="medium">Credential:</Text> {caller.credential}
            </Text>
            <Text color="secondary">
              <Text weight="medium">Models:</Text> {caller.models}
            </Text>
          </LinkCard>
        ))}
      </Grid>
    </Section>
  );
}

const SELF_HOSTED_LINKS = [
  { title: 'Install and enable the plugin', href: LINKS.install },
  { title: 'Configure models', href: LINKS.configureModels },
  { title: 'Choose who can use it', href: LINKS.accessMode },
  { title: 'Store model catalogs in Grafana', href: LINKS.storedCatalogs },
  { title: 'Authenticate in self-hosted Grafana', href: LINKS.selfHostedCallers },
];

function SelfHosted() {
  const s = useStyles2(getStyles);
  const [open, setOpen] = useState(false);

  return (
    <CollapsableSection label="Running Grafana yourself?" isOpen={open} onToggle={setOpen}>
      <Stack direction="column" gap={1}>
        <Text element="p" color="secondary">
          In self-hosted Grafana, the Gateway serves the models your administrator configures and refuses requests that
          bring their own provider keys. Use the quick start without the <code>byok</code> option, and select a model by
          its catalog ID or alias.
        </Text>
        <ul className={s.list}>
          {SELF_HOSTED_LINKS.map((link) => (
            <li key={link.href}>
              <TextLink href={link.href} external>
                {link.title}
              </TextLink>
            </li>
          ))}
        </ul>
      </Stack>
    </CollapsableSection>
  );
}

function Resources() {
  return (
    <Section title="Resources">
      <Grid gap={2} columns={{ xs: 1, sm: 2, lg: 4 }}>
        {RESOURCES.map((resource) => (
          <LinkCard key={resource.href} title={resource.title} href={resource.href}>
            <Text color="secondary">{resource.description}</Text>
          </LinkCard>
        ))}
      </Grid>
    </Section>
  );
}

function HeaderActions() {
  return (
    <Stack gap={1}>
      <LinkButton href={LINKS.gatewayGuide} target="_blank" rel="noopener noreferrer" icon="book" variant="secondary">
        Documentation
      </LinkButton>
      <LinkButton
        href={LINKS.repository}
        target="_blank"
        rel="noopener noreferrer"
        icon="github"
        variant="secondary"
        fill="text"
      >
        Source
      </LinkButton>
    </Stack>
  );
}

function Overview() {
  const s = useStyles2(getStyles);
  const endpoint = gatewayEndpoint(config.appUrl, config.namespace);
  const { read: readCatalogs } = catalogPermissions();

  return (
    <PluginPage
      subTitle="One API for Anthropic and OpenAI models, called through your Grafana stack."
      actions={<HeaderActions />}
    >
      <div className={s.page}>
        <Stack direction="column" gap={5}>
          <Text element="p">
            The AI Gateway speaks the AI SDK language-model protocol, so Go services use the Grafana provider and
            TypeScript services use the AI SDK Gateway provider, with the same streaming, tool calling and structured
            output as a direct provider.
          </Text>
          <GetStarted endpoint={endpoint} />
          {readCatalogs && <CatalogURLs endpoint={endpoint} />}
          <Callers />
          <SelfHosted />
          <Resources />
        </Stack>
      </div>
    </PluginPage>
  );
}

export default Overview;

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.xl,
  }),
  steps: css({
    listStyle: 'none',
    margin: 0,
    padding: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(3),
  }),
  link: css({
    color: theme.colors.text.link,
    fontWeight: theme.typography.fontWeightMedium,
  }),
  list: css({
    margin: 0,
    paddingLeft: theme.spacing(3),
    lineHeight: theme.typography.base.lineHeight * 1.4,
  }),
});
