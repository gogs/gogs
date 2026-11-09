import { type AnyRoute, createRoute, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import { fetchSettingsOrgs, fetchSettingsRepos, fetchUserSettings } from "@/lib/queries/settings";
import {
  UserAvatarSettings,
  UserDelete,
  UserOrgSettings,
  UserPasswordSettings,
  UserProfileSettings,
  UserRepoSettings,
} from "@/pages/user/Settings";

// loadSettingsPage maps the errors of loading a settings page to router
// signals.
async function loadSettingsPage<T>(load: () => Promise<T>): Promise<T> {
  try {
    return await load();
  } catch (err) {
    if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
      // The viewer is signed out, has not confirmed their email, or is
      // prohibited from signing in. Reload the page so the server sends them to
      // sign in, account activation, or the login prohibited page.
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
      throw redirect({ href: window.location.href, reloadDocument: true });
    }
    throw err;
  }
}

export function createSettingsRoutes(rootRoute: AnyRoute) {
  const profileRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings",
    loader: () => loadSettingsPage(fetchUserSettings),
    component: function Profile() {
      return <UserProfileSettings settings={profileRoute.useLoaderData()} />;
    },
  });

  const avatarRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/avatar",
    loader: () => loadSettingsPage(fetchUserSettings),
    component: function Avatar() {
      return <UserAvatarSettings settings={avatarRoute.useLoaderData()} />;
    },
  });

  const passwordRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/password",
    loader: () => loadSettingsPage(fetchUserSettings),
    component: function Password() {
      return <UserPasswordSettings isLocal={passwordRoute.useLoaderData().isLocal} />;
    },
  });

  const reposRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/repositories",
    loader: () => loadSettingsPage(fetchSettingsRepos),
    component: function Repos() {
      return <UserRepoSettings repos={reposRoute.useLoaderData()} />;
    },
  });

  const orgsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/organizations",
    loader: () => loadSettingsPage(fetchSettingsOrgs),
    component: function Orgs() {
      return <UserOrgSettings orgs={orgsRoute.useLoaderData()} />;
    },
  });

  const deleteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/delete",
    component: UserDelete,
  });

  return [profileRoute, avatarRoute, passwordRoute, reposRoute, orgsRoute, deleteRoute];
}
