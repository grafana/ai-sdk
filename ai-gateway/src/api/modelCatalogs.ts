import { config, getBackendSrv, isFetchError } from '@grafana/runtime';
import { API_GROUP, API_VERSION } from '../constants';
import { InlineSecureValue, ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { Spec } from '../generated/modelcatalog/v1alpha1/types.spec.gen';

const MODEL_CATALOG_KIND = 'ModelCatalog';

/** SecureChanges are the secure values a write sends, keyed by provider name. */
export type SecureChanges = Record<string, InlineSecureValue>;

interface ModelCatalogList {
  items?: ModelCatalog[];
}

const REQUEST_OPTIONS = { showErrorAlert: false };

/** modelCatalogsURL returns the ModelCatalog collection path of this Grafana's namespace. */
export function modelCatalogsURL(): string {
  return `${config.appSubUrl}/apis/${API_GROUP}/${API_VERSION}/namespaces/${encodeURIComponent(config.namespace)}/modelcatalogs`;
}

function modelCatalogURL(name: string): string {
  return `${modelCatalogsURL()}/${encodeURIComponent(name)}`;
}

/** listModelCatalogs returns every catalog in this Grafana's namespace. */
export async function listModelCatalogs(): Promise<ModelCatalog[]> {
  const list = await getBackendSrv().get<ModelCatalogList>(modelCatalogsURL(), undefined, undefined, REQUEST_OPTIONS);
  return list.items ?? [];
}

/** getModelCatalog returns one catalog. Its secure values carry names only. */
export function getModelCatalog(name: string): Promise<ModelCatalog> {
  return getBackendSrv().get<ModelCatalog>(modelCatalogURL(name), undefined, undefined, REQUEST_OPTIONS);
}

/** createModelCatalog creates a catalog with the given spec and secure values. */
export function createModelCatalog(name: string, spec: Spec, secure: SecureChanges): Promise<ModelCatalog> {
  const body = {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: MODEL_CATALOG_KIND,
    metadata: { name },
    spec,
    ...withSecure(secure),
  };
  return getBackendSrv().post<ModelCatalog>(modelCatalogsURL(), body, REQUEST_OPTIONS);
}

/**
 * updateModelCatalog replaces a stored catalog's spec. It keeps the stored
 * metadata, including its resource version, and sends only the given secure
 * values, so the platform keeps every key the update omits.
 */
export function updateModelCatalog(stored: ModelCatalog, spec: Spec, secure: SecureChanges): Promise<ModelCatalog> {
  const body = {
    apiVersion: stored.apiVersion,
    kind: stored.kind,
    metadata: stored.metadata,
    spec,
    ...withSecure(secure),
  };
  return getBackendSrv().put<ModelCatalog>(modelCatalogURL(stored.metadata.name), body, REQUEST_OPTIONS);
}

/** deleteModelCatalog deletes a catalog. */
export async function deleteModelCatalog(name: string): Promise<void> {
  await getBackendSrv().delete(modelCatalogURL(name), undefined, REQUEST_OPTIONS);
}

function withSecure(secure: SecureChanges): { secure?: SecureChanges } {
  return Object.keys(secure).length > 0 ? { secure } : {};
}

/** errorMessage returns the message the platform returned for a failed request. */
export function errorMessage(error: unknown): string {
  if (isFetchError<{ message?: string }>(error)) {
    return error.data?.message || error.statusText || error.message || `Request failed with status ${error.status}`;
  }
  return error instanceof Error ? error.message : String(error);
}
