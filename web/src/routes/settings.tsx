import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import {
  fetchAccessTokens,
  fetchRecoveryCodes,
  fetchSSHKeys,
  fetchSettingsOrgs,
  fetchSettingsRepos,
  fetchTwoFactorEnabled,
  fetchTwoFactorEnrollment,
  fetchUserEmails,
  fetchUserSettings,
} from "@/lib/queries/settings";
import {
  UserAvatarSettings,
  UserDelete,
  UserOrgSettings,
  UserPasswordSettings,
  UserProfileSettings,
  UserRepoSettings,
} from "@/pages/user/Settings";
import {
  UserEmailSettings,
  UserRecoveryCodes,
  UserSSHKeySettings,
  UserSecuritySettings,
  UserTokenSettings,
  UserTwoFactorEnable,
} from "@/pages/user/SettingsSecurity";

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
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
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

  const emailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/email",
    // The link that confirms an email address lands here with `activated`.
    validateSearch: (search: Record<string, unknown>): { activated?: true } =>
      search.activated === 1 || search.activated === "1" ? { activated: true } : {},
    loader: () => loadSettingsPage(fetchUserEmails),
    component: function Emails() {
      const search: { activated?: true } = emailRoute.useSearch();
      return <UserEmailSettings emails={emailRoute.useLoaderData()} activated={search.activated === true} />;
    },
  });

  const sshRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/ssh",
    loader: () => loadSettingsPage(fetchSSHKeys),
    component: function SSHKeys() {
      return <UserSSHKeySettings keys={sshRoute.useLoaderData()} />;
    },
  });

  const securityRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security",
    loader: () => loadSettingsPage(fetchTwoFactorEnabled),
    component: function Security() {
      return <UserSecuritySettings enabled={securityRoute.useLoaderData()} />;
    },
  });

  const twoFactorEnableRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security/two_factor_enable",
    loader: () => loadSettingsPage(fetchTwoFactorEnrollment),
    component: function TwoFactorEnable() {
      return <UserTwoFactorEnable enrollment={twoFactorEnableRoute.useLoaderData()} />;
    },
  });

  const recoveryCodesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security/two_factor_recovery_codes",
    loader: () => loadSettingsPage(fetchRecoveryCodes),
    component: function RecoveryCodes() {
      return <UserRecoveryCodes codes={recoveryCodesRoute.useLoaderData()} />;
    },
  });

  const applicationsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/applications",
    loader: () => loadSettingsPage(fetchAccessTokens),
    component: function Applications() {
      return <UserTokenSettings tokens={applicationsRoute.useLoaderData()} />;
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

  return [
    profileRoute,
    avatarRoute,
    passwordRoute,
    emailRoute,
    sshRoute,
    securityRoute,
    twoFactorEnableRoute,
    recoveryCodesRoute,
    applicationsRoute,
    reposRoute,
    orgsRoute,
    deleteRoute,
  ];
}
