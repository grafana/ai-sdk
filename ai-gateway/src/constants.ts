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

/** editCatalogURL returns the app URL of a catalog's page. */
export function editCatalogURL(name: string): string {
  return prefixRoute(`${ROUTES.EditCatalog}/${encodeURIComponent(name)}`);
}
