import pluginJson from './plugin.json';

export const PLUGIN_BASE_URL = `/a/${pluginJson.id}`;

export const API_GROUP = 'aigateway.ext.grafana.app';
export const API_VERSION = 'v1alpha1';

export enum ROUTES {
  Overview = 'overview',
}
