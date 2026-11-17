import pluginJson from './plugin.json';

export const API_GROUP = 'aigateway.ext.grafana.app';
export const API_VERSION = 'v1alpha1';

export const PLUGIN_BASE_URL = `/a/${pluginJson.id}`;

export enum ROUTES {
  Overview = 'overview',
  Catalogs = 'catalogs',
  NewCatalog = 'catalogs/new',
  EditCatalog = 'catalogs/edit',
}

/** prefixRoute returns the app URL of a route. */
export function prefixRoute(route: string): string {
  return `${PLUGIN_BASE_URL}/${route}`;
}

/** catalogPageURL returns the app URL of a catalog's page. */
export function catalogPageURL(name: string): string {
  return prefixRoute(`${ROUTES.EditCatalog}/${encodeURIComponent(name)}`);
}

/** gatewayEndpoint returns the Gateway URL of a namespace on a Grafana URL. */
export function gatewayEndpoint(appUrl: string, namespace: string): string {
  return `${appUrl.replace(/\/+$/, '')}/apis/${API_GROUP}/${API_VERSION}/namespaces/${encodeURIComponent(namespace)}`;
}

/** catalogBaseURL returns the base URL of one stored catalog's routes. */
export function catalogBaseURL(endpoint: string, name: string): string {
  return `${endpoint}/modelcatalogs/${encodeURIComponent(name)}`;
}
