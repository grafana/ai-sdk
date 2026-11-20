import { hasPermission } from '@grafana/runtime';
import { API_GROUP } from './constants';

/** CatalogActions are the Grafana RBAC actions on ModelCatalog resources. */
export const CatalogActions = {
  Read: `${API_GROUP}/modelcatalogs:get`,
  Create: `${API_GROUP}/modelcatalogs:create`,
  Update: `${API_GROUP}/modelcatalogs:update`,
  Delete: `${API_GROUP}/modelcatalogs:delete`,
} as const;

/** CatalogPermissions says which catalog actions the signed-in user holds. */
export interface CatalogPermissions {
  read: boolean;
  create: boolean;
  update: boolean;
  delete: boolean;
}

/**
 * catalogPermissions reads the signed-in user's catalog permissions from
 * Grafana, independently of their org role.
 */
export function catalogPermissions(): CatalogPermissions {
  return {
    read: hasPermission(CatalogActions.Read),
    create: hasPermission(CatalogActions.Create),
    update: hasPermission(CatalogActions.Update),
    delete: hasPermission(CatalogActions.Delete),
  };
}
