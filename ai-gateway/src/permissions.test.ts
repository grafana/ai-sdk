import { OrgRole } from '@grafana/data';
import { CatalogActions, catalogPermissions } from './permissions';
import { signInAs } from './testUtils';

describe('permissions', () => {
  test('names the ModelCatalog RBAC actions', () => {
    expect(CatalogActions).toEqual({
      Read: 'aigateway.ext.grafana.app/modelcatalogs:get',
      Create: 'aigateway.ext.grafana.app/modelcatalogs:create',
      Update: 'aigateway.ext.grafana.app/modelcatalogs:update',
      Delete: 'aigateway.ext.grafana.app/modelcatalogs:delete',
    });
  });

  test.each([
    [OrgRole.Viewer, [CatalogActions.Read], { read: true, create: false, update: false, delete: false }],
    [OrgRole.Editor, [CatalogActions.Read], { read: true, create: false, update: false, delete: false }],
    [
      OrgRole.Editor,
      [CatalogActions.Read, CatalogActions.Create, CatalogActions.Update],
      { read: true, create: true, update: true, delete: false },
    ],
    [OrgRole.Admin, [CatalogActions.Read], { read: true, create: false, update: false, delete: false }],
    [OrgRole.Admin, [], { read: false, create: false, update: false, delete: false }],
    [OrgRole.Viewer, Object.values(CatalogActions), { read: true, create: true, update: true, delete: true }],
  ])('follows the permissions of a %s holding %j, not the role', (role, actions, expected) => {
    signInAs(role, actions);
    expect(catalogPermissions()).toEqual(expected);
  });
});
