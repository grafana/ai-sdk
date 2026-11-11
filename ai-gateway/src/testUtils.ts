import { CurrentUser, OrgRole } from '@grafana/data';
import { setCurrentUser } from '@grafana/runtime';

let currentUser: CurrentUser | undefined;

/**
 * signInAs makes Grafana's current user hold the given org role and RBAC
 * actions. Grafana accepts the current user once, so later calls change it.
 */
export function signInAs(orgRole: OrgRole, actions: string[]) {
  if (!currentUser) {
    currentUser = { isSignedIn: true } as CurrentUser;
    setCurrentUser(currentUser);
  }
  currentUser.orgRole = orgRole;
  currentUser.permissions = Object.fromEntries(actions.map((action) => [action, true]));
}
